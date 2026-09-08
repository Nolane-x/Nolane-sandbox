package cube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type v28Fixture struct {
	bridge          v26BridgeFixture
	runtimeObserver *RuntimeRealizationObserver
	runtimeMetrics  *v27MutableMetrics
	runtimeProof    RuntimeRealizationProof
	runtimeAuth     RealmResourceRuntimeAuthority
	files           map[string]string
	cgroupObserver  *RuntimeCgroupReadbackObserver
}

func newV28Fixture(t *testing.T) v28Fixture {
	t.Helper()
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	runtimeObserver, runtimeMetrics, runtimeProof := observeV27RuntimeForWave26(
		t,
		"sandbox-v26",
		strings.Repeat("81", 32),
	)
	runtimeAuth, err := validateV27(t, f, wave26, runtimeObserver, runtimeProof)
	if err != nil {
		t.Fatalf("mint Wave27 authority: %v", err)
	}

	const root = "/v28-test-cgroup"
	target := root + "/cubes/sandbox-v26"
	files := map[string]string{
		root + "/cgroup.controllers": targetControllersV28(),
		target + "/cgroup.procs":     "777\n",
		target + "/cpu.max":          "50000 100000\n",
		target + "/cpu.stat":         "usage_usec 900\nnr_periods 12\nnr_throttled 3\nthrottled_usec 77\n",
		target + "/memory.max":       "268435456\n",
		target + "/memory.events":    "low 0\nhigh 0\nmax 1\noom 2\noom_kill 1\n",
	}
	observer := newRuntimeCgroupReadbackObserverForTest(root, func(path string) ([]byte, error) {
		value, ok := files[path]
		if !ok {
			return nil, fmt.Errorf("missing fixture path %s", path)
		}
		return []byte(value), nil
	})
	return v28Fixture{
		bridge:          f,
		runtimeObserver: runtimeObserver,
		runtimeMetrics:  runtimeMetrics,
		runtimeProof:    runtimeProof,
		runtimeAuth:     runtimeAuth,
		files:           files,
		cgroupObserver:  observer,
	}
}

func targetControllersV28() string { return "cpu memory pids\n" }

func validateV28(t *testing.T, f v28Fixture) (RuntimeCgroupReadbackAuthority, error) {
	t.Helper()
	return ValidateRuntimeCgroupReadbackAuthority(
		context.Background(),
		f.bridge.controller,
		f.bridge.realization,
		f.runtimeAuth,
		f.bridge.epochObserver,
		f.bridge.client,
		f.runtimeObserver,
		f.cgroupObserver,
	)
}

func TestV28ExactFreshRuntimeAndCgroupV2ReadbackMintAuthority(t *testing.T) {
	f := newV28Fixture(t)
	auth, err := validateV28(t, f)
	if err != nil {
		t.Fatalf("ValidateRuntimeCgroupReadbackAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("Wave28 authority invalid")
	}

	runtimeDigest, ok := auth.RuntimeDigest()
	if !ok || !strings.HasPrefix(runtimeDigest, "runtime-realization-v27:") {
		t.Fatalf("RuntimeDigest=(%q,%v)", runtimeDigest, ok)
	}
	readbackDigest, ok := auth.ReadbackDigest()
	if !ok || !strings.HasPrefix(readbackDigest, "runtime-cgroup-readback-v28:") || len(strings.TrimPrefix(readbackDigest, "runtime-cgroup-readback-v28:")) != 64 {
		t.Fatalf("ReadbackDigest=(%q,%v)", readbackDigest, ok)
	}

	snapshot, ok := auth.Snapshot()
	if !ok {
		t.Fatal("missing Wave28 snapshot")
	}
	if snapshot.RuntimeDigest != runtimeDigest || snapshot.SandboxID != "sandbox-v26" || snapshot.Generation != 1 || snapshot.HostPID != 777 || snapshot.CGroupPath != "/cubes/sandbox-v26" {
		t.Fatalf("unexpected runtime snapshot: %+v", snapshot)
	}
	if snapshot.CPUQuotaMicros != 50000 || snapshot.CPUPeriodMicros != 100000 || snapshot.NrThrottled != 3 || snapshot.ThrottledUsec != 77 {
		t.Fatalf("unexpected CPU readback: %+v", snapshot)
	}
	if snapshot.MemoryLimitBytes != 268435456 || snapshot.OOMEvents != 2 || snapshot.OOMKillEvents != 1 {
		t.Fatalf("unexpected memory readback: %+v", snapshot)
	}
}

func TestV28ReadbackDigestDeterministicForSameAuthorityAndSnapshot(t *testing.T) {
	f := newV28Fixture(t)
	first, err := validateV28(t, f)
	if err != nil {
		t.Fatal(err)
	}
	second, err := validateV28(t, f)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest, ok1 := first.ReadbackDigest()
	secondDigest, ok2 := second.ReadbackDigest()
	if !ok1 || !ok2 || firstDigest != secondDigest {
		t.Fatalf("nondeterministic Wave28 digest: %q %q", firstDigest, secondDigest)
	}
}

func TestV28ExactHostPIDMembershipRequired(t *testing.T) {
	f := newV28Fixture(t)
	f.files["/v28-test-cgroup/cubes/sandbox-v26/cgroup.procs"] = "778\n"
	_, err := validateV28(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCgroupReadbackAuthority) {
		t.Fatalf("wrong PID membership error=%v, want ErrInvalidRuntimeCgroupReadbackAuthority", err)
	}
}

func TestV28HostPIDMembershipMustBracketReadback(t *testing.T) {
	f := newV28Fixture(t)
	originalRead := f.cgroupObserver.readFile
	procsReads := 0
	f.cgroupObserver.readFile = func(filename string) ([]byte, error) {
		if strings.HasSuffix(filename, "/cgroup.procs") {
			procsReads++
			if procsReads >= 2 {
				return []byte("778\n"), nil
			}
		}
		return originalRead(filename)
	}

	_, err := validateV28(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCgroupReadbackAuthority) {
		t.Fatalf("membership changed during readback error=%v, want ErrInvalidRuntimeCgroupReadbackAuthority", err)
	}
	if procsReads < 2 {
		t.Fatalf("cgroup.procs reads=%d, want at least 2 to bracket readback", procsReads)
	}
}

func TestV28UnlimitedCPUOrMemoryFailsClosed(t *testing.T) {
	cases := map[string][2]string{
		"cpu unlimited":    {"/v28-test-cgroup/cubes/sandbox-v26/cpu.max", "max 100000\n"},
		"memory unlimited": {"/v28-test-cgroup/cubes/sandbox-v26/memory.max", "max\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newV28Fixture(t)
			f.files[tc[0]] = tc[1]
			_, err := validateV28(t, f)
			if !errors.Is(err, ErrInvalidRuntimeCgroupReadbackAuthority) {
				t.Fatalf("unlimited readback error=%v, want ErrInvalidRuntimeCgroupReadbackAuthority", err)
			}
		})
	}
}

func TestV28MalformedOrDuplicateStatsFailClosed(t *testing.T) {
	cases := map[string]struct {
		path  string
		value string
	}{
		"duplicate cpu key": {
			path:  "/v28-test-cgroup/cubes/sandbox-v26/cpu.stat",
			value: "nr_throttled 3\nnr_throttled 4\nthrottled_usec 77\n",
		},
		"missing cpu field": {
			path:  "/v28-test-cgroup/cubes/sandbox-v26/cpu.stat",
			value: "nr_throttled 3\n",
		},
		"duplicate memory key": {
			path:  "/v28-test-cgroup/cubes/sandbox-v26/memory.events",
			value: "oom 2\noom 3\noom_kill 1\n",
		},
		"malformed pid": {
			path:  "/v28-test-cgroup/cubes/sandbox-v26/cgroup.procs",
			value: "777\n+778\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newV28Fixture(t)
			f.files[tc.path] = tc.value
			_, err := validateV28(t, f)
			if !errors.Is(err, ErrInvalidRuntimeCgroupReadbackAuthority) {
				t.Fatalf("malformed readback error=%v, want ErrInvalidRuntimeCgroupReadbackAuthority", err)
			}
		})
	}
}

func TestV28StaleRuntimeCannotMintReadbackAuthority(t *testing.T) {
	f := newV28Fixture(t)
	f.runtimeMetrics.body = v27RuntimeMetrics(
		"sandbox-v26",
		1,
		strings.Repeat("81", 32),
		778,
		9002,
		"33333333-3333-4333-8333-333333333333",
	)
	_, err := validateV28(t, f)
	if err == nil {
		t.Fatal("stale Wave27 runtime minted Wave28 readback authority")
	}
}

func TestV28ZeroAuthorityFailsClosed(t *testing.T) {
	var zero RuntimeCgroupReadbackAuthority
	if zero.Valid() {
		t.Fatal("zero Wave28 authority became valid")
	}
	if _, ok := zero.RuntimeDigest(); ok {
		t.Fatal("zero authority exposed runtime digest")
	}
	if _, ok := zero.ReadbackDigest(); ok {
		t.Fatal("zero authority exposed readback digest")
	}
	if _, ok := zero.Snapshot(); ok {
		t.Fatal("zero authority exposed snapshot")
	}
}
