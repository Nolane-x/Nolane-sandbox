// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import "testing"

func TestV21CurrentGuestOOMVictimTokenReturnsOnlyOpenGeneration(t *testing.T) {
	store := newTaskOutcomeProofStore()
	controller := &controllerLocal{taskOutcomeProofs: store}
	const sandboxID = "sandbox-v21-current"

	generation := store.BeginRealization(sandboxID)
	token := v21Token(0x61)
	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, token); err != nil {
		t.Fatalf("begin Wave21 realization: %v", err)
	}

	gotToken, gotGeneration, ok := controller.CurrentGuestOOMVictimToken(sandboxID)
	if !ok {
		t.Fatal("expected current Wave21 authority")
	}
	if gotGeneration != generation || gotToken != token {
		t.Fatalf("authority = generation %d token %x, want generation %d token %x", gotGeneration, gotToken, generation, token)
	}

	store.Clear(sandboxID)
	if _, _, ok := controller.CurrentGuestOOMVictimToken(sandboxID); ok {
		t.Fatal("Create fence retained current Wave21 authority")
	}
}
