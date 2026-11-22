// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Every data source below is compared with the platform's own answer read
// directly from the API, not only checked for "is set". Several of them once
// returned empty values (a wrong JSON key, an unescaped id, a missing
// pagination loop), which "is set" style checks cannot catch.

// testAccDataSourceHealthRe matches the SSP health enum.
var testAccDataSourceHealthRe = regexp.MustCompile(`^(UP|PARTIALLY_UP|DOWN)$`)

// testAccCheckListLenEqualsAPI compares the length of a list attribute with the
// length of the array under key in the response of path.
func testAccCheckListLenEqualsAPI(t *testing.T, name, attr, path, key string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(name, attr+".#", func(got string) error {
		list, _ := testAccAPIGet(t, path)[key].([]any)
		if want := strconv.Itoa(len(list)); got != want {
			return fmt.Errorf("%s.%s has %s elements, the API %s returns %s under %q", name, attr, got, path, want, key)
		}
		return nil
	})
}

func TestAccPlatformStatusDataSource(t *testing.T) {
	const name = "data.ssp_platform_status.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_platform_status" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(name, "cluster_id"),
					resource.TestCheckResourceAttrSet(name, "cluster_name"),
					resource.TestCheckResourceAttrSet(name, "form_factor"),
					resource.TestCheckResourceAttrSet(name, "ingress_url"),
					resource.TestCheckResourceAttrSet(name, "k8s_version"),
					resource.TestCheckResourceAttrSet(name, "message_bus_endpoint"),
					resource.TestCheckResourceAttrSet(name, "product_version"),
					resource.TestCheckResourceAttrSet(name, "network_data_flow.total"),
					resource.TestCheckResourceAttrWith(name, "health", func(v string) error {
						if !testAccDataSourceHealthRe.MatchString(v) {
							return fmt.Errorf("health = %q, want UP, PARTIALLY_UP or DOWN", v)
						}
						return nil
					}),
					resource.TestCheckResourceAttrWith(name, "node_count", func(v string) error {
						if n, err := strconv.Atoi(v); err != nil || n < 1 {
							return fmt.Errorf("node_count = %q, want at least 1", v)
						}
						return nil
					}),
					// Values that do not change from one call to the next must equal the API.
					testAccCheckAttrEqualsAPI(t, name, "cluster_id", func() string {
						return fmt.Sprint(testAccAPIGet(t, "/ssp/cluster/monitor/platform/status")["cluster_id"])
					}),
					testAccCheckAttrEqualsAPI(t, name, "product_version", func() string {
						return fmt.Sprint(testAccAPIGet(t, "/ssp/cluster/monitor/platform/status")["product_version"])
					}),
					testAccCheckAttrEqualsAPI(t, name, "form_factor", func() string {
						return fmt.Sprint(testAccAPIGet(t, "/ssp/cluster/monitor/platform/status")["form_factor"])
					}),
				),
			},
		},
	})
}

// overall_status was once always empty because the API names it "status".
func TestAccFeatureHealthDataSource(t *testing.T) {
	const name = "data.ssp_feature_health.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_feature_health" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith(name, "overall_status", func(v string) error {
						if !testAccDataSourceHealthRe.MatchString(v) {
							return fmt.Errorf("overall_status = %q, want UP, PARTIALLY_UP or DOWN", v)
						}
						return nil
					}),
					testAccCheckAttrEqualsAPI(t, name, "overall_status", func() string {
						return fmt.Sprint(testAccAPIGet(t, "/ssp/cluster/monitor/feature/health")["status"])
					}),
					testAccCheckListLenEqualsAPI(t, name, "features", "/ssp/cluster/monitor/feature/health", "features"),
				),
			},
		},
	})
}

// testAccCheckSitesMatchAPI compares every site of ssp_sites, in order, with
// GET /ssp/sites. current_state once came back empty for every site.
func testAccCheckSitesMatchAPI(t *testing.T, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}
		sites, _ := testAccAPIGet(t, "/ssp/sites")["sites"].([]any)
		if got, want := rs.Primary.Attributes["sites.#"], strconv.Itoa(len(sites)); got != want {
			return fmt.Errorf("sites.# = %s, the API lists %s sites", got, want)
		}
		for i, raw := range sites {
			site, _ := raw.(map[string]any)
			status, _ := site["status"].(map[string]any)
			cfg, _ := status["configuration_status"].(map[string]any)
			want := map[string]string{
				"id":                fmt.Sprint(site["id"]),
				"site_name":         fmt.Sprint(site["site_name"]),
				"site_type":         fmt.Sprint(site["site_type"]),
				"connection_status": fmt.Sprint(status["connection_status"]),
				"current_state":     fmt.Sprint(cfg["current_status"]),
			}
			for attr, w := range want {
				key := fmt.Sprintf("sites.%d.%s", i, attr)
				if got := rs.Primary.Attributes[key]; got != w {
					return fmt.Errorf("%s = %q, the API reports %q", key, got, w)
				}
			}
			if rs.Primary.Attributes[fmt.Sprintf("sites.%d.current_state", i)] == "" {
				return fmt.Errorf("sites.%d.current_state is empty", i)
			}
		}
		return nil
	}
}

func TestAccSitesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_sites" "test" {}`,
				Check:  testAccCheckSitesMatchAPI(t, "data.ssp_sites.test"),
			},
		},
	})
}

// testAccFirstSiteID returns the id of the first onboarded site, or fails the test.
func testAccFirstSiteID(t *testing.T) string {
	t.Helper()

	sites, _ := testAccAPIGet(t, "/ssp/sites")["sites"].([]any)
	if len(sites) == 0 {
		t.Fatal("no site is onboarded to the SSP; onboard one to run this test")
	}
	site, _ := sites[0].(map[string]any)
	return fmt.Sprint(site["id"])
}

// ssp_site by id: every field must equal the API, current_state included.
func TestAccSiteDataSource_MatchesAPI(t *testing.T) {
	const name = "data.ssp_site.test"
	var id string

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			id = testAccFirstSiteID(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "ssp_sites" "all" {}

data "ssp_site" "test" {
  id = data.ssp_sites.all.sites[0].id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith(name, "id", func(got string) error {
						if got != id {
							return fmt.Errorf("id = %q, want %q", got, id)
						}
						return nil
					}),
					testAccCheckAttrEqualsAPI(t, name, "site_name", func() string {
						return fmt.Sprint(testAccAPIGet(t, "/ssp/sites/"+id)["site_name"])
					}),
					testAccCheckAttrEqualsAPI(t, name, "site_type", func() string {
						return fmt.Sprint(testAccAPIGet(t, "/ssp/sites/"+id)["site_type"])
					}),
					testAccCheckAttrEqualsAPI(t, name, "connection_status", func() string {
						st, _ := testAccAPIGet(t, "/ssp/sites/"+id)["status"].(map[string]any)
						return fmt.Sprint(st["connection_status"])
					}),
					testAccCheckAttrEqualsAPI(t, name, "cluster_status", func() string {
						st, _ := testAccAPIGet(t, "/ssp/sites/"+id)["status"].(map[string]any)
						return fmt.Sprint(st["cluster_status"])
					}),
					testAccCheckAttrEqualsAPI(t, name, "current_state", func() string {
						st, _ := testAccAPIGet(t, "/ssp/sites/"+id)["status"].(map[string]any)
						cfg, _ := st["configuration_status"].(map[string]any)
						return fmt.Sprint(cfg["current_status"])
					}),
					resource.TestCheckResourceAttrSet(name, "nsx_version"),
					resource.TestCheckResourceAttrSet(name, "nsx_cluster_id"),
				),
			},
		},
	})
}

// A site id that does not resolve to a site, and ids carrying path or query
// characters, must fail instead of returning an empty or the wrong site.
func TestAccSiteDataSource_InvalidID(t *testing.T) {
	cases := []struct {
		name, id, wantErr string
	}{
		{"nonexistent uuid", "00000000-0000-0000-0000-000000000000", `Site not found`},
		{"malformed id", "not-a-uuid", `Site not found`},
		{"empty id", "", `Site not found|Error reading site`},
		{"parent path", "../sites", `Error reading site|Site not found`},
		{"sub path", "00000000-0000-0000-0000-000000000000/settings", `Error reading site|Site not found`},
		{"query string", "00000000-0000-0000-0000-000000000000?x=1", `Site not found`},
	}

	var steps []resource.TestStep
	for _, c := range cases {
		steps = append(steps, resource.TestStep{
			Config: fmt.Sprintf(`
data "ssp_site" "test" {
  id = %q
}
`, c.id),
			ExpectError: regexp.MustCompile(c.wantErr),
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// The query string case needs an existing site: "<id>?x=1" used to be accepted
// and return that site.
func TestAccSiteDataSource_QueryStringOnRealID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccSSPRuntimePreCheck(t)
			testAccFirstSiteID(t) // fails when no site is onboarded
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "ssp_sites" "all" {}

data "ssp_site" "test" {
  id = "${data.ssp_sites.all.sites[0].id}?x=1"
}
`,
				ExpectError: regexp.MustCompile(`Site not found`),
			},
		},
	})
}

func TestAccLicensesDataSource(t *testing.T) {
	const name = "data.ssp_licenses.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_licenses" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckListLenEqualsAPI(t, name, "results", "/ssp/licensing-client/licenses", "results"),
					// When a license exists, the first one must match field for field.
					func(s *terraform.State) error {
						list, _ := testAccAPIGet(t, "/ssp/licensing-client/licenses")["results"].([]any)
						if len(list) == 0 {
							return nil
						}
						first, _ := list[0].(map[string]any)
						attrs := s.RootModule().Resources[name].Primary.Attributes
						for _, k := range []string{"license_id", "license_type", "sku_code", "source", "source_id", "unit_of_measure", "product_family", "product_display_name"} {
							if got, want := attrs["results.0."+k], fmt.Sprint(first[k]); got != want {
								return fmt.Errorf("results.0.%s = %q, the API reports %q", k, got, want)
							}
						}
						if got, want := attrs["results.0.quantity"], strconv.FormatInt(int64(first["quantity"].(float64)), 10); got != want {
							return fmt.Errorf("results.0.quantity = %q, the API reports %q", got, want)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccSensorsDataSource(t *testing.T) {
	const name = "data.ssp_sensors.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_sensors" "test" {}`,
				Check:  testAccCheckListLenEqualsAPI(t, name, "results", "/sensors/appliances", "results"),
			},
		},
	})
}

// ssp_backup_status once returned an empty list (the API key is "result") and,
// once that was fixed, only the first page of 10 records. It must return every
// backup, in the API's order.
func TestAccBackupStatusDataSource(t *testing.T) {
	const name = "data.ssp_backup_status.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_backup_status" "test" {}`,
				Check: func(s *terraform.State) error {
					api := testAccAPIGet(t, "/ssp/backup/status?page_size=1000")
					list, _ := api["result"].([]any)
					attrs := s.RootModule().Resources[name].Primary.Attributes
					if got, want := attrs["results.#"], strconv.Itoa(len(list)); got != want {
						return fmt.Errorf("results.# = %s, the API reports %s backups", got, want)
					}
					if total := int(api["total_result_count"].(float64)); total != len(list) {
						return fmt.Errorf("the API returned %d of %d backups for page_size=1000", len(list), total)
					}
					for i, raw := range list {
						b, _ := raw.(map[string]any)
						for attr, key := range map[string]string{"backup_id": "id", "status": "status", "backup_trigger_type": "backup_trigger_type", "backup_type": "backup_type"} {
							if got, want := attrs[fmt.Sprintf("results.%d.%s", i, attr)], fmt.Sprint(b[key]); got != want {
								return fmt.Errorf("results.%d.%s = %q, the API reports %q", i, attr, got, want)
							}
						}
					}
					return nil
				},
			},
		},
	})
}

// The list was once always empty because the API names it versions_for_upgrade.
func TestAccUpgradeAvailableVersionsDataSource(t *testing.T) {
	const name = "data.ssp_upgrade_available_versions.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_upgrade_available_versions" "test" {}`,
				Check:  testAccCheckListLenEqualsAPI(t, name, "versions", "/ssp/upgrade/available-versions", "versions_for_upgrade"),
			},
		},
	})
}

func TestAccUpgradeHistoryDataSource(t *testing.T) {
	const name = "data.ssp_upgrade_history.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "ssp_upgrade_history" "test" {}`,
				Check:  testAccCheckListLenEqualsAPI(t, name, "history", "/ssp/upgrade/history", "results"),
			},
		},
	})
}

// testAccFeatureNames are the values the API accepts for a feature.
var testAccFeatureNames = []string{
	"MALWARE_PREVENTION", "INTELLIGENCE", "NDR", "BAREMETAL_SECURITY", "RULE_ANALYSIS",
	"AI_ASSISTANT_PLATFORM", "AI_ASSISTANT_THREAT_DEFENSE", "MALWARE_ANALYSIS_VC", "SPAC",
	"NETWORK_TRAFFIC_ANALYSIS", "UPGRADE_COORDINATOR", "CLOUD_CONNECTOR", "METRICS", "VIRTUAL_PATCHING",
}

// Every valid feature name is read and compared with the platform.
func TestAccFeatureDataSource_AllFeatures(t *testing.T) {
	var steps []resource.TestStep
	for _, feature := range testAccFeatureNames {
		feature := feature
		const name = "data.ssp_feature.test"
		steps = append(steps, resource.TestStep{
			Config: fmt.Sprintf(`
data "ssp_feature" "test" {
  feature = %q
}
`, feature),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(name, "feature", feature),
				testAccCheckAttrEqualsAPI(t, name, "overall_status", func() string {
					return fmt.Sprint(testAccAPIGet(t, "/ssp/lcm/features/"+feature+"/status")["overall_status"])
				}),
				testAccCheckAttrEqualsAPI(t, name, "deployment_status", func() string {
					dr, _ := testAccAPIGet(t, "/ssp/lcm/features/"+feature+"/status")["deployment_results"].(map[string]any)
					return fmt.Sprint(dr["deployment_status"])
				}),
				// A feature without prechecks (METRICS) has no precheck status; an empty
				// string is stored as an absent attribute, so compare the map directly.
				func(s *terraform.State) error {
					pr, _ := testAccAPIGet(t, "/ssp/lcm/features/"+feature+"/status")["precheck_results"].(map[string]any)
					want := ""
					if v, ok := pr["overall_status"]; ok {
						want = fmt.Sprint(v)
					}
					if got := s.RootModule().Resources[name].Primary.Attributes["precheck_status"]; got != want {
						return fmt.Errorf("precheck_status = %q, the API reports %q", got, want)
					}
					return nil
				},
			),
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// Feature names are case-sensitive and must be one of the API's values.
func TestAccFeatureDataSource_InvalidFeature(t *testing.T) {
	var steps []resource.TestStep
	for _, feature := range []string{"FOO", "ndr", "", "ALL"} {
		steps = append(steps, resource.TestStep{
			Config: fmt.Sprintf(`
data "ssp_feature" "test" {
  feature = %q
}
`, feature),
			ExpectError: regexp.MustCompile(`(?s)Error reading feature status.*must be one of: MALWARE_PREVENTION`),
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// Required arguments are rejected at plan time.
func TestAccDataSources_MissingRequiredArguments(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "ssp_site" "test" {}`,
				ExpectError: regexp.MustCompile(`(?s)Missing required argument.*"id"`),
			},
			{
				Config:      `data "ssp_feature" "test" {}`,
				ExpectError: regexp.MustCompile(`(?s)Missing required argument.*"feature"`),
			},
		},
	})
}

// Reading the same data sources twice must not produce a diff.
func TestAccDataSources_NoDiffOnReplan(t *testing.T) {
	cfg := `
data "ssp_platform_status" "test" {}
data "ssp_feature_health" "test" {}
data "ssp_sites" "test" {}
data "ssp_licenses" "test" {}
data "ssp_sensors" "test" {}
data "ssp_backup_status" "test" {}
data "ssp_upgrade_available_versions" "test" {}
data "ssp_upgrade_history" "test" {}
data "ssp_feature" "test" {
  feature = "NDR"
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSSPRuntimePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{Config: cfg, PlanOnly: true},
		},
	})
}
