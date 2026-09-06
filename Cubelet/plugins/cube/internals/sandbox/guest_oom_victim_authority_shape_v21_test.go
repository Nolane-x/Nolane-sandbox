// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"reflect"
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
