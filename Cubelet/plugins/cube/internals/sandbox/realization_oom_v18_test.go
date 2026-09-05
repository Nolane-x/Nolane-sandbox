// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"testing"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRealizationOOMStoreBindsExactGenerationAndDelta(t *testing.T) {
	baselineAt := time.Date(2026, 9, 5, 5, 0, 0, 123456789, time.UTC)
	exitedAt := baselineAt.Add(30 * time.Second)
	observedAt := exitedAt.Add(time.Nanosecond)

	store := newTaskOutcomeProofStore()
	generation := store.BeginRealization("sandbox-a")
	if ok := store.RecordRealizationOOMBaseline("sandbox-a", generation, realizationOOMSnapshot{
		CGroupPath:    "/cube_sandbox_v2/sandbox/numa/7/sandbox-a",
		OOMKillsKnown: true,
		OOMKillsTotal: 7,
		CapturedAt:    baselineAt,
	}); !ok {
		t.Fatal("realization OOM baseline was not accepted")
	}
	outcome, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  137,
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceWait,
	})
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	proof, ok := store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{
		CGroupPath:    "/cube_sandbox_v2/sandbox/numa/7/sandbox-a",
		OOMKillsKnown: true,
		OOMKillsTotal: 9,
		CapturedAt:    observedAt,
	})
	if !ok {
		t.Fatal("exact realization OOM proof remained unknown")
	}
	if proof.SandboxID != "sandbox-a" || proof.Generation != generation || proof.BaselineOOMKills != 7 || proof.FinalOOMKills != 9 || proof.OOMKills != 2 {
		t.Fatalf("proof = %#v", proof)
	}
	if proof.CGroupPath != "/cube_sandbox_v2/sandbox/numa/7/sandbox-a" || !proof.BaselineAt.Equal(baselineAt) || !proof.ExitedAt.Equal(exitedAt) || !proof.ObservedAt.Equal(observedAt) {
		t.Fatalf("proof continuity = %#v", proof)
	}
	if proof.OutcomeSource != TaskOutcomeProofSourceWait {
		t.Fatalf("OutcomeSource = %q", proof.OutcomeSource)
	}
}

func TestRealizationOOMFinalizationFailureIsTerminalForGeneration(t *testing.T) {
	baselineAt := time.Date(2026, 9, 5, 5, 10, 0, 0, time.UTC)
	exitedAt := baselineAt.Add(time.Second)
	store := newTaskOutcomeProofStore()
	generation := store.BeginRealization("sandbox-a")
	if !store.RecordRealizationOOMBaseline("sandbox-a", generation, realizationOOMSnapshot{
		CGroupPath:    "/cube/path",
		OOMKillsKnown: true,
		OOMKillsTotal: 3,
		CapturedAt:    baselineAt,
	}) {
		t.Fatal("baseline not accepted")
	}
	outcome, err := store.Record(taskOutcomeCandidate{SandboxID: "sandbox-a", ExitCode: 1, ExitedAt: exitedAt, Source: TaskOutcomeProofSourceState})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{
		CGroupPath:    "/cube/path",
		OOMKillsKnown: false,
		CapturedAt:    exitedAt.Add(time.Second),
	}); ok {
		t.Fatal("unknown final cgroup signal became proof")
	}
	if _, ok := store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{
		CGroupPath:    "/cube/path",
		OOMKillsKnown: true,
		OOMKillsTotal: 4,
		CapturedAt:    exitedAt.Add(2 * time.Second),
	}); ok {
		t.Fatal("second finalization repaired a terminally unknown first attempt")
	}
}

func TestRealizationOOMRejectsBrokenContinuity(t *testing.T) {
	baselineAt := time.Date(2026, 9, 5, 5, 20, 0, 0, time.UTC)
	tests := map[string]struct {
		baseline  realizationOOMSnapshot
		outcomeAt time.Time
		final     realizationOOMSnapshot
	}{
		"cgroup changed": {
			baseline:  realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 2, CapturedAt: baselineAt},
			outcomeAt: baselineAt.Add(time.Second),
			final:     realizationOOMSnapshot{CGroupPath: "/cube/b", OOMKillsKnown: true, OOMKillsTotal: 3, CapturedAt: baselineAt.Add(2 * time.Second)},
		},
		"counter regressed": {
			baseline:  realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 4, CapturedAt: baselineAt},
			outcomeAt: baselineAt.Add(time.Second),
			final:     realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 3, CapturedAt: baselineAt.Add(2 * time.Second)},
		},
		"exit predates baseline": {
			baseline:  realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 2, CapturedAt: baselineAt},
			outcomeAt: baselineAt.Add(-time.Nanosecond),
			final:     realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 3, CapturedAt: baselineAt.Add(time.Second)},
		},
		"observation predates exit": {
			baseline:  realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 2, CapturedAt: baselineAt},
			outcomeAt: baselineAt.Add(2 * time.Second),
			final:     realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 3, CapturedAt: baselineAt.Add(time.Second)},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store := newTaskOutcomeProofStore()
			generation := store.BeginRealization("sandbox-a")
			if !store.RecordRealizationOOMBaseline("sandbox-a", generation, tc.baseline) {
				t.Fatal("baseline fixture rejected")
			}
			outcome, err := store.Record(taskOutcomeCandidate{SandboxID: "sandbox-a", ExitCode: 137, ExitedAt: tc.outcomeAt, Source: TaskOutcomeProofSourceWait})
			if err != nil {
				t.Fatal(err)
			}
			if proof, ok := store.FinalizeRealizationOOM(outcome, tc.final); ok {
				t.Fatalf("broken continuity produced proof %#v", proof)
			}
		})
	}
}

func TestRealizationOOMNewStartClearsPreviousProof(t *testing.T) {
	baselineAt := time.Date(2026, 9, 5, 5, 30, 0, 0, time.UTC)
	store := newTaskOutcomeProofStore()
	generation := store.BeginRealization("sandbox-a")
	if !store.RecordRealizationOOMBaseline("sandbox-a", generation, realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 5, CapturedAt: baselineAt}) {
		t.Fatal("baseline fixture rejected")
	}
	outcome, err := store.Record(taskOutcomeCandidate{SandboxID: "sandbox-a", ExitCode: 0, ExitedAt: baselineAt.Add(time.Second), Source: TaskOutcomeProofSourceState})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{CGroupPath: "/cube/a", OOMKillsKnown: true, OOMKillsTotal: 5, CapturedAt: baselineAt.Add(2 * time.Second)}); !ok {
		t.Fatal("zero-delta proof fixture not accepted")
	}
	if len(store.ListRealizationOOMProofs()) != 1 {
		t.Fatal("proof fixture missing")
	}
	store.BeginRealization("sandbox-a")
	if proofs := store.ListRealizationOOMProofs(); len(proofs) != 0 {
		t.Fatalf("new realization leaked old OOM proof: %#v", proofs)
	}
}

func TestRealizationOOMControllerCapturesStartBaselineAndFinalizesAfterExactWait(t *testing.T) {
	baselineAt := time.Date(2026, 9, 5, 5, 40, 0, 111222333, time.UTC)
	exitedAt := baselineAt.Add(10 * time.Second)
	observedAt := exitedAt.Add(time.Nanosecond)
	reads := 0
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		waitFn: func(context.Context, *task.WaitRequest) (*task.WaitResponse, error) {
			return &task.WaitResponse{ExitStatus: 137, ExitedAt: timestamppb.New(exitedAt)}, nil
		},
	})
	controller.SetRealizationOOMSnapshotReader(func(context.Context, string) (string, bool, uint64, time.Time, error) {
		reads++
		if reads == 1 {
			return "/cube/path", true, 11, baselineAt, nil
		}
		return "/cube/path", true, 13, observedAt, nil
	})
	if _, err := controller.Start(context.Background(), "sandbox-a"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status, err := controller.Wait(context.Background(), "sandbox-a")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if status.ExitStatus != 137 || !status.ExitedAt.Equal(exitedAt) {
		t.Fatalf("exact task outcome changed: %#v", status)
	}
	if reads != 2 {
		t.Fatalf("snapshot reads = %d, want baseline+final", reads)
	}
	proofs := controller.ListRealizationOOMProofs()
	if len(proofs) != 1 || proofs[0].OOMKills != 2 || !proofs[0].ExitedAt.Equal(exitedAt) {
		t.Fatalf("controller OOM proofs = %#v", proofs)
	}
}
