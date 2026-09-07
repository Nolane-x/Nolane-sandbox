// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import "testing"

func v21Token(seed byte) [32]byte {
	var token [32]byte
	for i := range token {
		token[i] = seed
	}
	return token
}

func TestV21GuestOOMVictimTokenIsGenerationScoped(t *testing.T) {
	store := newTaskOutcomeProofStore()
	const sandboxID = "sandbox-v21"

	generation := store.BeginRealization(sandboxID)
	token := v21Token(0x21)
	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, token); err != nil {
		t.Fatalf("begin Wave21 realization: %v", err)
	}

	got, ok := store.GuestOOMVictimToken(sandboxID, generation)
	if !ok {
		t.Fatal("expected exact Wave21 token for current generation")
	}
	if got != token {
		t.Fatalf("token mismatch: got %x want %x", got, token)
	}

	next := store.BeginRealization(sandboxID)
	if next == generation {
		t.Fatal("new realization must advance Wave17 generation")
	}
	if _, ok := store.GuestOOMVictimToken(sandboxID, generation); ok {
		t.Fatal("old Wave21 token survived new Wave17 generation")
	}
}

func TestV21GuestOOMVictimTokenIsImmutableWithinGeneration(t *testing.T) {
	store := newTaskOutcomeProofStore()
	const sandboxID = "sandbox-v21-immutable"
	generation := store.BeginRealization(sandboxID)
	first := v21Token(0x31)
	conflict := v21Token(0x32)

	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, first); err != nil {
		t.Fatalf("begin Wave21 realization: %v", err)
	}
	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, first); err != nil {
		t.Fatalf("exact duplicate token must be idempotent: %v", err)
	}
	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, conflict); err == nil {
		t.Fatal("conflicting token replaced immutable current-generation authority")
	}

	got, ok := store.GuestOOMVictimToken(sandboxID, generation)
	if !ok {
		t.Fatal("expected original current-generation token after conflicting write")
	}
	if got != first {
		t.Fatalf("conflicting write changed token: got %x want %x", got, first)
	}
}

func TestV21CreateFenceDestroysTokenAuthority(t *testing.T) {
	store := newTaskOutcomeProofStore()
	const sandboxID = "sandbox-v21-fence"
	generation := store.BeginRealization(sandboxID)
	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, v21Token(0x42)); err != nil {
		t.Fatalf("begin Wave21 realization: %v", err)
	}

	store.Clear(sandboxID)
	if _, ok := store.GuestOOMVictimToken(sandboxID, generation); ok {
		t.Fatal("Create fence retained stale Wave21 token")
	}
}

func TestV21RejectsAllZeroToken(t *testing.T) {
	store := newTaskOutcomeProofStore()
	generation := store.BeginRealization("sandbox-v21-zero")
	if err := store.BeginGuestOOMVictimRealization("sandbox-v21-zero", generation, [32]byte{}); err == nil {
		t.Fatal("all-zero realization token must be rejected")
	}
}
