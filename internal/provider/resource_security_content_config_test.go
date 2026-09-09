// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// securityContentConfigMockAPI simulates the singleton
// /ssp/security-content/config API.
type securityContentConfigMockAPI struct {
	mu       sync.Mutex
	revision int
	mode     string
}

func newSecurityContentConfigMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &securityContentConfigMockAPI{revision: 1, mode: "DISCONNECTED"}

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/security-content/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		writeJSON(w, map[string]any{
			"id":                           "scs-config",
			"_revision":                    m.revision,
			"connectivity_mode":            m.mode,
			"connectivity_mode_changeable": true,
		})
	})
	mux.HandleFunc("PUT /ssp/security-content/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mode, _ = body["connectivity_mode"].(string)
		m.revision++
		writeJSON(w, map[string]any{
			"id":                           "scs-config",
			"_revision":                    m.revision,
			"connectivity_mode":            m.mode,
			"connectivity_mode_changeable": true,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitSecurityContentConfigConfig(host, mode string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_security_content_config" "test" {
  connectivity_mode = "` + mode + `"
}
`
}

// TestUnitSecurityContentConfigResource exercises ssp_security_content_config's
// Create, Read, and Update against a mocked SSP runtime API.
func TestUnitSecurityContentConfigResource(t *testing.T) {
	srv := newSecurityContentConfigMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitSecurityContentConfigConfig(srv.URL, "DISCONNECTED"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_security_content_config.test", "id", "scs-config"),
					resource.TestCheckResourceAttr("ssp_security_content_config.test", "connectivity_mode", "DISCONNECTED"),
					resource.TestCheckResourceAttr("ssp_security_content_config.test", "connectivity_mode_changeable", "true"),
				),
			},
			// Update: change connectivity_mode in place.
			{
				Config: testUnitSecurityContentConfigConfig(srv.URL, "CONNECTED"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_security_content_config.test", "connectivity_mode", "CONNECTED"),
				),
			},
		},
	})
}
