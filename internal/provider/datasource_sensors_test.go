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

func newSensorsDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sensors/appliances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id":           "sensor-uuid-001",
					"display_name": "Host-Sensor-01",
					"description":  "Security sensor on ESXi node 01",
					"token_id":     "token-uuid-111",
					"algorithm":    "RSA",
					"key_size":     2048,
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitSensorsDataSource(t *testing.T) {
	srv := newSensorsDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_sensors" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_sensors.test", "results.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_sensors.test", "results.0.id", "sensor-uuid-001"),
					resource.TestCheckResourceAttr("data.ssp_sensors.test", "results.0.display_name", "Host-Sensor-01"),
					resource.TestCheckResourceAttr("data.ssp_sensors.test", "results.0.algorithm", "RSA"),
					resource.TestCheckResourceAttr("data.ssp_sensors.test", "results.0.key_size", "2048"),
				),
			},
		},
	})
}
