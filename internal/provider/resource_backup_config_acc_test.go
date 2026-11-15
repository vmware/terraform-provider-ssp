// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

const testAccBackupConfigName = "ssp_backup_config.test"

// testAccBackupFields are the arguments of ssp_backup_config as HCL literals, so
// a test can break one of them. Port is an HCL number literal.
type testAccBackupFields struct {
	Server, Protocol, Username, Password, Location, Passphrase, Key, Port string
}

// testAccBackupSettings reads the SFTP backup server from the environment, and
// skips the test without one. The password, passphrase and key are never kept in
// the repository.
func testAccBackupSettings(t *testing.T) testAccBackupFields {
	t.Helper()

	e := testAccRequireEnv(t,
		"SSP_ACC_BACKUP_SERVER", "SSP_ACC_BACKUP_USERNAME", "SSP_ACC_BACKUP_PASSWORD",
		"SSP_ACC_BACKUP_PASSPHRASE", "SSP_ACC_BACKUP_LOCATION", "SSP_ACC_BACKUP_SSH_PUBLIC_KEY")
	port := os.Getenv("SSP_ACC_BACKUP_PORT")
	if port == "" {
		port = "22"
	}
	return testAccBackupFields{
		Server:     e["SSP_ACC_BACKUP_SERVER"],
		Protocol:   "SFTP",
		Username:   e["SSP_ACC_BACKUP_USERNAME"],
		Password:   e["SSP_ACC_BACKUP_PASSWORD"],
		Location:   e["SSP_ACC_BACKUP_LOCATION"],
		Passphrase: e["SSP_ACC_BACKUP_PASSPHRASE"],
		Key:        e["SSP_ACC_BACKUP_SSH_PUBLIC_KEY"],
		Port:       port,
	}
}

func (f testAccBackupFields) hcl() string {
	return fmt.Sprintf(`
resource "ssp_backup_config" "test" {
  server_address  = %q
  protocol        = %q
  port            = %s
  username        = %q
  password        = %q
  backup_location = %q
  passphrase      = %q
  ssh_public_key  = %q
}
`, f.Server, f.Protocol, f.Port, f.Username, f.Password, f.Location, f.Passphrase, f.Key)
}

func testAccCheckBackupConfig(f testAccBackupFields) resource.TestCheckFunc {
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(testAccBackupConfigName, "id", "singleton"),
		resource.TestCheckResourceAttr(testAccBackupConfigName, "server_address", f.Server),
		resource.TestCheckResourceAttr(testAccBackupConfigName, "protocol", f.Protocol),
		resource.TestCheckResourceAttr(testAccBackupConfigName, "port", f.Port),
		resource.TestCheckResourceAttr(testAccBackupConfigName, "username", f.Username),
		resource.TestCheckResourceAttr(testAccBackupConfigName, "backup_location", f.Location),
	)
}

// testAccBackupAltLocation is a second directory that exists on the server.
func testAccBackupAltLocation() string {
	if v := os.Getenv("SSP_ACC_BACKUP_ALT_LOCATION"); v != "" {
		return v
	}
	return "/tmp"
}

// TestAccBackupConfigResource covers create, in-place updates of the location and
// the passphrase, a return to the baseline, and an import. The platform is left
// on the baseline from the environment.
func TestAccBackupConfigResource(t *testing.T) {
	base := testAccBackupSettings(t)
	moved := base
	moved.Location = testAccBackupAltLocation()
	rotated := base
	rotated.Passphrase = base.Passphrase + "-rotated"

	update := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccBackupConfigName, plancheck.ResourceActionUpdate)},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: base.hcl(), Check: testAccCheckBackupConfig(base)},
			{Config: base.hcl(), PlanOnly: true},
			{Config: moved.hcl(), ConfigPlanChecks: update, Check: testAccCheckBackupConfig(moved)},
			{Config: moved.hcl(), PlanOnly: true},
			{Config: rotated.hcl(), ConfigPlanChecks: update, Check: testAccCheckBackupConfig(rotated)},
			{Config: base.hcl(), ConfigPlanChecks: update, Check: testAccCheckBackupConfig(base)},
			{Config: base.hcl(), PlanOnly: true},
			{
				ResourceName:      testAccBackupConfigName,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				// The password and passphrase are write-only, and the platform does not
				// return the host key.
				ImportStateVerifyIgnore: []string{"password", "passphrase", "ssh_public_key"},
			},
			// Destroy is intentionally not tested: the API has no DELETE, so Delete only
			// removes the resource from state and the configuration stays on the platform.
		},
	})
}

// Values the provider can check itself are rejected at plan time.
func TestAccBackupConfigResource_PlanTimeValidation(t *testing.T) {
	base := testAccBackupSettings(t)
	with := func(f func(*testAccBackupFields)) string {
		c := base
		f(&c)
		return c.hcl()
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      with(func(c *testAccBackupFields) { c.Location = base.Location + "/" }),
				ExpectError: regexp.MustCompile(`(?s)trailing slash`),
			},
			{
				Config:      with(func(c *testAccBackupFields) { c.Password = "short" }),
				ExpectError: regexp.MustCompile(`(?s)password.*between 6 and 255`),
			},
			{
				Config:      with(func(c *testAccBackupFields) { c.Passphrase = "short-12" }),
				ExpectError: regexp.MustCompile(`(?s)passphrase.*between 12 and 255`),
			},
		},
	})
}

// Everything else is checked by the platform: the error reaches the user and the
// configuration on the platform is not changed. Each step fails on apply.
func TestAccBackupConfigResource_Rejected(t *testing.T) {
	base := testAccBackupSettings(t)

	cases := []struct {
		label   string
		change  func(*testAccBackupFields)
		wantErr string
	}{
		{"directory that does not exist", func(c *testAccBackupFields) { c.Location = base.Location + "/no-such-directory-acc" }, `(?s)Error (creating|updating) backup config.*API error 400`},
		{"wrong SSH password", func(c *testAccBackupFields) { c.Password = "WrongSshPassw0rd" }, `(?s)API error 400.*(972124|connect)`},
		{"port with no SFTP server", func(c *testAccBackupFields) { c.Port = "2222" }, `(?s)API error 400`},
		{"port 0", func(c *testAccBackupFields) { c.Port = "0" }, `(?s)API error 400`},
		{"port 70000", func(c *testAccBackupFields) { c.Port = "70000" }, `(?s)Error (creating|updating) backup config.*API error 400`},
		{"protocol FTP", func(c *testAccBackupFields) { c.Protocol = "FTP" }, `(?s)API error 400.*SFTP`},
		{"username of 2 characters", func(c *testAccBackupFields) { c.Username = "ab" }, `(?s)API error 400`},
		{"username of 31 characters", func(c *testAccBackupFields) { c.Username = testAccStringOfLen(31, "u") }, `(?s)API error 400`},
	}

	var steps []resource.TestStep
	for _, c := range cases {
		cfg := base
		c.change(&cfg)
		steps = append(steps, resource.TestStep{Config: cfg.hcl(), ExpectError: regexp.MustCompile(c.wantErr)})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// A change made on the platform is shown as a diff and put back by an apply.
func TestAccBackupConfigResource_Drift(t *testing.T) {
	base := testAccBackupSettings(t)
	port, err := strconv.Atoi(base.Port)
	if err != nil {
		t.Fatalf("SSP_ACC_BACKUP_PORT %q is not a number", base.Port)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t); testAccWaitNoBackupInProgress(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: base.hcl(), Check: testAccCheckBackupConfig(base)},
			{
				PreConfig: func() {
					testAccAPIPutWithRevision(t, "/ssp/backup/config", map[string]any{
						"server_address":  base.Server,
						"protocol":        base.Protocol,
						"port":            port,
						"username":        base.Username,
						"password":        base.Password,
						"passphrase":      base.Passphrase,
						"ssh_public_key":  base.Key,
						"backup_location": testAccBackupAltLocation(),
					})
				},
				Config: base.hcl(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(testAccBackupConfigName, plancheck.ResourceActionUpdate)},
				},
				Check: testAccCheckBackupConfig(base),
			},
			{Config: base.hcl(), PlanOnly: true},
		},
	})
}
