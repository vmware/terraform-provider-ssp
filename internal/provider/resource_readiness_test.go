// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// newReadinessMockServer simulates GET /ssp/cluster/monitor/platform/status
// returning HTTP 200 immediately, so ssp_readiness's WaitForSSPAPIReady call
// resolves on its first check without needing the real 30s retry interval.
func newReadinessMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/cluster/monitor/platform/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cluster_id":"cluster-1","health":"HEALTHY"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitReadinessConfig(host string, timeoutMinutes int) string {
	return testUnitRuntimeProviderConfig(host) + fmt.Sprintf(`
resource "ssp_readiness" "test" {
  timeout_minutes = %d
}
`, timeoutMinutes)
}

// TestUnitReadinessResource exercises ssp_readiness's Create (readiness
// poll), Read (no-op passthrough), and Update (new timeout_minutes value,
// no remote call) against a mocked SSP runtime API. No Delete verification:
// the resource's Delete() is a documented no-op (there is nothing to remove
// remotely).
func TestUnitReadinessResource(t *testing.T) {
	srv := newReadinessMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitReadinessConfig(srv.URL, 20),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_readiness.test", "id", "done"),
					resource.TestCheckResourceAttr("ssp_readiness.test", "timeout_minutes", "20"),
				),
			},
			// Update: change timeout_minutes; Update() stores the new value
			// without re-running the readiness poll.
			{
				Config: testUnitReadinessConfig(srv.URL, 5),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_readiness.test", "id", "done"),
					resource.TestCheckResourceAttr("ssp_readiness.test", "timeout_minutes", "5"),
				),
			},
		},
	})
}
