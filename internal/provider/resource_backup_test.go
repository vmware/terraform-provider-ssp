// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
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
			"id":             "backup-1",
			"status":         "SUCCESS",
			"backup_type":    "FULL_BACKUP",
			"progress":       100,
			"file_size":      104857600,
			"no_of_entities": 42,
			"error_messages": []string{},
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
					resource.TestCheckResourceAttr("ssp_backup.test", "file_size", "104857600"),
					resource.TestCheckResourceAttr("ssp_backup.test", "no_of_entities", "42"),
					resource.TestCheckResourceAttr("ssp_backup.test", "error_messages.#", "0"),
				),
			},
		},
	})
}

// TestUnitBackupResource_APIErrorSurfaced verifies that the API's error body
// (status code, error_code and message) reaches the user unchanged when the
// backup cannot be started, and that nothing is left in state.
func TestUnitBackupResource_APIErrorSurfaced(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/backup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error_code":972051,"error_message":"Registered NSX manager site is not in Ready state.","module_name":"cluster-api"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testUnitBackupConfig(srv.URL),
				ExpectError: regexp.MustCompile(`(?s)API error 400.*972051.*Registered NSX manager\s+site is not in Ready state`),
			},
		},
	})
}

// TestUnitBackupResource_RemovedWhenRecordPurged verifies that a refresh drops
// the resource from state when the status endpoint reports the backup as
// unknown. The real API answers HTTP 400 with error_code 972011 for this,
// not 404.
func TestUnitBackupResource_RemovedWhenRecordPurged(t *testing.T) {
	var purged atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/backup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "backup-1"})
	})
	mux.HandleFunc("GET /ssp/backup/status/backup-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if purged.Load() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error_code":972011,"error_message":"Backup status not found for ID: backup-1.","module_name":"cluster-api"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "backup-1", "status": "SUCCESS", "backup_type": "FULL_BACKUP", "progress": 100})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: testUnitBackupConfig(srv.URL)},
			{
				PreConfig:          func() { purged.Store(true) },
				Config:             testUnitBackupConfig(srv.URL),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true, // resource dropped from state => plan wants to re-create it
			},
		},
	})
}
