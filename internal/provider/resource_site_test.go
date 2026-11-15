// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// siteMockAPI simulates the subset of the SSP runtime site-service API
// (POST/GET/PUT/DELETE /ssp/sites[/{id}]) needed to drive
// ssp_site through a full onboard/reconnect/offboard cycle without a live
// SSP cluster or NSX Manager.
type siteMockAPI struct {
	mu           sync.Mutex
	exists       bool
	revision     int
	siteType     string
	siteName     string
	desiredState string
	// blockOffboard makes DELETE keep the site and record a failed offboard
	// precheck, like the platform does while features are enabled.
	blockOffboard bool
	precheckTime  int64
}

func newSiteMockServer(t *testing.T) *httptest.Server {
	srv, _ := newSiteMockServerFull(t, false)
	return srv
}

func newSiteMockServerWithReadiness(t *testing.T, nested bool) *httptest.Server {
	srv, _ := newSiteMockServerFull(t, nested)
	return srv
}

// newSiteMockServerFull reports the readiness either as a top-level
// current_state or, when nested is true, only as
// status.configuration_status.current_status (as the live platform does).
func newSiteMockServerFull(t *testing.T, nested bool) (*httptest.Server, *siteMockAPI) {
	t.Helper()
	m := &siteMockAPI{}

	writeSite := func(w http.ResponseWriter) {
		status := map[string]any{
			"connection_status": "HEALTHY",
			"nsx_cluster_id":    "nsx-cluster-1",
			"nsx_version":       "4.2.0",
		}
		body := map[string]any{
			"id":            "site-1",
			"_revision":     m.revision,
			"site_type":     m.siteType,
			"site_name":     m.siteName,
			"desired_state": m.desiredState,
			"status":        status,
		}
		if nested && m.desiredState == "PREPARE" {
			// A PREPARE site halts after its prechecks and never becomes READY.
			status["configuration_status"] = map[string]any{
				"current_status": "NOT_READY",
				"site_precheck_status": map[string]any{
					"site_precheck_phase": "ONBOARD",
					"site_precheck_state": "PASSED",
				},
			}
		} else if nested {
			status["configuration_status"] = map[string]any{"current_status": "READY"}
		} else {
			body["current_state"] = "READY"
		}
		if m.precheckTime != 0 {
			cs, _ := status["configuration_status"].(map[string]any)
			if cs == nil {
				cs = map[string]any{"current_status": "READY"}
				status["configuration_status"] = cs
			}
			cs["site_precheck_status"] = map[string]any{
				"site_precheck_phase": "OFFBOARD",
				"site_precheck_results": []map[string]any{{
					"precheck_name":          "FeaturesActivatePrecheck",
					"precheck_message":       "features are currently enabled on SSP",
					"precheck_result_status": "FAILED",
					"precheck_last_run_time": m.precheckTime,
				}},
			}
		}
		_ = json.NewEncoder(w).Encode(body)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/sites", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body struct {
			SiteType     string `json:"site_type"`
			SiteName     string `json:"site_name"`
			DesiredState string `json:"desired_state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.exists = true
		m.revision = 1
		m.siteType = body.SiteType
		m.siteName = body.SiteName
		m.desiredState = body.DesiredState
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "site-1"})
	})
	mux.HandleFunc("GET /ssp/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.exists {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeSite(w)
	})
	mux.HandleFunc("PUT /ssp/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body struct {
			SiteType     string `json:"site_type"`
			SiteName     string `json:"site_name"`
			DesiredState string `json:"desired_state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.revision++
		m.siteType = body.SiteType
		m.siteName = body.SiteName
		m.desiredState = body.DesiredState
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "site-1"})
	})
	mux.HandleFunc("DELETE /ssp/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.exists {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if m.blockOffboard {
			m.precheckTime = 2000
		} else {
			m.exists = false
		}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, m
}

func testUnitSiteConfig(host, desiredState string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_site" "test" {
  site_type     = "NSX_MANAGER"
  site_name     = "nsx-01"
  desired_state = "` + desiredState + `"

  site_connection_info = {
    connection_type = "DYNAMIC"
    hostname        = "nsx01.corp.local"
    username        = "admin"
    password        = "VMware1!"
    certificate     = "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----"
  }
}
`
}

// TestUnitSiteResource exercises ssp_site's Create (ONBOARD), Update
// (desired_state change to PREPARE), Read, and Delete (OFFBOARD) against a
// mocked SSP runtime API.
func TestUnitSiteResource(t *testing.T) {
	srv := newSiteMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: testUnitSiteConfig(srv.URL, "ONBOARD"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_site.test", "id", "site-1"),
					resource.TestCheckResourceAttr("ssp_site.test", "site_type", "NSX_MANAGER"),
					resource.TestCheckResourceAttr("ssp_site.test", "desired_state", "ONBOARD"),
					resource.TestCheckResourceAttr("ssp_site.test", "current_state", "READY"),
					resource.TestCheckResourceAttr("ssp_site.test", "connection_status", "HEALTHY"),
					resource.TestCheckResourceAttr("ssp_site.test", "force", "false"),
				),
			},
			// Update: change desired_state in place (no RequiresReplace on this attribute).
			{
				Config: testUnitSiteConfig(srv.URL, "PREPARE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_site.test", "id", "site-1"),
					resource.TestCheckResourceAttr("ssp_site.test", "desired_state", "PREPARE"),
				),
			},
			// Delete Testing is handled automatically by terraform-plugin-testing at the end of Test.
		},
	})
}

// The live platform returns no top-level current_state, so current_state used to
// be stored as "" even for a READY site.
func TestUnitSiteResource_CurrentStateFromConfigurationStatus(t *testing.T) {
	srv := newSiteMockServerWithReadiness(t, true)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitSiteConfig(srv.URL, "ONBOARD"),
				Check:  resource.TestCheckResourceAttr("ssp_site.test", "current_state", "READY"),
			},
		},
	})
}

// A failed offboard precheck used to leave destroy polling for 90 minutes; it
// must now fail the destroy with the precheck message.
func TestUnitSiteResource_DeleteBlockedByPrecheck(t *testing.T) {
	srv, m := newSiteMockServerFull(t, true)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: testUnitSiteConfig(srv.URL, "ONBOARD")},
			{
				PreConfig:   func() { m.mu.Lock(); m.blockOffboard = true; m.mu.Unlock() },
				Config:      testUnitSiteConfig(srv.URL, "ONBOARD"),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?s)offboard precheck failed.*FeaturesActivatePrecheck`),
			},
			// Unblock so the final automatic destroy can finish.
			{
				PreConfig: func() { m.mu.Lock(); m.blockOffboard = false; m.mu.Unlock() },
				Config:    testUnitSiteConfig(srv.URL, "ONBOARD"),
			},
		},
	})
}

// Importing used to leave desired_state and force null, so the next plan always
// showed an update.
func TestUnitSiteResource_ImportSetsDesiredState(t *testing.T) {
	srv := newSiteMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: testUnitSiteConfig(srv.URL, "ONBOARD")},
			{
				ResourceName:            "ssp_site.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"site_connection_info"},
			},
		},
	})
}

// A site created with desired_state = PREPARE only runs its prechecks and stays
// NOT_READY, so create must finish on the prechecks instead of waiting for READY.
func TestUnitSiteResource_CreatePrepare(t *testing.T) {
	srv := newSiteMockServerWithReadiness(t, true)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitSiteConfig(srv.URL, "PREPARE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_site.test", "desired_state", "PREPARE"),
					resource.TestCheckResourceAttr("ssp_site.test", "current_state", "NOT_READY"),
				),
			},
		},
	})
}
