// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// recurringBackupConfigMockAPI simulates the singleton
// /ssp/backup/recurring/config API: GET returns 204 until a config has been
// PUT, matching the resource's "204 means no config exists yet" logic.
type recurringBackupConfigMockAPI struct {
	mu       sync.Mutex
	exists   bool
	revision int
	body     map[string]any
}

func newRecurringBackupConfigMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &recurringBackupConfigMockAPI{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/backup/recurring/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.exists {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.body)
	})
	mux.HandleFunc("PUT /ssp/backup/recurring/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.exists = true
		m.revision++
		body["_revision"] = m.revision
		m.body = body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.body)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitRecurringBackupConfigConfig(host string, hourOfDay int) string {
	return testUnitRuntimeProviderConfig(host) + fmt.Sprintf(`
resource "ssp_recurring_backup_config" "test" {
  enabled              = true
  backup_type          = "FULL_BACKUP"
  backup_schedule_type = "WEEKLY"

  backup_schedule_weekly = {
    days_of_week  = ["MONDAY", "THURSDAY"]
    hour_of_day   = %d
    minute_of_day = 30
  }
}
`, hourOfDay)
}

// TestUnitRecurringBackupConfigResource exercises
// ssp_recurring_backup_config's Create, Read, and Update (hour_of_day change
// within the nested backup_schedule_weekly block) against a mocked SSP
// runtime API. No Delete verification: the API has no DELETE for this
// singleton resource (FSDD §5.1.2) and the resource's Delete() is a
// documented no-op.
func TestUnitRecurringBackupConfigResource(t *testing.T) {
	srv := newRecurringBackupConfigMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRecurringBackupConfigConfig(srv.URL, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "id", "singleton"),
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "enabled", "true"),
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "backup_schedule_type", "WEEKLY"),
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "backup_schedule_weekly.days_of_week.0", "MONDAY"),
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "backup_schedule_weekly.hour_of_day", "2"),
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "backup_schedule_weekly.minute_of_day", "30"),
				),
			},
			// Update: change hour_of_day within the nested weekly schedule block.
			{
				Config: testUnitRecurringBackupConfigConfig(srv.URL, 5),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "id", "singleton"),
					resource.TestCheckResourceAttr("ssp_recurring_backup_config.test", "backup_schedule_weekly.hour_of_day", "5"),
				),
			},
		},
	})
}
