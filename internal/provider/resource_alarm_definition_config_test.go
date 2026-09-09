// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

type alarmDefinitionMockAPI struct {
	mu      sync.Mutex
	enabled bool
}

func newAlarmDefinitionMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &alarmDefinitionMockAPI{enabled: true}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/alarms/definitions/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.enabled = body.Enabled
		m.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /ssp/alarms/definitions/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		enabled := m.enabled
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_result_count": 1,
			"results": []map[string]any{
				{
					"id":                   "alarm-def-1",
					"feature_name":         "nsx_manager_communication",
					"feature_display_name": "NSX Manager Communication",
					"event_type":           "nsx_manager_disconnected",
					"severity":             "HIGH",
					"summary":              "NSX Manager is disconnected",
					"enabled":              enabled,
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitAlarmDefinitionConfigConfig(host string, enabled bool) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_alarm_definition_config" "test" {
  id      = "alarm-def-1"
  enabled = ` + boolLiteral(enabled) + `
}
`
}

func boolLiteral(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TestUnitAlarmDefinitionConfigResource exercises ssp_alarm_definition_config's
// Create, Read, and Update against a mocked SSP runtime API. No Delete
// verification: alarm definitions can't be deleted via the API.
func TestUnitAlarmDefinitionConfigResource(t *testing.T) {
	srv := newAlarmDefinitionMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitAlarmDefinitionConfigConfig(srv.URL, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_alarm_definition_config.test", "id", "alarm-def-1"),
					resource.TestCheckResourceAttr("ssp_alarm_definition_config.test", "enabled", "true"),
					resource.TestCheckResourceAttr("ssp_alarm_definition_config.test", "feature_name", "nsx_manager_communication"),
					resource.TestCheckResourceAttr("ssp_alarm_definition_config.test", "severity", "HIGH"),
				),
			},
			{
				Config: testUnitAlarmDefinitionConfigConfig(srv.URL, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_alarm_definition_config.test", "enabled", "false"),
				),
			},
		},
	})
}
