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

func newAlarmsDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/alarms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_result_count": 1,
			"results": []map[string]any{
				{
					"id":            "alarm-1",
					"definition_id": "alarm-def-1",
					"feature_name":  "ndr",
					"severity":      "HIGH",
					"summary":       "NDR disconnected",
					"state":         "OPEN",
					"resource_id":   "res-1",
					"value":         "true",
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitAlarmsDataSource(t *testing.T) {
	srv := newAlarmsDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_alarms" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_alarms.test", "results.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_alarms.test", "results.0.id", "alarm-1"),
					resource.TestCheckResourceAttr("data.ssp_alarms.test", "results.0.state", "OPEN"),
				),
			},
		},
	})
}
