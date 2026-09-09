// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
}

func newSiteMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &siteMockAPI{}

	writeSite := func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "site-1",
			"_revision":     m.revision,
			"site_type":     m.siteType,
			"site_name":     m.siteName,
			"desired_state": m.desiredState,
			"current_state": "READY",
			"status": map[string]any{
				"connection_status": "HEALTHY",
				"nsx_cluster_id":    "nsx-cluster-1",
				"nsx_version":       "4.2.0",
			},
		})
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
		m.exists = false
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
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
