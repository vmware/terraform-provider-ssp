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

func TestAccTelemetryConfigResource(t *testing.T) {
	resourceName := "ssp_telemetry_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read Testing
			{
				Config: testAccTelemetryConfigConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "ceip_acceptance", "true"),
					resource.TestCheckResourceAttr(resourceName, "telemetry_agreement_displayed", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "telemetry_collector_instance_id"),
					resource.TestCheckResourceAttrSet(resourceName, "telemetry_schedule.frequency_type"),
				),
			},
			// ImportState Testing
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			// Destroy Testing is intentionally not performed: telemetry_config is a
			// persistent platform setting with no DELETE API, so Delete() is a
			// documented no-op (removes from Terraform state only, leaves the
			// remote configuration in place).
		},
	})
}

func testAccTelemetryConfigConfig(ceipAcceptance bool) string {
	return fmt.Sprintf(`
resource "ssp_telemetry_config" "test" {
  ceip_acceptance               = %t
  telemetry_agreement_displayed = true
}
`, ceipAcceptance)
}

const testAccTelemetryName = "ssp_telemetry_config.test"

func testAccTelemetryScheduleConfig(ceip bool, schedule string) string {
	return fmt.Sprintf(`
resource "ssp_telemetry_config" "test" {
  ceip_acceptance               = %t
  telemetry_agreement_displayed = %t
%s
}
`, ceip, ceip, schedule)
}

func testAccTelemetrySchedule(frequency string, attrs map[string]string) string {
	body := fmt.Sprintf("    frequency_type = %q\n", frequency)
	for _, k := range []string{"day_of_week", "day_of_month", "hour_of_day", "minutes"} {
		if v, ok := attrs[k]; ok {
			body += fmt.Sprintf("    %s = %s\n", k, v)
		}
	}
	return "  telemetry_schedule = {\n" + body + "  }\n"
}

func testAccCheckTelemetrySchedule(frequency string, attrs map[string]string) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(testAccTelemetryName, "telemetry_schedule.frequency_type", frequency),
	}
	for k, v := range attrs {
		checks = append(checks, resource.TestCheckResourceAttr(testAccTelemetryName, "telemetry_schedule."+k, v))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

// TestAccTelemetryConfigResource_Schedule moves the schedule through every
// frequency, keeps it when the block is omitted, toggles CEIP, and imports. It
// ends on the platform default (CEIP on, weekly on Sunday at 02:00).
func TestAccTelemetryConfigResource_Schedule(t *testing.T) {
	update := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccTelemetryName, plancheck.ResourceActionUpdate)},
	}
	weekly := testAccTelemetryScheduleConfig(true, testAccTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": `"FRIDAY"`, "hour_of_day": "3", "minutes": "45"}))
	daily := testAccTelemetryScheduleConfig(true, testAccTelemetrySchedule("DAILY_TELEMETRY_SCHEDULE", map[string]string{"hour_of_day": "23", "minutes": "15"}))
	monthly := testAccTelemetryScheduleConfig(true, testAccTelemetrySchedule("MONTHLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_month": "15", "hour_of_day": "5", "minutes": "30"}))
	omitted := testAccTelemetryScheduleConfig(true, "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: weekly,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccTelemetryName, "id", "singleton"),
					resource.TestCheckResourceAttrSet(testAccTelemetryName, "telemetry_collector_instance_id"),
					testAccCheckTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": "FRIDAY", "hour_of_day": "3", "minutes": "45"}),
				),
			},
			{Config: weekly, PlanOnly: true},
			{
				Config:           daily,
				ConfigPlanChecks: update,
				Check:            testAccCheckTelemetrySchedule("DAILY_TELEMETRY_SCHEDULE", map[string]string{"hour_of_day": "23", "minutes": "15"}),
			},
			{Config: daily, PlanOnly: true},
			{
				Config:           monthly,
				ConfigPlanChecks: update,
				Check:            testAccCheckTelemetrySchedule("MONTHLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_month": "15", "hour_of_day": "5", "minutes": "30"}),
			},
			{Config: monthly, PlanOnly: true},
			{
				Config:           weekly,
				ConfigPlanChecks: update,
				Check:            testAccCheckTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": "FRIDAY", "hour_of_day": "3", "minutes": "45"}),
			},

			// Omitting the block keeps the schedule that is in state, and plans no change.
			{
				Config: omitted,
				Check:  testAccCheckTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": "FRIDAY", "hour_of_day": "3", "minutes": "45"}),
			},
			{Config: omitted, PlanOnly: true},

			// Disabling CEIP.
			{
				Config: testAccTelemetryScheduleConfig(false, testAccTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": `"SUNDAY"`, "hour_of_day": "2", "minutes": "0"})),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccTelemetryName, "ceip_acceptance", "false"),
					resource.TestCheckResourceAttr(testAccTelemetryName, "telemetry_agreement_displayed", "false"),
				),
			},

			// Back to the platform default.
			{
				Config: testAccTelemetryScheduleConfig(true, testAccTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": `"SUNDAY"`, "hour_of_day": "2", "minutes": "0"})),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccTelemetryName, "ceip_acceptance", "true"),
					testAccCheckTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": "SUNDAY", "hour_of_day": "2", "minutes": "0"}),
				),
			},
			{
				ResourceName:      testAccTelemetryName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
		},
	})
}

// A schedule that mixes in a field of another frequency, an unsupported
// frequency, and out-of-range values are all rejected at plan time.
func TestAccTelemetryConfigResource_InvalidSchedule(t *testing.T) {
	cases := []struct {
		name      string
		frequency string
		attrs     map[string]string
		wantErr   string
	}{
		{"day_of_month on weekly", "WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_month": "10"}, `day_of_month is only valid when frequency_type is MONTHLY`},
		{"day_of_week on daily", "DAILY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": `"MONDAY"`}, `day_of_week is only valid when frequency_type is WEEKLY`},
		{"day_of_week on monthly", "MONTHLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": `"MONDAY"`}, `day_of_week is only valid when frequency_type is WEEKLY`},
		{"unsupported frequency", "HOURLY_TELEMETRY_SCHEDULE", nil, `(?s)frequency_type.*must be one of`},
		{"hour_of_day 24", "DAILY_TELEMETRY_SCHEDULE", map[string]string{"hour_of_day": "24"}, `(?s)hour_of_day.*between 0 and 23`},
		{"minutes 60", "DAILY_TELEMETRY_SCHEDULE", map[string]string{"minutes": "60"}, `(?s)minutes.*between 0 and 59`},
		{"day_of_month 32", "MONTHLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_month": "32"}, `(?s)day_of_month.*between 1 and 31`},
		{"day_of_month 0", "MONTHLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_month": "0"}, `(?s)day_of_month.*between 1 and 31`},
		{"invalid day_of_week", "WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": `"FUNDAY"`}, `(?s)day_of_week.*must be one of`},
	}

	var steps []resource.TestStep
	for _, c := range cases {
		steps = append(steps, resource.TestStep{
			Config:      testAccTelemetryScheduleConfig(true, testAccTelemetrySchedule(c.frequency, c.attrs)),
			ExpectError: regexp.MustCompile(c.wantErr),
		})
	}
	steps = append(steps, resource.TestStep{
		Config: `
resource "ssp_telemetry_config" "test" {
  telemetry_agreement_displayed = true
}
`,
		ExpectError: regexp.MustCompile(`(?s)Missing required argument.*ceip_acceptance`),
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// A value changed on the platform is shown as a diff and put back by an apply.
func TestAccTelemetryConfigResource_Drift(t *testing.T) {
	cfg := testAccTelemetryScheduleConfig(true, testAccTelemetrySchedule("WEEKLY_TELEMETRY_SCHEDULE", map[string]string{"day_of_week": `"SUNDAY"`, "hour_of_day": "2", "minutes": "0"}))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttr(testAccTelemetryName, "ceip_acceptance", "true")},
			{
				PreConfig: func() {
					current := testAccAPIGet(t, "/ssp/telemetry/config")
					testAccAPIPutWithRevision(t, "/ssp/telemetry/config", map[string]any{
						"ceip_acceptance":               false,
						"telemetry_agreement_displayed": true,
						"telemetry_schedule":            current["telemetry_schedule"],
					})
				},
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccTelemetryName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(testAccTelemetryName, "ceip_acceptance", "true"),
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}
