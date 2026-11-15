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

func TestAccNdrConfigResource(t *testing.T) {
	resourceName := "ssp_ndr_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read Testing
			{
				Config: testAccNdrConfigConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "data_sharing", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "revision"),
				),
			},
			// Update Testing
			{
				Config: testAccNdrConfigConfig(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "data_sharing", "false"),
				),
			},
			// ImportState Testing
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			// Leave the platform on the default instead of the last update value.
			{
				Config: testAccNdrConfigConfig(true),
				Check:  resource.TestCheckResourceAttr(resourceName, "data_sharing", "true"),
			},
			// Destroy Testing is intentionally not performed: ndr_config is a
			// persistent platform setting with no DELETE API, so Delete() is a
			// documented no-op.
		},
	})
}

func testAccNdrConfigConfig(dataSharing bool) string {
	return fmt.Sprintf(`
resource "ssp_ndr_config" "test" {
  data_sharing = %t
}
`, dataSharing)
}

const testAccNdrName = "ssp_ndr_config.test"

const testAccNdrOmittedConfig = `
resource "ssp_ndr_config" "test" {}
`

// The platform is left on data_sharing = true (the default).
func TestAccNdrConfigResource_Lifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// data_sharing omitted: the default of true applies.
			{
				Config: testAccNdrOmittedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccNdrName, "id", "singleton"),
					resource.TestCheckResourceAttr(testAccNdrName, "data_sharing", "true"),
					resource.TestCheckResourceAttrSet(testAccNdrName, "config_status"),
				),
			},
			{Config: testAccNdrOmittedConfig, PlanOnly: true},

			// true -> false -> true are in-place updates.
			{
				Config: testAccNdrConfigConfig(false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccNdrName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(testAccNdrName, "data_sharing", "false"),
			},
			{Config: testAccNdrConfigConfig(false), PlanOnly: true},
			{
				Config: testAccNdrConfigConfig(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccNdrName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(testAccNdrName, "data_sharing", "true"),
			},
			{Config: testAccNdrConfigConfig(true), PlanOnly: true},

			// Removing the attribute after setting false re-applies the default true,
			// unlike telemetry and malware prevention, which keep the old value.
			{Config: testAccNdrConfigConfig(false), Check: resource.TestCheckResourceAttr(testAccNdrName, "data_sharing", "false")},
			{Config: testAccNdrOmittedConfig, Check: resource.TestCheckResourceAttr(testAccNdrName, "data_sharing", "true")},

			// An explicit null is the same as omitted.
			{
				Config: `
resource "ssp_ndr_config" "test" {
  data_sharing = null
}
`,
				PlanOnly: true,
			},

			{
				ResourceName:      testAccNdrName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNdrConfigResource_InvalidType(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "ssp_ndr_config" "test" {
  data_sharing = "maybe"
}
`,
				ExpectError: regexp.MustCompile(`(?s)Incorrect attribute value type|bool`),
			},
		},
	})
}

// A value changed on the platform is shown as a diff and put back by an apply.
func TestAccNdrConfigResource_Drift(t *testing.T) {
	cfg := testAccNdrConfigConfig(true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttr(testAccNdrName, "data_sharing", "true")},
			{
				PreConfig: func() {
					testAccAPIPutWithRevision(t, "/ssp/lcm/ndr/config", map[string]any{"data_sharing": false})
				},
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccNdrName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(testAccNdrName, "data_sharing", "true"),
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}
