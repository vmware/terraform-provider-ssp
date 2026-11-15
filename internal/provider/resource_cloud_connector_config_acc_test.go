// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"regexp"
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

const testAccCloudConnectorName = "ssp_cloud_connector_config.test"

// testAccCheckCloudConnectorRegion checks the region and the two values the
// platform resolves from it. fqdn and region_name must follow the region.
func testAccCheckCloudConnectorRegion(region, fqdn, regionName string) resource.TestCheckFunc {
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(testAccCloudConnectorName, "id", "singleton"),
		resource.TestCheckResourceAttr(testAccCloudConnectorName, "region", region),
		resource.TestCheckResourceAttr(testAccCloudConnectorName, "fqdn", fqdn),
		resource.TestCheckResourceAttr(testAccCloudConnectorName, "region_name", regionName),
		resource.TestCheckResourceAttr(testAccCloudConnectorName, "configurable", "true"),
		resource.TestCheckResourceAttrSet(testAccCloudConnectorName, "config_status"),
	)
}

// Changing the region used to fail the apply with "inconsistent result", because
// fqdn and region_name were carried over from state instead of being resolved
// from the new region. The platform is left on west.us.
func TestAccCloudConnectorConfigResource_RegionChange(t *testing.T) {
	west := testAccCloudConnectorConfigConfig("west.us")
	emea := testAccCloudConnectorConfigConfig("nl.emea")

	updateOnly := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccCloudConnectorName, plancheck.ResourceActionUpdate)},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: west, Check: testAccCheckCloudConnectorRegion("west.us", "nsx.west.us.lastline.com", "United States 1")},
			{Config: west, PlanOnly: true},
			{
				Config:           emea,
				ConfigPlanChecks: updateOnly,
				Check:            testAccCheckCloudConnectorRegion("nl.emea", "nsx.nl.emea.lastline.com", "European Union 1"),
			},
			{Config: emea, PlanOnly: true},
			{
				Config:           west,
				ConfigPlanChecks: updateOnly,
				Check:            testAccCheckCloudConnectorRegion("west.us", "nsx.west.us.lastline.com", "United States 1"),
			},
			{Config: west, PlanOnly: true},
			{
				ResourceName:      testAccCloudConnectorName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
		},
	})
}

// A region the platform does not offer is rejected with the list of the ones it
// does, and nothing is changed.
func TestAccCloudConnectorConfigResource_UnknownRegion(t *testing.T) {
	var steps []resource.TestStep
	for _, region := range []string{"mars.one", "WEST.US", ""} {
		steps = append(steps, resource.TestStep{
			Config:      testAccCloudConnectorConfigConfig(region),
			ExpectError: regexp.MustCompile(`(?s)not found in available regions; available:.*west\.us`),
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func TestAccCloudConnectorConfigResource_MissingRegion(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `resource "ssp_cloud_connector_config" "test" {}`,
				ExpectError: regexp.MustCompile(`(?s)Missing required argument.*region`),
			},
		},
	})
}

// A region changed on the platform is shown as a diff and put back by an apply.
func TestAccCloudConnectorConfigResource_Drift(t *testing.T) {
	west := testAccCloudConnectorConfigConfig("west.us")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: west, Check: testAccCheckCloudConnectorRegion("west.us", "nsx.west.us.lastline.com", "United States 1")},
			{
				PreConfig: func() {
					testAccAPIPutWithRevision(t, "/ssp/lcm/cloud-connector/config", map[string]any{
						"region":      "nl.emea",
						"fqdn":        "nsx.nl.emea.lastline.com",
						"region_name": "European Union 1",
					})
				},
				Config: west,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccCloudConnectorName, plancheck.ResourceActionUpdate)},
				},
				Check: testAccCheckCloudConnectorRegion("west.us", "nsx.west.us.lastline.com", "United States 1"),
			},
			{Config: west, PlanOnly: true},
		},
	})
}
