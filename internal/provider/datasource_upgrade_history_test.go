package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newUpgradeHistoryDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/upgrade/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id":             "upgrade-rec-001",
					"target_version": "5.2.0",
					"status":         "SUCCESS",
					"current_step":   "COMPLETED",
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitUpgradeHistoryDataSource(t *testing.T) {
	srv := newUpgradeHistoryDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_upgrade_history" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_upgrade_history.test", "history.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_upgrade_history.test", "history.0.id", "upgrade-rec-001"),
					resource.TestCheckResourceAttr("data.ssp_upgrade_history.test", "history.0.target_version", "5.2.0"),
					resource.TestCheckResourceAttr("data.ssp_upgrade_history.test", "history.0.status", "SUCCESS"),
					resource.TestCheckResourceAttr("data.ssp_upgrade_history.test", "history.0.current_step", "COMPLETED"),
				),
			},
		},
	})
}
