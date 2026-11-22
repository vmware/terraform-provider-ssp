// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

// Acceptance tests run with TF_ACC=1 against a live SSP and read SSP_HOST,
// SSP_USERNAME, SSP_PASSWORD and SSP_INSECURE like the provider does, and fail
// without SSP_HOST, SSP_USERNAME and SSP_PASSWORD. Some tests need more, and skip
// without it:
//
//	SSP_ACC_BACKUP_SERVER, _USERNAME, _PASSWORD, _PASSPHRASE, _LOCATION,
//	_SSH_PUBLIC_KEY       an SFTP server for ssp_backup_config (optional _PORT, _ALT_LOCATION)
//	SSP_ACC_ALLOW_BACKUP     take real backups (ssp_backup); default true
//	SSP_ACC_ALLOW_RESTORE    restore the whole platform (ssp_restore); default true
//	SSP_ACC_ALLOW_BAD_CREDENTIALS  send a wrong password (can lock the account); default true
//	SSP_ACC_TEST_FEATURE     a feature to deploy and undeploy (ssp_feature); default INTELLIGENCE
//	SSP_ACC_MALWARE_CLOUD_UNSUPPORTED  CLOUD analysis mode is not supported here; default true
//	NSX_HOST, NSX_USERNAME, NSX_PASSWORD, NSX_CERTIFICATE, NSX_SITE_NAME
//	                         an NSX Manager that is not onboarded yet (ssp_site)
//	SSP_ACC_ALLOW_SITE_ONBOARD  send onboarding requests for that NSX Manager; default true
//
// Most tests change platform settings and leave them on the defaults noted in
// each test. Do not run the suite against a cluster whose settings matter.

// testAccClient returns the provider's own API client, built from the same
// SSP_* variables the provider reads. Acceptance tests use it, instead of a
// second HTTP implementation, to inject drift, read the platform's view of a
// resource and verify results.
func testAccClient() *client.Client {
	return client.NewClient(
		strings.TrimRight(os.Getenv("SSP_HOST"), "/"),
		os.Getenv("SSP_USERNAME"),
		os.Getenv("SSP_PASSWORD"),
		os.Getenv("SSP_INSECURE") == "true",
	)
}

// testAccDoAPI sends one request through the provider's client and returns the
// HTTP status and body. A 4xx/5xx is not an error here: the status is returned
// and the body is the platform's error text. err is set only when no response
// arrived.
func testAccDoAPI(method, path string, in any) (status int, body []byte, err error) {
	c := testAccClient()
	ctx := context.Background()

	var raw json.RawMessage
	switch method {
	case http.MethodGet:
		status, err = c.Get(ctx, path, &raw)
	case http.MethodPost:
		status, err = c.Post(ctx, path, in, &raw)
	case http.MethodPut:
		status, err = c.Put(ctx, path, in, &raw)
	case http.MethodDelete:
		status, err = c.Delete(ctx, path)
	default:
		return 0, nil, fmt.Errorf("unsupported method %s", method)
	}
	if err != nil {
		if status == 0 {
			return 0, nil, err
		}
		return status, []byte(err.Error()), nil
	}
	return status, raw, nil
}

// testAccAPIRequest calls the SSP API through the provider's client and fails
// the test if no response arrives.
func testAccAPIRequest(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()

	status, out, err := testAccDoAPI(method, path, body)
	if err != nil {
		t.Fatalf("%s %s: %s", method, path, err)
	}
	return status, out
}

// testAccAPIGet GETs path, requires HTTP 200, and decodes the JSON object.
func testAccAPIGet(t *testing.T, path string) map[string]any {
	t.Helper()

	status, body := testAccAPIRequest(t, http.MethodGet, path, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", path, status, body)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode GET %s: %s: %s", path, err, body)
	}
	return out
}

// testAccAPIPutWithRevision reads path, copies its _revision into payload and
// PUTs it, the same optimistic-locking sequence the provider uses. It is used
// to change a singleton behind Terraform's back.
func testAccAPIPutWithRevision(t *testing.T, path string, payload map[string]any) {
	t.Helper()

	current := testAccAPIGet(t, path)
	payload["_revision"] = current["_revision"]
	status, body := testAccAPIRequest(t, http.MethodPut, path, payload)
	if status < 200 || status > 299 {
		t.Fatalf("PUT %s: status %d: %s", path, status, body)
	}
}

// testAccRequireEnv fails the test unless every named variable is set, and
// returns their values. Used for tests that need real external infrastructure
// (an SFTP server, an NSX Manager) or an explicit opt-in.
func testAccRequireEnv(t *testing.T, names ...string) map[string]string {
	t.Helper()

	values := make(map[string]string, len(names))
	for _, name := range names {
		v := os.Getenv(name)
		if v == "" {
			t.Fatalf("%s must be set for this acceptance test", name)
		}
		values[name] = v
	}
	return values
}

// testAccRequireOptIn gates the tests that are destructive or slow on a shared
// cluster. They run on the lab, so each switch defaults to true and needs no
// environment variable; set it to anything else (for example false) to fail the
// test on a cluster where it must not run.
func testAccRequireOptIn(t *testing.T, name, why string) {
	t.Helper()

	if v, ok := os.LookupEnv(name); ok && v != "true" {
		t.Fatalf("%s=%q turns this test off (unset it or set it to true to run it): %s", name, v, why)
	}
}

// testAccCheckAttrEqualsAPI compares a state attribute with a value read from
// the platform at check time. get returns the expected value as a string.
func testAccCheckAttrEqualsAPI(t *testing.T, name, attr string, get func() string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(name, attr, func(got string) error {
		want := get()
		if got != want {
			return fmt.Errorf("%s.%s = %q in state, but the API reports %q", name, attr, got, want)
		}
		return nil
	})
}

// testAccWaitNoBackupInProgress waits until no backup is running. The platform
// refuses to change the backup or recurring-backup configuration, and to start
// another backup, while one runs, and it starts automatic ones itself (after a
// site is onboarded, and on the recurring schedule).
func testAccWaitNoBackupInProgress(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Minute)
	for {
		status, body := testAccAPIRequest(t, http.MethodGet, "/ssp/backup/status?page_size=3", nil)
		if status != http.StatusOK || !strings.Contains(string(body), `"IN_PROGRESS"`) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("a backup was still in progress after 15 minutes")
		}
		time.Sleep(15 * time.Second)
	}
}

// testAccStringOfLen returns a string of n copies of ch.
func testAccStringOfLen(n int, ch string) string {
	return strings.Repeat(ch, n)
}
