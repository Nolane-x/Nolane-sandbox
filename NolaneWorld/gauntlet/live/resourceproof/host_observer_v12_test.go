package resourceproof

import (
	"context"
	"errors"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

type fakeHostFiles map[string]string

func (f fakeHostFiles) ReadFile(path string) ([]byte, error) {
	value, ok := f[path]
	if !ok {
		return nil, errors.New("missing host file")
	}
	return []byte(value), nil
}

type fakeHostPressure struct {
	cpuErr       error
	memoryStatus HostTaskStatus
	memoryErr    error
}

func (f fakeHostPressure) RunCPU(context.Context) error { return f.cpuErr }
func (f fakeHostPressure) RunMemory(context.Context, uint64) (HostTaskStatus, error) {
	return f.memoryStatus, f.memoryErr
}

func TestHostObserverV2BuildsTrustedCPUAndMemoryProof(t *testing.T) {
	root := "/private/cgroup/sandbox-secret-sentinel"
	files := fakeHostFiles{
		root + "/cpu.max":       "50000 100000\n",
		root + "/cpu.stat":      "nr_throttled 10\nthrottled_usec 1000\n",
		root + "/memory.max":    "67108864\n",
		root + "/memory.events": "oom 2\noom_kill 2\n",
	}
	observer := newHostResourceObserver(HostObserverConfig{
		Version:    HostCgroupV2,
		CgroupRoot: root,
		Files:      files,
		Pressure: fakeHostPressure{memoryStatus: HostTaskStatus{ExitCode: 137, Reason: "OOMKilled"}},
	})

	// Mutate counters only after the corresponding pressure probe runs.
	observer.afterCPU = func() {
		files[root+"/cpu.stat"] = "nr_throttled 14\nthrottled_usec 1750\n"
	}
	observer.afterMemory = func() {
		files[root+"/memory.events"] = "oom 3\noom_kill 3\n"
	}

	trusted, err := observer.observe(context.Background(), live.ModeRequireLive, validBinding(), HostRequestedLimits{
		CPUQuotaMicros: 50_000,
		CPUPeriodMicros: 100_000,
		MemoryBytes: 64 << 20,
		MemoryAttemptBytes: 96 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	report := trusted.Report()
	if !report.Approved || report.Status != live.StatusLivePass {
		t.Fatalf("v2 host observation did not produce trusted pass: %+v", report)
	}
	if err := VerifyTrustedReport(trusted); err != nil {
		t.Fatalf("trusted report rejected: %v", err)
	}
	encoded, err := MarshalTrustedReport(trusted)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), root) || strings.Contains(string(encoded), "sandbox-secret-sentinel") {
		t.Fatalf("private cgroup locator leaked into evidence: %s", encoded)
	}
}

func TestHostObserverV1NormalizesEquivalentProof(t *testing.T) {
	root := "/private/cgroup-v1"
	files := fakeHostFiles{
		root + "/cpu.cfs_quota_us":      "50000\n",
		root + "/cpu.cfs_period_us":     "100000\n",
		root + "/cpu.stat":              "nr_throttled 4\nthrottled_time 2000000\n",
		root + "/memory.limit_in_bytes": "67108864\n",
		root + "/memory.failcnt":        "7\n",
	}
	observer := newHostResourceObserver(HostObserverConfig{Version: HostCgroupV1, CgroupRoot: root, Files: files, Pressure: fakeHostPressure{memoryStatus: HostTaskStatus{ExitCode: 137, Reason: "OOMKilled"}}})
	observer.afterCPU = func() { files[root+"/cpu.stat"] = "nr_throttled 6\nthrottled_time 5000000\n" }
	observer.afterMemory = func() { files[root+"/memory.failcnt"] = "8\n" }
	trusted, err := observer.observe(context.Background(), live.ModeRequireLive, validBinding(), HostRequestedLimits{CPUQuotaMicros: 50_000, CPUPeriodMicros: 100_000, MemoryBytes: 64 << 20, MemoryAttemptBytes: 96 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if report := trusted.Report(); !report.Approved || report.CPU.Observation.ThrottledUsecBefore != 2000 || report.CPU.Observation.ThrottledUsecAfter != 5000 || report.Memory.Observation.OOMEventsAfter <= report.Memory.Observation.OOMEventsBefore {
		t.Fatalf("v1 observations were not normalized causally: %+v", report)
	}
}

func TestHostObserverRejectsVoluntary137AndMissingThrottleDelta(t *testing.T) {
	root := "/cg"
	files := fakeHostFiles{root + "/cpu.max": "50000 100000\n", root + "/cpu.stat": "nr_throttled 1\nthrottled_usec 10\n", root + "/memory.max": "67108864\n", root + "/memory.events": "oom_kill 1\n"}
	observer := newHostResourceObserver(HostObserverConfig{Version: HostCgroupV2, CgroupRoot: root, Files: files, Pressure: fakeHostPressure{memoryStatus: HostTaskStatus{ExitCode: 137, Reason: "Exited"}}})
	observer.afterMemory = func() { files[root+"/memory.events"] = "oom_kill 2\n" }
	trusted, err := observer.observe(context.Background(), live.ModeRequireLive, validBinding(), HostRequestedLimits{CPUQuotaMicros: 50_000, CPUPeriodMicros: 100_000, MemoryBytes: 64 << 20, MemoryAttemptBytes: 96 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if report := trusted.Report(); report.Approved || report.Status == live.StatusLivePass {
		t.Fatalf("voluntary 137 / missing throttle delta manufactured proof: %+v", report)
	}
}

func TestHostObserverUnavailableIsNotPass(t *testing.T) {
	observer := newHostResourceObserver(HostObserverConfig{Version: HostCgroupV2, CgroupRoot: "/missing", Files: fakeHostFiles{}, Pressure: fakeHostPressure{}})
	trusted, err := observer.observe(context.Background(), live.ModeProbe, validBinding(), HostRequestedLimits{CPUQuotaMicros: 50_000, CPUPeriodMicros: 100_000, MemoryBytes: 64 << 20, MemoryAttemptBytes: 96 << 20})
	if err == nil {
		t.Fatal("missing live host files did not surface observer error")
	}
	if report := trusted.Report(); report.Approved || report.Status == live.StatusLivePass {
		t.Fatalf("unavailable host observer became PASS: %+v", report)
	}
}

func TestHostObserverRejectsMalformedAndUnlimitedBoundedLimits(t *testing.T) {
	tests := []struct {
		name    string
		version HostCgroupVersion
		files   fakeHostFiles
	}{
		{
			name:    "v2 unlimited cpu",
			version: HostCgroupV2,
			files: fakeHostFiles{
				"/cg/cpu.max":       "max 100000\n",
				"/cg/cpu.stat":      "nr_throttled 1\nthrottled_usec 10\n",
				"/cg/memory.max":    "67108864\n",
				"/cg/memory.events": "oom_kill 1\n",
			},
		},
		{
			name:    "v2 unlimited memory",
			version: HostCgroupV2,
			files: fakeHostFiles{
				"/cg/cpu.max":       "50000 100000\n",
				"/cg/cpu.stat":      "nr_throttled 1\nthrottled_usec 10\n",
				"/cg/memory.max":    "max\n",
				"/cg/memory.events": "oom_kill 1\n",
			},
		},
		{
			name:    "v1 unlimited cpu",
			version: HostCgroupV1,
			files: fakeHostFiles{
				"/cg/cpu.cfs_quota_us":      "-1\n",
				"/cg/cpu.cfs_period_us":     "100000\n",
				"/cg/cpu.stat":              "nr_throttled 1\nthrottled_time 1000\n",
				"/cg/memory.limit_in_bytes": "67108864\n",
				"/cg/memory.failcnt":        "1\n",
			},
		},
		{
			name:    "v1 unlimited memory sentinel",
			version: HostCgroupV1,
			files: fakeHostFiles{
				"/cg/cpu.cfs_quota_us":      "50000\n",
				"/cg/cpu.cfs_period_us":     "100000\n",
				"/cg/cpu.stat":              "nr_throttled 1\nthrottled_time 1000\n",
				"/cg/memory.limit_in_bytes": "9223372036854771712\n",
				"/cg/memory.failcnt":        "1\n",
			},
		},
		{
			name:    "v2 malformed quota",
			version: HostCgroupV2,
			files: fakeHostFiles{
				"/cg/cpu.max":       "not-a-number 100000\n",
				"/cg/cpu.stat":      "nr_throttled 1\nthrottled_usec 10\n",
				"/cg/memory.max":    "67108864\n",
				"/cg/memory.events": "oom_kill 1\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer := newHostResourceObserver(HostObserverConfig{Version: tt.version, CgroupRoot: "/cg", Files: tt.files, Pressure: fakeHostPressure{memoryStatus: HostTaskStatus{ExitCode: 137, Reason: "OOMKilled"}}})
			trusted, err := observer.observe(context.Background(), live.ModeRequireLive, validBinding(), HostRequestedLimits{CPUQuotaMicros: 50_000, CPUPeriodMicros: 100_000, MemoryBytes: 64 << 20, MemoryAttemptBytes: 96 << 20})
			if err == nil {
				t.Fatal("malformed or unlimited bounded limit did not fail closed")
			}
			if report := trusted.Report(); report.Approved || report.Status == live.StatusLivePass {
				t.Fatalf("malformed or unlimited observation became PASS: %+v", report)
			}
		})
	}
}

func TestHostObserverCarriesEffectiveReadbackMismatchIntoV11Verifier(t *testing.T) {
	root := "/cg"
	files := fakeHostFiles{
		root + "/cpu.max":       "60000 100000\n",
		root + "/cpu.stat":      "nr_throttled 1\nthrottled_usec 10\n",
		root + "/memory.max":    "67108864\n",
		root + "/memory.events": "oom_kill 1\n",
	}
	observer := newHostResourceObserver(HostObserverConfig{Version: HostCgroupV2, CgroupRoot: root, Files: files, Pressure: fakeHostPressure{memoryStatus: HostTaskStatus{ExitCode: 137, Reason: "OOMKilled"}}})
	observer.afterCPU = func() { files[root+"/cpu.stat"] = "nr_throttled 2\nthrottled_usec 20\n" }
	observer.afterMemory = func() { files[root+"/memory.events"] = "oom_kill 2\n" }
	trusted, err := observer.observe(context.Background(), live.ModeRequireLive, validBinding(), HostRequestedLimits{CPUQuotaMicros: 50_000, CPUPeriodMicros: 100_000, MemoryBytes: 64 << 20, MemoryAttemptBytes: 96 << 20})
	if err != nil {
		t.Fatal(err)
	}
	report := trusted.Report()
	if report.Status != live.StatusLiveFail || report.Reason != ReasonCPULimitMismatch || report.CPU.Observation.EffectiveQuotaMicros != 60_000 {
		t.Fatalf("effective readback mismatch was not delegated to v11 verifier: %+v", report)
	}
}

func TestHostObserverPressureFailureIsUnavailableNotPass(t *testing.T) {
	root := "/cg"
	files := fakeHostFiles{
		root + "/cpu.max":       "50000 100000\n",
		root + "/cpu.stat":      "nr_throttled 1\nthrottled_usec 10\n",
		root + "/memory.max":    "67108864\n",
		root + "/memory.events": "oom_kill 1\n",
	}
	observer := newHostResourceObserver(HostObserverConfig{Version: HostCgroupV2, CgroupRoot: root, Files: files, Pressure: fakeHostPressure{cpuErr: errors.New("pressure unavailable")}})
	trusted, err := observer.observe(context.Background(), live.ModeRequireLive, validBinding(), HostRequestedLimits{CPUQuotaMicros: 50_000, CPUPeriodMicros: 100_000, MemoryBytes: 64 << 20, MemoryAttemptBytes: 96 << 20})
	if err == nil {
		t.Fatal("CPU pressure failure did not surface observer error")
	}
	if report := trusted.Report(); report.Approved || report.Status == live.StatusLivePass {
		t.Fatalf("pressure failure became PASS: %+v", report)
	}
}
