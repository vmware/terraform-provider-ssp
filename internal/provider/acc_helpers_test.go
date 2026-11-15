// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Acceptance tests run with TF_ACC=1 against a live SSP and read SSP_HOST,
// SSP_USERNAME, SSP_PASSWORD and SSP_INSECURE like the provider does. Some tests
// need more, and skip without it:
//
//	SSP_ACC_BACKUP_SERVER, _USERNAME, _PASSWORD, _PASSPHRASE, _LOCATION,
//	_SSH_PUBLIC_KEY       an SFTP server for ssp_backup_config (optional _PORT, _ALT_LOCATION)
//	SSP_ACC_ALLOW_BACKUP     take real backups (ssp_backup)
//	SSP_ACC_ALLOW_RESTORE    restore the whole platform (ssp_restore)
//	SSP_ACC_ALLOW_UPGRADE    upgrade the platform (ssp_upgrade)
//	SSP_ACC_ALLOW_BAD_CREDENTIALS  send a wrong password (can lock the account)
//	SSP_ACC_TEST_FEATURE     a feature to deploy and undeploy (ssp_feature)
//	SSP_ACC_FEATURE_PRECHECK_FAILS  a feature whose pre-checks fail
//	SSP_ACC_MALWARE_CLOUD_UNSUPPORTED  CLOUD analysis mode is not supported here
//	NSX_HOST, NSX_USERNAME, NSX_PASSWORD, NSX_CERTIFICATE, NSX_SITE_NAME
//	                         an NSX Manager that is not onboarded yet (ssp_site)
//	SSP_ACC_ALLOW_SITE_ONBOARD  send onboarding requests for that NSX Manager
//
// Most tests change platform settings and leave them on the defaults noted in
// each test. Do not run the suite against a cluster whose settings matter.

// testAccAPIRequest calls the SSP API directly (outside the provider under
// test) with the acceptance-test credentials. It is used to inject drift,
// to read the platform's own view of a resource, and to verify results.
func testAccAPIRequest(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()

	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %s", err)
		}
		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, testAccTrimHost(os.Getenv("SSP_HOST"))+path, rdr) //nolint:gosec // operator-configured SSP_HOST acceptance-test target
	if err != nil {
		t.Fatalf("build request: %s", err)
	}
	req.SetBasicAuth(os.Getenv("SSP_USERNAME"), os.Getenv("SSP_PASSWORD"))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := testAccInsecureHTTPClient().Do(req) //nolint:gosec // same operator-configured target as above
	if err != nil {
		t.Fatalf("%s %s: %s", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response of %s %s: %s", method, path, err)
	}
	return resp.StatusCode, out
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

// testAccRequireEnv skips the test unless every named variable is set, and
// returns their values. Used for tests that need real external infrastructure
// (an SFTP server, an NSX Manager) or an explicit opt-in.
func testAccRequireEnv(t *testing.T, names ...string) map[string]string {
	t.Helper()

	values := make(map[string]string, len(names))
	for _, name := range names {
		v := os.Getenv(name)
		if v == "" {
			t.Skipf("%s must be set for this acceptance test", name)
		}
		values[name] = v
	}
	return values
}

// testAccRequireOptIn skips the test unless name is "true". It guards tests
// that are destructive or slow on a shared cluster.
func testAccRequireOptIn(t *testing.T, name, why string) {
	t.Helper()

	if os.Getenv(name) != "true" {
		t.Skipf("%s=true must be set to opt in to this test: %s", name, why)
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
