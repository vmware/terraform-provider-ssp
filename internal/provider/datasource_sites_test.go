// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newSitesDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sites": []map[string]any{
				{
					"id":            "site-uuid-555",
					"site_type":     "NSX_MANAGER",
					"site_name":     "NSX-Manager-Prod",
					"current_state": "READY",
					"status": map[string]any{
						"connection_status": "HEALTHY",
					},
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitSitesDataSource(t *testing.T) {
	srv := newSitesDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_sites" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_sites.test", "sites.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_sites.test", "sites.0.id", "site-uuid-555"),
					resource.TestCheckResourceAttr("data.ssp_sites.test", "sites.0.site_type", "NSX_MANAGER"),
					resource.TestCheckResourceAttr("data.ssp_sites.test", "sites.0.site_name", "NSX-Manager-Prod"),
					resource.TestCheckResourceAttr("data.ssp_sites.test", "sites.0.current_state", "READY"),
					resource.TestCheckResourceAttr("data.ssp_sites.test", "sites.0.connection_status", "HEALTHY"),
				),
			},
		},
	})
}
