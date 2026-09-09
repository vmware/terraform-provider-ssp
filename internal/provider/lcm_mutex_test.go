// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLcmMuSerializesConcurrentLCMActions verifies the concurrency guarantee
// lcmMu exists for: two goroutines racing to run an "LCM action" (modeled
// here as a critical section that increments a counter, holds the lock for a
// short duration, then decrements it) must never both be inside the critical
// section at the same time. Without the lock, both goroutines would enter
// concurrently and observe a concurrent count > 1; with it, the count must
// never exceed 1, matching the guarantee ssp_feature/ssp_site/ssp_backup/
// ssp_restore/ssp_upgrade/ssp_ndr_config/ssp_cloud_connector_config/
// ssp_malware_prevention_config all rely on (FSDD §9's mutex-serialization
// design).
func TestLcmMuSerializesConcurrentLCMActions(t *testing.T) {
	var concurrent int32
	var maxConcurrent int32
	var wg sync.WaitGroup

	criticalSection := func() {
		lcmMu.Lock()
		defer lcmMu.Unlock()

		n := atomic.AddInt32(&concurrent, 1)
		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if n <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&concurrent, -1)
	}

	const goroutines = 5
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			criticalSection()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxConcurrent); got != 1 {
		t.Fatalf("expected lcmMu to serialize all critical sections (max concurrent = 1), observed max concurrent = %d", got)
	}
}
