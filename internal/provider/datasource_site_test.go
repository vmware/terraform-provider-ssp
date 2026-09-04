package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newSiteDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/site-service/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "site-uuid-555",
			"site_type":     "NSX_MANAGER",
			"site_name":     "NSX-Manager-Prod",
			"current_state": "READY",
			"status": map[string]any{
				"connection_status": "HEALTHY",
				"cluster_status":    "STABLE",
				"nsx_version":       "4.2.0",
				"nsx_cluster_id":    "nsx-cluster-111",
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
