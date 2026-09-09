// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func newPlatformStatusDataSourceMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/cluster/monitor/platform/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cluster_id":           "cluster-uuid-999",
			"cluster_name":         "Prod-SSP-Cluster",
			"product_version":      "5.2.0",
			"node_count":           3,
			"form_factor":          "MEDIUM",
			"health":               "UP",
			"message_bus_endpoint": "messagebus.example.com:9092",
			"k8s_version":          "1.24.6",
			"ingress_url":          "ssp.example.com:443",
			"network_data_flow": map[string]any{
				"transmit": 34.7,
				"receive":  24.2,
				"total":    58.9,
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitPlatformStatusDataSource(t *testing.T) {
	srv := newPlatformStatusDataSourceMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitRuntimeProviderConfig(srv.URL) + `
data "ssp_platform_status" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "cluster_id", "cluster-uuid-999"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "cluster_name", "Prod-SSP-Cluster"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "product_version", "5.2.0"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "node_count", "3"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "form_factor", "MEDIUM"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "health", "UP"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "message_bus_endpoint", "messagebus.example.com:9092"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "k8s_version", "1.24.6"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "ingress_url", "ssp.example.com:443"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "network_data_flow.transmit", "34.7"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "network_data_flow.receive", "24.2"),
					resource.TestCheckResourceAttr("data.ssp_platform_status.test", "network_data_flow.total", "58.9"),
				),
			},
		},
	})
}
