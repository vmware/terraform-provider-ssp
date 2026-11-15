// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// newBackupStatusDataSourceMockServer serves 25 backups in the platform's shape:
// the list is under "result" and, without page_size, only 10 are returned.
func newBackupStatusDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	const total = 25
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/backup/status", func(w http.ResponseWriter, r *http.Request) {
		offset, pageSize := 0, 10
		if v := r.URL.Query().Get("offset"); v != "" {
			offset, _ = strconv.Atoi(v)
		}
		if v := r.URL.Query().Get("page_size"); v != "" {
			pageSize, _ = strconv.Atoi(v)
		}
		if pageSize > 10 {
			pageSize = 10 // the server caps a page at 10 here, so the provider must page
		}
		var page []map[string]any
		for n := offset; n < total && len(page) < pageSize; n++ {
			page = append(page, map[string]any{
				"id":                  fmt.Sprintf("backup-job-%d", n+1),
				"status":              "SUCCESS",
				"backup_type":         "FULL_BACKUP",
				"backup_trigger_type": "MANUAL",
				"backup_start_time":   1700000000000,
				"backup_end_time":     1700000300000,
				"progress":            100,
				"progress_message":    "Backup completed",
				"version":             "5.2.0",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result":             page,
			"number_of_results":  len(page),
			"total_result_count": total,
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
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.#", "25"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.backup_id", "backup-job-1"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.status", "SUCCESS"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.backup_type", "FULL_BACKUP"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.backup_trigger_type", "MANUAL"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.progress", "100"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.0.version", "5.2.0"),
					resource.TestCheckResourceAttr("data.ssp_backup_status.test", "results.24.backup_id", "backup-job-25"),
				),
			},
		},
	})
}
