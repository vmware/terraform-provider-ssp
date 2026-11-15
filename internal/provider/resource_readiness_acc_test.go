// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func testAccReadinessConfig(timeout string) string {
	attr := ""
	if timeout != "" {
		attr = "timeout_minutes = " + timeout
	}
	return fmt.Sprintf(`
resource "ssp_readiness" "test" {
  %s
}
`, attr)
}

// TestAccReadinessResource covers create, an empty replan, an in-place update
// of timeout_minutes, and the default that applies when it is removed.
func TestAccReadinessResource(t *testing.T) {
	resourceName := "ssp_readiness.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccReadinessConfig("5"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", "done"),
					resource.TestCheckResourceAttr(resourceName, "timeout_minutes", "5"),
				),
			},
			// No-op on subsequent plans.
			{
				Config:   testAccReadinessConfig("5"),
				PlanOnly: true,
			},
			// timeout_minutes is updated in place, not replaced.
			{
				Config: testAccReadinessConfig("7"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr(resourceName, "timeout_minutes", "7"),
			},
			// Removing the attribute falls back to the documented default of 20.
			{
				Config: testAccReadinessConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", "done"),
					resource.TestCheckResourceAttr(resourceName, "timeout_minutes", "20"),
				),
			},
			{
				Config:   testAccReadinessConfig(""),
				PlanOnly: true,
			},
		},
	})
}

// timeout_minutes must be at least 1.
func TestAccReadinessResource_InvalidTimeout(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccReadinessConfig("0"),
				ExpectError: regexp.MustCompile(`(?s)timeout_minutes.*must be at least 1`),
			},
			{
				Config:      testAccReadinessConfig("-1"),
				ExpectError: regexp.MustCompile(`(?s)timeout_minutes.*must be at least 1`),
			},
		},
	})
}

// A 401 from the platform is not retried: it fails at once with the reason.
// This sends a single request, so it cannot lock the account the way a plan
// with many wrong-password data sources can.
func TestAccReadinessResource_BadCredentials(t *testing.T) {
	testAccRequireOptIn(t, "SSP_ACC_ALLOW_BAD_CREDENTIALS", "a wrong password can lock the admin account on a shared cluster")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "ssp" {
  host     = %q
  username = %q
  password = "WrongPassw0rd!x"
  insecure = true
}

resource "ssp_readiness" "test" {
  timeout_minutes = 1
}
`, os.Getenv("SSP_HOST"), os.Getenv("SSP_USERNAME")),
				ExpectError: regexp.MustCompile(`(?s)rejected the provider credentials.*401`),
			},
		},
	})
}

// An unreachable host keeps being polled until timeout_minutes, then fails with
// the last error. A 1 minute timeout keeps this test short.
func TestAccReadinessResource_UnreachableHost(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "ssp" {
  host     = "https://192.0.2.1"
  username = "admin"
  password = "unused-password-1"
  insecure = true
}

resource "ssp_readiness" "test" {
  timeout_minutes = 1
}
`,
				ExpectError: regexp.MustCompile(`(?s)timed out`),
			},
		},
	})
}
