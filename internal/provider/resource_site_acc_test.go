// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const testAccSiteName = "ssp_site.test"

// testAccRequireNSXNotOnboarded skips the test when the NSX Manager is already
// onboarded to the SSP: onboarding it a second time conflicts, and offboarding the
// only site would disrupt the cluster. Offboard it first to run these tests.
func testAccRequireNSXNotOnboarded(t *testing.T) {
	t.Helper()

	host := os.Getenv("NSX_HOST")
	sites, _ := testAccAPIGet(t, "/ssp/sites")["sites"].([]any)
	for _, raw := range sites {
		site, _ := raw.(map[string]any)
		info, _ := site["site_connection_info"].(map[string]any)
		ips, _ := site["cluster_manager_ips"].([]any)
		found := fmt.Sprint(info["hostname"]) == host
		for _, ip := range ips {
			found = found || fmt.Sprint(ip) == host
		}
		if found {
			t.Skipf("NSX Manager %s is already onboarded as site %v; offboard it to run this test", host, site["site_name"])
		}
	}
}

// testAccSiteConnection renders site_connection_info from the environment.
func testAccSiteConnection(certificate string) string {
	return fmt.Sprintf(`
  site_connection_info = {
    connection_type = "DYNAMIC"
    hostname        = %q
    username        = %q
    password        = %q
    certificate     = %q
  }
`, os.Getenv("NSX_HOST"), os.Getenv("NSX_USERNAME"), os.Getenv("NSX_PASSWORD"), certificate)
}

func testAccSiteConfigRaw(siteType, desiredState, connection string) string {
	return fmt.Sprintf(`
resource "ssp_site" "test" {
  site_type     = %q
  site_name     = %q
  desired_state = %q
  force         = true
%s
}
`, siteType, testAccSiteDisplayName(), desiredState, connection)
}

// testAccSiteDisplayName is the name the site is onboarded under. For an NSX
// Manager it must match the name configured on NSX, so it can be overridden.
func testAccSiteDisplayName() string {
	if v := os.Getenv("NSX_SITE_NAME"); v != "" {
		return v
	}
	return "tf-acc-test-nsx-site"
}

// TestAccSiteResource onboards a real NSX Manager and checks that Create waits
// until the site is READY, that the platform's own limits on updating a site are
// reported, that an import round-trips, and (through CheckDestroy) that destroy
// really offboards it. It needs the NSX_* variables (see testAccNSXSitePreCheck)
// and an NSX Manager that is not onboarded yet. Destroy also needs every feature
// to be removed first, or the platform refuses to offboard.
func TestAccSiteResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccNSXSitePreCheck(t)
			testAccRequireNSXNotOnboarded(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy,
		Steps: []resource.TestStep{
			// Create (ONBOARD) returns only when the site is READY. It used to return
			// as soon as the connection was HEALTHY, with the site still onboarding.
			{
				Config: testAccSiteConfig("ONBOARD"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(testAccSiteName, "id"),
					resource.TestCheckResourceAttr(testAccSiteName, "site_type", "NSX_MANAGER"),
					resource.TestCheckResourceAttr(testAccSiteName, "desired_state", "ONBOARD"),
					resource.TestCheckResourceAttr(testAccSiteName, "current_state", "READY"),
					resource.TestCheckResourceAttr(testAccSiteName, "connection_status", "HEALTHY"),
					resource.TestCheckResourceAttrSet(testAccSiteName, "nsx_version"),
					resource.TestCheckResourceAttrSet(testAccSiteName, "nsx_cluster_id"),
				),
			},
			// Nothing is left to change once the apply returned.
			{Config: testAccSiteConfig("ONBOARD"), PlanOnly: true},

			// The platform accepts only ONBOARD when updating an existing site, and
			// the error is shown. The site is left as it was.
			{
				Config:      testAccSiteConfig("PREPARE"),
				ExpectError: regexp.MustCompile(`(?s)Error updating site.*Invalid\s+desired\s+action:\s+PREPARE`),
			},
			{
				Config:      testAccSiteConfig("OFFBOARD"),
				ExpectError: regexp.MustCompile(`(?s)Error updating site.*Invalid\s+desired\s+action:\s+OFFBOARD`),
			},
			// Applying the same config again changes nothing.
			{
				Config: testAccSiteConfig("ONBOARD"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(testAccSiteName, "current_state", "READY"),
			},

			// Import. The credentials are never returned by the API and force is not
			// part of the site, so they are left out of the comparison.
			{
				ResourceName:            testAccSiteName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"site_connection_info", "force"},
			},
			// Delete (OFFBOARD) is handled by terraform-plugin-testing at the end of
			// the test; CheckDestroy verifies it really happened on the platform.
		},
	})
}

// Plan-time checks, which need no NSX Manager.
func TestAccSiteResource_InvalidArguments(t *testing.T) {
	conn := `
  site_connection_info = {
    connection_type = "DYNAMIC"
    hostname        = "nsx.example.invalid"
    username        = "admin"
    password        = "unused-password-1"
    certificate     = "unused"
  }
`
	hcl := func(siteType, desired, connection string) string {
		return fmt.Sprintf(`
resource "ssp_site" "test" {
  site_type     = %q
  site_name     = "tf-acc-invalid"
  desired_state = %q
%s
}
`, siteType, desired, connection)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: hcl("VCENTER", "ONBOARD", conn), ExpectError: regexp.MustCompile(`(?s)site_type.*must be one of`)},
			{Config: hcl("NSX_MANAGER", "onboard", conn), ExpectError: regexp.MustCompile(`(?s)desired_state.*must be one of`)},
			{Config: hcl("NSX_MANAGER", "REMOVE", conn), ExpectError: regexp.MustCompile(`(?s)desired_state.*must be one of`)},
			{Config: hcl("NSX_MANAGER", "ONBOARD", ""), ExpectError: regexp.MustCompile(`(?s)Missing required argument.*site_connection_info`)},
			{
				Config: hcl("NSX_MANAGER", "ONBOARD", `
  site_connection_info = {
    connection_type = "DYNAMIC"
    hostname        = "nsx.example.invalid"
    username        = "admin"
    password        = "unused-password-1"
  }
`),
				ExpectError: regexp.MustCompile(`(?s)certificate`),
			},
			{
				Config: hcl("NSX_MANAGER", "ONBOARD", `
  site_connection_info = {
    connection_type = "DYNAMIC"
    hostname        = "nsx.example.invalid"
    password        = "unused-password-1"
    certificate     = "unused"
  }
`),
				ExpectError: regexp.MustCompile(`(?s)username`),
			},
		},
	})
}

// A certificate the platform cannot parse is rejected, and no site is created. It
// is opt-in because it sends an onboarding request for the real NSX Manager.
func TestAccSiteResource_InvalidCertificate(t *testing.T) {
	testAccRequireOptIn(t, "SSP_ACC_ALLOW_SITE_ONBOARD", "it sends an onboarding request for the NSX Manager")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccNSXSitePreCheck(t)
			testAccRequireNSXNotOnboarded(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccSiteConfigRaw("NSX_MANAGER", "ONBOARD", testAccSiteConnection("not a certificate")),
				ExpectError: regexp.MustCompile(`(?s)Error creating site.*(200013|certificate)`),
			},
		},
	})
}

// With desired_state = PREPARE the platform only runs the prechecks and never
// onboards the site, so it never becomes READY. Create must finish on the
// prechecks instead of waiting for READY. It is opt-in because it registers the
// NSX Manager with the platform.
func TestAccSiteResource_Prepare(t *testing.T) {
	testAccRequireOptIn(t, "SSP_ACC_ALLOW_SITE_ONBOARD", "it runs the onboarding prechecks for the NSX Manager")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccNSXSitePreCheck(t)
			testAccRequireNSXNotOnboarded(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccSiteConfigRaw("NSX_MANAGER", "PREPARE", testAccSiteConnection(os.Getenv("NSX_CERTIFICATE"))),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(testAccSiteName, "id"),
					resource.TestCheckResourceAttr(testAccSiteName, "desired_state", "PREPARE"),
				),
			},
		},
	})
}

func testAccSiteConfig(desiredState string) string {
	return testAccSiteConfigRaw("NSX_MANAGER", desiredState, testAccSiteConnection(os.Getenv("NSX_CERTIFICATE")))
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
