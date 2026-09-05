package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/kernelvictim"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeV20KernelVictimSource struct {
	event kernelvictim.Event
	ok    bool
}

func (f fakeV20KernelVictimSource) Find(bootID string, tgid uint32, startTimeTicks, minBootNS, maxBootNS uint64) (kernelvictim.Event, bool) {
	if !f.ok || f.event.BootID != bootID || f.event.TGID != tgid || f.event.StartTimeTicks != startTimeTicks {
		return kernelvictim.Event{}, false
	}
	if f.event.EventBootTimeNS < minBootNS || f.event.EventBootTimeNS > maxBootNS {
		return kernelvictim.Event{}, false
	}
	return f.event, true
}

func TestV20ControllerLifecycleFeedsExactKernelVictimProof(t *testing.T) {
	exitedAt := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		waitFn: func(context.Context, *task.WaitRequest) (*task.WaitResponse, error) {
			return &task.WaitResponse{ExitStatus: 137, ExitedAt: timestamppb.New(exitedAt)}, nil
		},
	})
	clockValues := []uint64{10_000_000_000, 30_000_000_000}
	controller.kernelVictimBootTimeNS = func() (uint64, error) {
		if len(clockValues) == 0 {
			return 0, errors.New("unexpected boottime call")
		}
		v := clockValues[0]
		clockValues = clockValues[1:]
		return v, nil
	}
	controller.kernelVictimSource = fakeV20KernelVictimSource{ok: true, event: kernelvictim.Event{
		BootID:          "11111111-2222-3333-4444-555555555555",
		VictimTID:       4247,
		TGID:            4242,
		StartTimeTicks:  1234,
		EventBootTimeNS: 20_000_000_000,
		CgroupV2ID:      88,
	}}
	controller.kernelVictimCgroupV2Resolver = func(path string) (uint64, bool) {
		if path != "/cube_sandbox_v1/42" {
			t.Fatalf("unexpected cgroup path %q", path)
		}
		return 88, true
	}

	if _, err := controller.Start(context.Background(), "sandbox-a"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	store := controller.ensureTaskOutcomeProofStore()
	store.mu.Lock()
	generation := store.generations["sandbox-a"]
	store.hostProcessBindings["sandbox-a"] = HostProcessRealizationBinding{
		SandboxID:      "sandbox-a",
		Generation:     generation,
		CGroupPath:     "/cube_sandbox_v1/42",
		BootID:         "11111111-2222-3333-4444-555555555555",
		HostPID:        4242,
		StartTimeTicks: 1234,
		PlacedAt:       time.Date(2026, 9, 5, 7, 59, 0, 0, time.UTC),
		BoundAt:        time.Date(2026, 9, 5, 7, 59, 1, 0, time.UTC),
		Source:         HostProcessPlacementSourceCubeBoxAddProc,
	}
	store.mu.Unlock()

	outcome, err := controller.Wait(context.Background(), "sandbox-a")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if outcome.ExitStatus != 137 || !outcome.ExitedAt.Equal(exitedAt) {
		t.Fatalf("exact task outcome changed: %+v", outcome)
	}
	proofs := store.ListHostKernelOOMVictimProofs()
	if len(proofs) != 1 {
		t.Fatalf("kernel victim proofs = %+v", proofs)
	}
	proof := proofs[0]
	if proof.Generation != generation || proof.HostPID != 4242 || proof.VictimTID != 4247 || !proof.CgroupV2Correlated || proof.CgroupV2ID != 88 {
		t.Fatalf("unexpected kernel victim proof: %+v", proof)
	}
}

func TestV20ControllerCapabilityFailureNeverFailsWorkloadOutcome(t *testing.T) {
	exitedAt := time.Date(2026, 9, 5, 8, 1, 0, 0, time.UTC)
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		waitFn: func(context.Context, *task.WaitRequest) (*task.WaitResponse, error) {
			return &task.WaitResponse{ExitStatus: 9, ExitedAt: timestamppb.New(exitedAt)}, nil
		},
	})
	controller.kernelVictimBootTimeNS = func() (uint64, error) {
		return 0, errors.New("CLOCK_BOOTTIME unavailable")
	}
	controller.kernelVictimSource = fakeV20KernelVictimSource{ok: false}

	if _, err := controller.Start(context.Background(), "sandbox-a"); err != nil {
		t.Fatalf("Wave20 clock failure changed Start: %v", err)
	}
	outcome, err := controller.Wait(context.Background(), "sandbox-a")
	if err != nil {
		t.Fatalf("Wave20 clock failure changed Wait: %v", err)
	}
	if outcome.ExitStatus != 9 || !outcome.ExitedAt.Equal(exitedAt) {
		t.Fatalf("exact task outcome changed: %+v", outcome)
	}
	if proofs := controller.ensureTaskOutcomeProofStore().ListHostKernelOOMVictimProofs(); len(proofs) != 0 {
		t.Fatalf("capability failure fabricated victim proof: %+v", proofs)
	}
}
