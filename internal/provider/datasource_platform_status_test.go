package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newPlatformStatusDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/cluster/monitor/platform/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cluster_id":      "cluster-uuid-999",
			"cluster_name":    "Prod-SSP-Cluster",
			"product_version": "5.2.0",
			"node_count":      3,
			"form_factor":     "MEDIUM",
			"health":          "UP",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitPlatformStatusDataSource(t *testing.T) {
	srv := newPlatformStatusDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_platform_status" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "cluster_id", "cluster-uuid-999"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "cluster_name", "Prod-SSP-Cluster"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "product_version", "5.2.0"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "node_count", "3"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "form_factor", "MEDIUM"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "health", "UP"),
				),
			},
		},
	})
}
