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

// newCloudConnectorConfigMockServer simulates
// GET /ssp/lcm/cloud-connector/regions,
// GET/PUT /ssp/lcm/cloud-connector/config and
// GET /ssp/lcm/cloud-connector/config/status.
func newCloudConnectorConfigMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	var mu sync.Mutex
	revision := 0
	body := map[string]any{
		"region":      "west.us",
		"fqdn":        "nsx.west.us.lastline.com",
		"region_name": "United States 1",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/lcm/cloud-connector/regions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cloud_regions": []map[string]any{
				{"region": "west.us", "fqdn": "nsx.west.us.lastline.com", "region_name": "United States 1"},
				{"region": "nl.emea", "fqdn": "nsx.nl.emea.lastline.com", "region_name": "European Union 1"},
			},
		})
	})
	mux.HandleFunc("GET /ssp/lcm/cloud-connector/config", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		resp := map[string]any{"_revision": revision, "configurable": true}
		for k, v := range body {
			resp[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("PUT /ssp/lcm/cloud-connector/config", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		revision++
		body = map[string]any{
			"region":      in["region"],
			"fqdn":        in["fqdn"],
			"region_name": in["region_name"],
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /ssp/lcm/cloud-connector/config/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "DEPLOYMENT_REQUIRED",
			"message": "Cloud Connector configuration has been updated but the feature has not been deployed.",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testUnitCloudConnectorConfig(host, region string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_cloud_connector_config" "test" {
  region = "` + region + `"
}
`
}

// TestUnitCloudConnectorConfigResource_RegionChange guards against "inconsistent
// result after apply" when region changes: fqdn and region_name are resolved
// from the new region, so they must be planned as unknown rather than carried
// over from state.
func TestUnitCloudConnectorConfigResource_RegionChange(t *testing.T) {
	srv := newCloudConnectorConfigMockServer(t)
	const res = "ssp_cloud_connector_config.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testUnitCloudConnectorConfig(srv.URL, "west.us"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "id", "singleton"),
					resource.TestCheckResourceAttr(res, "fqdn", "nsx.west.us.lastline.com"),
					resource.TestCheckResourceAttr(res, "region_name", "United States 1"),
				),
			},
			{
				Config: testUnitCloudConnectorConfig(srv.URL, "nl.emea"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "region", "nl.emea"),
					resource.TestCheckResourceAttr(res, "fqdn", "nsx.nl.emea.lastline.com"),
					resource.TestCheckResourceAttr(res, "region_name", "European Union 1"),
				),
			},
			{
				Config: testUnitCloudConnectorConfig(srv.URL, "west.us"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "fqdn", "nsx.west.us.lastline.com"),
					resource.TestCheckResourceAttr(res, "region_name", "United States 1"),
				),
			},
		},
	})
}

// TestUnitCloudConnectorConfigResource_UnknownRegion checks that a region not
// returned by the platform is rejected with the list of available regions.
func TestUnitCloudConnectorConfigResource_UnknownRegion(t *testing.T) {
	srv := newCloudConnectorConfigMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testUnitCloudConnectorConfig(srv.URL, "mars.one"),
				ExpectError: regexp.MustCompile(`not found in available regions`),
			},
		},
	})
}
