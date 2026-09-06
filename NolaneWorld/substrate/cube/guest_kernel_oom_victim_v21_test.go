// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package cube

import (
	"strings"
	"testing"
)

func validV21GuestVictimLabels() map[string]string {
	return map[string]string{
		"sandbox_id":                  "sandbox-a",
		"generation":                  "7",
		"realization_token":           strings.Repeat("71", 32),
		"guest_boot_id":               "11111111-2222-3333-4444-555555555555",
		"victim_tid":                  "42",
		"victim_tgid":                 "41",
		"victim_starttime_ticks":      "9002",
		"main_pid":                    "41",
		"main_starttime_ticks":        "9001",
		"scope":                       "member",
		"event_boot_time_ns":          "150",
		"cgroup_v2_id":                "77",
		"realization_started_boot_ns": "100",
		"outcome_observed_boot_ns":    "200",
		"source":                      "guest.kernel.oom.mark_victim.raw_tracepoint",
	}
}

func TestV21GuestVictimParserAndCorrelationAreExact(t *testing.T) {
	proof, err := exactGuestKernelOOMVictimFromSample(validV21GuestVictimLabels(), "1")
	if err != nil {
		t.Fatalf("parse Wave21 proof: %v", err)
	}
	if proof.VictimClass != GuestKernelOOMVictimMember || proof.TID != 42 || proof.TGID != 41 || proof.CgroupV2ID != 77 || proof.MainPID != 41 || proof.MainStartTimeTicks != 9001 {
		t.Fatalf("unexpected Wave21 proof: %+v", proof)
	}
	if !canonicalGuestKernelOOMVictimTokenHex(proof.RealizationTokenHex) {
		t.Fatalf("realization token lost canonical authority: %q", proof.RealizationTokenHex)
	}
	if err := correlateGuestKernelOOMVictim(TaskOutcomeProof{SandboxID: "sandbox-a", Generation: 7}, proof); err != nil {
		t.Fatalf("correlate Wave21 proof: %v", err)
	}
}

func TestV21GuestVictimRejectsDetachedTimingAndGeneration(t *testing.T) {
	labels := validV21GuestVictimLabels()
	labels["event_boot_time_ns"] = "99"
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err == nil {
		t.Fatal("event before realization start must be rejected")
	}

	labels = validV21GuestVictimLabels()
	proof, err := exactGuestKernelOOMVictimFromSample(labels, "1")
	if err != nil {
		t.Fatalf("parse Wave21 proof: %v", err)
	}
	if err := correlateGuestKernelOOMVictim(TaskOutcomeProof{SandboxID: "sandbox-a", Generation: 8}, proof); err == nil {
		t.Fatal("generation mismatch must fail closed")
	}
}

func TestV21MemberRequiresExactCgroupIdentity(t *testing.T) {
	labels := validV21GuestVictimLabels()
	labels["cgroup_v2_id"] = ""
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err == nil {
		t.Fatal("MEMBER proof without exact cgroup-v2 identity must be rejected")
	}

	labels = validV21GuestVictimLabels()
	labels["scope"] = "main"
	labels["victim_starttime_ticks"] = "9001"
	labels["cgroup_v2_id"] = ""
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err != nil {
		t.Fatalf("MAIN proof may have unknown cgroup identity: %v", err)
	}

	labels = validV21GuestVictimLabels()
	labels["scope"] = "main"
	labels["victim_starttime_ticks"] = "9001"
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err != nil {
		t.Fatalf("MAIN proof may preserve exact cgroup identity as additional provenance: %v", err)
	}
}

func TestV21GuestVictimRejectsNonCanonicalToken(t *testing.T) {
	labels := validV21GuestVictimLabels()
	labels["realization_token"] = strings.Repeat("00", 32)
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err == nil {
		t.Fatal("all-zero realization token must fail closed")
	}
	labels = validV21GuestVictimLabels()
	labels["realization_token"] = strings.ToUpper(labels["realization_token"])
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err == nil {
		t.Fatal("non-lowercase realization token must fail closed")
	}
}
