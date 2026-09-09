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

// newBackupMockServer simulates POST /ssp/backup (trigger) and
// GET /ssp/backup/status/{id} (poll), resolving to a terminal SUCCESS status
// on the first poll — this test exercises ssp_backup's Create/Read wiring,
// not the multi-iteration polling loop itself (that is covered separately
// against client.WaitForBackupComplete in internal/provider/client).
func newBackupMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/backup", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "backup-1", "status_url": "/ssp/backup/status/backup-1"})
	})
	mux.HandleFunc("GET /ssp/backup/status/backup-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "backup-1",
			"status":      "SUCCESS",
			"backup_type": "FULL_BACKUP",
			"progress":    100,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitBackupConfig(host string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_backup" "test" {
  backup_type = "FULL_BACKUP"
  name        = "tf-unit-test-backup"
}
`
}

// TestUnitBackupResource exercises ssp_backup's Create (trigger + poll to
// completion) and Read against a mocked SSP runtime API. No Update test:
// ssp_backup.Update() unconditionally returns an error (backups are
// immutable), and every schema attribute besides the Computed status fields
// carries RequiresReplace. No Delete verification: backup records are audit
// trails the API never deletes, and the resource's Delete() is a documented
// no-op.
func TestUnitBackupResource(t *testing.T) {
	srv := newBackupMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitBackupConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_backup.test", "id", "backup-1"),
					resource.TestCheckResourceAttr("ssp_backup.test", "backup_type", "FULL_BACKUP"),
					resource.TestCheckResourceAttr("ssp_backup.test", "action", "BACKUP"),
					resource.TestCheckResourceAttr("ssp_backup.test", "name", "tf-unit-test-backup"),
					resource.TestCheckResourceAttr("ssp_backup.test", "status", "SUCCESS"),
					resource.TestCheckResourceAttr("ssp_backup.test", "progress", "100"),
				),
			},
		},
	})
}
