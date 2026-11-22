// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// withFastPolling shrinks defaultPollInterval/defaultPollTimeout for the
// duration of a test, restoring the real 15s/90min production values on
// cleanup. This is what makes multi-iteration polling loops testable at all
// (they are package-level vars, not consts, specifically for this purpose).
func withFastPolling(t *testing.T, timeout time.Duration) {
	t.Helper()
	origInterval, origTimeout := defaultPollInterval, defaultPollTimeout
	defaultPollInterval = 10 * time.Millisecond
	defaultPollTimeout = timeout
	t.Cleanup(func() {
		defaultPollInterval, defaultPollTimeout = origInterval, origTimeout
	})
}

func newTestClient(url string) *Client {
	return NewClient(url, "admin", "admin", true)
}

// TestWaitForFeatureDeployment_MultiIteration verifies the poll loop actually
// iterates: the mock reports IN_PROGRESS for the first two calls and only
// resolves to a terminal state on the third.
func TestWaitForFeatureDeployment_MultiIteration(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/lcm/features/NDR/status", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		status := "DEPLOYMENT_IN_PROGRESS"
		if n >= 3 {
			status = "DEPLOYMENT_SUCCESSFUL"
		}
		_ = json.NewEncoder(w).Encode(FeatureDeploymentStatus{Feature: "NDR", OverallStatus: status})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	status, err := c.WaitForFeatureDeployment(context.Background(), "NDR")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if status.OverallStatus != "DEPLOYMENT_SUCCESSFUL" {
		t.Fatalf("expected DEPLOYMENT_SUCCESSFUL, got %s", status.OverallStatus)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// TestWaitForSiteReady_MultiIteration verifies the site-readiness poll loop
// iterates past an unhealthy state before resolving.
func TestWaitForSiteReady_MultiIteration(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		state := "ONBOARDING"
		if n >= 3 {
			state = "READY"
		}
		_ = json.NewEncoder(w).Encode(Site{ID: "site-1", CurrentState: state})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	if err := c.WaitForSiteReady(context.Background(), "site-1"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// siteWithReadiness builds a site whose readiness is only in
// status.configuration_status, as the live platform returns it.
func siteWithReadiness(readiness, connection string) Site {
	st := &NsxSiteStatus{ConnectionStatus: connection}
	if readiness != "" {
		st.ConfigurationStatus = &SiteConfigurationStatus{CurrentStatus: readiness}
	}
	return Site{ID: "site-1", Status: st}
}

// TestWaitForSiteReady_WaitsPastHealthyConnection verifies a HEALTHY connection
// does not end the wait while the site is still ONBOARD_IN_PROGRESS.
func TestWaitForSiteReady_WaitsPastHealthyConnection(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		readiness := "ONBOARD_IN_PROGRESS"
		if atomic.AddInt32(&calls, 1) >= 4 {
			readiness = "READY"
		}
		_ = json.NewEncoder(w).Encode(siteWithReadiness(readiness, "HEALTHY"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := newTestClient(srv.URL).WaitForSiteReady(context.Background(), "site-1"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got := atomic.LoadInt32(&calls); got < 4 {
		t.Fatalf("returned after %d polls, before the site was READY", got)
	}
}

// TestWaitForSiteReady_FailsOnNestedNotReady verifies a terminal state that is
// only reported under status.configuration_status ends the wait with an error.
func TestWaitForSiteReady_FailsOnNestedNotReady(t *testing.T) {
	withFastPolling(t, time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(siteWithReadiness("NOT_READY", "HEALTHY"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := newTestClient(srv.URL).WaitForSiteReady(context.Background(), "site-1")
	if err == nil || !strings.Contains(err.Error(), "terminal failure state: current_state=NOT_READY") {
		t.Fatalf("expected a terminal failure, got %v", err)
	}
}

// TestWaitForSiteReady_TransientNotReady verifies the NOT_READY a new onboard
// reports for its first moments does not fail the wait.
func TestWaitForSiteReady_TransientNotReady(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		readiness := []string{"NOT_READY", "ONBOARD_IN_PROGRESS", "NOT_READY", "ONBOARD_IN_PROGRESS", "READY"}
		n := int(atomic.AddInt32(&calls, 1))
		if n > len(readiness) {
			n = len(readiness)
		}
		_ = json.NewEncoder(w).Encode(siteWithReadiness(readiness[n-1], "UNKNOWN"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := newTestClient(srv.URL).WaitForSiteReady(context.Background(), "site-1"); err != nil {
		t.Fatalf("a transient NOT_READY must not fail the wait: %s", err)
	}
}

// sitePrechecks builds a site that only reports prechecks, like a PREPARE site.
func sitePrechecks(state string, results ...SitePrecheckRun) Site {
	return Site{ID: "site-1", Status: &NsxSiteStatus{ConfigurationStatus: &SiteConfigurationStatus{
		CurrentStatus:      "NOT_READY",
		SitePrecheckStatus: &SitePrecheckStatus{Phase: "ONBOARD", State: state, Results: results},
	}}}
}

// TestWaitForSitePrechecks_PassesWhileNotReady verifies a PREPARE site, which stays
// NOT_READY, completes once the prechecks leave IN_PROGRESS and passed.
func TestWaitForSitePrechecks_PassesWhileNotReady(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		state := "IN_PROGRESS"
		if atomic.AddInt32(&calls, 1) >= 3 {
			state = "PASSED"
		}
		_ = json.NewEncoder(w).Encode(sitePrechecks(state))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := newTestClient(srv.URL).WaitForSitePrechecks(context.Background(), "site-1"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("returned after %d polls, before the prechecks finished", got)
	}
}

// TestWaitForSitePrechecks_Failed verifies failed prechecks are reported by name.
func TestWaitForSitePrechecks_Failed(t *testing.T) {
	withFastPolling(t, time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sitePrechecks("FAILED",
			SitePrecheckRun{Name: "SiteConnectionPrecheck", Status: "PASSED"},
			SitePrecheckRun{Name: "SiteRegistrationPrecheck", Status: "FAILED", Message: "site is registered to another SSP"},
		))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := newTestClient(srv.URL).WaitForSitePrechecks(context.Background(), "site-1")
	if err == nil || !strings.Contains(err.Error(), "SiteRegistrationPrecheck (FAILED): site is registered to another SSP") ||
		strings.Contains(err.Error(), "SiteConnectionPrecheck") {
		t.Fatalf("expected only the failed precheck in the error, got %v", err)
	}
}

// TestWaitForSiteReady_HealthyWithoutReadiness verifies a HEALTHY connection still
// completes the wait when the platform reports no readiness state at all.
func TestWaitForSiteReady_HealthyWithoutReadiness(t *testing.T) {
	withFastPolling(t, time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(siteWithReadiness("", "HEALTHY"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := newTestClient(srv.URL).WaitForSiteReady(context.Background(), "site-1"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}

// TestWaitForBackupComplete_MultiIteration verifies the backup poll loop
// iterates through IN_PROGRESS before reaching a terminal SUCCESS.
func TestWaitForBackupComplete_MultiIteration(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/backup/status/backup-1", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		status := "IN_PROGRESS"
		if n >= 3 {
			status = "SUCCESS"
		}
		_ = json.NewEncoder(w).Encode(BackupStatus{ID: "backup-1", Status: status})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	status, err := c.WaitForBackupComplete(context.Background(), "backup-1")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if status.Status != "SUCCESS" {
		t.Fatalf("expected SUCCESS, got %s", status.Status)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// TestWaitForBackupComplete_Failure verifies a terminal FAILED status is
// surfaced as an error rather than treated as success.
func TestWaitForBackupComplete_Failure(t *testing.T) {
	withFastPolling(t, time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/backup/status/backup-2", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(BackupStatus{ID: "backup-2", Status: "FAILED"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	_, err := c.WaitForBackupComplete(context.Background(), "backup-2")
	if err == nil {
		t.Fatal("expected an error for a FAILED backup status, got nil")
	}
}

// TestWaitForRestoreComplete_MultiIteration mirrors the backup test for restore.
func TestWaitForRestoreComplete_MultiIteration(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/restore/status/restore-1", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		status := "IN_PROGRESS"
		if n >= 3 {
			status = "SUCCESS"
		}
		_ = json.NewEncoder(w).Encode(RestoreStatus{ID: "restore-1", Status: status})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	status, err := c.WaitForRestoreComplete(context.Background(), "restore-1")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if status.Status != "SUCCESS" {
		t.Fatalf("expected SUCCESS, got %s", status.Status)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// TestWaitForUpgradeStatus_MultiIteration verifies the upgrade status poll
// loop iterates through IN_PROGRESS before resolving to a stable state.
func TestWaitForUpgradeStatus_MultiIteration(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/upgrade/status", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		status := "IN_PROGRESS"
		if n >= 3 {
			status = "SUCCESS"
		}
		_ = json.NewEncoder(w).Encode(UpgradeStatus{OverallStatus: status, CurrentVersion: "5.1.0", TargetVersion: "5.2.0"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	status, err := c.WaitForUpgradeStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if status.OverallStatus != "SUCCESS" {
		t.Fatalf("expected SUCCESS, got %s", status.OverallStatus)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// TestWaitForBackupComplete_Timeout verifies the deadline logic: if the
// status never reaches a terminal state, the poller returns a timeout error
// rather than blocking forever.
func TestWaitForBackupComplete_Timeout(t *testing.T) {
	withFastPolling(t, 50*time.Millisecond)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/backup/status/backup-3", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(BackupStatus{ID: "backup-3", Status: "IN_PROGRESS"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	_, err := c.WaitForBackupComplete(context.Background(), "backup-3")
	if err == nil {
		t.Fatal("expected a timeout error when the backup never reaches a terminal state, got nil")
	}
}

// withFastAPIReadyPolling shrinks sspAPIReadyPollInterval for the duration of
// a test, restoring the real 30s production value on cleanup.
func withFastAPIReadyPolling(t *testing.T) {
	t.Helper()
	orig := sspAPIReadyPollInterval
	sspAPIReadyPollInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		sspAPIReadyPollInterval = orig
	})
}

// TestWaitForSSPAPIReady_MultiIteration verifies the readiness poll loop
// iterates through connection errors before the API starts responding.
func TestWaitForSSPAPIReady_MultiIteration(t *testing.T) {
	withFastAPIReadyPolling(t)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/cluster/monitor/platform/status", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(ClusterStatus{ClusterID: "cluster-1", Health: "HEALTHY"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	if err := c.WaitForSSPAPIReady(context.Background(), time.Second); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// TestWaitForSSPAPIReady_Timeout verifies the deadline logic: if the API
// never returns a successful response, the poller returns a timeout error
// rather than blocking forever.
func TestWaitForSSPAPIReady_Timeout(t *testing.T) {
	withFastAPIReadyPolling(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/cluster/monitor/platform/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	if err := c.WaitForSSPAPIReady(context.Background(), 50*time.Millisecond); err == nil {
		t.Fatal("expected a timeout error when the API never becomes ready, got nil")
	}
}

// TestWaitForSSPAPIReady_TimeoutReportsLastError verifies the timeout error
// carries the last API error body instead of a generic message.
func TestWaitForSSPAPIReady_TimeoutReportsLastError(t *testing.T) {
	withFastAPIReadyPolling(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/cluster/monitor/platform/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error_message":"platform starting"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := newTestClient(srv.URL).WaitForSSPAPIReady(context.Background(), 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "platform starting") {
		t.Fatalf("expected timeout error containing the last API error, got %v", err)
	}
}

// TestWaitForSSPAPIReady_UnauthorizedFailsFast verifies 401 is not retried.
func TestWaitForSSPAPIReady_UnauthorizedFailsFast(t *testing.T) {
	withFastAPIReadyPolling(t)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/cluster/monitor/platform/status", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_code":401,"error_message":"Unauthorized Access"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := newTestClient(srv.URL).WaitForSSPAPIReady(context.Background(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "Unauthorized Access") {
		t.Fatalf("expected credentials error with API message, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 call, got %d", got)
	}
}

// TestWaitForSSPAPIReady_InvalidHost verifies a host without a scheme fails
// immediately.
func TestWaitForSSPAPIReady_InvalidHost(t *testing.T) {
	err := newTestClient("ssp.example.com").WaitForSSPAPIReady(context.Background(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "invalid SSP host") {
		t.Fatalf("expected invalid host error, got %v", err)
	}
}

// TestWaitForFeaturePrechecks_MultiIteration verifies the precheck poll loop
// iterates through IN_PROGRESS before reaching a terminal SUCCESS.
func TestWaitForFeaturePrechecks_MultiIteration(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/lcm/features/NDR/status", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		status := "IN_PROGRESS"
		if n >= 3 {
			status = "SUCCESS"
		}
		_ = json.NewEncoder(w).Encode(FeatureDeploymentStatus{
			Feature:         "NDR",
			PrecheckResults: PrecheckResults{OverallStatus: status},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	status, err := c.WaitForFeaturePrechecks(context.Background(), "NDR")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if status.PrecheckResults.OverallStatus != "SUCCESS" {
		t.Fatalf("expected SUCCESS, got %s", status.PrecheckResults.OverallStatus)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// TestWaitForSiteDeleted_MultiIteration verifies the site-deletion poll loop
// iterates while the site is still present (200) before resolving on 404.
func TestWaitForSiteDeleted_MultiIteration(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			_ = json.NewEncoder(w).Encode(Site{ID: "site-1", CurrentState: "OFFBOARDING"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	if err := c.WaitForSiteDeleted(context.Background(), "site-1", 0); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}

// TestWaitForSiteDeleted_OffboardPrecheckFailed verifies a failed offboard
// precheck from a run after the delete request ends the wait with an error, while
// the same result from an earlier run is ignored.
func TestWaitForSiteDeleted_OffboardPrecheckFailed(t *testing.T) {
	withFastPolling(t, time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/sites/site-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Site{ID: "site-1", Status: &NsxSiteStatus{
			ConfigurationStatus: &SiteConfigurationStatus{
				CurrentStatus: "READY",
				SitePrecheckStatus: &SitePrecheckStatus{
					Phase: "OFFBOARD",
					Results: []SitePrecheckRun{{
						Name: "FeaturesActivatePrecheck", Status: "FAILED", RunTime: 2000,
						Message: "features are currently enabled on SSP",
					}},
				},
			},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL)

	err := c.WaitForSiteDeleted(context.Background(), "site-1", 1000)
	if err == nil || !strings.Contains(err.Error(), "FeaturesActivatePrecheck: features are currently enabled on SSP") {
		t.Fatalf("expected the failed precheck in the error, got %v", err)
	}

	// A failure from before the request (run time <= since) is stale: keep polling
	// until the timeout instead of failing.
	defaultPollTimeout = 100 * time.Millisecond
	err = c.WaitForSiteDeleted(context.Background(), "site-1", 2000)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout for a stale precheck, got %v", err)
	}
}

func TestIsBackupNotFound(t *testing.T) {
	notFound := errors.New(`API error 400: {"error_code":972011,"error_message":"Backup status not found for ID: x.","module_name":"cluster-api"}`)
	other400 := errors.New(`API error 400: {"error_code":972051,"error_message":"site not ready"}`)
	if !IsBackupNotFound(404, errors.New("API error 404: ")) {
		t.Error("404 should be not-found")
	}
	if !IsBackupNotFound(400, notFound) {
		t.Error("400/972011 should be not-found")
	}
	if IsBackupNotFound(400, other400) {
		t.Error("other 400 must not be not-found")
	}
	if IsBackupNotFound(500, notFound) || IsBackupNotFound(200, nil) {
		t.Error("non-400/404 must not be not-found")
	}
}

func preChecksBody(overall string, lastRun int64) UpgradeStatus {
	return UpgradeStatus{
		OverallStatus: "NOT_STARTED", // stays NOT_STARTED for a PRECHECKS_ONLY run
		PreChecksStatus: &UpgradePreChecksStatus{
			OverallStatus: overall,
			PreChecks:     []UpgradePreCheck{{DisplayName: "Disk", Status: overall, LastRunTime: lastRun}},
		},
	}
}

// TestWaitForUpgradePrechecks_WaitsPastNotStarted reproduces the live failure:
// while pre-checks run, the top-level overall_status is NOT_STARTED, which
// WaitForUpgradeStatus treats as settled. WaitForUpgradePrechecks must keep
// polling until pre_checks_status itself is terminal.
func TestWaitForUpgradePrechecks_WaitsPastNotStarted(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/upgrade/status", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		body := preChecksBody("IN_PROGRESS", 0)
		if n >= 3 {
			body = preChecksBody("SUCCESS", 100)
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, err := newTestClient(srv.URL).WaitForUpgradePrechecks(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if status.PreChecksStatus == nil || status.PreChecksStatus.OverallStatus != "SUCCESS" {
		t.Fatalf("expected pre-checks SUCCESS, got %+v", status.PreChecksStatus)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 polls, got %d", got)
	}
}

// TestWaitForUpgradePrechecks_IgnoresPreviousRun verifies a terminal result left
// over from an earlier pre-checks run is not mistaken for the run just started:
// the stale SUCCESS has last_run_time == since, so polling continues until a
// check runs again.
func TestWaitForUpgradePrechecks_IgnoresPreviousRun(t *testing.T) {
	withFastPolling(t, time.Second)

	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/upgrade/status", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		body := preChecksBody("SUCCESS", 100) // previous run, unchanged
		if n >= 3 {
			body = preChecksBody("FAILED", 200) // the new run
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, err := newTestClient(srv.URL).WaitForUpgradePrechecks(context.Background(), 100)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if status.PreChecksStatus.OverallStatus != "FAILED" {
		t.Fatalf("expected the new run's FAILED, got the previous run's %s", status.PreChecksStatus.OverallStatus)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 polls, got %d", got)
	}
}

// TestWaitForUpgradePrechecks_Timeout verifies a pre-check run that never
// finishes ends in an error rather than hanging.
func TestWaitForUpgradePrechecks_Timeout(t *testing.T) {
	withFastPolling(t, 100*time.Millisecond)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ssp/upgrade/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(preChecksBody("IN_PROGRESS", 0))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if _, err := newTestClient(srv.URL).WaitForUpgradePrechecks(context.Background(), 0); err == nil ||
		!strings.Contains(err.Error(), "pre-checks") {
		t.Fatalf("expected a pre-checks timeout error, got %v", err)
	}
}
