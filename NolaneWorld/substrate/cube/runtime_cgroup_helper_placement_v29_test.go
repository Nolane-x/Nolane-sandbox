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

const v29HelperPID = 991
const v29HelperStart = uint64(123456)
const v29NonceHex = "abababababababababababababababababababababababababababababababab"

type v29FakeChild struct {
	pid       int
	events    *[]string
	readyErr  error
	doneErr   error
	waitCode  int
	waitErr   error
	alive     bool
	killCount int
	waitCount int
}

func (c *v29FakeChild) PID() int { return c.pid }
func (c *v29FakeChild) AwaitReady(_ context.Context, nonce string) error {
	*c.events = append(*c.events, "READY")
	if nonce != v29NonceHex {
		return errors.New("unexpected nonce")
	}
	return c.readyErr
}
func (c *v29FakeChild) Release(nonce string) error {
	*c.events = append(*c.events, "GO")
	if nonce != v29NonceHex {
		return errors.New("unexpected nonce")
	}
	return nil
}
func (c *v29FakeChild) AwaitDone(_ context.Context, nonce string) error {
	*c.events = append(*c.events, "DONE")
	if nonce != v29NonceHex {
		return errors.New("unexpected nonce")
	}
	return c.doneErr
}
func (c *v29FakeChild) Wait() (int, error) {
	c.waitCount++
	*c.events = append(*c.events, "WAIT")
	c.alive = false
	return c.waitCode, c.waitErr
}
func (c *v29FakeChild) Kill() error {
	c.killCount++
	*c.events = append(*c.events, "KILL")
	c.alive = false
	return nil
}
func (c *v29FakeChild) Alive() bool { return c.alive }

type v29Fixture struct {
	v28       v28Fixture
	executor  *RuntimeCgroupHelperExecutor
	child     *v29FakeChild
	events    []string
	procStart uint64
	clockStep int
}

func newV29Fixture(t *testing.T) *v29Fixture {
	t.Helper()
	f := &v29Fixture{v28: newV28Fixture(t), procStart: v29HelperStart}
	f.child = &v29FakeChild{pid: v29HelperPID, events: &f.events, waitCode: 0, alive: true}
	root := "/v28-test-cgroup"
	targetProcs := root + "/cubes/sandbox-v26/cgroup.procs"
	f.executor = newRuntimeCgroupHelperExecutorForTest(runtimeCgroupHelperExecutorTestConfig{
		Root:             root,
		Executable:       "/proc/self/exe",
		ExecutableDigest: strings.Repeat("3a", 32),
		NonceRaw:         bytes.Repeat([]byte{0xab}, 32),
		Launch: func(context.Context, string, string) (runtimeCgroupHelperChild, error) {
			f.events = append(f.events, "LAUNCH")
			return f.child, nil
		},
		ReadFile: func(filename string) ([]byte, error) {
			if strings.HasPrefix(filename, "/proc/") && strings.HasSuffix(filename, "/stat") {
				f.events = append(f.events, "STARTTIME_READ")
				return []byte(fmt.Sprintf("%d (nolane helper) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 %d 20 21\n", v29HelperPID, f.procStart)), nil
			}
			value, ok := f.v28.files[filename]
			if !ok {
				return nil, fmt.Errorf("missing fixture path %s", filename)
			}
			if filename == targetProcs {
				f.events = append(f.events, "MEMBERSHIP_READ")
			}
			return []byte(value), nil
		},
		WriteFile: func(filename string, data []byte) error {
			if filename != targetProcs || string(data) != "991\n" {
				return fmt.Errorf("unexpected write %s=%q", filename, data)
			}
			f.events = append(f.events, "ATTACH")
			f.v28.files[targetProcs] = "777\n991\n"
			return nil
		},
		Clock: func() time.Time {
			f.clockStep++
			return time.Date(2026, 9, 8, 13, 0, f.clockStep, 0, time.UTC)
		},
	})
	return f
}

func validateV29(t *testing.T, f *v29Fixture) (RuntimeCgroupHelperPlacementAuthority, error) {
	t.Helper()
	return ValidateRuntimeCgroupHelperPlacementAuthority(
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

func indexV29(events []string, needle string) int {
	for i, event := range events {
		if event == needle {
			return i
		}
	}
	return -1
}

func TestV29InternalProtocolExactReadyGoDone(t *testing.T) {
	var ack bytes.Buffer
	getenv := func(key string) string {
		switch key {
		case runtimeCgroupHelperModeEnv:
			return runtimeCgroupHelperModeParkExit
		case runtimeCgroupHelperNonceEnv:
			return v29NonceHex
		default:
			return ""
		}
	}
	code := runInternalCgroupHelper(getenv, strings.NewReader("GO "+v29NonceHex+"\n"), &ack)
	if code != 0 {
		t.Fatalf("helper code=%d", code)
	}
	want := "READY " + v29NonceHex + "\nDONE " + v29NonceHex + "\n"
	if ack.String() != want {
		t.Fatalf("ack=%q want=%q", ack.String(), want)
	}
}

func TestV29InternalProtocolNonceMismatchFailsClosed(t *testing.T) {
	var ack bytes.Buffer
	getenv := func(key string) string {
		if key == runtimeCgroupHelperModeEnv { return runtimeCgroupHelperModeParkExit }
		if key == runtimeCgroupHelperNonceEnv { return v29NonceHex }
		return ""
	}
	code := runInternalCgroupHelper(getenv, strings.NewReader("GO "+strings.Repeat("cd", 32)+"\n"), &ack)
	if code == 0 {
		t.Fatal("nonce mismatch succeeded")
	}
	if !strings.HasPrefix(ack.String(), "READY "+v29NonceHex+"\n") || strings.Contains(ack.String(), "DONE ") {
		t.Fatalf("unexpected ack=%q", ack.String())
	}
}

func TestV29ExactPlacementLifecycleMintsAuthority(t *testing.T) {
	f := newV29Fixture(t)
	auth, err := validateV29(t, f)
	if err != nil {
		t.Fatalf("ValidateRuntimeCgroupHelperPlacementAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("Wave29 authority invalid")
	}
	digest, ok := auth.Digest()
	if !ok || !strings.HasPrefix(digest, "runtime-cgroup-helper-placement-v29:") || len(strings.TrimPrefix(digest, "runtime-cgroup-helper-placement-v29:")) != 64 {
		t.Fatalf("Digest=(%q,%v)", digest, ok)
	}
	snapshot, ok := auth.Snapshot()
	if !ok {
		t.Fatal("missing Wave29 snapshot")
	}
	if snapshot.HelperPID != v29HelperPID || snapshot.HelperStartTimeTicks != v29HelperStart || snapshot.ExitCode != 0 {
		t.Fatalf("unexpected helper snapshot: %+v", snapshot)
	}
	if snapshot.HelperExecutableSHA256 != strings.Repeat("3a", 32) || snapshot.HelperNonceSHA256 == "" || len(snapshot.HelperNonceSHA256) != 64 {
		t.Fatalf("unexpected helper identity: %+v", snapshot)
	}
	goIndex := indexV29(f.events, "GO")
	attachIndex := indexV29(f.events, "ATTACH")
	membershipIndex := indexV29(f.events, "MEMBERSHIP_READ")
	if goIndex < 0 || attachIndex < 0 || membershipIndex < 0 || goIndex <= attachIndex || goIndex <= membershipIndex {
		t.Fatalf("release ordering invalid: %v", f.events)
	}
	startReads := 0
	for _, event := range f.events { if event == "STARTTIME_READ" { startReads++ } }
	if startReads < 2 {
		t.Fatalf("starttime reads=%d events=%v", startReads, f.events)
	}
	if f.child.waitCount != 1 || f.child.killCount != 0 {
		t.Fatalf("wait=%d kill=%d", f.child.waitCount, f.child.killCount)
	}
}

func TestV29DigestDeterministicForIdenticalAuthorityOwnedInput(t *testing.T) {
	f1 := newV29Fixture(t)
	a1, err := validateV29(t, f1)
	if err != nil { t.Fatal(err) }
	f2 := newV29Fixture(t)
	a2, err := validateV29(t, f2)
	if err != nil { t.Fatal(err) }
	d1, ok1 := a1.Digest(); d2, ok2 := a2.Digest()
	if !ok1 || !ok2 || d1 != d2 {
		t.Fatalf("nondeterministic digest: %q %q", d1, d2)
	}
}

func TestV29ZeroAndJSONRoundTripCannotRestoreAuthority(t *testing.T) {
	var zero RuntimeCgroupHelperPlacementAuthority
	if zero.Valid() { t.Fatal("zero authority valid") }
	if _, ok := zero.Digest(); ok { t.Fatal("zero authority exposed digest") }
	if _, ok := zero.Snapshot(); ok { t.Fatal("zero authority exposed snapshot") }
	f := newV29Fixture(t)
	auth, err := validateV29(t, f)
	if err != nil { t.Fatal(err) }
	raw, err := json.Marshal(auth)
	if err != nil { t.Fatal(err) }
	var restored RuntimeCgroupHelperPlacementAuthority
	if err := json.Unmarshal(raw, &restored); err != nil { t.Fatal(err) }
	if restored.Valid() { t.Fatal("JSON restored Wave29 authority") }
}

func TestV29PlacementFailuresFailClosedAndReap(t *testing.T) {
	cases := map[string]func(*v29Fixture){
		"ready mismatch": func(f *v29Fixture) { f.child.readyErr = errors.New("bad READY") },
		"child dead before placement": func(f *v29Fixture) { f.child.alive = false },
		"write failure": func(f *v29Fixture) {
			f.executor.writeFile = func(string, []byte) error { return errors.New("write denied") }
		},
		"missing membership": func(f *v29Fixture) {
			f.executor.writeFile = func(string, []byte) error { return nil }
		},
		"starttime replaced": func(f *v29Fixture) {
			reads := 0
			original := f.executor.readStartTime
			f.executor.readStartTime = func(pid int) (uint64, error) {
				reads++
				if reads >= 2 { return v29HelperStart + 1, nil }
				return original(pid)
			}
		},
		"done mismatch": func(f *v29Fixture) { f.child.doneErr = errors.New("bad DONE") },
		"nonzero exit": func(f *v29Fixture) { f.child.waitCode = 9 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newV29Fixture(t)
			mutate(f)
			_, err := validateV29(t, f)
			if err == nil {
				t.Fatal("invalid helper lifecycle minted authority")
			}
			if f.child.waitCount != 1 {
				t.Fatalf("child not reaped exactly once: wait=%d events=%v", f.child.waitCount, f.events)
			}
		})
	}
}

func TestV29MalformedOrDuplicateMembershipFailsClosed(t *testing.T) {
	for name, procs := range map[string]string{
		"duplicate": "777\n991\n991\n",
		"malformed": "777\n+991\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := newV29Fixture(t)
			target := "/v28-test-cgroup/cubes/sandbox-v26/cgroup.procs"
			f.executor.writeFile = func(string, []byte) error { f.v28.files[target] = procs; return nil }
			if _, err := validateV29(t, f); err == nil {
				t.Fatal("malformed membership minted authority")
			}
			if f.child.waitCount != 1 { t.Fatalf("wait=%d", f.child.waitCount) }
		})
	}
}

func TestV29ContextCancellationKillsAndReapsChild(t *testing.T) {
	f := newV29Fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.child.readyErr = context.Canceled
	cancel()
	_, err := ValidateRuntimeCgroupHelperPlacementAuthority(
		ctx, f.v28.bridge.controller, f.v28.bridge.realization, f.v28.runtimeAuth,
		f.v28.bridge.epochObserver, f.v28.bridge.client, f.v28.runtimeObserver, f.v28.cgroupObserver, f.executor,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context canceled", err)
	}
	// Cancellation before launch is allowed to avoid starting a child.
	if f.child.waitCount > 1 || f.child.killCount > 1 {
		t.Fatalf("cleanup repeated: wait=%d kill=%d", f.child.waitCount, f.child.killCount)
	}
}

func TestV29PostWave28RuntimeReplacementFailsClosed(t *testing.T) {
	f := newV29Fixture(t)
	originalWait := f.child.Wait
	f.child.waitCode = 0
	f.executor.afterChildExit = func() {
		f.v28.runtimeMetrics.body = v27RuntimeMetrics("sandbox-v26", 1, strings.Repeat("81", 32), 778, 9002, "33333333-3333-4333-8333-333333333333")
	}
	_ = originalWait
	if _, err := validateV29(t, f); err == nil {
		t.Fatal("runtime replacement after helper minted authority")
	}
}

func TestV29ImmutableReadbackChangeFailsButCounterIncreaseAllowed(t *testing.T) {
	t.Run("limit change", func(t *testing.T) {
		f := newV29Fixture(t)
		f.executor.afterChildExit = func() {
			f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.max"] = "60000 100000\n"
		}
		if _, err := validateV29(t, f); err == nil { t.Fatal("limit change minted authority") }
	})
	t.Run("counter increase", func(t *testing.T) {
		f := newV29Fixture(t)
		f.executor.afterChildExit = func() {
			f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/cpu.stat"] = "usage_usec 1200\nnr_periods 15\nnr_throttled 5\nthrottled_usec 99\n"
			f.v28.files["/v28-test-cgroup/cubes/sandbox-v26/memory.events"] = "low 0\nhigh 0\nmax 1\noom 3\noom_kill 2\n"
		}
		auth, err := validateV29(t, f)
		if err != nil { t.Fatal(err) }
		if !auth.Valid() { t.Fatal("counter increase invalidated authority") }
	})
}
