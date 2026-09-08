package cube

import (
	"context"
	"testing"
	"time"
)

// The helper schedstat baseline must be sampled only after the quiet control
// window has completed. Otherwise CPU time consumed while the helper is merely
// parked during control can be counted toward the intervention runtime delta.
func TestV30SchedstatBaselineIsTakenAfterQuietControl(t *testing.T) {
	f := newV30Fixture(t)
	controlComplete := false
	f.executor.sleep = func(ctx context.Context, d time.Duration) error {
		if d != 200*time.Millisecond {
			t.Fatalf("control duration=%s want 200ms", d)
		}
		f.events = append(f.events, "CONTROL_SLEEP")
		controlComplete = true
		return ctx.Err()
	}
	originalSchedstat := f.executor.readSchedstat
	reads := 0
	f.executor.readSchedstat = func(pid int) (uint64, error) {
		reads++
		if reads == 1 && !controlComplete {
			t.Fatalf("schedstat baseline sampled before quiet control completed; events=%v", f.events)
		}
		return originalSchedstat(pid)
	}

	auth, err := validateV30(t, f)
	if err != nil {
		t.Fatalf("ValidateRuntimeCPUThrottleInterventionAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("Wave30 authority invalid")
	}
	if reads < 2 {
		t.Fatalf("schedstat reads=%d want at least 2", reads)
	}
}

// Once the quiet control window has completed, the helper schedstat baseline
// must be the next attribution sample before Wave28 takes the final control
// cgroup snapshot. This leaves the control-after throttle counters as the
// freshest cgroup baseline immediately before START while excluding parked CPU
// time from the helper runtime delta.
func TestV30SchedstatBaselinePrecedesFinalControlAfterReadback(t *testing.T) {
	f := newV30Fixture(t)
	controlComplete := false
	startIssued := false

	f.executor.sleep = func(ctx context.Context, d time.Duration) error {
		if d != 200*time.Millisecond {
			t.Fatalf("control duration=%s want 200ms", d)
		}
		f.events = append(f.events, "CONTROL_SLEEP")
		controlComplete = true
		return ctx.Err()
	}
	f.child.onStart = func() {
		startIssued = true
	}

	originalRead := f.v28.cgroupObserver.readFile
	f.v28.cgroupObserver.readFile = func(filename string) ([]byte, error) {
		if controlComplete && !startIssued {
			f.events = append(f.events, "POST_CONTROL_CGROUP_READ")
		}
		return originalRead(filename)
	}

	auth, err := validateV30(t, f)
	if err != nil {
		t.Fatalf("ValidateRuntimeCPUThrottleInterventionAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("Wave30 authority invalid")
	}

	baseline := indexV30(f.events, "SCHEDSTAT_READ")
	postControlRead := indexV30(f.events, "POST_CONTROL_CGROUP_READ")
	start := indexV30(f.events, "START")
	if baseline < 0 || postControlRead < 0 || start < 0 {
		t.Fatalf("missing attribution events: %v", f.events)
	}
	if baseline >= postControlRead {
		t.Fatalf("schedstat baseline must precede final control-after cgroup readback: %v", f.events)
	}
	if postControlRead >= start {
		t.Fatalf("final control-after cgroup readback must precede START: %v", f.events)
	}
}
