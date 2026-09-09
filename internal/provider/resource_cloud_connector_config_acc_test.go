// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCloudConnectorConfigResource(t *testing.T) {
	resourceName := "ssp_cloud_connector_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read Testing
			{
				Config: testAccCloudConnectorConfigConfig("west.us"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "region", "west.us"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "fqdn"),
					resource.TestCheckResourceAttrSet(resourceName, "region_name"),
				),
			},
			// ImportState Testing
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			// Destroy Testing is intentionally not performed: cloud_connector_config
			// is a persistent platform setting with no DELETE API, so Delete() is a
			// documented no-op. Region also cannot be changed once Cloud Connector
			// is deployed ("configurable" becomes false), so no Update step is
			// included here.
		},
	})
}

func testAccCloudConnectorConfigConfig(region string) string {
	return fmt.Sprintf(`
resource "ssp_cloud_connector_config" "test" {
  region = %q
}
`, region)
}
