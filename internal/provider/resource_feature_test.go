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

// featureMockAPI simulates the subset of the SSP runtime feature-LCM API
// (GET/PUT /ssp/lcm/features/{feature}, GET /ssp/lcm/features/{feature}/status)
// needed to drive ssp_feature through a full deploy/read/undeploy cycle
// without a live SSP cluster.
type featureMockAPI struct {
	mu       sync.Mutex
	revision int
	deployed bool
}

func newFeatureMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &featureMockAPI{revision: 1}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/lcm/features/{feature}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"_revision": m.revision})
	})
	mux.HandleFunc("PUT /ssp/lcm/features/{feature}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body struct {
			Action string `json:"action"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.revision++
		switch body.Action {
		case "RUN_PRECHECK", "DEPLOY":
			m.deployed = true
		case "UNDEPLOY":
			m.deployed = false
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /ssp/lcm/features/{feature}/status", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		overall := "NOT_DEPLOYED"
		if m.deployed {
			overall = "DEPLOYMENT_SUCCESSFUL"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"feature":          r.PathValue("feature"),
			"action":           "DEPLOY",
			"overall_status":   overall,
			"overall_progress": 100,
			"precheck_results": map[string]any{
				"overall_status": "SUCCESS",
				"prechecks":      []any{},
			},
			"deployment_results": map[string]any{
				"deployment_status": overall,
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestUnitFeatureResource exercises ssp_feature's Create (RUN_PRECHECK ->
// DEPLOY), Read, and Delete (UNDEPLOY) against a mocked SSP runtime API.
func TestUnitFeatureResource(t *testing.T) {
	srv := newFeatureMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
resource "ssp_feature" "test" {
  feature = "MALWARE_PREVENTION"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_feature.test", "id", "MALWARE_PREVENTION"),
					resource.TestCheckResourceAttr("ssp_feature.test", "feature", "MALWARE_PREVENTION"),
					resource.TestCheckResourceAttr("ssp_feature.test", "overall_status", "DEPLOYMENT_SUCCESSFUL"),
					resource.TestCheckResourceAttr("ssp_feature.test", "precheck_status", "SUCCESS"),
					resource.TestCheckResourceAttr("ssp_feature.test", "overall_progress", "100"),
				),
			},
			// ImportState Testing
			{
				Config:            testUnitRuntimeProviderConfig(srv.URL) + `resource "ssp_feature" "test" { feature = "MALWARE_PREVENTION" }`,
				ResourceName:      "ssp_feature.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
