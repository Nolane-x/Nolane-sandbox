// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"testing"
	"time"

	"github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTaskOutcomeProofStoreStartsUnknown(t *testing.T) {
	store := newTaskOutcomeProofStore()
	if _, ok := store.Get("sandbox-a"); ok {
		t.Fatal("fresh task-outcome proof store must be unknown")
	}
}

func TestTaskOutcomeProofStorePreservesExactExitCodeAndRuntimeTime(t *testing.T) {
	store := newTaskOutcomeProofStore()
	generation := store.BeginRealization("sandbox-a")
	if generation != 1 {
		t.Fatalf("first realization generation = %d, want 1", generation)
	}

	exitedAt := time.Unix(1_725_000_000, 123_456_789).UTC()
	proof, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  137,
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceWait,
	})
	if err != nil {
		t.Fatalf("record exact outcome: %v", err)
	}
	if proof.SandboxID != "sandbox-a" {
		t.Fatalf("sandbox ID = %q, want sandbox-a", proof.SandboxID)
	}
	if proof.Generation != generation {
		t.Fatalf("generation = %d, want %d", proof.Generation, generation)
	}
	if proof.ExitCode != 137 {
		t.Fatalf("exit code = %d, want exact 137", proof.ExitCode)
	}
	if !proof.ExitedAt.Equal(exitedAt) {
		t.Fatalf("exit time = %s, want %s", proof.ExitedAt, exitedAt)
	}
	if proof.Source != TaskOutcomeProofSourceWait {
		t.Fatalf("source = %q, want %q", proof.Source, TaskOutcomeProofSourceWait)
	}
}

func TestTaskOutcomeProofStoreRepeatedExactOutcomeIsIdempotent(t *testing.T) {
	store := newTaskOutcomeProofStore()
	store.BeginRealization("sandbox-a")
	exitedAt := time.Unix(1_725_000_001, 222).UTC()
	candidate := taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  23,
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceState,
	}

	first, err := store.Record(candidate)
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	secondCandidate := candidate
	secondCandidate.Source = TaskOutcomeProofSourceWait
	second, err := store.Record(secondCandidate)
	if err != nil {
		t.Fatalf("idempotent repeat: %v", err)
	}
	if second != first {
		t.Fatalf("idempotent repeat mutated proof: first=%+v second=%+v", first, second)
	}
}

func TestTaskOutcomeProofStoreRejectsConflictingOutcome(t *testing.T) {
	store := newTaskOutcomeProofStore()
	store.BeginRealization("sandbox-a")
	exitedAt := time.Unix(1_725_000_002, 333).UTC()
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  0,
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("first record: %v", err)
	}

	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  1,
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceWait,
	}); err == nil {
		t.Fatal("conflicting exit code must be rejected")
	}
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  0,
		ExitedAt:  exitedAt.Add(time.Nanosecond),
		Source:    TaskOutcomeProofSourceWait,
	}); err == nil {
		t.Fatal("conflicting exit timestamp must be rejected")
	}
}

func TestTaskOutcomeProofStoreNewRealizationInvalidatesOldProof(t *testing.T) {
	store := newTaskOutcomeProofStore()
	firstGeneration := store.BeginRealization("sandbox-a")
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  9,
		ExitedAt:  time.Unix(1_725_000_003, 444).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("record first realization: %v", err)
	}

	secondGeneration := store.BeginRealization("sandbox-a")
	if secondGeneration != firstGeneration+1 {
		t.Fatalf("second generation = %d, want %d", secondGeneration, firstGeneration+1)
	}
	if _, ok := store.Get("sandbox-a"); ok {
		t.Fatal("old proof survived a new realization")
	}
}

func TestOutcomeCandidateFromWaitRequiresAuthoritativeRuntimeTimestamp(t *testing.T) {
	if _, err := outcomeCandidateFromWait("sandbox-a", nil); err == nil {
		t.Fatal("nil wait response must not become proof")
	}
	if _, err := outcomeCandidateFromWait("sandbox-a", &task.WaitResponse{
		ExitStatus: 137,
	}); err == nil {
		t.Fatal("wait response without runtime exit timestamp must not become proof")
	}

	exitedAt := time.Unix(1_725_000_004, 555_666_777).UTC()
	candidate, err := outcomeCandidateFromWait("sandbox-a", &task.WaitResponse{
		ExitStatus: 137,
		ExitedAt:   timestamppb.New(exitedAt),
	})
	if err != nil {
		t.Fatalf("valid wait response: %v", err)
	}
	if candidate.ExitCode != 137 {
		t.Fatalf("exit code = %d, want exact 137", candidate.ExitCode)
	}
	if !candidate.ExitedAt.Equal(exitedAt) {
		t.Fatalf("exit time = %s, want %s", candidate.ExitedAt, exitedAt)
	}
	if candidate.Source != TaskOutcomeProofSourceWait {
		t.Fatalf("source = %q, want wait", candidate.Source)
	}
}

func TestOutcomeCandidateFromStateOnlyAcceptsStoppedRuntimeState(t *testing.T) {
	exitedAt := time.Unix(1_725_000_005, 888_999).UTC()

	if _, ok, err := outcomeCandidateFromState("sandbox-a", &task.StateResponse{
		Status:     tasktypes.Status_RUNNING,
		ExitStatus: 42,
		ExitedAt:   timestamppb.New(exitedAt),
	}); err != nil || ok {
		t.Fatalf("running state became proof candidate: ok=%v err=%v", ok, err)
	}

	if _, ok, err := outcomeCandidateFromState("sandbox-a", &task.StateResponse{
		Status:     tasktypes.Status_STOPPED,
		ExitStatus: 42,
	}); err == nil || ok {
		t.Fatalf("stopped state without runtime exit timestamp must fail closed: ok=%v err=%v", ok, err)
	}

	candidate, ok, err := outcomeCandidateFromState("sandbox-a", &task.StateResponse{
		Status:     tasktypes.Status_STOPPED,
		ExitStatus: 42,
		ExitedAt:   timestamppb.New(exitedAt),
	})
	if err != nil {
		t.Fatalf("valid stopped state: %v", err)
	}
	if !ok {
		t.Fatal("valid stopped state did not produce proof candidate")
	}
	if candidate.ExitCode != 42 || !candidate.ExitedAt.Equal(exitedAt) {
		t.Fatalf("candidate lost exact runtime outcome: %+v", candidate)
	}
	if candidate.Source != TaskOutcomeProofSourceState {
		t.Fatalf("source = %q, want state", candidate.Source)
	}
}

func TestControllerTaskOutcomeProviderStartsUnknown(t *testing.T) {
	controller := &controllerLocal{}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("controller exposed proof before an authoritative outcome")
	}
}

func TestControllerTaskOutcomeRealizationInvalidatesProof(t *testing.T) {
	controller := &controllerLocal{}
	firstGeneration := controller.beginTaskOutcomeRealization("sandbox-a")
	if firstGeneration != 1 {
		t.Fatalf("first controller generation = %d, want 1", firstGeneration)
	}

	exitedAt := time.Unix(1_725_000_006, 111_222_333).UTC()
	proof, err := controller.recordTaskOutcomeCandidate(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  137,
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceWait,
	})
	if err != nil {
		t.Fatalf("record controller proof: %v", err)
	}
	if proof.Generation != firstGeneration {
		t.Fatalf("proof generation = %d, want %d", proof.Generation, firstGeneration)
	}

	secondGeneration := controller.beginTaskOutcomeRealization("sandbox-a")
	if secondGeneration != firstGeneration+1 {
		t.Fatalf("second controller generation = %d, want %d", secondGeneration, firstGeneration+1)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("controller exposed stale proof after a new realization")
	}
}

func TestControllerRejectsInvalidTaskOutcomeCandidate(t *testing.T) {
	controller := &controllerLocal{}
	controller.beginTaskOutcomeRealization("sandbox-a")

	if _, err := controller.recordTaskOutcomeCandidate(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  137,
		Source:    TaskOutcomeProofSourceWait,
	}); err == nil {
		t.Fatal("controller accepted a candidate without authoritative runtime exit time")
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("invalid controller candidate created proof")
	}
}
