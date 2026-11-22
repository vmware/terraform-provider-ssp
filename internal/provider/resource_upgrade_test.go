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
)

// upgradeMockAPI simulates the subset of the SSP runtime API needed to drive
// ssp_upgrade through UPGRADE_COORDINATOR deploy, PRECHECKS_ONLY -> CONTINUE,
// and undeploy, without a live SSP cluster.
type upgradeMockAPI struct {
	mu              sync.Mutex
	revision        int
	coordinatorLive bool
	overallStatus   string
	cfgRevision     int
	targetVersion   string
	// Pre-checks, modeled on the real API: a PRECHECKS_ONLY run leaves the
	// top-level overall_status at NOT_STARTED and reports its result only in
	// pre_checks_status; CONTINUE is rejected with 409 until they succeed.
	prechecks     string // "", "SUCCESS" or "FAILED"
	prechecksRun  int64
	failPrechecks bool
}

func newUpgradeMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newUpgradeMockServerWith(t, false)
}

// newUpgradeMockServerWith lets a test make the pre-checks fail.
func newUpgradeMockServerWith(t *testing.T, failPrechecks bool) *httptest.Server {
	t.Helper()
	m := &upgradeMockAPI{revision: 1, overallStatus: "NOT_STARTED", failPrechecks: failPrechecks}

	mux := http.NewServeMux()
	// Upgrade coordinator config. Starts empty, like a freshly deployed cluster,
	// and (like the real backend) ignores a PUT carrying a stale _revision.
	mux.HandleFunc("GET /ssp/lcm/upgrade-coordinator/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"_revision":      m.cfgRevision,
			"target_version": m.targetVersion,
		})
	})
	mux.HandleFunc("PUT /ssp/lcm/upgrade-coordinator/config", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var body struct {
			Revision      int    `json:"_revision"`
			TargetVersion string `json:"target_version"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Revision == m.cfgRevision {
			m.cfgRevision++
			m.targetVersion = body.TargetVersion
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /ssp/lcm/upgrade-coordinator/config/status", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		status := "NOT_CONFIGURED"
		if m.targetVersion != "" {
			status = "DEPLOYMENT_REQUIRED"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": "mock"})
	})
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
		body := map[string]any{
			"overall_status":  m.overallStatus,
			"current_version": "5.1.0",
			"target_version":  "5.2.0",
			"upgrade_steps": []map[string]any{
				{"id": "precheck", "display_name": "Pre-checks", "status": "SUCCESS"},
				{"id": "upgrade", "display_name": "Upgrade", "status": "SUCCESS"},
			},
		}
		if m.prechecks != "" {
			body["pre_checks_status"] = map[string]any{
				"overall_status": m.prechecks,
				"pre_checks": []map[string]any{
					{"display_name": "Disk Space Check", "status": m.prechecks, "last_run_time": m.prechecksRun},
				},
			}
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("POST /ssp/upgrade", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch r.URL.Query().Get("action") {
		case "PRECHECKS_ONLY":
			m.prechecksRun++
			m.prechecks = "SUCCESS"
			if m.failPrechecks {
				m.prechecks = "FAILED"
			}
		case "CONTINUE":
			if m.prechecks != "SUCCESS" {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error_code":976006,"error_message":"CONTINUE action requires pre-checks to have completed successfully"}`))
				return
			}
			m.overallStatus = "SUCCESS"
		case "START", "RETRY":
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
// (undeploy UPGRADE_COORDINATOR) against a mocked SSP runtime API, with the
// target version set first through ssp_upgrade_coordinator_config.
func TestUnitUpgradeResource(t *testing.T) {
	srv := newUpgradeMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
resource "ssp_upgrade_coordinator_config" "test" {
  target_version = "5.2.0"
}

resource "ssp_upgrade" "test" {
  run_prechecks = true
  depends_on    = [ssp_upgrade_coordinator_config.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "id", "singleton"),
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "target_version", "5.2.0"),
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "config_status", "DEPLOYMENT_REQUIRED"),
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

// TestUnitUpgradeCoordinatorConfigResource exercises Create, Read and Update
// (changing target_version) of ssp_upgrade_coordinator_config. The mock ignores
// a PUT with a stale revision, so this also proves the resource sends the
// current revision rather than a cached one.
func TestUnitUpgradeCoordinatorConfigResource(t *testing.T) {
	srv := newUpgradeMockServer(t)

	cfg := func(version string) string {
		return testUnitRuntimeProviderConfig(srv.URL) + `
resource "ssp_upgrade_coordinator_config" "test" {
  target_version = "` + version + `"
}
`
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("5.2.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "id", "singleton"),
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "target_version", "5.2.0"),
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "revision", "1"),
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "config_status", "DEPLOYMENT_REQUIRED"),
				),
			},
			{
				Config: cfg("5.2.1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "target_version", "5.2.1"),
					resource.TestCheckResourceAttr("ssp_upgrade_coordinator_config.test", "revision", "2"),
				),
			},
		},
	})
}

// TestUnitUpgradeResource_MissingTargetVersion verifies ssp_upgrade fails early
// with an actionable message, without deploying the coordinator, when no target
// version has been configured.
func TestUnitUpgradeResource_MissingTargetVersion(t *testing.T) {
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
				ExpectError: regexp.MustCompile(`no target\s+version\s+configured`),
			},
		},
	})
}

// TestUnitUpgradeResource_PrechecksFail verifies a failed pre-check run stops
// the upgrade before CONTINUE and names the failing check, and that the state
// saved by the failed create holds no unknown values.
func TestUnitUpgradeResource_PrechecksFail(t *testing.T) {
	srv := newUpgradeMockServerWith(t, true)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
resource "ssp_upgrade_coordinator_config" "test" {
  target_version = "5.2.0"
}

resource "ssp_upgrade" "test" {
  run_prechecks = true
  depends_on    = [ssp_upgrade_coordinator_config.test]
}
`,
				ExpectError: regexp.MustCompile(`pre-checks failed: Disk Space Check`),
			},
		},
	})
}
