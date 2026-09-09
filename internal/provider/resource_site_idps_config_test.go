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

// siteIdpsConfigMockAPI simulates
// /ssp/security-content/feature-config/idps/{siteID}.
type siteIdpsConfigMockAPI struct {
	mu       sync.Mutex
	exists   bool
	deleted  bool
	revision int
	body     map[string]any
}

func newSiteIdpsConfigMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &siteIdpsConfigMockAPI{}

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/security-content/feature-config/idps/{siteID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.exists = true
		m.deleted = false
		m.revision = 1
		body["id"] = "idps-cfg-1"
		body["_revision"] = m.revision
		body["site_id"] = r.PathValue("siteID")
		m.body = body
		writeJSON(w, m.body)
	})
	mux.HandleFunc("GET /ssp/security-content/feature-config/idps/{siteID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.exists || m.deleted {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, m.body)
	})
	mux.HandleFunc("PUT /ssp/security-content/feature-config/idps/{siteID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.revision++
		body["id"] = "idps-cfg-1"
		body["_revision"] = m.revision
		body["site_id"] = r.PathValue("siteID")
		m.body = body
		writeJSON(w, m.body)
	})
	mux.HandleFunc("DELETE /ssp/security-content/feature-config/idps/{siteID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.deleted = true
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitSiteIdpsConfigConfig(host, siteID, version string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_site_idps_config" "test" {
  site_id          = "` + siteID + `"
  assigned_version = "` + version + `"
  auto_update      = false
}
`
}

// TestUnitSiteIdpsConfigResource exercises ssp_site_idps_config's Create,
// Read, Update, and Delete (real DELETE API, unlike most resources in this
// codebase) against a mocked SSP runtime API.
func TestUnitSiteIdpsConfigResource(t *testing.T) {
	srv := newSiteIdpsConfigMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitSiteIdpsConfigConfig(srv.URL, "site-1", "1.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_site_idps_config.test", "id", "idps-cfg-1"),
					resource.TestCheckResourceAttr("ssp_site_idps_config.test", "site_id", "site-1"),
					resource.TestCheckResourceAttr("ssp_site_idps_config.test", "assigned_version", "1.0"),
					resource.TestCheckResourceAttr("ssp_site_idps_config.test", "auto_update", "false"),
				),
			},
			// Update: change assigned_version in place.
			{
				Config: testUnitSiteIdpsConfigConfig(srv.URL, "site-1", "2.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_site_idps_config.test", "assigned_version", "2.0"),
				),
			},
			// Delete Testing is handled automatically by terraform-plugin-testing
			// at the end of Test; the mock's DELETE handler sets `deleted`, so a
			// subsequent GET (if any) would correctly 404.
		},
	})
}
