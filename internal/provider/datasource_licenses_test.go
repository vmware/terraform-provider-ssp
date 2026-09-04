package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newLicensesDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/licenses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"license_id":           "LIC-12345",
					"product_display_name": "VMware vDefend Firewall",
					"product_family":       "FIREWALL WITH ATP",
					"quantity":             64,
					"unit_of_measure":      "Core",
					"source":               "VDLS",
					"sku_code":             "VDEF-FW-ATP",
					"expiration_date":      1800000000000,
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitLicensesDataSource(t *testing.T) {
	srv := newLicensesDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_licenses" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_licenses.test", "results.#", "1"),
					resource.TestCheckResourceAttr("data.ssp_licenses.test", "results.0.license_id", "LIC-12345"),
					resource.TestCheckResourceAttr("data.ssp_licenses.test", "results.0.product_display_name", "VMware vDefend Firewall"),
					resource.TestCheckResourceAttr("data.ssp_licenses.test", "results.0.quantity", "64"),
					resource.TestCheckResourceAttr("data.ssp_licenses.test", "results.0.unit_of_measure", "Core"),
				),
			},
		},
	})
}
