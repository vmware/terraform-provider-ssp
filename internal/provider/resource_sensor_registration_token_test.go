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

// newSensorTokenMockServer simulates POST/GET/DELETE
// /sensors/registration-tokens. Like the real API it defaults a missing
// allowed_instance_count to 1, rejects an explicit value outside 1-10 with
// error 201016, and echoes description back as "" when it was sent empty.
func newSensorTokenMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	var mu sync.Mutex
	tokens := map[string]map[string]any{}
	next := 0

	mux := http.NewServeMux()
	mux.HandleFunc("POST /sensors/registration-tokens", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		w.Header().Set("Content-Type", "application/json")

		count := 1.0
		if v, ok := in["allowed_instance_count"]; ok {
			count, _ = v.(float64)
			if count < 1 || count > 10 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error_code": 201016, "error_message": "allowed_instance_count out of range"})
				return
			}
		}

		mu.Lock()
		defer mu.Unlock()
		next++
		id := "token-" + string(rune('a'+next))
		desc, _ := in["description"].(string)
		tok := map[string]any{
			"id":                     id,
			"display_name":           in["display_name"],
			"description":            desc,
			"allowed_instance_count": count,
			"status":                 "AVAILABLE",
			"expiration":             1794750274830,
			"usage_count":            0,
			"number_available":       count,
			"registration_manifest":  "manifest-" + id,
		}
		tokens[id] = tok
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(tok)
	})
	mux.HandleFunc("GET /sensors/registration-tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		tok, ok := tokens[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tok)
	})
	mux.HandleFunc("DELETE /sensors/registration-tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		delete(tokens, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sensorTokenConfig(host, extra string) string {
	return testUnitRuntimeProviderConfig(host) + `
resource "ssp_sensor_registration_token" "test" {
  display_name = "tf-token"
  passphrase   = "Str0ng-Passphrase-123!"
` + extra + `
}
`
}

func TestUnitSensorRegistrationTokenResource_Basic(t *testing.T) {
	srv := newSensorTokenMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sensorTokenConfig(srv.URL, `description = "d"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ssp_sensor_registration_token.test", "description", "d"),
					resource.TestCheckResourceAttr("ssp_sensor_registration_token.test", "allowed_instance_count", "1"),
					resource.TestCheckResourceAttr("ssp_sensor_registration_token.test", "status", "AVAILABLE"),
				),
			},
		},
	})
}

// description = "" used to be stored as null while the API returned "", which
// failed the apply with "inconsistent result".
func TestUnitSensorRegistrationTokenResource_EmptyDescription(t *testing.T) {
	srv := newSensorTokenMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sensorTokenConfig(srv.URL, `description = ""`),
				Check:  resource.TestCheckResourceAttr("ssp_sensor_registration_token.test", "description", ""),
			},
		},
	})
}

// allowed_instance_count = 0 used to be dropped from the request (omitempty),
// so the API defaulted it to 1 and the apply failed with "inconsistent result".
// It must now reach the API, which rejects it.
func TestUnitSensorRegistrationTokenResource_ZeroInstanceCount(t *testing.T) {
	srv := newSensorTokenMockServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      sensorTokenConfig(srv.URL, `allowed_instance_count = 0`),
				ExpectError: regexp.MustCompile(`201016`),
			},
		},
	})
}
