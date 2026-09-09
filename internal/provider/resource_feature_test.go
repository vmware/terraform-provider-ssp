// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// featureMockAPI simulates the subset of the SSP runtime feature-LCM API
// (GET/PUT /ssp/lcm/features/{feature}, GET /ssp/lcm/features/{feature}/status)
// needed to drive ssp_feature through a full deploy/read/undeploy cycle
// without a live SSP cluster.
type featureMockAPI struct {
	mu         sync.Mutex
	revision   int
	deployed   bool
	lastAction string
}

func newFeatureMockServer(t *testing.T) (*httptest.Server, *featureMockAPI) {
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
		m.lastAction = body.Action
		switch body.Action {
		case "RUN_PRECHECK", "DEPLOY":
			m.deployed = true
		case "UNDEPLOY", "FORCE_UNDEPLOY":
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
	return srv, m
}

// TestUnitFeatureResource exercises ssp_feature's Create (RUN_PRECHECK ->
// DEPLOY), Read, and Delete (UNDEPLOY) against a mocked SSP runtime API.
func TestUnitFeatureResource(t *testing.T) {
	srv, _ := newFeatureMockServer(t)

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

// TestUnitFeatureResource_MetricsRejected verifies that METRICS -- pre-installed
// and not independently deploy/undeploy-able per the SspFeature spec -- is
// rejected at plan time by the `feature` attribute's OneOf validator, rather
// than failing confusingly against the live API at apply time.
func TestUnitFeatureResource_MetricsRejected(t *testing.T) {
	srv, _ := newFeatureMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
resource "ssp_feature" "test" {
  feature = "METRICS"
}
`,
				ExpectError: regexp.MustCompile(`(?s)Attribute feature value must be one of:.*got:\s*"METRICS"`),
			},
		},
	})
}

// TestUnitFeatureResource_ForceUndeploy verifies that setting force_undeploy
// = true causes Delete to issue action=FORCE_UNDEPLOY instead of UNDEPLOY.
func TestUnitFeatureResource_ForceUndeploy(t *testing.T) {
	srv, m := newFeatureMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
resource "ssp_feature" "test" {
  feature        = "NDR"
  force_undeploy = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_feature.test", "force_undeploy", "true"),
				),
			},
			// Empty step so the framework destroys the resource before we
			// inspect m.lastAction below (Destroy Testing at the very end of
			// resource.UnitTest happens after this function returns, too late
			// to observe here) -- an empty config forces destruction of the
			// still-existing resource from the previous step.
			{
				Config: testUnitRuntimeProviderConfig(srv.URL),
				Check: func(s *terraform.State) error {
					m.mu.Lock()
					defer m.mu.Unlock()
					if m.lastAction != "FORCE_UNDEPLOY" {
						t.Fatalf("expected Delete to issue action=FORCE_UNDEPLOY, last action was %q", m.lastAction)
					}
					return nil
				},
			},
		},
	})
}
