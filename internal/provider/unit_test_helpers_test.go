// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

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
