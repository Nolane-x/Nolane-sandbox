package cube

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestV30SchedstatParserCanonicalThreeFields(t *testing.T) {
	got, err := parseRuntimeCPUSchedstat([]byte("123456 7 8\n"))
	if err != nil || got != 123456 {
		t.Fatalf("parse got=%d err=%v", got, err)
	}
	for _, raw := range []string{
		"",
		"0 1 2\n",
		"01 1 2\n",
		"+1 1 2\n",
		"-1 1 2\n",
		"1 2\n",
		"1 2 3 4\n",
		"1  2 3\n",
		"1 2 x\n",
		"1 2 3\nextra\n",
	} {
		t.Run(fmt.Sprintf("%q", raw), func(t *testing.T) {
			if _, err := parseRuntimeCPUSchedstat([]byte(raw)); !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
				t.Fatalf("raw=%q err=%v, want invalid Wave30 authority", raw, err)
			}
		})
	}
}

func TestV30SchedstatDeltaMustReachOneQuotaQuantum(t *testing.T) {
	f := newV30Fixture(t)
	f.schedAfter = f.schedBefore + 24_999_999
	f.child.onBurnDone = func() {
		f.schedCurrent = f.schedAfter
		f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = "usage_usec 1200\nnr_periods 16\nnr_throttled 4\nthrottled_usec 101\n"
	}
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
		t.Fatalf("error=%v want invalid Wave30 authority", err)
	}
	if f.child.waitCount != 1 {
		t.Fatalf("child not reaped: wait=%d", f.child.waitCount)
	}
}

func TestV30HelperLeavingExactCgroupFailsClosed(t *testing.T) {
	f := newV30Fixture(t)
	f.child.onBurnDone = func() {
		f.schedCurrent = f.schedAfter
		f.v28.files[f.targetProcs] = "777\n"
		f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = "usage_usec 1200\nnr_periods 16\nnr_throttled 4\nthrottled_usec 101\n"
	}
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
		t.Fatalf("error=%v want invalid Wave30 authority", err)
	}
	if f.child.waitCount != 1 {
		t.Fatalf("child not reaped: wait=%d", f.child.waitCount)
	}
}

func TestV30LimitDriftDuringInterventionFailsClosed(t *testing.T) {
	f := newV30Fixture(t)
	f.child.onBurnDone = func() {
		f.schedCurrent = f.schedAfter
		f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = "usage_usec 1200\nnr_periods 16\nnr_throttled 4\nthrottled_usec 101\n"
		f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.max"] = "26000 100000\n"
	}
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
		t.Fatalf("error=%v want invalid Wave30 authority", err)
	}
}

func TestV30ThrottleCounterRollbackFailsClosed(t *testing.T) {
	f := newV30Fixture(t)
	f.child.onBurnDone = func() {
		f.schedCurrent = f.schedAfter
		f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = "usage_usec 1200\nnr_periods 16\nnr_throttled 2\nthrottled_usec 70\n"
	}
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
		t.Fatalf("error=%v want invalid Wave30 authority", err)
	}
}

func TestV30NonZeroExitFailsClosed(t *testing.T) {
	f := newV30Fixture(t)
	f.child.waitCode = 7
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
		t.Fatalf("error=%v want invalid Wave30 authority", err)
	}
	if f.child.waitCount != 1 {
		t.Fatalf("child not reaped: wait=%d", f.child.waitCount)
	}
}

func TestV30RunningChildExecutableReplacementFailsClosed(t *testing.T) {
	f := newV30Fixture(t)
	reads := 0
	f.executor.readHelperExecutableDigest = func(int) (string, error) {
		reads++
		if reads >= 2 {
			return strings.Repeat("5c", 32), nil
		}
		return strings.Repeat("4b", 32), nil
	}
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
		t.Fatalf("error=%v want invalid Wave30 authority", err)
	}
	if f.child.waitCount != 1 {
		t.Fatalf("child not reaped: wait=%d", f.child.waitCount)
	}
}
