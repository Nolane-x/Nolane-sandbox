// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"testing"
	"time"

	coresandbox "github.com/containerd/containerd/v2/core/sandbox"
)

func TestTaskOutcomeCreateClearsStaleProof(t *testing.T) {
	controller := &controllerLocal{}
	controller.beginTaskOutcomeRealization("sandbox-a")
	if _, err := controller.recordTaskOutcomeCandidate(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  12,
		ExitedAt:  time.Unix(1_725_000_007, 101).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("record stale proof fixture: %v", err)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); !ok {
		t.Fatal("stale proof fixture was not recorded")
	}

	if err := controller.Create(context.Background(), coresandbox.Sandbox{ID: "sandbox-a"}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("Create left stale task-outcome proof attached to a new sandbox lifecycle")
	}
	if _, err := controller.recordTaskOutcomeCandidate(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  99,
		ExitedAt:  time.Unix(1_725_000_007, 202).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	}); err == nil {
		t.Fatal("Create left the previous realization active for a late outcome")
	}
}

func TestTaskOutcomeStartBeginsNewRealization(t *testing.T) {
	controller := &controllerLocal{}
	firstGeneration := controller.beginTaskOutcomeRealization("sandbox-a")
	if _, err := controller.recordTaskOutcomeCandidate(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  0,
		ExitedAt:  time.Unix(1_725_000_008, 202).UTC(),
		Source:    TaskOutcomeProofSourceState,
	}); err != nil {
		t.Fatalf("record first realization proof: %v", err)
	}

	if _, err := controller.Start(context.Background(), "sandbox-a"); err != nil {
		t.Fatalf("start sandbox: %v", err)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("Start exposed proof from the previous realization")
	}

	proof, err := controller.recordTaskOutcomeCandidate(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  7,
		ExitedAt:  time.Unix(1_725_000_009, 303).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	})
	if err != nil {
		t.Fatalf("record second realization proof: %v", err)
	}
	if proof.Generation != firstGeneration+1 {
		t.Fatalf("second realization generation = %d, want %d", proof.Generation, firstGeneration+1)
	}
}
