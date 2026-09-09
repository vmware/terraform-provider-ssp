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

type alarmStateMockAPI struct {
	mu    sync.Mutex
	state string
}

func newAlarmStateMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &alarmStateMockAPI{state: "OPEN"}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/alarms/states", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("updated_state") {
		case "ACKNOWLEDGE":
			m.mu.Lock()
			m.state = "ACKNOWLEDGED"
			m.mu.Unlock()
		case "SUPPRESS":
			m.mu.Lock()
			m.state = "SUPPRESSED"
			m.mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /ssp/alarms/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		state := m.state
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "alarm-1",
			"feature_name": "ndr",
			"severity":     "MEDIUM",
			"summary":      "Test alarm",
			"resource_id":  "res-1",
			"value":        "42",
			"state":        state,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitAlarmStateConfig(host, desiredState string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_alarm_state" "test" {
  id            = "alarm-1"
  desired_state = "` + desiredState + `"
}
`
}

// TestUnitAlarmStateResource exercises ssp_alarm_state's Create and Read
// against a mocked SSP runtime API. No Update test: id has RequiresReplace
// and desired_state changes are covered by re-Create semantics implicitly;
// no Delete verification since there is no API to undo an acknowledgement.
func TestUnitAlarmStateResource(t *testing.T) {
	srv := newAlarmStateMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitAlarmStateConfig(srv.URL, "ACKNOWLEDGE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_alarm_state.test", "id", "alarm-1"),
					resource.TestCheckResourceAttr("ssp_alarm_state.test", "desired_state", "ACKNOWLEDGE"),
					resource.TestCheckResourceAttr("ssp_alarm_state.test", "state", "ACKNOWLEDGED"),
					resource.TestCheckResourceAttr("ssp_alarm_state.test", "feature_name", "ndr"),
				),
			},
		},
	})
}
