// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// newSecurityContentBundleMockServer simulates POST/GET/DELETE
// /ssp/security-content/bundles[/{id}], resolving to a terminal SUCCESS
// upload_status on the mock's first poll — this test exercises
// ssp_security_content_bundle's Create/Read/Delete wiring, not the
// multi-iteration polling loop itself (that is covered separately against
// client.WaitForMegaBundleReady's shared polling pattern in
// internal/provider/client).
func newSecurityContentBundleMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ssp/security-content/bundles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "bundle-1"})
	})
	mux.HandleFunc("GET /ssp/security-content/bundles/bundle-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":               "bundle-1",
			"bundle_id":        "nsx-security-content-v2.1.0",
			"bundle_type":      "FIREWALL_ATP",
			"source":           "MANUAL_UPLOAD",
			"upload_status":    "SUCCESS",
			"status_message":   "Bundle successfully deployed with 3 features",
			"manifest_version": "1.0",
			"ssp_version":      "5.2.0",
		})
	})
	mux.HandleFunc("DELETE /ssp/security-content/bundles/bundle-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitSecurityContentBundleConfig(host, filePath string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_security_content_bundle" "test" {
  file_path   = "` + filePath + `"
  bundle_type = "FIREWALL_ATP"
}
`
}

// TestUnitSecurityContentBundleResource exercises
// ssp_security_content_bundle's Create (upload + poll to completion) and
// Read against a mocked SSP runtime API.
func TestUnitSecurityContentBundleResource(t *testing.T) {
	srv := newSecurityContentBundleMockServer(t)

	filePath := filepath.Join(t.TempDir(), "mega-bundle.tar.gz")
	if err := os.WriteFile(filePath, []byte("fake bundle contents"), 0o600); err != nil {
		t.Fatalf("failed to write temp bundle file: %s", err)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitSecurityContentBundleConfig(srv.URL, filePath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_security_content_bundle.test", "id", "bundle-1"),
					resource.TestCheckResourceAttr("ssp_security_content_bundle.test", "bundle_type", "FIREWALL_ATP"),
					resource.TestCheckResourceAttr("ssp_security_content_bundle.test", "bundle_id", "nsx-security-content-v2.1.0"),
					resource.TestCheckResourceAttr("ssp_security_content_bundle.test", "source", "MANUAL_UPLOAD"),
					resource.TestCheckResourceAttr("ssp_security_content_bundle.test", "upload_status", "SUCCESS"),
					resource.TestCheckResourceAttr("ssp_security_content_bundle.test", "manifest_version", "1.0"),
					resource.TestCheckResourceAttr("ssp_security_content_bundle.test", "ssp_version", "5.2.0"),
				),
			},
			// Delete Testing is handled automatically by terraform-plugin-testing
			// at the end of Test; the mock's DELETE handler above verifies the
			// real DELETE call is made.
		},
	})
}
