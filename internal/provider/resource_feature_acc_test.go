// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccFeatureResource exercises ssp_feature's Create (RUN_PRECHECK then
// DEPLOY) and Read against a real SSP cluster, plus a real CheckDestroy
// verifying the feature actually reaches NOT_DEPLOYED after Terraform's
// automatic end-of-test Destroy (UNDEPLOY). No Update step: `feature` is the
// only user-settable attribute and carries RequiresReplace, so there is no
// in-place update to exercise (see resource_feature.go's Update(), which is
// a documented no-op).
//
// Deploying/undeploying a real vDefend feature is a meaningful, non-trivial
// operation against a live cluster, so — beyond the standard
// SSP_HOST/SSP_USERNAME/SSP_PASSWORD — this test additionally requires
// SSP_ACC_TEST_FEATURE to name the feature to exercise; it defaults to
// INTELLIGENCE (see testAccFeatureSetup).
func TestAccFeatureResource(t *testing.T) {
	resourceName := "ssp_feature.test"
	feature := testAccFeatureSetup(t)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFeatureDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccFeatureConfig(feature),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", feature),
					resource.TestCheckResourceAttr(resourceName, "feature", feature),
					resource.TestCheckResourceAttr(resourceName, "force_undeploy", "false"),
					resource.TestCheckResourceAttr(resourceName, "overall_status", "DEPLOYMENT_SUCCESSFUL"),
					resource.TestCheckResourceAttr(resourceName, "deployment_status", "DEPLOYMENT_SUCCESSFUL"),
					resource.TestCheckResourceAttr(resourceName, "overall_progress", "100"),
					resource.TestCheckResourceAttrSet(resourceName, "precheck_status"),
					// The state must be what the platform reports.
					testAccCheckAttrEqualsAPI(t, resourceName, "overall_status", func() string {
						return fmt.Sprint(testAccAPIGet(t, "/ssp/lcm/features/"+feature+"/status")["overall_status"])
					}),
				),
			},
			// A deployed feature plans no change.
			{Config: testAccFeatureConfig(feature), PlanOnly: true},
			// force_undeploy only changes how Delete behaves, so it is updated in
			// place and never redeploys the feature.
			{
				Config: testAccFeatureConfigForce(feature, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "force_undeploy", "true"),
					resource.TestCheckResourceAttr(resourceName, "overall_status", "DEPLOYMENT_SUCCESSFUL"),
				),
			},
			{Config: testAccFeatureConfig(feature), Check: resource.TestCheckResourceAttr(resourceName, "force_undeploy", "false")},
			// Import by feature name. force_undeploy has no server-side value.
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateId:           feature,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"force_undeploy"},
			},
			// Delete Testing (UNDEPLOY) is handled automatically by
			// terraform-plugin-testing at the end of Test; CheckDestroy above
			// verifies it actually reached NOT_DEPLOYED server-side.
		},
	})
}

// testAccDefaultFeature is the feature the ssp_feature tests deploy when
// SSP_ACC_TEST_FEATURE is not set.
const testAccDefaultFeature = "INTELLIGENCE"

// testAccFeatureSetup makes the ssp_feature acceptance tests always run: TF_ACC
// defaults to 1 when it is unset (a value the user sets is kept), so the tests
// are never silently skipped. It returns the feature to deploy:
// SSP_ACC_TEST_FEATURE, or INTELLIGENCE when it is unset.
func testAccFeatureSetup(t *testing.T) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Setenv("TF_ACC", "1")
	}
	if v := os.Getenv("SSP_ACC_TEST_FEATURE"); v != "" {
		return v
	}
	return testAccDefaultFeature
}

func testAccFeatureConfig(feature string) string {
	return fmt.Sprintf(`
resource "ssp_feature" "test" {
  feature = %q
}
`, feature)
}

// testAccCheckFeatureDestroy confirms every ssp_feature in the test's final
// state actually reached NOT_DEPLOYED on the SSP cluster, rather than the
// resource's Delete succeeding on the API call but the undeploy silently
// stalling server-side.
func testAccCheckFeatureDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ssp_feature" {
			continue
		}

		feature := rs.Primary.Attributes["feature"]
		status, body, err := testAccDoAPI(http.MethodGet, "/ssp/lcm/features/"+feature+"/status", nil)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			// Never deployed / already fully torn down — acceptable.
			continue
		}

		var out struct {
			OverallStatus string `json:"overall_status"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return fmt.Errorf("decode status of feature %s (HTTP %d): %w: %s", feature, status, err, body)
		}
		if out.OverallStatus != "NOT_DEPLOYED" {
			return fmt.Errorf("feature %s did not reach NOT_DEPLOYED after destroy, overall_status: %s", feature, out.OverallStatus)
		}
	}

	return nil
}

func testAccFeatureConfigForce(feature string, force bool) string {
	return fmt.Sprintf(`
resource "ssp_feature" "test" {
  feature        = %q
  force_undeploy = %t
}
`, feature, force)
}

// A feature name that is not one of the supported ones is rejected at plan time,
// before anything is deployed. Names are case-sensitive.
func TestAccFeatureResource_InvalidFeature(t *testing.T) {
	testAccFeatureSetup(t)

	var steps []resource.TestStep
	for _, name := range []string{"FOO", "ndr", ""} {
		steps = append(steps, resource.TestStep{
			Config:      testAccFeatureConfig(name),
			ExpectError: regexp.MustCompile(`(?s)feature.*must be one of`),
		})
	}
	steps = append(steps, resource.TestStep{
		Config:      `resource "ssp_feature" "test" {}`,
		ExpectError: regexp.MustCompile(`(?s)Missing required argument.*feature`),
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}
