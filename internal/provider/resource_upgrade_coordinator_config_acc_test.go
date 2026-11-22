// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

const testAccUpgradeCoordinatorConfigName = "ssp_upgrade_coordinator_config.test"

const upgradeCoordinatorConfigPath = "/ssp/lcm/upgrade-coordinator/config"

func testAccUpgradeCoordinatorConfigConfig(version string) string {
	return fmt.Sprintf(`
resource "ssp_upgrade_coordinator_config" "test" {
  target_version = %q
}
`, version)
}

// testAccUpgradeTargets returns the versions the cluster can be upgraded to,
// newest first. It skips the test when there are none, because the config
// holds a target version that only makes sense for a version the cluster
// offers.
//
// It also refuses to run while an upgrade is in progress: these tests change
// the Upgrade Coordinator's target version, which a running upgrade reads. It
// also requires the coordinator to be undeployed, the only time the backend
// accepts a change.
func testAccUpgradeTargets(t *testing.T) []string {
	t.Helper()

	// These helpers call the API before resource.Test runs, so they have to
	// honour the same TF_ACC switch that resource.Test uses to skip.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless TF_ACC is set")
	}
	testAccSSPRuntimePreCheck(t)

	if status, body, err := testAccDoAPI(http.MethodGet, "/ssp/upgrade/status", nil); err == nil && status == http.StatusOK {
		var st struct {
			OverallStatus string `json:"overall_status"`
		}
		_ = json.Unmarshal(body, &st)
		if st.OverallStatus == "IN_PROGRESS" {
			t.Fatal("an SSP upgrade is in progress; this test changes the Upgrade Coordinator target version and must not run now")
		}
	}

	// The backend refuses to change the configuration while the coordinator is
	// deployed ("upgrade coordinator is already deployed"). ssp_upgrade deploys
	// it on create and undeploys it on destroy.
	if status, body, err := testAccDoAPI(http.MethodGet, "/ssp/lcm/features/UPGRADE_COORDINATOR/status", nil); err == nil && status == http.StatusOK {
		var fs struct {
			OverallStatus string `json:"overall_status"`
		}
		_ = json.Unmarshal(body, &fs)
		if strings.HasPrefix(fs.OverallStatus, "DEPLOYMENT_SUCCESS") || fs.OverallStatus == "DEPLOYMENT_PARTIAL_SUCCESS" {
			t.Fatalf("the UPGRADE_COORDINATOR feature is deployed (%s), so its target version cannot be changed; "+
				"destroy ssp_upgrade (or undeploy the feature) first", fs.OverallStatus)
		}
	}

	raw, _ := testAccAPIGet(t, "/ssp/upgrade/available-versions")["versions_for_upgrade"].([]any)
	var versions []string
	for _, v := range raw {
		if s, ok := v.(string); ok {
			versions = append(versions, s)
		}
	}
	if len(versions) == 0 {
		t.Skip("the cluster reports no versions available for upgrade, so there is no valid target_version to configure")
	}
	return versions
}

// testAccRestoreUpgradeCoordinatorConfig puts the target version back after the
// test. The config has no DELETE API, so Terraform destroy leaves it behind.
func testAccRestoreUpgradeCoordinatorConfig(t *testing.T) {
	t.Helper()

	original, _ := testAccAPIGet(t, upgradeCoordinatorConfigPath)["target_version"].(string)
	t.Cleanup(func() {
		current, _ := testAccAPIGet(t, upgradeCoordinatorConfigPath)["target_version"].(string)
		if current != original {
			testAccAPIPutWithRevision(t, upgradeCoordinatorConfigPath, map[string]any{"target_version": original})
		}
	})
}

// Create, update (when a second version is available), no-op re-plan and
// import. Destroy is not verified: the config is a singleton with no DELETE API,
// so Delete() only removes it from state.
func TestAccUpgradeCoordinatorConfigResource(t *testing.T) {
	versions := testAccUpgradeTargets(t)
	testAccRestoreUpgradeCoordinatorConfig(t)

	steps := []resource.TestStep{
		{
			Config: testAccUpgradeCoordinatorConfigConfig(versions[0]),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(testAccUpgradeCoordinatorConfigName, "id", "singleton"),
				resource.TestCheckResourceAttr(testAccUpgradeCoordinatorConfigName, "target_version", versions[0]),
				resource.TestCheckResourceAttrSet(testAccUpgradeCoordinatorConfigName, "revision"),
				resource.TestCheckResourceAttrSet(testAccUpgradeCoordinatorConfigName, "config_status"),
				testAccCheckAttrEqualsAPI(t, testAccUpgradeCoordinatorConfigName, "target_version", func() string {
					v, _ := testAccAPIGet(t, upgradeCoordinatorConfigPath)["target_version"].(string)
					return v
				}),
			),
		},
		{Config: testAccUpgradeCoordinatorConfigConfig(versions[0]), PlanOnly: true},
	}

	if len(versions) > 1 {
		steps = append(steps,
			resource.TestStep{
				Config: testAccUpgradeCoordinatorConfigConfig(versions[1]),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccUpgradeCoordinatorConfigName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(testAccUpgradeCoordinatorConfigName, "target_version", versions[1]),
			},
			resource.TestStep{Config: testAccUpgradeCoordinatorConfigConfig(versions[1]), PlanOnly: true},
		)
	}

	steps = append(steps, resource.TestStep{
		ResourceName:      testAccUpgradeCoordinatorConfigName,
		ImportState:       true,
		ImportStateId:     "singleton",
		ImportStateVerify: true,
		// The import round trip reads the same config and status; nothing is
		// write-only here, so every attribute must match.
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// A target version changed on the cluster is shown as a diff and put back by an apply.
func TestAccUpgradeCoordinatorConfigResource_Drift(t *testing.T) {
	versions := testAccUpgradeTargets(t)
	if len(versions) < 2 {
		t.Skip("drift needs two versions available for upgrade: one to configure and a different one to change it to behind Terraform's back")
	}
	testAccRestoreUpgradeCoordinatorConfig(t)

	cfg := testAccUpgradeCoordinatorConfigConfig(versions[0])

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttr(testAccUpgradeCoordinatorConfigName, "target_version", versions[0])},
			{
				PreConfig: func() {
					testAccAPIPutWithRevision(t, upgradeCoordinatorConfigPath, map[string]any{"target_version": versions[1]})
				},
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccUpgradeCoordinatorConfigName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(testAccUpgradeCoordinatorConfigName, "target_version", versions[0]),
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}
