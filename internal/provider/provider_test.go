// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/vmware/terraform-provider-ssp/internal/provider"
)

// testAccProtoV6ProviderFactories passes the provider factory into test assertions.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ssp": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// testAccSSPRuntimePreCheck fails, rather than skips, when the platform
// connection is not configured: with TF_ACC=1 the operator asked for the live
// suite, and a green run in which every test was skipped would hide that
// nothing ran. Tests that need optional infrastructure fail too (see
// testAccRequireEnv and testAccRequireOptIn).
func testAccSSPRuntimePreCheck(t *testing.T) {
	t.Helper()

	for _, name := range []string{"SSP_HOST", "SSP_USERNAME", "SSP_PASSWORD"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s must be set for the SSP acceptance tests (they run against a live platform when TF_ACC=1)", name)
		}
	}
}

// testAccNSXSitePreCheck additionally requires a real NSX Manager to onboard
// ssp_site against. Onboarding a site is a mutating, non-trivial operation
// against real NSX infrastructure, so — unlike testAccSSPRuntimePreCheck's
// requirements — TestAccSiteResource will fail without them.
func testAccNSXSitePreCheck(t *testing.T) {
	testAccSSPRuntimePreCheck(t)
	if v := os.Getenv("NSX_HOST"); v == "" {
		t.Fatal("NSX_HOST must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
	if v := os.Getenv("NSX_USERNAME"); v == "" {
		t.Fatal("NSX_USERNAME must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
	if v := os.Getenv("NSX_PASSWORD"); v == "" {
		t.Fatal("NSX_PASSWORD must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
	if v := os.Getenv("NSX_CERTIFICATE"); v == "" {
		t.Fatal("NSX_CERTIFICATE must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
}
