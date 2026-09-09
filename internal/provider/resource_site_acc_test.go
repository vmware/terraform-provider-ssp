// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccSiteResource exercises ssp_site's full onboard/reconnect/offboard
// lifecycle against a real SSP cluster and NSX Manager: Create (ONBOARD),
// Read, Update (desired_state change to PREPARE, in place per resource_site.go
// — no RequiresReplace on this attribute), ImportState, and a real
// CheckDestroy verifying the site is actually gone from the SSP cluster
// after Terraform's automatic end-of-test Destroy (OFFBOARD), rather than
// trusting that Delete() succeeded.
//
// Requires NSX_HOST/NSX_USERNAME/NSX_PASSWORD/NSX_CERTIFICATE in addition to
// the standard SSP_HOST/SSP_USERNAME/SSP_PASSWORD (see
// testAccNSXSitePreCheck) — this is not expected to run in most CI
// environments and will Skip without a real NSX Manager to onboard against.
func TestAccSiteResource(t *testing.T) {
	resourceName := "ssp_site.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNSXSitePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy,
		Steps: []resource.TestStep{
			// Create (ONBOARD) and Read.
			{
				Config: testAccSiteConfig("ONBOARD"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "site_type", "NSX_MANAGER"),
					resource.TestCheckResourceAttr(resourceName, "desired_state", "ONBOARD"),
					resource.TestCheckResourceAttrSet(resourceName, "current_state"),
				),
			},
			// Update: change desired_state in place.
			{
				Config: testAccSiteConfig("PREPARE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "desired_state", "PREPARE"),
				),
			},
			// ImportState Testing.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				// site_connection_info's password/certificate are write-only/sensitive
				// and not returned by the API.
				ImportStateVerifyIgnore: []string{"site_connection_info.password", "site_connection_info.certificate"},
			},
			// Delete Testing (OFFBOARD) is handled automatically by
			// terraform-plugin-testing at the end of Test; CheckDestroy above
			// verifies it actually happened server-side.
		},
	})
}

func testAccSiteConfig(desiredState string) string {
	return fmt.Sprintf(`
resource "ssp_site" "test" {
  site_type     = "NSX_MANAGER"
  site_name     = "tf-acc-test-nsx-site"
  desired_state = %q
  force         = true

  site_connection_info = {
    connection_type = "DYNAMIC"
    hostname        = %q
    username        = %q
    password        = %q
    certificate     = %q
  }
}
`, desiredState, os.Getenv("NSX_HOST"), os.Getenv("NSX_USERNAME"), os.Getenv("NSX_PASSWORD"), os.Getenv("NSX_CERTIFICATE"))
}

// testAccCheckSiteDestroy confirms that every ssp_site in the test's final
// state was actually offboarded from the SSP cluster (GET returns 404),
// rather than the resource's Delete silently no-oping while the site
// registration remains server-side.
func testAccCheckSiteDestroy(s *terraform.State) error {
	return testAccCheckSSPResourceDestroyed(s, "ssp_site", "/ssp/sites/")
}

// testAccCheckSSPResourceDestroyed is a shared CheckDestroy helper: it GETs
// each resource of the given type by ID from the SSP runtime API and fails
// if any still returns something other than 404.
func testAccCheckSSPResourceDestroyed(s *terraform.State, resourceType, pathPrefix string) error {
	host := os.Getenv("SSP_HOST")
	username := os.Getenv("SSP_USERNAME")
	password := os.Getenv("SSP_PASSWORD")

	httpClient := testAccInsecureHTTPClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != resourceType {
			continue
		}

		req, err := http.NewRequest(http.MethodGet, testAccTrimHost(host)+pathPrefix+rs.Primary.ID, nil) //nolint:gosec // host is the operator-configured SSP_HOST acceptance-test target, not user input
		if err != nil {
			return err
		}
		req.SetBasicAuth(username, password)

		resp, err := httpClient.Do(req) //nolint:gosec // same operator-configured target as above
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("%s %s still exists (status %d)", resourceType, rs.Primary.ID, resp.StatusCode)
		}
	}

	return nil
}

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
