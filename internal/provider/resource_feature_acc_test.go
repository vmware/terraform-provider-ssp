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
					resource.TestCheckResourceAttr(resourceName, "feature", feature),
					resource.TestCheckResourceAttrSet(resourceName, "overall_status"),
					resource.TestCheckResourceAttrSet(resourceName, "overall_progress"),
				),
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
