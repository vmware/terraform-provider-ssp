package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVsphereProviderDataSource(t *testing.T) {
	resourceName := "data.ssp_vsphere_provider.test"

	// SSP_TEST_VSPHERE_PROVIDER_{ID,SERVER,USER} let this test target a
	// vCenter provider registration that already exists in whichever SSPI
	// lab environment CI points at, instead of being tied to one specific
	// lab's snapshot (a fixed vCenter hostname/UUID/user).
	id := os.Getenv("SSP_TEST_VSPHERE_PROVIDER_ID")
	if id == "" {
		id = "bbed6f48-368f-4ad6-a857-0bb494ba206a"
	}
	server := os.Getenv("SSP_TEST_VSPHERE_PROVIDER_SERVER")
	if server == "" {
		server = "vxlan-vm-111-185.nimbus-tb.nimbus.internal"
	}
	user := os.Getenv("SSP_TEST_VSPHERE_PROVIDER_USER")
	if user == "" {
		user = "ssp-op-9e60d44a@vsphere.local"
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVsphereProviderDataSourceConfig(id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", id),
					resource.TestCheckResourceAttr(resourceName, "server", server),
					resource.TestCheckResourceAttr(resourceName, "user", user),
				),
			},
		},
	})
}

func testAccVsphereProviderDataSourceConfig(id string) string {
	return fmt.Sprintf(`
data "ssp_vsphere_provider" "test" {
  id = %q
}
`, id)
}
