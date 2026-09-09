// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"crypto/tls"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"golang.org/x/net/proxy"

	"github.com/vmware/terraform-provider-ssp/internal/provider"
)

// testAccProtoV6ProviderFactories passes the provider factory into test assertions.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ssp": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func testAccSSPRuntimePreCheck(t *testing.T) {
	if v := os.Getenv("SSP_HOST"); v == "" {
		t.Skip("SSP_HOST must be set for SSP runtime acceptance tests")
	}
	if v := os.Getenv("SSP_USERNAME"); v == "" {
		t.Skip("SSP_USERNAME must be set for SSP runtime acceptance tests")
	}
	if v := os.Getenv("SSP_PASSWORD"); v == "" {
		t.Skip("SSP_PASSWORD must be set for SSP runtime acceptance tests")
	}
}

// testAccNSXSitePreCheck additionally requires a real NSX Manager to onboard
// ssp_site against. Onboarding a site is a mutating, non-trivial operation
// against real NSX infrastructure, so — unlike testAccSSPRuntimePreCheck's
// requirements — these variables are not expected to be present in most CI
// environments; TestAccSiteResource will skip (not fail) without them.
func testAccNSXSitePreCheck(t *testing.T) {
	testAccSSPRuntimePreCheck(t)
	if v := os.Getenv("NSX_HOST"); v == "" {
		t.Skip("NSX_HOST must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
	if v := os.Getenv("NSX_USERNAME"); v == "" {
		t.Skip("NSX_USERNAME must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
	if v := os.Getenv("NSX_PASSWORD"); v == "" {
		t.Skip("NSX_PASSWORD must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
	if v := os.Getenv("NSX_CERTIFICATE"); v == "" {
		t.Skip("NSX_CERTIFICATE must be set for ssp_site acceptance tests (requires a real NSX Manager)")
	}
}

// testAccInsecureHTTPClient builds an *http.Client suitable for a
// CheckDestroy function to call the SSP runtime API directly (outside the
// provider under test), matching the transport settings
// (proxy-aware, TLS-verification-skipping) used elsewhere in this package's
// acceptance tests.
func testAccInsecureHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	if cd, ok := proxy.FromEnvironment().(proxy.ContextDialer); ok {
		transport.DialContext = cd.DialContext
	}
	return &http.Client{Transport: transport}
}

// testAccTrimHost strips a trailing slash from an SSP_HOST-style base URL so
// callers can safely append a leading-slash path.
func testAccTrimHost(host string) string {
	return strings.TrimRight(host, "/")
}
