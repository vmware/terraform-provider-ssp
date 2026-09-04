package provider_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"golang.org/x/net/proxy"
)

func TestAccSensorRegistrationTokenResource(t *testing.T) {
	resourceName := "ssp_sensor_registration_token.test"
	displayName := acctest.RandomWithPrefix("tf-acc-test-token")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSensorRegistrationTokenDestroy,
		Steps: []resource.TestStep{
			// Create and Read Testing
			{
				Config: testAccSensorRegistrationTokenConfig(displayName, "SecretPassphrase123!"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", displayName),
					resource.TestCheckResourceAttr(resourceName, "allowed_instance_count", "1"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
				),
			},
			// ImportState Testing
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				// Ignore passphrase since it is write-only/sensitive and not returned by API
				ImportStateVerifyIgnore: []string{"passphrase"},
			},
			// Delete Testing is handled automatically by terraform-plugin-testing at the end of Test!
		},
	})
}

func testAccSensorRegistrationTokenConfig(displayName, passphrase string) string {
	return fmt.Sprintf(`
resource "ssp_sensor_registration_token" "test" {
  display_name           = %q
  passphrase             = %q
  allowed_instance_count = 1
}
`, displayName, passphrase)
}

// testAccCheckSensorRegistrationTokenDestroy confirms that every
// ssp_sensor_registration_token in the test's final state was actually
// deleted from the SSP cluster (GET returns 404), rather than the resource's
// Delete silently no-oping while the token remains usable server-side.
func testAccCheckSensorRegistrationTokenDestroy(s *terraform.State) error {
	host := os.Getenv("SSP_HOST")
	username := os.Getenv("SSP_USERNAME")
	password := os.Getenv("SSP_PASSWORD")

	transport := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	if cd, ok := proxy.FromEnvironment().(proxy.ContextDialer); ok {
		transport.DialContext = cd.DialContext
	}
	httpClient := &http.Client{
		Transport: transport,
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ssp_sensor_registration_token" {
			continue
		}

		url := strings.TrimRight(host, "/") + "/sensors/registration-tokens/" + rs.Primary.ID
		req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:gosec // host is the operator-configured SSP_HOST acceptance-test target, not user input
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
			return fmt.Errorf("sensor registration token %s still exists (status %d)", rs.Primary.ID, resp.StatusCode)
		}
	}

	return nil
}
