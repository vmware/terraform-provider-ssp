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

func newAlarmDefinitionsDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/alarms/definitions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_result_count": 1,
			"results": []map[string]any{
				{
					"id":                   "alarm-def-1",
					"feature_name":         "ndr",
					"feature_display_name": "NDR",
					"event_type":           "ndr_disconnected",
					"severity":             "HIGH",
					"summary":              "NDR disconnected",
					"enabled":              true,
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitAlarmDefinitionsDataSource(t *testing.T) {
	srv := newAlarmDefinitionsDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_alarm_definitions" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_alarm_definitions.test", "results.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_alarm_definitions.test", "results.0.id", "alarm-def-1"),
					resource.TestCheckResourceAttr("data.ssp_alarm_definitions.test", "results.0.severity", "HIGH"),
				),
			},
		},
	})
}
