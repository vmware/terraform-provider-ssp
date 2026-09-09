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

func newAlarmCountsDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/alarms/counts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_instance_count": 3,
			"open_instance_counts": []map[string]any{
				{
					"categories": []map[string]any{
						{
							"category": "ndr",
							"severity_counts": map[string]any{
								"severity_counts": []map[string]any{
									{"severity": "HIGH", "count": 2},
								},
							},
						},
					},
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitAlarmCountsDataSource(t *testing.T) {
	srv := newAlarmCountsDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_alarm_counts" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_alarm_counts.test", "total_instance_count", "3"),
					resource.TestCheckResourceAttr("data.ssp_alarm_counts.test", "open_instance_counts.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_alarm_counts.test", "open_instance_counts.0.category", "ndr"),
					resource.TestCheckResourceAttr("data.ssp_alarm_counts.test", "open_instance_counts.0.counts.0.severity", "HIGH"),
					resource.TestCheckResourceAttr("data.ssp_alarm_counts.test", "open_instance_counts.0.counts.0.count", "2"),
				),
			},
		},
	})
}
