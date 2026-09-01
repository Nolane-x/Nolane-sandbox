// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"testing"
	"time"

	coresandbox "github.com/containerd/containerd/v2/core/sandbox"
)

func TestTaskOutcomeStoreCanRecoverFreshAuthoritativeRealization(t *testing.T) {
	store := newTaskOutcomeProofStore()
	generation, ok := store.RecoverRealization("sandbox-a")
	if !ok {
		t.Fatal("fresh store could not recover a realization from authoritative runtime evidence")
	}
	if generation != 1 {
		t.Fatalf("recovered generation = %d, want 1", generation)
	}

	proof, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  17,
		ExitedAt:  time.Unix(1_725_000_010, 404).UTC(),
		Source:    TaskOutcomeProofSourceState,
	})
	if err != nil {
		t.Fatalf("record recovered realization proof: %v", err)
	}
	if proof.Generation != generation {
		t.Fatalf("proof generation = %d, want recovered generation %d", proof.Generation, generation)
	}
}

func TestTaskOutcomeStoreCreateFenceBlocksRecoveryUntilStart(t *testing.T) {
	store := newTaskOutcomeProofStore()
	store.BeginRealization("sandbox-a")
	store.Clear("sandbox-a")

	if _, ok := store.RecoverRealization("sandbox-a"); ok {
		t.Fatal("Create-style fence allowed stale authoritative outcome to recover a realization")
	}
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  19,
		ExitedAt:  time.Unix(1_725_000_011, 505).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	}); err == nil {
		t.Fatal("Create-style fence left a realization active")
	}

	generation := store.BeginRealization("sandbox-a")
	if generation == 0 {
		t.Fatal("Start-style begin did not reactivate realization after fence")
	}
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  19,
		ExitedAt:  time.Unix(1_725_000_011, 505).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("record after Start-style begin: %v", err)
	}
}

func TestControllerAuthoritativeRecoveryWorksAfterRestartButNotAfterCreate(t *testing.T) {
	controller := &controllerLocal{}
	candidate := taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  23,
		ExitedAt:  time.Unix(1_725_000_012, 606).UTC(),
		Source:    TaskOutcomeProofSourceState,
	}

	proof, err := controller.recordAuthoritativeTaskOutcomeCandidate(candidate)
	if err != nil {
		t.Fatalf("recover authoritative outcome on fresh controller: %v", err)
	}
	if proof.Generation != 1 {
		t.Fatalf("recovered controller generation = %d, want 1", proof.Generation)
	}

	if err := controller.Create(context.Background(), coresandbox.Sandbox{ID: "sandbox-a"}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	candidate.ExitCode = 24
	candidate.ExitedAt = candidate.ExitedAt.Add(time.Second)
	if _, err := controller.recordAuthoritativeTaskOutcomeCandidate(candidate); err == nil {
		t.Fatal("authoritative recovery bypassed Create fence")
	}
}
