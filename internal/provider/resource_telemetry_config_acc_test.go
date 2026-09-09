// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
