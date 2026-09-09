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

func newAlarmDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/alarms/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "alarm-1",
			"definition_id": "alarm-def-1",
			"feature_name":  "ndr",
			"event_type":    "ndr_disconnected",
			"severity":      "HIGH",
			"summary":       "NDR disconnected",
			"description":   "NDR service is disconnected",
			"resource_id":   "res-1",
			"value":         "true",
			"state":         "OPEN",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitAlarmDataSource(t *testing.T) {
	srv := newAlarmDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_alarm" "test" {
  id = "alarm-1"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_alarm.test", "id", "alarm-1"),
					resource.TestCheckResourceAttr("data.ssp_alarm.test", "state", "OPEN"),
					resource.TestCheckResourceAttr("data.ssp_alarm.test", "severity", "HIGH"),
				),
			},
		},
	})
}
