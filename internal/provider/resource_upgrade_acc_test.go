// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccUpgradeResource exercises ssp_upgrade's Create (deploy
// UPGRADE_COORDINATOR, then RETRY/PRECHECKS_ONLY+CONTINUE/START to a
// terminal state) and Read against a real SSP cluster.
//
// This is the one acceptance test in this package that is genuinely
// destructive and irreversible: triggering a real cluster upgrade cannot be
// undone by `terraform destroy` (Delete only undeploys UPGRADE_COORDINATOR;
// it does not roll back the platform version — see resource_upgrade.go).
// Beyond the standard SSP_HOST/SSP_USERNAME/SSP_PASSWORD, it requires an
// explicit SSP_ACC_ALLOW_UPGRADE=true opt-in so a plain `TF_ACC=1` test run
// can never accidentally upgrade a real cluster. It also requires an
// upgrade package to already be staged in the depot (out of scope for this
// test to set up) — without one, `action=START`/`CONTINUE` will fail
// server-side and the test will report that failure rather than silently
// skipping, since SSP_ACC_ALLOW_UPGRADE=true is an explicit statement of
// intent to run this for real.
func TestAccUpgradeResource(t *testing.T) {
	resourceName := "ssp_upgrade.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			if os.Getenv("SSP_ACC_ALLOW_UPGRADE") != "true" {
				t.Skip("SSP_ACC_ALLOW_UPGRADE=true must be set to opt in to this destructive, irreversible " +
					"acceptance test (it triggers a real cluster upgrade); requires a package already staged in the depot")
			}
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUpgradeConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", "upgrade"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
					resource.TestCheckResourceAttrSet(resourceName, "current_version"),
					resource.TestCheckResourceAttrSet(resourceName, "target_version"),
				),
			},
			// Delete Testing (undeploy UPGRADE_COORDINATOR only — does not
			// revert the platform version) is handled automatically by
			// terraform-plugin-testing at the end of Test. No CheckDestroy:
			// there is nothing meaningful to verify was "undone", since the
			// upgrade itself is permanent by design.
		},
	})
}

func testAccUpgradeConfig() string {
	return `
resource "ssp_upgrade" "test" {
  run_prechecks = true
}
`
}
