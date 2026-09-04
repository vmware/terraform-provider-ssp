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

func testAccPreCheck(t *testing.T) {
	if v := os.Getenv("SSPI_HOST"); v == "" {
		if v2 := os.Getenv("SSPI_ENDPOINT"); v2 == "" {
			t.Skip("SSPI_HOST or SSPI_ENDPOINT must be set for SSPI acceptance tests")
		}
	}
	if v := os.Getenv("SSPI_USERNAME"); v == "" {
		t.Skip("SSPI_USERNAME must be set for SSPI acceptance tests")
	}
	if v := os.Getenv("SSPI_PASSWORD"); v == "" {
		t.Skip("SSPI_PASSWORD must be set for SSPI acceptance tests")
	}
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
