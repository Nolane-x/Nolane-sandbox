package sandbox

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func hostProcStatFixture(pid uint32, comm, starttime string) string {
	// Fields after comm start at Linux stat field 3. Field 22 is index 19.
	rest := []string{"S"}
	for i := 4; i <= 21; i++ {
		rest = append(rest, "0")
	}
	rest = append(rest, starttime, "0", "0")
	return fmt.Sprintf("%d (%s) %s\n", pid, comm, strings.Join(rest, " "))
}

func TestHostProcessStatParserHandlesParenthesizedCommAndExactStartTime(t *testing.T) {
	got, err := parseHostProcessStatStartTime(hostProcStatFixture(4242, "cube shim ) worker (alpha)", "18446744073709551615"), 4242)
	if err != nil {
		t.Fatalf("parse stat: %v", err)
	}
	if got != math.MaxUint64 {
		t.Fatalf("starttime=%d want %d", got, uint64(math.MaxUint64))
	}
}

func TestHostProcessStatParserRejectsIdentityAmbiguity(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		pid  uint32
	}{
		{"pid mismatch", hostProcStatFixture(7, "shim", "99"), 8},
		{"zero starttime", hostProcStatFixture(7, "shim", "0"), 7},
		{"signed starttime", hostProcStatFixture(7, "shim", "+99"), 7},
		{"leading zero starttime", hostProcStatFixture(7, "shim", "099"), 7},
		{"missing close delimiter", "7 (shim S 0 0", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseHostProcessStatStartTime(tc.raw, tc.pid); err == nil {
				t.Fatal("expected fail-closed stat parse")
			}
		})
	}
}

func TestHostProcessCgroupMembershipRequiresExactStructuralPath(t *testing.T) {
	for _, raw := range []string{
		"0::/cube_sandbox_v1/42\n",
		"9:memory:/cube_sandbox_v1/42\n8:cpu,cpuacct:/cube_sandbox_v1/42\n",
	} {
		if err := validateHostProcessCgroupMembership(raw, "/cube_sandbox_v1/42"); err != nil {
			t.Fatalf("expected exact membership: %v", err)
		}
	}

	for _, raw := range []string{
		"0::/parent/cube_sandbox_v1/42\n",
		"0::/cube_sandbox_v1/420\n",
		"0::/other/42\n",
		"0::/cube_sandbox_v1/42/child\n",
	} {
		if err := validateHostProcessCgroupMembership(raw, "/cube_sandbox_v1/42"); err == nil {
			t.Fatalf("must reject fuzzy cgroup match %q", raw)
		}
	}
}

func TestHostProcessInspectorSandwichRejectsPIDReuse(t *testing.T) {
	placedAt := time.Date(2026, 9, 5, 6, 0, 0, 123456789, time.UTC)
	now := placedAt.Add(time.Millisecond)
	statReads := 0
	inspector := newHostProcessInspector(func(name string) ([]byte, error) {
		switch name {
		case "/proc/sys/kernel/random/boot_id":
			return []byte("11111111-2222-3333-8444-555555555555\n"), nil
		case "/proc/1234/stat":
			statReads++
			if statReads == 1 {
				return []byte(hostProcStatFixture(1234, "cube shim", "9001")), nil
			}
			return []byte(hostProcStatFixture(1234, "reused pid", "9002")), nil
		case "/proc/1234/cgroup":
			return []byte("0::/cube_sandbox_v1/42\n"), nil
		default:
			return nil, fmt.Errorf("unexpected read %s", name)
		}
	}, func() time.Time { return now })

	if _, err := inspector.CapturePlacement("sandbox-a", "/cube_sandbox_v1/42", 1234, placedAt); err == nil {
		t.Fatal("PID/starttime change across stat sandwich must fail closed")
	}
}

func TestHostProcessInspectorCapturesPIDReuseResistantPlacement(t *testing.T) {
	placedAt := time.Date(2026, 9, 5, 6, 0, 0, 123456789, time.UTC)
	observedAt := placedAt.Add(100 * time.Millisecond)
	inspector := newHostProcessInspector(func(name string) ([]byte, error) {
		switch name {
		case "/proc/sys/kernel/random/boot_id":
			return []byte("11111111-2222-3333-8444-555555555555\n"), nil
		case "/proc/1234/stat":
			return []byte(hostProcStatFixture(1234, "cube shim ) worker", "18446744073709551615")), nil
		case "/proc/1234/cgroup":
			return []byte("9:memory:/cube_sandbox_v1/42\n8:cpu,cpuacct:/cube_sandbox_v1/42\n"), nil
		default:
			return nil, fmt.Errorf("unexpected read %s", name)
		}
	}, func() time.Time { return observedAt })

	proof, err := inspector.CapturePlacement("sandbox-a", "/cube_sandbox_v1/42", 1234, placedAt)
	if err != nil {
		t.Fatalf("capture placement: %v", err)
	}
	if proof.SandboxID != "sandbox-a" || proof.CGroupPath != "/cube_sandbox_v1/42" || proof.HostPID != 1234 || proof.StartTimeTicks != math.MaxUint64 {
		t.Fatalf("unexpected proof: %+v", proof)
	}
	if proof.BootID != "11111111-2222-3333-8444-555555555555" || proof.Source != HostProcessPlacementSourceCubeBoxAddProc {
		t.Fatalf("unexpected provenance: %+v", proof)
	}
	if !proof.PlacedAt.Equal(placedAt) || !proof.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected timestamps: %+v", proof)
	}
}

func TestHostProcessPlacementBindsOnlyCurrentOpenGeneration(t *testing.T) {
	store := newTaskOutcomeProofStore()
	generation := store.BeginRealization("sandbox-a")
	if generation != 1 {
		t.Fatalf("generation=%d", generation)
	}
	token := store.BeginHostProcessPlacementCapture("sandbox-a")
	placedAt := time.Date(2026, 9, 5, 6, 0, 0, 0, time.UTC)
	proof := HostProcessPlacementProof{
		SandboxID: "sandbox-a", CGroupPath: "/cube_sandbox_v1/42",
		BootID: "11111111-2222-3333-8444-555555555555", HostPID: 1234, StartTimeTicks: 9001,
		PlacedAt: placedAt, ObservedAt: placedAt.Add(time.Millisecond), Source: HostProcessPlacementSourceCubeBoxAddProc,
	}
	binding, ok := store.CommitHostProcessPlacement(token, proof)
	if !ok {
		t.Fatal("current open generation should bind placement")
	}
	if binding.Generation != generation || binding.HostPID != proof.HostPID || binding.StartTimeTicks != proof.StartTimeTicks {
		t.Fatalf("unexpected binding: %+v", binding)
	}

	candidate := taskOutcomeCandidate{SandboxID: "sandbox-a", ExitCode: 137, ExitedAt: placedAt.Add(time.Second), Source: TaskOutcomeProofSourceWait}
	if _, err := store.Record(candidate); err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	lateToken := store.BeginHostProcessPlacementCapture("sandbox-a")
	late := proof
	late.ObservedAt = late.ObservedAt.Add(time.Second)
	if _, ok := store.CommitHostProcessPlacement(lateToken, late); ok {
		t.Fatal("closed generation must not accept late realization binding")
	}
}

func TestHostProcessStaleCaptureCannotCrossNewStartOrCreateFence(t *testing.T) {
	store := newTaskOutcomeProofStore()
	store.BeginRealization("sandbox-a")
	stale := store.BeginHostProcessPlacementCapture("sandbox-a")
	store.BeginRealization("sandbox-a")
	proof := HostProcessPlacementProof{
		SandboxID: "sandbox-a", CGroupPath: "/cube/1", BootID: "11111111-2222-3333-8444-555555555555",
		HostPID: 99, StartTimeTicks: 100, PlacedAt: time.Now().UTC(), ObservedAt: time.Now().UTC(), Source: HostProcessPlacementSourceCubeBoxAddProc,
	}
	if _, ok := store.CommitHostProcessPlacement(stale, proof); ok {
		t.Fatal("capture from prior generation crossed new Start")
	}

	stale = store.BeginHostProcessPlacementCapture("sandbox-a")
	store.Clear("sandbox-a")
	if _, ok := store.CommitHostProcessPlacement(stale, proof); ok {
		t.Fatal("capture from prior lifetime crossed Create fence")
	}
}

func TestControllerHostProcessVisitorPreservesMaxGeneration(t *testing.T) {
	store := newTaskOutcomeProofStore()
	store.generations["sandbox-a"] = math.MaxUint64 - 1
	controller := &controllerLocal{taskOutcomeProofs: store}
	generation := controller.beginTaskOutcomeRealization("sandbox-a")
	if generation != math.MaxUint64 {
		t.Fatalf("generation=%d", generation)
	}
	token := store.BeginHostProcessPlacementCapture("sandbox-a")
	now := time.Date(2026, 9, 5, 6, 0, 0, 987654321, time.UTC)
	_, ok := store.CommitHostProcessPlacement(token, HostProcessPlacementProof{
		SandboxID: "sandbox-a", CGroupPath: "/cube/1", BootID: "11111111-2222-3333-8444-555555555555",
		HostPID: math.MaxUint32, StartTimeTicks: math.MaxUint64, PlacedAt: now, ObservedAt: now, Source: HostProcessPlacementSourceCubeBoxAddProc,
	})
	if !ok {
		t.Fatal("commit binding")
	}
	var gotGeneration uint64
	controller.VisitHostProcessIdentityProofs(func(_ string, generation uint64, _ uint32, _ uint64, _, _, _, _ string, _, _ time.Time) {
		gotGeneration = generation
	})
	if gotGeneration != math.MaxUint64 {
		t.Fatalf("visitor generation=%d", gotGeneration)
	}

	// Compile-time contract for CubeBox's package-neutral recorder interface.
	var _ interface {
		RecordHostProcessPlacement(context.Context, string, string, uint32, time.Time) error
	} = controller
}
