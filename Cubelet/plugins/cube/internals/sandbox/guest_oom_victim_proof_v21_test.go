// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestV21GuestVictimProofRequiresExactOutcomeTokenAndGeneration(t *testing.T) {
	store := newTaskOutcomeProofStore()
	const sandboxID = "sandbox-v21-proof"
	generation := store.BeginRealization(sandboxID)
	token := v21Token(0x71)
	if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, token); err != nil {
		t.Fatalf("begin Wave21 realization: %v", err)
	}
	proof := GuestKernelOOMVictimProof{
		SandboxID:                sandboxID,
		Generation:               generation,
		RealizationTokenHex:      hex.EncodeToString(token[:]),
		GuestBootID:              "11111111-2222-3333-4444-555555555555",
		TID:                      42,
		TGID:                     41,
		StartTimeTicks:           9001,
		MainPID:                  41,
		MainStartTimeTicks:       9001,
		EventBootNS:              150,
		CgroupV2ID:               77,
		Class:                    GuestKernelOOMVictimClassMain,
		RealizationStartedBootNS: 100,
		OutcomeObservedBootNS:    200,
		Source:                   guestKernelOOMVictimSource,
	}
	if err := store.AcceptGuestKernelOOMVictimProofs(sandboxID, generation, token, []GuestKernelOOMVictimProof{proof}); err == nil {
		t.Fatal("accepted Wave21 proof before exact Wave17 outcome")
	}
	if _, err := store.Record(taskOutcomeCandidate{
		SandboxID: sandboxID,
		ExitCode:  137,
		ExitedAt:  time.Unix(1700000000, 0).UTC(),
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("record exact outcome: %v", err)
	}
	wrong := token
	wrong[0] ^= 0xff
	if err := store.AcceptGuestKernelOOMVictimProofs(sandboxID, generation, wrong, []GuestKernelOOMVictimProof{proof}); err == nil {
		t.Fatal("accepted Wave21 proof with wrong token")
	}
	if err := store.AcceptGuestKernelOOMVictimProofs(sandboxID, generation, token, []GuestKernelOOMVictimProof{proof}); err != nil {
		t.Fatalf("accept exact Wave21 proof: %v", err)
	}

	got := store.listGuestKernelOOMVictimProofs()
	if len(got) != 1 || got[0] != proof {
		t.Fatalf("stored proofs = %+v, want %+v", got, proof)
	}

	store.BeginRealization(sandboxID)
	if got := store.listGuestKernelOOMVictimProofs(); len(got) != 0 {
		t.Fatal("new generation retained old Wave21 proof")
	}
}

func TestV21GuestVictimMemberRequiresExactCgroupIdentity(t *testing.T) {
	token := v21Token(0x72)
	proof := GuestKernelOOMVictimProof{
		SandboxID: "sandbox-a", Generation: 1,
		RealizationTokenHex: hex.EncodeToString(token[:]),
		GuestBootID:         "11111111-2222-3333-4444-555555555555",
		TID:                 42, TGID: 41, StartTimeTicks: 9001,
		MainPID: 41, MainStartTimeTicks: 9001, EventBootNS: 150,
		Class:                    GuestKernelOOMVictimClassMember,
		RealizationStartedBootNS: 100, OutcomeObservedBootNS: 200,
		Source: guestKernelOOMVictimSource,
	}
	if err := validateGuestKernelOOMVictimProof(proof); err == nil {
		t.Fatal("MEMBER proof without cgroup-v2 identity was accepted")
	}
	proof.Class = GuestKernelOOMVictimClassMain
	if err := validateGuestKernelOOMVictimProof(proof); err != nil {
		t.Fatalf("MAIN proof with unknown cgroup should remain valid: %v", err)
	}
}

func TestV21GuestVictimProofSetRejectsMixedAuthority(t *testing.T) {
	token := v21Token(0x73)
	base := GuestKernelOOMVictimProof{
		SandboxID:                "sandbox-a",
		Generation:               1,
		RealizationTokenHex:      hex.EncodeToString(token[:]),
		GuestBootID:              "11111111-2222-3333-4444-555555555555",
		TID:                      42,
		TGID:                     41,
		StartTimeTicks:           9001,
		MainPID:                  41,
		MainStartTimeTicks:       9001,
		EventBootNS:              150,
		Class:                    GuestKernelOOMVictimClassMain,
		RealizationStartedBootNS: 100,
		OutcomeObservedBootNS:    200,
		Source:                   guestKernelOOMVictimSource,
	}
	mixed := base
	mixed.TID = 52
	mixed.TGID = 51
	mixed.StartTimeTicks = 9101
	mixed.Class = GuestKernelOOMVictimClassMember
	mixed.CgroupV2ID = 77
	mixed.EventBootNS = 160
	mixed.MainStartTimeTicks = 9002
	if _, err := normalizedGuestKernelOOMVictimProofs([]GuestKernelOOMVictimProof{base, mixed}); err == nil {
		t.Fatal("mixed main-lifetime authority was accepted in one Wave21 proof set")
	}
}
