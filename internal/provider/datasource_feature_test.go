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

func newFeatureDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/lcm/features/{feature}/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"feature":          "NDR",
			"overall_status":   "DEPLOYMENT_SUCCESSFUL",
			"overall_progress": 100,
			"precheck_results": map[string]any{
				"overall_status": "SUCCESS",
			},
			"deployment_results": map[string]any{
				"deployment_status": "DEPLOYED",
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestUnitFeatureDataSource exercises data.ssp_feature's Read against a
// mocked SSP runtime API.
func TestUnitFeatureDataSource(t *testing.T) {
	srv := newFeatureDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_feature" "test" {
  feature = "NDR"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_feature.test", "feature", "NDR"),
					resource.TestCheckResourceAttr("data.ssp_feature.test", "overall_status", "DEPLOYMENT_SUCCESSFUL"),
					resource.TestCheckResourceAttr("data.ssp_feature.test", "overall_progress", "100"),
					resource.TestCheckResourceAttr("data.ssp_feature.test", "precheck_status", "SUCCESS"),
					resource.TestCheckResourceAttr("data.ssp_feature.test", "deployment_status", "DEPLOYED"),
				),
			},
		},
	})
}
