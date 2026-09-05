package sandbox

import (
	"testing"
	"time"

	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/kernelvictim"
)

func seedV20ClosedGeneration(t *testing.T, store *taskOutcomeProofStore) {
	t.Helper()
	store.generations["sandbox-a"] = 7
	store.proofs["sandbox-a"] = TaskOutcomeProof{
		SandboxID: "sandbox-a", Generation: 7, ExitCode: 137,
		ExitedAt: time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC), Source: TaskOutcomeProofSourceWait,
	}
	store.hostProcessBindings["sandbox-a"] = HostProcessRealizationBinding{
		SandboxID: "sandbox-a", Generation: 7, CGroupPath: "/cube_sandbox_v1/42",
		BootID: "11111111-2222-3333-4444-555555555555", HostPID: 4242,
		StartTimeTicks: 1234, PlacedAt: time.Date(2026, 9, 5, 7, 59, 0, 0, time.UTC),
		BoundAt: time.Date(2026, 9, 5, 7, 59, 1, 0, time.UTC), Source: HostProcessPlacementSourceCubeBoxAddProc,
	}
	if !store.RecordVictimWindowStart("sandbox-a", 7, 10_000_000_000) {
		t.Fatal("start window rejected")
	}
	if !store.RecordVictimOutcomeObserved("sandbox-a", 7, 30_000_000_000) {
		t.Fatal("outcome window close rejected")
	}
}

func TestV20VictimCorrelationUsesTGIDNotVictimTID(t *testing.T) {
	store := newTaskOutcomeProofStore()
	seedV20ClosedGeneration(t, store)
	event := kernelvictim.Event{
		BootID:    "11111111-2222-3333-4444-555555555555",
		VictimTID: 4247, TGID: 4242, StartTimeTicks: 1234,
		EventBootTimeNS: 20_000_000_000,
	}
	proof, ok := store.FinalizeHostKernelOOMVictim("sandbox-a", event, 0, false)
	if !ok {
		t.Fatal("exact process victim event was not accepted")
	}
	if proof.HostPID != 4242 || proof.VictimTID != 4247 || proof.Generation != 7 {
		t.Fatalf("unexpected proof: %+v", proof)
	}
	if proof.CgroupV2Correlated || proof.CgroupV2ID != 0 {
		t.Fatalf("unknown cgroup must not be upgraded: %+v", proof)
	}
}

func TestV20VictimCorrelationRejectsLifetimeAndWindowMismatch(t *testing.T) {
	base := kernelvictim.Event{BootID: "11111111-2222-3333-4444-555555555555", VictimTID: 4247, TGID: 4242, StartTimeTicks: 1234, EventBootTimeNS: 20_000_000_000}
	cases := []struct {
		name   string
		mutate func(*kernelvictim.Event)
	}{
		{"tgid", func(e *kernelvictim.Event) { e.TGID = 9999 }},
		{"boot", func(e *kernelvictim.Event) { e.BootID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" }},
		{"starttime", func(e *kernelvictim.Event) { e.StartTimeTicks = 1235 }},
		{"before", func(e *kernelvictim.Event) { e.EventBootTimeNS = 9_999_999_999 }},
		{"after", func(e *kernelvictim.Event) { e.EventBootTimeNS = 30_000_000_001 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTaskOutcomeProofStore()
			seedV20ClosedGeneration(t, store)
			e := base
			tc.mutate(&e)
			if _, ok := store.FinalizeHostKernelOOMVictim("sandbox-a", e, 0, false); ok {
				t.Fatal("mismatched event accepted")
			}
		})
	}
}

func TestV20CgroupCorrelationIsExactAndNeverSynthesized(t *testing.T) {
	base := kernelvictim.Event{BootID: "11111111-2222-3333-4444-555555555555", VictimTID: 4247, TGID: 4242, StartTimeTicks: 1234, EventBootTimeNS: 20_000_000_000, CgroupV2ID: 88}
	store := newTaskOutcomeProofStore()
	seedV20ClosedGeneration(t, store)
	proof, ok := store.FinalizeHostKernelOOMVictim("sandbox-a", base, 88, true)
	if !ok || !proof.CgroupV2Correlated || proof.CgroupV2ID != 88 {
		t.Fatalf("exact cgroup match not proven: %+v %v", proof, ok)
	}

	store = newTaskOutcomeProofStore()
	seedV20ClosedGeneration(t, store)
	if _, ok := store.FinalizeHostKernelOOMVictim("sandbox-a", base, 89, true); ok {
		t.Fatal("known cgroup mismatch accepted")
	}

	store = newTaskOutcomeProofStore()
	seedV20ClosedGeneration(t, store)
	proof, ok = store.FinalizeHostKernelOOMVictim("sandbox-a", base, 0, false)
	if !ok || proof.CgroupV2Correlated || proof.CgroupV2ID != 0 {
		t.Fatalf("unavailable resolver must preserve only process proof: %+v %v", proof, ok)
	}
}

func TestV20CreateFenceClearsVictimAuthority(t *testing.T) {
	store := newTaskOutcomeProofStore()
	seedV20ClosedGeneration(t, store)
	store.Clear("sandbox-a")
	if _, ok := store.VictimWindow("sandbox-a"); ok {
		t.Fatal("victim window survived Create fence")
	}
	if got := store.ListHostKernelOOMVictimProofs(); len(got) != 0 {
		t.Fatalf("victim proof survived Create: %+v", got)
	}
}
