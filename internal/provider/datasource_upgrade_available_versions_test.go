package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newUpgradeAvailableVersionsDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/upgrade/available-versions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"versions": []string{"5.2.1", "5.3.0"},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitUpgradeAvailableVersionsDataSource(t *testing.T) {
	srv := newUpgradeAvailableVersionsDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_upgrade_available_versions" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_upgrade_available_versions.test", "versions.#", "2"),
					resource.TestCheckResourceAttr("data.ssp_upgrade_available_versions.test", "versions.0", "5.2.1"),
					resource.TestCheckResourceAttr("data.ssp_upgrade_available_versions.test", "versions.1", "5.3.0"),
				),
			},
		},
	})
}
