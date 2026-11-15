// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newSiteDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	// Match on the escaped path so a request that smuggles extra path
	// segments (an unescaped id) is not mistaken for the real site.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/ssp/sites/site-uuid-555" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "site-uuid-555",
			"site_type": "NSX_MANAGER",
			"site_name": "NSX-Manager-Prod",
			// The live platform reports readiness only under
			// status.configuration_status, not as a top-level current_state.
			"status": map[string]any{
				"connection_status": "HEALTHY",
				"cluster_status":    "STABLE",
				"nsx_version":       "4.2.0",
				"nsx_cluster_id":    "nsx-cluster-111",
				"configuration_status": map[string]any{
					"current_status": "READY",
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitSiteDataSource(t *testing.T) {
	srv := newSiteDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_site" "test" {
  id = "site-uuid-555"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_site.test", "id", "site-uuid-555"),
					resource.TestCheckResourceAttr("data.ssp_site.test", "site_type", "NSX_MANAGER"),
					resource.TestCheckResourceAttr("data.ssp_site.test", "site_name", "NSX-Manager-Prod"),
					resource.TestCheckResourceAttr("data.ssp_site.test", "current_state", "READY"),
					resource.TestCheckResourceAttr("data.ssp_site.test", "connection_status", "HEALTHY"),
					resource.TestCheckResourceAttr("data.ssp_site.test", "cluster_status", "STABLE"),
					resource.TestCheckResourceAttr("data.ssp_site.test", "nsx_version", "4.2.0"),
					resource.TestCheckResourceAttr("data.ssp_site.test", "nsx_cluster_id", "nsx-cluster-111"),
				),
			},
		},
	})
}

// An id with path or query characters used to be appended to the URL as is, so
// "<id>?x=1" read the real site and "../sites" or "<id>/settings" returned an
// all-empty site instead of an error.
func TestUnitSiteDataSource_IDIsEscaped(t *testing.T) {
	srv := newSiteDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_site" "test" {
  id = "site-uuid-555?x=1"
}
`,
				ExpectError: regexp.MustCompile(`Site not found`),
			},
		},
	})
}
