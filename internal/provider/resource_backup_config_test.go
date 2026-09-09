// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// backupConfigMockAPI simulates the singleton /ssp/backup/config API: GET
// returns 204 until a config has been PUT, matching the resource's
// "204 means no config exists yet" Create/Read/ImportState logic.
type backupConfigMockAPI struct {
	mu       sync.Mutex
	exists   bool
	revision int
	body     map[string]any
}

func newBackupConfigMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &backupConfigMockAPI{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/backup/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.exists {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.body)
	})
	mux.HandleFunc("PUT /ssp/backup/config", func(w http.ResponseWriter, r *http.Request) {
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

func testUnitBackupConfigConfig(host, serverAddress, backupLocation string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_backup_config" "test" {
  server_address  = "` + serverAddress + `"
  protocol        = "SFTP"
  port            = 22
  username        = "sftpuser"
  ssh_public_key  = "ssh-rsa AAAA..."
  backup_location = "` + backupLocation + `"
  password        = "P@ssw0rd!"
  passphrase      = "correct-horse-battery-staple"
}
`
}

// TestUnitBackupConfigResource exercises ssp_backup_config's Create, Read,
// and Update (server_address change) against a mocked SSP runtime API. No
// Delete verification: the API has no DELETE for this singleton resource
// (FSDD §5.1.2) and the resource's Delete() is a documented no-op.
func TestUnitBackupConfigResource(t *testing.T) {
	srv := newBackupConfigMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitBackupConfigConfig(srv.URL, "backup.corp.local", "/backups/ssp"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_backup_config.test", "id", "singleton"),
					resource.TestCheckResourceAttr("ssp_backup_config.test", "server_address", "backup.corp.local"),
					resource.TestCheckResourceAttr("ssp_backup_config.test", "protocol", "SFTP"),
					resource.TestCheckResourceAttr("ssp_backup_config.test", "port", "22"),
					resource.TestCheckResourceAttr("ssp_backup_config.test", "username", "sftpuser"),
					resource.TestCheckResourceAttr("ssp_backup_config.test", "backup_location", "/backups/ssp"),
				),
			},
			// Update: change server_address in place.
			{
				Config: testUnitBackupConfigConfig(srv.URL, "backup2.corp.local", "/backups/ssp"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_backup_config.test", "id", "singleton"),
					resource.TestCheckResourceAttr("ssp_backup_config.test", "server_address", "backup2.corp.local"),
				),
			},
			// Delete Testing is handled automatically by terraform-plugin-testing at
			// the end of Test; the mock's GET keeps returning the last PUT body
			// since ssp_backup_config's Delete() makes no remote call.
		},
	})
}

// TestUnitBackupConfigResource_TrailingSlashRejected verifies the
// noTrailingSlash() validator catches a backup_location ending in "/" at
// plan/validate time with a clear error, instead of either silently
// normalizing it or letting it through to fail apply with a confusing
// "Provider produced inconsistent result after apply" (the API strips a
// trailing slash server-side, and backup_location is a Required,
// non-Computed attribute whose post-apply state must exactly equal its
// planned value).
func TestUnitBackupConfigResource_TrailingSlashRejected(t *testing.T) {
	srv := newBackupConfigMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testUnitBackupConfigConfig(srv.URL, "backup.corp.local", "/backups/ssp/"),
				ExpectError: regexp.MustCompile(`Trailing Slash Not Allowed`),
			},
		},
	})
}
