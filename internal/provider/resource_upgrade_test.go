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

// upgradeMockAPI simulates the subset of the SSP runtime API needed to drive
// ssp_upgrade through UPGRADE_COORDINATOR deploy, PRECHECKS_ONLY -> CONTINUE,
// and undeploy, without a live SSP cluster.
type upgradeMockAPI struct {
	mu              sync.Mutex
	revision        int
	coordinatorLive bool
	overallStatus   string
}

func newUpgradeMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &upgradeMockAPI{revision: 1, overallStatus: "NOT_STARTED"}

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
		case "DEPLOY":
			m.coordinatorLive = true
		case "UNDEPLOY":
			m.coordinatorLive = false
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /ssp/lcm/features/{feature}/status", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		overall := "NOT_DEPLOYED"
		if m.coordinatorLive {
			overall = "DEPLOYMENT_SUCCESSFUL"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"feature":            r.PathValue("feature"),
			"overall_status":     overall,
			"deployment_results": map[string]any{"deployment_status": overall},
		})
	})
	mux.HandleFunc("GET /ssp/upgrade/status", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"overall_status":  m.overallStatus,
			"current_version": "5.1.0",
			"target_version":  "5.2.0",
			"upgrade_steps": []map[string]any{
				{"id": "precheck", "display_name": "Pre-checks", "status": "SUCCESS"},
				{"id": "upgrade", "display_name": "Upgrade", "status": "SUCCESS"},
			},
		})
	})
	mux.HandleFunc("POST /ssp/upgrade", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch r.URL.Query().Get("action") {
		case "PRECHECKS_ONLY", "CONTINUE", "START", "RETRY":
			m.overallStatus = "SUCCESS"
		}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestUnitUpgradeResource exercises ssp_upgrade's Create (deploy
// UPGRADE_COORDINATOR, then PRECHECKS_ONLY -> CONTINUE), Read, and Delete
// (undeploy UPGRADE_COORDINATOR) against a mocked SSP runtime API.
func TestUnitUpgradeResource(t *testing.T) {
	srv := newUpgradeMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
resource "ssp_upgrade" "test" {
  run_prechecks = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_upgrade.test", "id", "upgrade"),
					resource.TestCheckResourceAttr("ssp_upgrade.test", "status", "SUCCESS"),
					resource.TestCheckResourceAttr("ssp_upgrade.test", "current_version", "5.1.0"),
					resource.TestCheckResourceAttr("ssp_upgrade.test", "target_version", "5.2.0"),
					resource.TestCheckResourceAttr("ssp_upgrade.test", "progress", "100"),
				),
			},
		},
	})
}
