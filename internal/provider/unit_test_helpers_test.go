package provider_test

import "fmt"

// testUnitRuntimeProviderConfig renders a `provider "ssp"` block pointing the
// SSP runtime (Day-2) client at a local httptest server, for unit tests that
// exercise runtime resources (ssp_feature, ssp_site, ssp_upgrade, ...)
// without any live SSP cluster.
func testUnitRuntimeProviderConfig(host string) string {
	return fmt.Sprintf(`
provider "ssp" {
  host     = %q
  username = "admin"
  password = "admin"
  insecure = true
}
`, host)
}

// testUnitSSPIProviderConfig renders a `provider "ssp"` block pointing the
// SSPI appliance (Day-0/1) client at a local httptest server, for unit tests
// that exercise installer resources (ssp_platform, ...) without any live
// SSPI appliance.
func testUnitSSPIProviderConfig(host string) string {
	return fmt.Sprintf(`
provider "ssp" {
  sspi_host     = %q
  sspi_username = "admin"
  sspi_password = "admin"
  sspi_insecure = true
}
`, host)
}
