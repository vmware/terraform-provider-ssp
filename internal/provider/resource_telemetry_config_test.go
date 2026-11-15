// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// telemetryMockAPI simulates GET/PUT /ssp/telemetry/config the way the live
// appliance behaves: the collector ID only appears once CEIP is accepted,
// day_of_week / day_of_month are only returned for the frequency they apply to,
// omitted hour_of_day falls back to 2, and an out-of-range value is a 400.
type telemetryMockAPI struct {
	mu       sync.Mutex
	revision int
	body     map[string]any
	lastPut  map[string]any
	failPut  bool
}

func newTelemetryMockServer(t *testing.T) (*httptest.Server, *telemetryMockAPI) {
	t.Helper()
	m := &telemetryMockAPI{body: map[string]any{
		"ceip_acceptance":               false,
		"telemetry_agreement_displayed": false,
		"telemetry_schedule": map[string]any{
			"frequency_type": "WEEKLY_TELEMETRY_SCHEDULE", "hour_of_day": 2, "minutes": 0, "day_of_week": "SUNDAY",
		},
	}}
	writeErr := func(w http.ResponseWriter, code int, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": code, "module_name": "common-services", "error_message": msg})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/telemetry/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		resp := map[string]any{"_revision": m.revision}
		for k, v := range m.body {
			resp[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("PUT /ssp/telemetry/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.failPut {
			writeErr(w, http.StatusBadRequest, "Invalid request")
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.lastPut = body
		if int(body["_revision"].(float64)) != m.revision {
			writeErr(w, http.StatusBadRequest, "Invalid request")
			return
		}
		out := map[string]any{
			"ceip_acceptance":               body["ceip_acceptance"],
			"telemetry_agreement_displayed": body["telemetry_agreement_displayed"],
		}
		// Assigned the first time CEIP is accepted, then kept for good.
		if id, ok := m.body["telemetry_collector_instance_id"]; ok {
			out["telemetry_collector_instance_id"] = id
		} else if body["ceip_acceptance"] == true {
			out["telemetry_collector_instance_id"] = "21abaecc-80b6-4c8e-a798-846c8b1f91af"
		}
		in, ok := body["telemetry_schedule"].(map[string]any)
		if !ok {
			in = map[string]any{"frequency_type": "WEEKLY_TELEMETRY_SCHEDULE"}
		}
		{
			hour, minutes := 2.0, 0.0
			if v, ok := in["hour_of_day"].(float64); ok {
				hour = v
			}
			if v, ok := in["minutes"].(float64); ok {
				minutes = v
			}
			if hour > 23 {
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("hour_of_day value %d has exceeded its maximum valid value 23", int(hour)))
				return
			}
			sched := map[string]any{"frequency_type": in["frequency_type"], "hour_of_day": hour, "minutes": minutes}
			switch in["frequency_type"] {
			case "WEEKLY_TELEMETRY_SCHEDULE":
				sched["day_of_week"] = "SUNDAY"
				if v, ok := in["day_of_week"]; ok {
					sched["day_of_week"] = v
				}
			case "MONTHLY_TELEMETRY_SCHEDULE":
				sched["day_of_month"] = 1.0
				if v, ok := in["day_of_month"]; ok {
					sched["day_of_month"] = v
				}
			}
			out["telemetry_schedule"] = sched
		}
		m.body = out
		m.revision++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, m
}

func testUnitTelemetryConfig(host string, ceip bool, schedule string) string {
	return testUnitRuntimeProviderConfig(host) + fmt.Sprintf(`
resource "ssp_telemetry_config" "test" {
  ceip_acceptance               = %t
  telemetry_agreement_displayed = %t
  %s
}
`, ceip, ceip, schedule)
}

// TestUnitTelemetryConfigResource_Schedules walks every frequency type (including
// hour_of_day = 0, which must reach the API) and checks each apply converges with
// no follow-up diff.
func TestUnitTelemetryConfigResource_Schedules(t *testing.T) {
	srv, m := newTelemetryMockServer(t)
	const r = "ssp_telemetry_config.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// First apply: collector ID and schedule are unknown until the API answers.
				Config: testUnitTelemetryConfig(srv.URL, true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(r, "id", "singleton"),
					resource.TestCheckResourceAttr(r, "telemetry_collector_instance_id", "21abaecc-80b6-4c8e-a798-846c8b1f91af"),
					resource.TestCheckResourceAttr(r, "telemetry_schedule.frequency_type", "WEEKLY_TELEMETRY_SCHEDULE"),
					resource.TestCheckResourceAttr(r, "telemetry_schedule.day_of_week", "SUNDAY"),
					resource.TestCheckNoResourceAttr(r, "telemetry_schedule.day_of_month"),
				),
			},
			{
				Config: testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { frequency_type = "MONTHLY_TELEMETRY_SCHEDULE", day_of_month = 15, hour_of_day = 0, minutes = 30 }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(r, "telemetry_schedule.day_of_month", "15"),
					resource.TestCheckResourceAttr(r, "telemetry_schedule.hour_of_day", "0"),
					resource.TestCheckNoResourceAttr(r, "telemetry_schedule.day_of_week"),
					func(*terraform.State) error {
						m.mu.Lock()
						defer m.mu.Unlock()
						sched := m.lastPut["telemetry_schedule"].(map[string]any)
						if v, ok := sched["hour_of_day"]; !ok || v.(float64) != 0 {
							return fmt.Errorf("hour_of_day=0 must be sent to the API, payload: %v", sched)
						}
						if _, ok := sched["day_of_week"]; ok {
							return fmt.Errorf("day_of_week must not be sent for a monthly schedule, payload: %v", sched)
						}
						return nil
					},
				),
			},
			{
				Config: testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { frequency_type = "DAILY_TELEMETRY_SCHEDULE", hour_of_day = 23, minutes = 15 }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(r, "telemetry_schedule.frequency_type", "DAILY_TELEMETRY_SCHEDULE"),
					resource.TestCheckNoResourceAttr(r, "telemetry_schedule.day_of_month"),
					resource.TestCheckNoResourceAttr(r, "telemetry_schedule.day_of_week"),
				),
			},
			{
				Config: testUnitTelemetryConfig(srv.URL, false, `telemetry_schedule = { day_of_week = "FRIDAY" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(r, "ceip_acceptance", "false"),
					resource.TestCheckResourceAttr(r, "telemetry_schedule.frequency_type", "WEEKLY_TELEMETRY_SCHEDULE"),
					resource.TestCheckResourceAttr(r, "telemetry_schedule.day_of_week", "FRIDAY"),
				),
			},
			{ResourceName: r, ImportState: true, ImportStateId: "singleton", ImportStateVerify: true},
		},
	})
}

// TestUnitTelemetryConfigResource_Negative checks config-time validation and that an
// API error body is surfaced to the user unchanged.
func TestUnitTelemetryConfigResource_Negative(t *testing.T) {
	srv, m := newTelemetryMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { hour_of_day = 24 }`),
				ExpectError: regexp.MustCompile(`(?s)hour_of_day.*between 0 and 23`),
			},
			{
				Config:      testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { minutes = 60 }`),
				ExpectError: regexp.MustCompile(`(?s)minutes.*between 0 and 59`),
			},
			{
				Config:      testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { frequency_type = "MONTHLY_TELEMETRY_SCHEDULE", day_of_month = 32 }`),
				ExpectError: regexp.MustCompile(`(?s)day_of_month.*between 1 and 31`),
			},
			{
				Config:      testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { frequency_type = "HOURLY_TELEMETRY_SCHEDULE" }`),
				ExpectError: regexp.MustCompile(`(?s)frequency_type.*DAILY_TELEMETRY_SCHEDULE`),
			},
			{
				Config:      testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { frequency_type = "DAILY_TELEMETRY_SCHEDULE", day_of_week = "MONDAY" }`),
				ExpectError: regexp.MustCompile(`day_of_week is only valid when frequency_type is WEEKLY_TELEMETRY_SCHEDULE`),
			},
			{
				Config:      testUnitTelemetryConfig(srv.URL, true, `telemetry_schedule = { day_of_month = 5 }`),
				ExpectError: regexp.MustCompile(`day_of_month is only valid when frequency_type is MONTHLY_TELEMETRY_SCHEDULE`),
			},
			{
				PreConfig:   func() { m.mu.Lock(); m.failPut = true; m.mu.Unlock() },
				Config:      testUnitTelemetryConfig(srv.URL, true, ""),
				ExpectError: regexp.MustCompile(`(?s)API error 400: \{.*"error_code":400.*"error_message":"Invalid\s+request"`),
			},
		},
	})
}
