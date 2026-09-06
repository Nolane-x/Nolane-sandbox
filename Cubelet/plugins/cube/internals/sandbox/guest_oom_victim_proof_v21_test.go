// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
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
		GuestBootID:              "11111111-2222-3333-4444-555555555555",
		TID:                      42,
		TGID:                     41,
		StartTimeTicks:           9001,
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

	var got []GuestKernelOOMVictimProof
	store.VisitGuestKernelOOMVictimProofs(func(sandboxID string, generation uint64, guestBootID string, tid, tgid uint32, starttimeTicks, eventBootNS, cgroupV2ID uint64, class string, startedBootNS, outcomeBootNS uint64, source string) {
		got = append(got, GuestKernelOOMVictimProof{
			SandboxID: sandboxID, Generation: generation, GuestBootID: guestBootID,
			TID: tid, TGID: tgid, StartTimeTicks: starttimeTicks, EventBootNS: eventBootNS,
			CgroupV2ID: cgroupV2ID, Class: GuestKernelOOMVictimClass(class),
			RealizationStartedBootNS: startedBootNS, OutcomeObservedBootNS: outcomeBootNS, Source: source,
		})
	})
	if len(got) != 1 || got[0] != proof {
		t.Fatalf("visited proofs = %+v, want %+v", got, proof)
	}

	store.BeginRealization(sandboxID)
	got = got[:0]
	store.VisitGuestKernelOOMVictimProofs(func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string) {
		got = append(got, GuestKernelOOMVictimProof{})
	})
	if len(got) != 0 {
		t.Fatal("new generation retained old Wave21 proof")
	}
}

func TestV21GuestVictimMemberRequiresExactCgroupIdentity(t *testing.T) {
	proof := GuestKernelOOMVictimProof{
		SandboxID: "sandbox-a", Generation: 1,
		GuestBootID: "11111111-2222-3333-4444-555555555555",
		TID: 42, TGID: 41, StartTimeTicks: 9001, EventBootNS: 150,
		Class: GuestKernelOOMVictimClassMember,
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
