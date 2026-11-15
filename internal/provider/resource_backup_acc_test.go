// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const testAccBackupName = "ssp_backup.test"

func testAccBackupConfig(attrs string) string {
	return fmt.Sprintf(`
resource "ssp_backup" "test" {
  backup_type = "FULL_BACKUP"
%s
}
`, attrs)
}

// testAccRequireBackupConfigured skips the test unless the platform already has
// a backup server configured; taking a backup needs one.
func testAccRequireBackupConfigured(t *testing.T) {
	t.Helper()

	status, body := testAccAPIRequest(t, http.MethodGet, "/ssp/backup/config", nil)
	if status != http.StatusOK || !regexp.MustCompile(`"server_address"\s*:\s*"[^"]+"`).Match(body) {
		t.Skip("a backup server must be configured on the platform (see ssp_backup_config) to take a backup")
	}
}

// testAccCheckBackupsKept confirms that destroying ssp_backup leaves the backup
// on the platform: backup records are an audit trail and Delete is state-only.
func testAccCheckBackupsKept(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ssp_backup" {
				continue
			}
			status, _ := testAccAPIRequest(t, http.MethodGet, "/ssp/backup/status/"+rs.Primary.ID, nil)
			if status != http.StatusOK {
				return fmt.Errorf("backup %s was removed from the platform by destroy (status %d)", rs.Primary.ID, status)
			}
		}
		return nil
	}
}

// TestAccBackupResource takes real backups: a create, then a replacement for
// every argument that changes. A backup cannot be updated, so each change takes
// a new one, and the earlier backups stay on the platform.
func TestAccBackupResource(t *testing.T) {
	testAccRequireOptIn(t, "SSP_ACC_ALLOW_BACKUP", "it takes real backups on the configured SFTP server")
	name := acctest.RandomWithPrefix("tf-acc-backup")
	var ids []string
	capture := resource.TestCheckResourceAttrWith(testAccBackupName, "id", func(v string) error {
		ids = append(ids, v)
		return nil
	})
	replaced := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccBackupName, plancheck.ResourceActionReplace)},
	}
	notSame := func(*terraform.State) error {
		if len(ids) >= 2 && ids[len(ids)-1] == ids[len(ids)-2] {
			return fmt.Errorf("the backup was not replaced, id %s is unchanged", ids[len(ids)-1])
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			testAccRequireBackupConfigured(t)
			testAccWaitNoBackupInProgress(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBackupsKept(t),
		Steps: []resource.TestStep{
			{
				Config: testAccBackupConfig(fmt.Sprintf("name = %q", name+"-1")),
				Check: resource.ComposeAggregateTestCheckFunc(
					capture,
					resource.TestCheckResourceAttr(testAccBackupName, "status", "SUCCESS"),
					resource.TestCheckResourceAttr(testAccBackupName, "progress", "100"),
					resource.TestCheckResourceAttr(testAccBackupName, "action", "BACKUP"),
					resource.TestCheckResourceAttr(testAccBackupName, "error_messages.#", "0"),
					resource.TestCheckResourceAttrWith(testAccBackupName, "file_size", func(v string) error {
						if v == "" || v == "0" {
							return fmt.Errorf("file_size = %q, want a positive size", v)
						}
						return nil
					}),
					resource.TestCheckResourceAttrWith(testAccBackupName, "no_of_entities", func(v string) error {
						if v == "" || v == "0" {
							return fmt.Errorf("no_of_entities = %q, want at least 1", v)
						}
						return nil
					}),
				),
			},
			{Config: testAccBackupConfig(fmt.Sprintf("name = %q", name+"-1")), PlanOnly: true},
			{
				Config:           testAccBackupConfig(fmt.Sprintf("name = %q", name+"-2")),
				ConfigPlanChecks: replaced,
				Check:            resource.ComposeAggregateTestCheckFunc(capture, notSame, resource.TestCheckResourceAttr(testAccBackupName, "status", "SUCCESS")),
			},
			{
				Config:           testAccBackupConfig(fmt.Sprintf("name = %q\ndescription = \"changed description\"", name+"-2")),
				ConfigPlanChecks: replaced,
				Check:            resource.ComposeAggregateTestCheckFunc(capture, notSame, resource.TestCheckResourceAttr(testAccBackupName, "description", "changed description")),
			},
			// Removing name and description replaces it again.
			{
				Config:           testAccBackupConfig(""),
				ConfigPlanChecks: replaced,
				Check: resource.ComposeAggregateTestCheckFunc(
					capture, notSame,
					resource.TestCheckNoResourceAttr(testAccBackupName, "name"),
					resource.TestCheckNoResourceAttr(testAccBackupName, "description"),
				),
			},
			{Config: testAccBackupConfig(""), PlanOnly: true},
			// Every earlier backup must still be on the platform.
			{
				Config: testAccBackupConfig(""),
				Check: func(*terraform.State) error {
					for _, id := range ids[:len(ids)-1] {
						if status, _ := testAccAPIRequest(t, http.MethodGet, "/ssp/backup/status/"+id, nil); status != http.StatusOK {
							return fmt.Errorf("replaced backup %s is gone from the platform (status %d)", id, status)
						}
					}
					return nil
				},
			},
		},
	})
}

// The longest name is accepted, and special characters survive a round trip.
func TestAccBackupResource_NameBoundaries(t *testing.T) {
	testAccRequireOptIn(t, "SSP_ACC_ALLOW_BACKUP", "it takes real backups on the configured SFTP server")

	long := testAccStringOfLen(255, "n")
	special := `q"uote & <tag> %s ü ø`

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			testAccRequireBackupConfigured(t)
			testAccWaitNoBackupInProgress(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBackupsKept(t),
		Steps: []resource.TestStep{
			{
				Config: testAccBackupConfig(fmt.Sprintf("name = %q", long)),
				Check:  resource.TestCheckResourceAttr(testAccBackupName, "name", long),
			},
			{Config: testAccBackupConfig(fmt.Sprintf("name = %q", long)), PlanOnly: true},
			{
				Config: testAccBackupConfig(fmt.Sprintf("name = %q\ndescription = %q", special, special)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccBackupName, "name", special),
					resource.TestCheckResourceAttr(testAccBackupName, "description", special),
				),
			},
			{Config: testAccBackupConfig(fmt.Sprintf("name = %q\ndescription = %q", special, special)), PlanOnly: true},
		},
	})
}

// Unsupported values are rejected before anything is backed up: the type and the
// action at plan time, over-long text by the platform.
func TestAccBackupResource_Invalid(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "ssp_backup" "test" {
  backup_type = "INCREMENTAL"
}
`,
				ExpectError: regexp.MustCompile(`(?s)backup_type.*must be one of`),
			},
			{
				Config:      testAccBackupConfig(`action = "RESTORE"`),
				ExpectError: regexp.MustCompile(`(?s)action.*must be one of`),
			},
			{
				Config:      testAccBackupConfig(fmt.Sprintf("name = %q", testAccStringOfLen(256, "n"))),
				ExpectError: regexp.MustCompile(`(?s)API error 400.*255`),
			},
			{
				Config:      testAccBackupConfig(fmt.Sprintf("description = %q", testAccStringOfLen(256, "d"))),
				ExpectError: regexp.MustCompile(`(?s)API error 400.*255`),
			},
			{
				Config:      `resource "ssp_backup" "test" {}`,
				ExpectError: regexp.MustCompile(`(?s)Missing required argument.*backup_type`),
			},
		},
	})
}
