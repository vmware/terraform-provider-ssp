package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
