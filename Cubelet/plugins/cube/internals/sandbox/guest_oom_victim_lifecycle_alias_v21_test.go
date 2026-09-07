// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestV21CreateFencePreventsGenerationAliasResurrection(t *testing.T) {
	store := newTaskOutcomeProofStore()
	const sandboxID = "sandbox-v21-create-alias"

	generation := store.BeginRealization(sandboxID)
	if generation != 1 {
		t.Fatalf("first generation = %d, want 1", generation)
	}
	token := v21Token(0x79)
	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, token); err != nil {
		t.Fatalf("begin first Wave21 realization: %v", err)
	}
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: sandboxID,
		ExitCode:  137,
		ExitedAt:  time.Unix(1_725_100_030, 0).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("record first exact outcome: %v", err)
	}
	proof := GuestKernelOOMVictimProof{
		SandboxID:                sandboxID,
		Generation:               generation,
		RealizationTokenHex:      hex.EncodeToString(token[:]),
		GuestBootID:              "11111111-2222-3333-4444-555555555555",
		TID:                      42,
		TGID:                     42,
		StartTimeTicks:           9001,
		MainPID:                  42,
		MainStartTimeTicks:       9001,
		EventBootNS:              150,
		Class:                    GuestKernelOOMVictimClassMain,
		RealizationStartedBootNS: 100,
		OutcomeObservedBootNS:    200,
		Source:                   guestKernelOOMVictimSource,
	}
	if err := store.AcceptGuestKernelOOMVictimProofs(sandboxID, generation, token, []GuestKernelOOMVictimProof{proof}); err != nil {
		t.Fatalf("accept first Wave21 proof: %v", err)
	}

	// Create explicitly destroys all authority from the previous runtime
	// lifecycle. BeginRealization then starts again at generation 1. Neither the
	// old token nor old positive proof may become current merely because the
	// numeric generation aliases the previous lifecycle.
	store.Clear(sandboxID)
	generation = store.BeginRealization(sandboxID)
	if generation != 1 {
		t.Fatalf("post-Create generation = %d, want reset generation 1", generation)
	}
	if _, ok := store.GuestOOMVictimToken(sandboxID, generation); ok {
		t.Fatal("post-Create generation alias resurrected stale Wave21 token")
	}
	if proofs := store.listGuestKernelOOMVictimProofs(); len(proofs) != 0 {
		t.Fatalf("post-Create generation alias resurrected %d stale Wave21 proofs", len(proofs))
	}
}
