// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package guestvictimbridge

import "testing"

func v21Token(seed byte) [32]byte {
	var token [32]byte
	for i := range token {
		token[i] = seed + byte(i)
	}
	return token
}

func TestV21BridgeClaimIsOneShotAndTerminal(t *testing.T) {
	const sandboxID = "sandbox-v21-bridge"
	Clear(sandboxID)
	t.Cleanup(func() { Clear(sandboxID) })

	binding := StartBinding{SandboxID: sandboxID, Generation: 7, Token: v21Token(1)}
	if err := PublishStartBinding(binding); err != nil {
		t.Fatalf("PublishStartBinding: %v", err)
	}
	if IsBound(binding) {
		t.Fatal("pending binding reported bound before live claim")
	}
	if got, ok := CurrentStartBinding(sandboxID); !ok || got != binding {
		t.Fatalf("CurrentStartBinding = (%+v,%v), want exact binding", got, ok)
	}
	claimed, ok := ClaimStartBinding(sandboxID)
	if !ok || claimed != binding {
		t.Fatalf("ClaimStartBinding = (%+v,%v), want exact binding", claimed, ok)
	}
	if IsBound(binding) {
		t.Fatal("claimed binding reported bound before successful delivery")
	}
	if _, ok := ClaimStartBinding(sandboxID); ok {
		t.Fatal("binding was claimable more than once")
	}
	if !MarkBound(binding) {
		t.Fatal("exact claimed binding was not accepted as bound")
	}
	if !IsBound(binding) {
		t.Fatal("exact terminal bound state was not observable")
	}
	wrong := binding
	wrong.Token[0] ^= 0xff
	if IsBound(wrong) {
		t.Fatal("wrong token observed exact bound authority")
	}
	if _, ok := CurrentStartBinding(sandboxID); ok {
		t.Fatal("bound binding remained available")
	}
}

func TestV21BridgeConflictNeverReplacesFirstToken(t *testing.T) {
	const sandboxID = "sandbox-v21-conflict"
	Clear(sandboxID)
	t.Cleanup(func() { Clear(sandboxID) })

	first := StartBinding{SandboxID: sandboxID, Generation: 3, Token: v21Token(2)}
	conflict := StartBinding{SandboxID: sandboxID, Generation: 3, Token: v21Token(9)}
	if err := PublishStartBinding(first); err != nil {
		t.Fatalf("publish first: %v", err)
	}
	if err := PublishStartBinding(conflict); err == nil {
		t.Fatal("same-generation conflicting token replaced bridge authority")
	}
	if got, ok := CurrentStartBinding(sandboxID); !ok || got != first {
		t.Fatalf("bridge authority = (%+v,%v), want first token", got, ok)
	}
}

func TestV21BridgeStaleTerminalReportCannotMutateNewGeneration(t *testing.T) {
	const sandboxID = "sandbox-v21-stale"
	Clear(sandboxID)
	t.Cleanup(func() { Clear(sandboxID) })

	old := StartBinding{SandboxID: sandboxID, Generation: 1, Token: v21Token(3)}
	if err := PublishStartBinding(old); err != nil {
		t.Fatalf("publish old: %v", err)
	}
	if _, ok := ClaimStartBinding(sandboxID); !ok {
		t.Fatal("old binding was not claimable")
	}

	fresh := StartBinding{SandboxID: sandboxID, Generation: 2, Token: v21Token(4)}
	if err := PublishStartBinding(fresh); err != nil {
		t.Fatalf("publish fresh: %v", err)
	}
	if MarkUnavailable(old) {
		t.Fatal("stale terminal report mutated fresh bridge generation")
	}
	if IsBound(old) {
		t.Fatal("stale binding remained terminally bound after new generation")
	}
	if got, ok := CurrentStartBinding(sandboxID); !ok || got != fresh {
		t.Fatalf("fresh bridge authority = (%+v,%v), want %+v", got, ok, fresh)
	}
}
