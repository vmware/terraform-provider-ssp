// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"regexp"
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
// SSP_ACC_TEST_FEATURE to name the specific feature to exercise (no default,
// so a plain `TF_ACC=1` run never accidentally deploys something on someone
// else's cluster); it will Skip without one.
func TestAccFeatureResource(t *testing.T) {
	resourceName := "ssp_feature.test"
	feature := os.Getenv("SSP_ACC_TEST_FEATURE")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			if feature == "" {
				t.Skip("SSP_ACC_TEST_FEATURE must be set (to a real vDefend feature name) for ssp_feature acceptance tests")
			}
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
	host := os.Getenv("SSP_HOST")
	username := os.Getenv("SSP_USERNAME")
	password := os.Getenv("SSP_PASSWORD")
	httpClient := testAccInsecureHTTPClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ssp_feature" {
			continue
		}

		feature := rs.Primary.Attributes["feature"]
		req, err := http.NewRequest(http.MethodGet, testAccTrimHost(host)+"/ssp/lcm/features/"+feature+"/status", nil) //nolint:gosec // host is the operator-configured SSP_HOST acceptance-test target, not user input
		if err != nil {
			return err
		}
		req.SetBasicAuth(username, password)

		resp, err := httpClient.Do(req) //nolint:gosec // same operator-configured target as above
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			// Never deployed / already fully torn down — acceptable.
			continue
		}

		var status struct {
			OverallStatus string `json:"overall_status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			return err
		}
		if status.OverallStatus != "NOT_DEPLOYED" {
			return fmt.Errorf("feature %s did not reach NOT_DEPLOYED after destroy, overall_status: %s", feature, status.OverallStatus)
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

// A feature whose pre-checks fail is not deployed, and each failed check is
// listed. SSP_ACC_FEATURE_PRECHECK_FAILS names a feature that is known to fail its
// pre-checks on the cluster. It is opt-in because a feature whose pre-checks pass
// would be deployed.
func TestAccFeatureResource_PrechecksFail(t *testing.T) {
	feature := os.Getenv("SSP_ACC_FEATURE_PRECHECK_FAILS")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			if feature == "" {
				t.Skip("SSP_ACC_FEATURE_PRECHECK_FAILS must name a feature whose pre-checks fail on this cluster")
			}
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFeatureDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccFeatureConfig(feature),
				ExpectError: regexp.MustCompile(`(?s)Pre-checks failed for feature ` + feature + `.*pre-checks failed`),
			},
		},
	})
}
