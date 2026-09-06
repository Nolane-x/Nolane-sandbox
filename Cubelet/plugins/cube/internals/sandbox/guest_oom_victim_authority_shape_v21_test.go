// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"reflect"
	"strings"
	"testing"
)

func TestV21AcceptedGuestVictimProofPreservesFullAuthority(t *testing.T) {
	typ := reflect.TypeOf(GuestKernelOOMVictimProof{})
	for _, name := range []string{
		"SandboxID",
		"Generation",
		"RealizationTokenHex",
		"GuestBootID",
		"TID",
		"TGID",
		"StartTimeTicks",
		"MainPID",
		"MainStartTimeTicks",
		"EventBootNS",
		"CgroupV2ID",
		"Class",
		"RealizationStartedBootNS",
		"OutcomeObservedBootNS",
		"Source",
	} {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("Wave21 accepted proof dropped normative authority field %s", name)
		}
	}
}

func TestV21AcceptedGuestVictimProofValidatesTokenAndExactMainLifetime(t *testing.T) {
	proof := GuestKernelOOMVictimProof{
		SandboxID:                "sandbox-authority",
		Generation:               7,
		GuestBootID:              "11111111-2222-3333-4444-555555555555",
		TID:                      42,
		TGID:                     41,
		StartTimeTicks:           9001,
		EventBootNS:              150,
		Class:                    GuestKernelOOMVictimClassMain,
		RealizationStartedBootNS: 100,
		OutcomeObservedBootNS:    200,
		Source:                   guestKernelOOMVictimSource,
	}

	value := reflect.ValueOf(&proof).Elem()
	token := value.FieldByName("RealizationTokenHex")
	mainPID := value.FieldByName("MainPID")
	mainStart := value.FieldByName("MainStartTimeTicks")
	if !token.IsValid() || !mainPID.IsValid() || !mainStart.IsValid() {
		t.Fatal("Wave21 full authority fields are unavailable")
	}
	token.SetString(strings.Repeat("71", 32))
	mainPID.SetUint(41)
	mainStart.SetUint(9001)

	if err := validateGuestKernelOOMVictimProof(proof); err != nil {
		t.Fatalf("exact MAIN authority rejected: %v", err)
	}

	wrongMain := proof
	reflect.ValueOf(&wrongMain).Elem().FieldByName("MainPID").SetUint(99)
	if err := validateGuestKernelOOMVictimProof(wrongMain); err == nil {
		t.Fatal("MAIN proof with mismatched main PID was accepted")
	}

	wrongLifetime := proof
	reflect.ValueOf(&wrongLifetime).Elem().FieldByName("MainStartTimeTicks").SetUint(9002)
	if err := validateGuestKernelOOMVictimProof(wrongLifetime); err == nil {
		t.Fatal("MAIN proof with mismatched main lifetime was accepted")
	}

	badToken := proof
	reflect.ValueOf(&badToken).Elem().FieldByName("RealizationTokenHex").SetString(strings.Repeat("AA", 32))
	if err := validateGuestKernelOOMVictimProof(badToken); err == nil {
		t.Fatal("non-canonical realization token was accepted")
	}
}
