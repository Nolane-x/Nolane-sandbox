package cube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const v30HelperPID = 992
const v30HelperStart = uint64(223344)
const v30NonceHex = "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"

type v30FakeChild struct {
	pid          int
	events       *[]string
	readyErr     error
	startErr     error
	burnDoneErr  error
	exitErr      error
	doneErr      error
	waitCode     int
	waitErr      error
	alive        bool
	killCount    int
	waitCount    int
	onStart      func()
	onBurnDone   func()
	onExit       func()
}

func (c *v30FakeChild) PID() int { return c.pid }
func (c *v30FakeChild) AwaitReady(_ context.Context, nonce string) error {
	*c.events = append(*c.events, "READY")
	if nonce != v30NonceHex {
		return errors.New("unexpected nonce")
	}
	return c.readyErr
}
func (c *v30FakeChild) Start(nonce string) error {
	*c.events = append(*c.events, "START")
	if nonce != v30NonceHex {
		return errors.New("unexpected nonce")
	}
	if c.onStart != nil {
		c.onStart()
	}
	return c.startErr
}
func (c *v30FakeChild) AwaitBurnDone(_ context.Context, nonce string) error {
	*c.events = append(*c.events, "BURN_DONE")
	if nonce != v30NonceHex {
		return errors.New("unexpected nonce")
	}
	if c.onBurnDone != nil {
		c.onBurnDone()
	}
	return c.burnDoneErr
}
func (c *v30FakeChild) Exit(nonce string) error {
	*c.events = append(*c.events, "EXIT")
	if nonce != v30NonceHex {
		return errors.New("unexpected nonce")
	}
	if c.onExit != nil {
		c.onExit()
	}
	return c.exitErr
}
func (c *v30FakeChild) AwaitDone(_ context.Context, nonce string) error {
	*c.events = append(*c.events, "DONE")
	if nonce != v30NonceHex {
		return errors.New("unexpected nonce")
	}
	return c.doneErr
}
func (c *v30FakeChild) Wait() (int, error) {
	c.waitCount++
	*c.events = append(*c.events, "WAIT")
	c.alive = false
	return c.waitCode, c.waitErr
}
func (c *v30FakeChild) Kill() error {
	c.killCount++
	*c.events = append(*c.events, "KILL")
	c.alive = false
	return nil
}
func (c *v30FakeChild) Alive() bool { return c.alive }

type v30Fixture struct {
	v28          v28Fixture
	executor     *RuntimeCPUInterventionExecutor
	child        *v30FakeChild
	events       []string
	procStart    uint64
	schedBefore  uint64
	schedAfter   uint64
	schedCurrent uint64
	clockStep    int
	targetProcs  string
}

func newV30Fixture(t *testing.T) *v30Fixture {
	t.Helper()
	f := &v30Fixture{
		v28:          newV28Fixture(t),
		procStart:    v30HelperStart,
		schedBefore:  1_000_000,
		schedAfter:   31_000_000,
		schedCurrent: 1_000_000,
		targetProcs:  "/v28-test-cgroup/cubes/sandbox-v26/cgroup.procs",
	}
	f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.max"] = "25000 100000\n"
	f.child = &v30FakeChild{pid: v30HelperPID, events: &f.events, waitCode: 0, alive: true}
	f.child.onBurnDone = func() {
		f.schedCurrent = f.schedAfter
		f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = "usage_usec 1200\nnr_periods 16\nnr_throttled 4\nthrottled_usec 101\n"
	}
	f.executor = newRuntimeCPUInterventionExecutorForTest(runtimeCPUInterventionExecutorTestConfig{
		Root:             "/v28-test-cgroup",
		Executable:       "/proc/self/exe",
		ExecutableDigest: strings.Repeat("4b", 32),
		NonceRaw:         bytes.Repeat([]byte{0xcd}, 32),
		Launch: func(_ context.Context, executable, nonce string, burnMicros uint64) (runtimeCPUInterventionChild, error) {
			f.events = append(f.events, "LAUNCH")
			if executable != "/proc/self/exe" || nonce != v30NonceHex || burnMicros != 400000 {
				return nil, fmt.Errorf("unexpected launch executable=%q nonce=%q burn=%d", executable, nonce, burnMicros)
			}
			return f.child, nil
		},
		ReadFile: func(filename string) ([]byte, error) {
			switch {
			case filename == fmt.Sprintf("/proc/%d/stat", v30HelperPID):
				f.events = append(f.events, "STARTTIME_READ")
				return []byte(fmt.Sprintf("%d (nolane cpu helper) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 %d 20 21\n", v30HelperPID, f.procStart)), nil
			case filename == fmt.Sprintf("/proc/%d/schedstat", v30HelperPID):
				f.events = append(f.events, "SCHEDSTAT_READ")
				return []byte(fmt.Sprintf("%d 1 2\n", f.schedCurrent)), nil
			}
			value, ok := f.v28.files[filename]
			if !ok {
				return nil, fmt.Errorf("missing fixture path %s", filename)
			}
			if filename == f.targetProcs {
				f.events = append(f.events, "MEMBERSHIP_READ")
			}
			return []byte(value), nil
		},
		WriteFile: func(filename string, data []byte) error {
			if filename != f.targetProcs || string(data) != "992\n" {
				return fmt.Errorf("unexpected write %s=%q", filename, data)
			}
			f.events = append(f.events, "ATTACH")
			f.v28.files[f.targetProcs] = "777\n992\n"
			return nil
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			f.events = append(f.events, "CONTROL_SLEEP")
			if d != 200*time.Millisecond {
				return fmt.Errorf("control duration=%s want 200ms", d)
			}
			return ctx.Err()
		},
		Clock: func() time.Time {
			f.clockStep++
			return time.Date(2026, 9, 8, 15, 30, f.clockStep, 0, time.UTC)
		},
	})
	return f
}

func validateV30(t *testing.T, f *v30Fixture) (RuntimeCPUThrottleInterventionAuthority, error) {
	t.Helper()
	return ValidateRuntimeCPUThrottleInterventionAuthority(
		context.Background(),
		f.v28.bridge.controller,
		f.v28.bridge.realization,
		f.v28.runtimeAuth,
		f.v28.bridge.epochObserver,
		f.v28.bridge.client,
		f.v28.runtimeObserver,
		f.v28.cgroupObserver,
		f.executor,
	)
}

func indexV30(events []string, needle string) int {
	for i, event := range events {
		if event == needle {
			return i
		}
	}
	return -1
}

func TestRuntimeCPUThrottleInterventionAuthorityValidPath(t *testing.T) {
	f := newV30Fixture(t)
	auth, err := validateV30(t, f)
	if err != nil {
		t.Fatalf("ValidateRuntimeCPUThrottleInterventionAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("Wave30 authority invalid")
	}
	digest, ok := auth.Digest()
	if !ok || !strings.HasPrefix(digest, "runtime-cpu-throttle-intervention-v30:") || len(strings.TrimPrefix(digest, "runtime-cpu-throttle-intervention-v30:")) != 64 {
		t.Fatalf("Digest=(%q,%v)", digest, ok)
	}
	snapshot, ok := auth.Snapshot()
	if !ok {
		t.Fatal("missing Wave30 snapshot")
	}
	if snapshot.CPUQuotaMicros != 25000 || snapshot.CPUPeriodMicros != 100000 || snapshot.HelperPID != v30HelperPID || snapshot.HelperStartTimeTicks != v30HelperStart || snapshot.ExitCode != 0 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.ControlBeforeNrThrottled != 3 || snapshot.ControlAfterNrThrottled != 3 || snapshot.PressureAfterNrThrottled != 4 {
		t.Fatalf("unexpected throttle counters: %+v", snapshot)
	}
	if snapshot.ControlBeforeThrottledUsec != 77 || snapshot.ControlAfterThrottledUsec != 77 || snapshot.PressureAfterThrottledUsec != 101 {
		t.Fatalf("unexpected throttled_usec counters: %+v", snapshot)
	}
	if snapshot.HelperSchedstatAfterNS-snapshot.HelperSchedstatBeforeNS < uint64(snapshot.CPUQuotaMicros)*1000 {
		t.Fatalf("helper runtime delta too small: %+v", snapshot)
	}
	if indexV30(f.events, "START") <= indexV30(f.events, "CONTROL_SLEEP") {
		t.Fatalf("START occurred before control completed: %v", f.events)
	}
	if indexV30(f.events, "EXIT") <= indexV30(f.events, "BURN_DONE") {
		t.Fatalf("EXIT occurred before BURN_DONE evidence: %v", f.events)
	}
	if f.child.waitCount != 1 || f.child.killCount != 0 {
		t.Fatalf("wait=%d kill=%d events=%v", f.child.waitCount, f.child.killCount, f.events)
	}
}

func TestRuntimeCPUThrottleInterventionAuthorityDeterministicDigest(t *testing.T) {
	f := newV30Fixture(t)
	auth, err := validateV30(t, f)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := auth.Snapshot()
	if !ok {
		t.Fatal("missing snapshot")
	}
	d1, err := deriveV30AuthorityDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := deriveV30AuthorityDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := auth.Digest()
	if !ok || d1 != d2 || d1 != actual {
		t.Fatalf("nondeterministic digest d1=%q d2=%q actual=%q", d1, d2, actual)
	}
}

func TestRuntimeCPUThrottleInterventionAuthorityZeroAndJSONInvalid(t *testing.T) {
	var zero RuntimeCPUThrottleInterventionAuthority
	if zero.Valid() {
		t.Fatal("zero Wave30 authority valid")
	}
	if _, ok := zero.Digest(); ok {
		t.Fatal("zero Wave30 authority exposed digest")
	}
	if _, ok := zero.Snapshot(); ok {
		t.Fatal("zero Wave30 authority exposed snapshot")
	}

	f := newV30Fixture(t)
	auth, err := validateV30(t, f)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeCPUThrottleInterventionAuthority
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Valid() {
		t.Fatal("JSON restored Wave30 authority")
	}
}

func TestRuntimeCPUThrottleInterventionUnavailableForQuotaAtLeastPeriod(t *testing.T) {
	f := newV30Fixture(t)
	f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.max"] = "100000 100000\n"
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrRuntimeCPUInterventionUnavailable) {
		t.Fatalf("error=%v want ErrRuntimeCPUInterventionUnavailable", err)
	}
	if indexV30(f.events, "LAUNCH") >= 0 {
		t.Fatalf("unprovable tuple launched helper: %v", f.events)
	}
}

func TestRuntimeCPUThrottleInterventionControlThrottleFailsClosed(t *testing.T) {
	f := newV30Fixture(t)
	f.executor.sleep = func(context.Context, time.Duration) error {
		f.events = append(f.events, "CONTROL_SLEEP")
		f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = "usage_usec 950\nnr_periods 13\nnr_throttled 4\nthrottled_usec 80\n"
		return nil
	}
	_, err := validateV30(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
		t.Fatalf("error=%v want invalid Wave30 authority", err)
	}
	if indexV30(f.events, "START") >= 0 {
		t.Fatalf("START sent despite background throttle: %v", f.events)
	}
	if f.child.waitCount != 1 {
		t.Fatalf("child not reaped: wait=%d events=%v", f.child.waitCount, f.events)
	}
}

func TestRuntimeCPUThrottleInterventionRequiresBothThrottleDeltas(t *testing.T) {
	cases := map[string]string{
		"nr_throttled unchanged": "usage_usec 1200\nnr_periods 16\nnr_throttled 3\nthrottled_usec 101\n",
		"throttled_usec unchanged": "usage_usec 1200\nnr_periods 16\nnr_throttled 4\nthrottled_usec 77\n",
	}
	for name, cpuStat := range cases {
		t.Run(name, func(t *testing.T) {
			f := newV30Fixture(t)
			f.child.onBurnDone = func() {
				f.schedCurrent = f.schedAfter
				f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = cpuStat
			}
			_, err := validateV30(t, f)
			if !errors.Is(err, ErrInvalidRuntimeCPUThrottleInterventionAuthority) {
				t.Fatalf("error=%v want invalid Wave30 authority", err)
			}
			if indexV30(f.events, "EXIT") >= 0 {
				t.Fatalf("EXIT sent before proof failure: %v", f.events)
			}
			if f.child.waitCount != 1 {
				t.Fatalf("child not reaped: wait=%d", f.child.waitCount)
			}
		})
	}
}

func TestRuntimeCPUThrottleInterventionRequiresHelperSchedstatDelta(t *testing.T) {
	f := newV30Fixture(t)
	f.child.onBurnDone = func() {
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

func TestRuntimeCPUThrottleInterventionPIDReuseFailsClosed(t *testing.T) {
	f := newV30Fixture(t)
	f.child.onBurnDone = func() {
		f.schedCurrent = f.schedAfter
		f.procStart++
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

func TestRuntimeCPUThrottleInterventionCancellationReapsChild(t *testing.T) {
	f := newV30Fixture(t)
	f.child.burnDoneErr = context.Canceled
	_, err := validateV30(t, f)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context.Canceled", err)
	}
	if f.child.waitCount != 1 {
		t.Fatalf("child not reaped: wait=%d events=%v", f.child.waitCount, f.events)
	}
}
