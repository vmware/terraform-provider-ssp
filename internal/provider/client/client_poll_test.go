// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if err := c.WaitForSiteDeleted(context.Background(), "site-1"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("expected at least 3 poll calls (loop must iterate), got %d", got)
	}
}
