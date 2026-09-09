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

// newRestoreMockServer simulates POST /ssp/restore (trigger) and
// GET /ssp/restore/status/{id} (poll), resolving to a terminal SUCCESS status
// on the first poll — this test exercises ssp_restore's Create/Read wiring,
// not the multi-iteration polling loop itself (that is covered separately
// against client.WaitForRestoreComplete in internal/provider/client).
func newRestoreMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/restore", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "restore-1", "status_url": "/ssp/restore/status/restore-1"})
	})
	mux.HandleFunc("GET /ssp/restore/status/restore-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "restore-1",
			"backup_id": "backup-1",
			"status":    "SUCCESS",
			"progress":  100,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitRestoreConfig(host string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_restore" "test" {
  backup_id = "backup-1"
}
`
}

// TestUnitRestoreResource exercises ssp_restore's Create (trigger + poll to
// completion) and Read against a mocked SSP runtime API. No Update test:
// ssp_restore.Update() unconditionally returns an error (restores are
// immutable), and every schema attribute besides the Computed status fields
// carries RequiresReplace. No Delete verification: restore records are audit
// trails the API never deletes, and the resource's Delete() is a documented
// no-op.
func TestUnitRestoreResource(t *testing.T) {
	srv := newRestoreMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRestoreConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_restore.test", "id", "restore-1"),
					resource.TestCheckResourceAttr("ssp_restore.test", "backup_id", "backup-1"),
					resource.TestCheckResourceAttr("ssp_restore.test", "action", "RESTORE"),
					resource.TestCheckResourceAttr("ssp_restore.test", "force_restore", "false"),
					resource.TestCheckResourceAttr("ssp_restore.test", "status", "SUCCESS"),
					resource.TestCheckResourceAttr("ssp_restore.test", "progress", "100"),
				),
			},
		},
	})
}
