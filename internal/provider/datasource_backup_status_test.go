package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newBackupStatusDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/backup/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id":                  "backup-job-123",
					"status":              "SUCCESS",
					"backup_type":         "FULL_BACKUP",
					"backup_trigger_type": "ON_DEMAND",
					"backup_start_time":   1700000000000,
					"backup_end_time":     1700000300000,
					"progress":            100,
					"progress_message":    "Backup completed",
					"version":             "5.2.0",
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitBackupStatusDataSource(t *testing.T) {
	srv := newBackupStatusDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_backup_status" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.backup_id", "backup-job-123"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.status", "SUCCESS"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.backup_type", "FULL_BACKUP"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.backup_trigger_type", "ON_DEMAND"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.progress", "100"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.version", "5.2.0"),
				),
			},
		},
	})
}
