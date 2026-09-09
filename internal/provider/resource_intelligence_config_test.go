// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// intelligenceConfigMockAPI simulates
// GET/PUT /ssp/lcm/intelligence/config and
// GET /ssp/lcm/intelligence/config/status.
type intelligenceConfigMockAPI struct {
	mu       sync.Mutex
	revision int
	body     map[string]any
}

func newIntelligenceConfigMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &intelligenceConfigMockAPI{
		body: map[string]any{"enable_advanced_features": false},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/lcm/intelligence/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		resp := map[string]any{}
		for k, v := range m.body {
			resp[k] = v
		}
		resp["_revision"] = m.revision
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("PUT /ssp/lcm/intelligence/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.revision++
		m.body = body
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /ssp/lcm/intelligence/config/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "CONFIGURATION_APPLIED",
			"message": "Configuration applied successfully.",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitIntelligenceConfig(host string, enableAdvancedFeatures bool) string {
	return testUnitRuntimeProviderConfig(host) + fmt.Sprintf(`
resource "ssp_intelligence_config" "test" {
  enable_advanced_features = %t
}
`, enableAdvancedFeatures)
}

// TestUnitIntelligenceConfigResource exercises ssp_intelligence_config's
// Create, Read, and Update (toggling enable_advanced_features) against a
// mocked SSP runtime API. No Delete verification: the API has no DELETE for
// this singleton resource and the resource's Delete() is a documented no-op.
func TestUnitIntelligenceConfigResource(t *testing.T) {
	srv := newIntelligenceConfigMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitIntelligenceConfig(srv.URL, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_intelligence_config.test", "id", "singleton"),
					resource.TestCheckResourceAttr("ssp_intelligence_config.test", "enable_advanced_features", "true"),
					resource.TestCheckResourceAttr("ssp_intelligence_config.test", "config_status", "CONFIGURATION_APPLIED"),
				),
			},
			// Update: toggle enable_advanced_features in place.
			{
				Config: testUnitIntelligenceConfig(srv.URL, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_intelligence_config.test", "enable_advanced_features", "false"),
				),
			},
		},
	})
}
