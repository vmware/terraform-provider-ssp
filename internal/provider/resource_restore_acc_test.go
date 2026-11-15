// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testAccRestoreName = "ssp_restore.test"

func testAccRestoreConfig(backupID, attrs string) string {
	return fmt.Sprintf(`
resource "ssp_restore" "test" {
  backup_id = %q
%s
}
`, backupID, attrs)
}

// testAccWaitForPlatformUp polls the platform health until it is UP. A restore
// leaves the platform DEGRADED for several minutes.
func testAccWaitForPlatformUp(t *testing.T, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		status, body := testAccAPIRequest(t, http.MethodGet, "/ssp/cluster/monitor/platform/status", nil)
		if status == http.StatusOK && regexp.MustCompile(`"health"\s*:\s*"UP"`).Match(body) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the platform was not UP after %s", timeout)
		}
		time.Sleep(30 * time.Second)
	}
}

// A restore is refused up front when the action is not supported.
func TestAccRestoreResource_InvalidAction(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccRestoreConfig("00000000-0000-0000-0000-000000000000", `action = "ROLLBACK"`),
				ExpectError: regexp.MustCompile(`(?s)action.*must be one of`),
			},
			{
				Config:      `resource "ssp_restore" "test" {}`,
				ExpectError: regexp.MustCompile(`(?s)Missing required argument.*backup_id`),
			},
		},
	})
}

// A backup id the platform does not know is rejected at once, before any restore
// starts, and the platform error is shown.
func TestAccRestoreResource_UnknownBackup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccRestoreConfig("00000000-0000-0000-0000-000000000000", ""),
				ExpectError: regexp.MustCompile(`(?s)API error 400.*(972024|Backup\s+not\s+found)`),
			},
		},
	})
}

// TestAccRestoreResource restores the platform from a backup that the test takes
// itself. It is destructive and slow (10 to 15 minutes, with the platform
// DEGRADED afterwards), so it is opt-in. Changing force_restore or backup_id
// replaces the resource, which would start another restore.
func TestAccRestoreResource(t *testing.T) {
	testAccRequireOptIn(t, "SSP_ACC_ALLOW_RESTORE", "it restores the whole platform from a backup")

	cfg := func(attrs string) string {
		return `
resource "ssp_backup" "source" {
  backup_type = "FULL_BACKUP"
  name        = "tf-acc-restore-source"
}

resource "ssp_restore" "test" {
  backup_id = ssp_backup.source.id
` + attrs + `
}
`
	}

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			testAccRequireBackupConfigured(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(testAccRestoreName, "id"),
					resource.TestCheckResourceAttr(testAccRestoreName, "status", "SUCCESS"),
					resource.TestCheckResourceAttr(testAccRestoreName, "progress", "100"),
					resource.TestCheckResourceAttr(testAccRestoreName, "action", "RESTORE"),
					resource.TestCheckResourceAttr(testAccRestoreName, "force_restore", "false"),
					resource.TestCheckResourceAttr(testAccRestoreName, "error_messages.#", "0"),
					resource.TestCheckResourceAttrWith(testAccRestoreName, "no_of_entities", func(v string) error {
						if v == "" || v == "0" {
							return fmt.Errorf("no_of_entities = %q, want at least 1", v)
						}
						return nil
					}),
				),
			},
			{
				PreConfig: func() { testAccWaitForPlatformUp(t, 30*time.Minute) },
				Config:    cfg(""),
				PlanOnly:  true,
			},
			// force_restore forces a replacement, so the plan is not empty.
			{
				Config:             cfg("force_restore = true"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
