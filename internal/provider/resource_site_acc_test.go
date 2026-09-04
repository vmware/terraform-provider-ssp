package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSiteDataSource(t *testing.T) {
	resourceName := "data.ssp_sites.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSitesDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "sites.0.id"),
				),
			},
		},
	})
}

func testAccSitesDataSourceConfig() string {
	return `
data "ssp_sites" "test" {}
`
}
