// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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

const testAccSensorRegResourceAddr = "ssp_sensor_registration_token.test"

const (
	testAccTokenPassphrase  = "SecretPassphrase123!"
	testAccTokenPassphrase2 = "TfTokenPass#2026-Abc"
)

// testAccTokenConfig builds a token with the given display name, passphrase and
// any extra attribute lines.
func testAccTokenConfig(displayName, passphrase, extra string) string {
	return fmt.Sprintf(`
resource "ssp_sensor_registration_token" "test" {
  display_name = %q
  passphrase   = %q
%s
}
`, displayName, passphrase, extra)
}

func testAccTokenReplace() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccSensorRegResourceAddr, plancheck.ResourceActionDestroyBeforeCreate)},
	}
}

// testAccTokenStatus returns the HTTP status of GET on a token.
func testAccTokenStatus(t *testing.T, id string) int {
	t.Helper()
	status, _ := testAccAPIRequest(t, http.MethodGet, "/sensors/registration-tokens/"+id, nil)
	return status
}

// TestAccSensorRegistrationTokenResource_Replacement changes each argument in
// turn. Every one forces a replacement, since a token cannot be updated, and the
// token it replaces must be deleted on the platform.
func TestAccSensorRegistrationTokenResource_Replacement(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-token")
	var ids []string
	capture := resource.TestCheckResourceAttrWith(testAccSensorRegResourceAddr, "id", func(v string) error {
		ids = append(ids, v)
		return nil
	})
	oldDeleted := func() error {
		if len(ids) < 2 {
			return fmt.Errorf("expected two ids, have %d", len(ids))
		}
		if status := testAccTokenStatus(t, ids[len(ids)-2]); status != http.StatusNotFound {
			return fmt.Errorf("replaced token %s still exists (status %d)", ids[len(ids)-2], status)
		}
		return nil
	}
	check := func() resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(capture, func(*terraform.State) error {
			if len(ids) < 2 {
				return nil
			}
			if ids[len(ids)-1] == ids[len(ids)-2] {
				return fmt.Errorf("the token was not replaced, id %s is unchanged", ids[len(ids)-1])
			}
			return oldDeleted()
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSensorRegistrationTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTokenConfig(name, testAccTokenPassphrase, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					capture,
					resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "display_name", name),
					resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "allowed_instance_count", "1"),
					resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "status", "AVAILABLE"),
					resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "usage_count", "0"),
					resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "number_available", "1"),
					resource.TestCheckNoResourceAttr(testAccSensorRegResourceAddr, "description"),
					resource.TestCheckResourceAttrSet(testAccSensorRegResourceAddr, "expiration"),
					resource.TestCheckResourceAttrSet(testAccSensorRegResourceAddr, "registration_manifest"),
				),
			},
			{Config: testAccTokenConfig(name, testAccTokenPassphrase, ""), PlanOnly: true},
			{
				Config:           testAccTokenConfig(name, testAccTokenPassphrase, `description = "first description"`),
				ConfigPlanChecks: testAccTokenReplace(),
				Check:            resource.ComposeAggregateTestCheckFunc(check(), resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "description", "first description")),
			},
			{
				Config:           testAccTokenConfig(name+"-renamed", testAccTokenPassphrase, `description = "first description"`),
				ConfigPlanChecks: testAccTokenReplace(),
				Check:            check(),
			},
			{
				Config:           testAccTokenConfig(name+"-renamed", testAccTokenPassphrase, "description = \"first description\"\nallowed_instance_count = 3"),
				ConfigPlanChecks: testAccTokenReplace(),
				Check:            resource.ComposeAggregateTestCheckFunc(check(), resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "number_available", "3")),
			},
			{
				Config:           testAccTokenConfig(name+"-renamed", testAccTokenPassphrase2, "description = \"first description\"\nallowed_instance_count = 3"),
				ConfigPlanChecks: testAccTokenReplace(),
				Check:            check(),
			},
			{Config: testAccTokenConfig(name+"-renamed", testAccTokenPassphrase2, "description = \"first description\"\nallowed_instance_count = 3"), PlanOnly: true},
		},
	})
}

// The API limits are enforced by the platform and its error is shown.
func TestAccSensorRegistrationTokenResource_Invalid(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-token")

	cases := []struct {
		label, name, passphrase, extra, wantErr string
	}{
		// Plan time: the provider's own minimum length.
		{"passphrase of 14 characters", name, "Short-Pass-14c", "", `(?s)passphrase.*at least 15`},
		// Platform: passphrase policy.
		{"weak passphrase", name, testAccStringOfLen(20, "a"), "", `201012`},
		// Platform: allowed_instance_count range 1 to 10. A configured 0 used to be
		// dropped from the request, so the platform defaulted it to 1 and the apply
		// failed with "inconsistent result" and left a tainted token.
		{"count 0", name, testAccTokenPassphrase, "allowed_instance_count = 0", `201016`},
		{"count -1", name, testAccTokenPassphrase, "allowed_instance_count = -1", `201016`},
		{"count 11", name, testAccTokenPassphrase, "allowed_instance_count = 11", `201016`},
		{"count 100000", name, testAccTokenPassphrase, "allowed_instance_count = 100000", `201016`},
		// Platform: display_name is 3 to 64 characters of letters, digits, _ and -.
		{"name of 2 characters", "ab", testAccTokenPassphrase, "", `201013`},
		{"name of 65 characters", testAccStringOfLen(65, "n"), testAccTokenPassphrase, "", `201013`},
		{"empty name", "", testAccTokenPassphrase, "", `201013`},
		{"name with spaces", "has space in name", testAccTokenPassphrase, "", `201025`},
		{"name with unicode", "tökën-name", testAccTokenPassphrase, "", `201025`},
		// Platform: description is at most 1024 characters.
		{"description of 1025 characters", name, testAccTokenPassphrase, fmt.Sprintf("description = %q", testAccStringOfLen(1025, "d")), `(?s)API error 400.*1024`},
	}

	var steps []resource.TestStep
	for _, c := range cases {
		steps = append(steps, resource.TestStep{
			Config:      testAccTokenConfig(c.name, c.passphrase, c.extra),
			ExpectError: regexp.MustCompile(c.wantErr),
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSensorRegistrationTokenDestroy,
		Steps:                    steps,
	})
}

// The edges of each limit are accepted. An empty description used to be stored
// as null while the platform returns "", which failed the apply with
// "inconsistent result" and left a tainted token.
func TestAccSensorRegistrationTokenResource_Boundaries(t *testing.T) {
	long := acctest.RandomWithPrefix("tf-acc-long")
	long += testAccStringOfLen(64-len(long), "x")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSensorRegistrationTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTokenConfig("t-1", testAccTokenPassphrase, ""),
				Check:  resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "display_name", "t-1"),
			},
			{
				Config: testAccTokenConfig(long, testAccTokenPassphrase, ""),
				Check:  resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "display_name", long),
			},
			{
				Config: testAccTokenConfig(long, testAccTokenPassphrase, "allowed_instance_count = 10"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "allowed_instance_count", "10"),
					resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "number_available", "10"),
				),
			},
			{
				Config: testAccTokenConfig(long, testAccTokenPassphrase, fmt.Sprintf("description = %q", testAccStringOfLen(1024, "d"))),
				Check:  resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "description", testAccStringOfLen(1024, "d")),
			},
			{
				Config: testAccTokenConfig(long, testAccTokenPassphrase, fmt.Sprintf("description = %q", testAccStringOfLen(1024, "d"))),

				PlanOnly: true,
			},
			{
				Config: testAccTokenConfig(long, testAccTokenPassphrase, `description = ""`),
				Check:  resource.TestCheckResourceAttr(testAccSensorRegResourceAddr, "description", ""),
			},
			{Config: testAccTokenConfig(long, testAccTokenPassphrase, `description = ""`), PlanOnly: true},
		},
	})
}

// A token deleted outside Terraform is noticed and created again.
func TestAccSensorRegistrationTokenResource_Drift(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-token")
	cfg := testAccTokenConfig(name, testAccTokenPassphrase, "")
	var firstID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSensorRegistrationTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.TestCheckResourceAttrWith(testAccSensorRegResourceAddr, "id", func(v string) error {
					firstID = v
					return nil
				}),
			},
			{
				PreConfig: func() {
					status, body := testAccAPIRequest(t, http.MethodDelete, "/sensors/registration-tokens/"+firstID, nil)
					if status < 200 || status > 299 {
						t.Fatalf("delete token %s: status %d: %s", firstID, status, body)
					}
				},
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccSensorRegResourceAddr, plancheck.ResourceActionCreate)},
				},
				Check: resource.TestCheckResourceAttrWith(testAccSensorRegResourceAddr, "id", func(v string) error {
					if v == firstID {
						return fmt.Errorf("the token was not created again, id %s is unchanged", v)
					}
					return nil
				}),
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}

// Importing an id that does not exist is reported.
func TestAccSensorRegistrationTokenResource_ImportUnknown(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-token")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSensorRegistrationTokenDestroy,
		Steps: []resource.TestStep{
			{Config: testAccTokenConfig(name, testAccTokenPassphrase, "")},
			{
				ResourceName:  testAccSensorRegResourceAddr,
				ImportState:   true,
				ImportStateId: "00000000-0000-0000-0000-000000000000",
				ExpectError:   regexp.MustCompile(`Token not found`),
			},
		},
	})
}
