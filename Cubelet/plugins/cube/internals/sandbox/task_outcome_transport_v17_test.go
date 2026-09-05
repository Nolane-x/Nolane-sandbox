// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"math"
	"testing"
	"time"
)

func TestTaskOutcomeTransportListIsSortedDetachedAndRespectsClear(t *testing.T) {
	store := newTaskOutcomeProofStore()

	store.BeginRealization("sandbox-b")
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-b",
		ExitCode:  137,
		ExitedAt:  time.Date(2026, 9, 5, 4, 5, 7, 987654321, time.UTC),
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("record sandbox-b proof: %v", err)
	}

	store.BeginRealization("sandbox-a")
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  0,
		ExitedAt:  time.Date(2026, 9, 5, 4, 5, 6, 123456789, time.UTC),
		Source:    TaskOutcomeProofSourceState,
	}); err != nil {
		t.Fatalf("record sandbox-a proof: %v", err)
	}

	proofs := store.List()
	if len(proofs) != 2 {
		t.Fatalf("List length = %d, want 2", len(proofs))
	}
	if proofs[0].SandboxID != "sandbox-a" || proofs[1].SandboxID != "sandbox-b" {
		t.Fatalf("List order = [%q, %q], want sandbox-a then sandbox-b", proofs[0].SandboxID, proofs[1].SandboxID)
	}

	proofs[0].SandboxID = "mutated-by-caller"
	again := store.List()
	if len(again) != 2 || again[0].SandboxID != "sandbox-a" {
		t.Fatalf("List exposed mutable internal proof state: %#v", again)
	}

	store.Clear("sandbox-a")
	remaining := store.List()
	if len(remaining) != 1 || remaining[0].SandboxID != "sandbox-b" {
		t.Fatalf("List after Clear = %#v, want only sandbox-b", remaining)
	}
}

func TestTaskOutcomeTransportControllerListsFullUint64Generation(t *testing.T) {
	controller := &controllerLocal{}
	store := controller.ensureTaskOutcomeProofStore()
	if store == nil {
		t.Fatal("controller proof store is nil")
	}
	store.generations["sandbox-max"] = math.MaxUint64
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: "sandbox-max",
		ExitCode:  137,
		ExitedAt:  time.Date(2026, 9, 5, 4, 5, 6, 123456789, time.UTC),
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("record max-generation proof: %v", err)
	}

	proofs := controller.ListTaskOutcomeProofs()
	if len(proofs) != 1 {
		t.Fatalf("ListTaskOutcomeProofs length = %d, want 1", len(proofs))
	}
	if proofs[0].Generation != math.MaxUint64 {
		t.Fatalf("generation = %d, want %d", proofs[0].Generation, uint64(math.MaxUint64))
	}
}

var _ TaskOutcomeProofLister = (*controllerLocal)(nil)
