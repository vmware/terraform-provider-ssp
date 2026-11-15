// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

const testAccRecurringName = "ssp_recurring_backup_config.test"

// testAccRecurringConfig builds the resource with the schedule type and the
// schedule blocks given as HCL. enabled stays false in these tests: enabling an
// INTERVAL schedule makes the platform start a backup at once.
func testAccRecurringConfig(enabled bool, scheduleType, backupType, blocks string) string {
	return fmt.Sprintf(`
resource "ssp_recurring_backup_config" "test" {
  enabled              = %t
  backup_type          = %q
  backup_schedule_type = %q
%s
}
`, enabled, backupType, scheduleType, blocks)
}

func testAccRecurringWeekly(days, extra string) string {
	return fmt.Sprintf(`
  backup_schedule_weekly = {
    days_of_week = %s
%s
  }
`, days, extra)
}

func testAccRecurringInterval(attr string) string {
	return fmt.Sprintf(`
  backup_schedule_interval = {
%s
  }
`, attr)
}

func testAccRecurringUpdate() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccRecurringName, plancheck.ResourceActionUpdate)},
	}
}

// TestAccRecurringBackupConfigResource moves between WEEKLY and INTERVAL, relies
// on the documented defaults, and imports. The schedule stays disabled.
func TestAccRecurringBackupConfigResource(t *testing.T) {
	weekly := testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`["MONDAY", "THURSDAY"]`, "    hour_of_day = 2\n    minute_of_day = 30"))
	weekly2 := testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`["TUESDAY", "FRIDAY", "SUNDAY"]`, "    hour_of_day = 23\n    minute_of_day = 45"))
	weeklyDefaults := testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`["MONDAY"]`, ""))
	interval := testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", testAccRecurringInterval("    hours_between_backups = 24"))
	intervalDefault := testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", testAccRecurringInterval(""))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: weekly,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccRecurringName, "id", "singleton"),
					resource.TestCheckResourceAttr(testAccRecurringName, "enabled", "false"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_type", "WEEKLY"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.days_of_week.#", "2"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.hour_of_day", "2"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.minute_of_day", "30"),
					resource.TestCheckNoResourceAttr(testAccRecurringName, "backup_schedule_interval"),
				),
			},
			{Config: weekly, PlanOnly: true},
			{
				Config:           weekly2,
				ConfigPlanChecks: testAccRecurringUpdate(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.days_of_week.#", "3"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.hour_of_day", "23"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.minute_of_day", "45"),
				),
			},
			// hour_of_day and minute_of_day omitted: both default to 0.
			{
				Config: weeklyDefaults,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.hour_of_day", "0"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_weekly.minute_of_day", "0"),
				),
			},
			{Config: weeklyDefaults, PlanOnly: true},

			// WEEKLY -> INTERVAL: the weekly block is dropped, not sent to the API.
			{
				Config:           interval,
				ConfigPlanChecks: testAccRecurringUpdate(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_type", "INTERVAL"),
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_interval.hours_between_backups", "24"),
					resource.TestCheckNoResourceAttr(testAccRecurringName, "backup_schedule_weekly"),
				),
			},
			{Config: interval, PlanOnly: true},
			// hours_between_backups omitted: defaults to 168.
			{
				Config: intervalDefault,
				Check:  resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_interval.hours_between_backups", "168"),
			},
			{Config: intervalDefault, PlanOnly: true},
			// The boundaries of 6 to 720 hours.
			{
				Config: testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", testAccRecurringInterval("    hours_between_backups = 6")),
				Check:  resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_interval.hours_between_backups", "6"),
			},
			{
				Config: testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", testAccRecurringInterval("    hours_between_backups = 720")),
				Check:  resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_interval.hours_between_backups", "720"),
			},

			// INTERVAL -> WEEKLY.
			{
				Config:           weekly,
				ConfigPlanChecks: testAccRecurringUpdate(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_type", "WEEKLY"),
					resource.TestCheckNoResourceAttr(testAccRecurringName, "backup_schedule_interval"),
				),
			},
			{
				ResourceName:      testAccRecurringName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			// Destroy is intentionally not tested: the API has only GET and PUT, so
			// Delete only removes the resource from state.
		},
	})
}

// A schedule type without its matching block is rejected at plan time.
func TestAccRecurringBackupConfigResource_MissingBlock(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", ""),
				ExpectError: regexp.MustCompile(`(?s)backup_schedule_weekly must be set`),
			},
			{
				Config:      testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", ""),
				ExpectError: regexp.MustCompile(`(?s)backup_schedule_interval must be set`),
			},
			{
				Config:      testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringInterval("    hours_between_backups = 24")),
				ExpectError: regexp.MustCompile(`(?s)backup_schedule_weekly must be set`),
			},
		},
	})
}

// Everything else is checked by the platform, and its error reaches the user.
func TestAccRecurringBackupConfigResource_Rejected(t *testing.T) {
	weeklyOK := testAccRecurringWeekly(`["MONDAY"]`, "")

	cases := []struct {
		label, cfg, wantErr string
	}{
		{"weekly with an interval block too", testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", weeklyOK+testAccRecurringInterval("    hours_between_backups = 24")), `(?s)API error 400.*interval backup schedule\s+must\s+be\s+empty`},
		{"interval of 5 hours", testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", testAccRecurringInterval("    hours_between_backups = 5")), `(?s)API error 400`},
		{"interval of 721 hours", testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", testAccRecurringInterval("    hours_between_backups = 721")), `(?s)API error 400`},
		{"hour_of_day 24", testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`["MONDAY"]`, "    hour_of_day = 24")), `(?s)API error 400`},
		{"minute_of_day 60", testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`["MONDAY"]`, "    minute_of_day = 60")), `(?s)API error 400`},
		{"invalid day", testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`["FUNDAY"]`, "")), `(?s)API error 400`},
		{"duplicate days", testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`["MONDAY", "MONDAY"]`, "")), `(?s)API error 400`},
		{"no days", testAccRecurringConfig(false, "WEEKLY", "FULL_BACKUP", testAccRecurringWeekly(`[]`, "")), `(?s)API error 400`},
		{"schedule type DAILY", testAccRecurringConfig(false, "DAILY", "FULL_BACKUP", weeklyOK), `(?s)API error 400`},
		{"backup type INCREMENTAL", testAccRecurringConfig(false, "WEEKLY", "INCREMENTAL", weeklyOK), `(?s)API error 400`},
	}

	var steps []resource.TestStep
	for _, c := range cases {
		steps = append(steps, resource.TestStep{Config: c.cfg, ExpectError: regexp.MustCompile(c.wantErr)})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// A change made on the platform is shown as a diff and put back by an apply.
func TestAccRecurringBackupConfigResource_Drift(t *testing.T) {
	cfg := testAccRecurringConfig(false, "INTERVAL", "FULL_BACKUP", testAccRecurringInterval("    hours_between_backups = 24"))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_interval.hours_between_backups", "24")},
			{
				PreConfig: func() {
					testAccAPIPutWithRevision(t, "/ssp/backup/recurring/config", map[string]any{
						"enabled":                  false,
						"backup_type":              "FULL_BACKUP",
						"backup_schedule_type":     "INTERVAL",
						"backup_schedule_interval": map[string]any{"hours_between_backups": 48},
					})
				},
				Config:           cfg,
				ConfigPlanChecks: testAccRecurringUpdate(),
				Check:            resource.TestCheckResourceAttr(testAccRecurringName, "backup_schedule_interval.hours_between_backups", "24"),
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}
