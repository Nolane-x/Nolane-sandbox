// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package cube

import "testing"

func validV21GuestVictimLabels() map[string]string {
	return map[string]string{
		"sandbox_id":                  "sandbox-a",
		"generation":                  "7",
		"guest_boot_id":               "11111111-2222-3333-4444-555555555555",
		"tid":                         "42",
		"tgid":                        "41",
		"starttime_ticks":             "9001",
		"event_boot_ns":               "150",
		"cgroup_v2_id":                "77",
		"victim_class":                "MEMBER",
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
	if proof.VictimClass != GuestKernelOOMVictimMember || proof.TID != 42 || proof.TGID != 41 || proof.CgroupV2ID != 77 {
		t.Fatalf("unexpected Wave21 proof: %+v", proof)
	}
	if err := correlateGuestKernelOOMVictim(TaskOutcomeProof{SandboxID: "sandbox-a", Generation: 7}, proof); err != nil {
		t.Fatalf("correlate Wave21 proof: %v", err)
	}
}

func TestV21GuestVictimRejectsDetachedTimingAndGeneration(t *testing.T) {
	labels := validV21GuestVictimLabels()
	labels["event_boot_ns"] = "99"
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
	labels["victim_class"] = "MAIN"
	labels["cgroup_v2_id"] = ""
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err != nil {
		t.Fatalf("MAIN proof may have unknown cgroup identity: %v", err)
	}
}
