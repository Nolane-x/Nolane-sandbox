// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package cube

import (
	"reflect"
	"strings"
	"testing"
)

func normativeV21GuestVictimLabels() map[string]string {
	return map[string]string{
		"sandbox_id":                  "sandbox-a",
		"generation":                  "7",
		"realization_token":           strings.Repeat("71", 32),
		"guest_boot_id":               "11111111-2222-3333-4444-555555555555",
		"victim_tid":                  "42",
		"victim_tgid":                 "41",
		"victim_starttime_ticks":      "9001",
		"main_pid":                    "41",
		"main_starttime_ticks":        "9001",
		"scope":                       "main",
		"event_boot_time_ns":          "150",
		"cgroup_v2_id":                "77",
		"realization_started_boot_ns": "100",
		"outcome_observed_boot_ns":    "200",
		"source":                      "guest.kernel.oom.mark_victim.raw_tracepoint",
	}
}

func TestV21NormativeGuestVictimMetricCarriesFullAuthority(t *testing.T) {
	proof, err := exactGuestKernelOOMVictimFromSample(normativeV21GuestVictimLabels(), "1")
	if err != nil {
		t.Fatalf("parse normative Wave21 sample: %v", err)
	}

	typ := reflect.TypeOf(proof)
	for _, name := range []string{"RealizationTokenHex", "MainPID", "MainStartTimeTicks"} {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("Wave21 consumer proof dropped authority field %s", name)
		}
	}
}

func TestV21NormativeGuestVictimMetricRejectsMixedTokenAndMainAuthority(t *testing.T) {
	base, err := exactGuestKernelOOMVictimFromSample(normativeV21GuestVictimLabels(), "1")
	if err != nil {
		t.Fatalf("parse base Wave21 sample: %v", err)
	}

	labels := normativeV21GuestVictimLabels()
	labels["realization_token"] = strings.Repeat("72", 32)
	other, err := exactGuestKernelOOMVictimFromSample(labels, "1")
	if err != nil {
		t.Fatalf("parse second Wave21 sample: %v", err)
	}

	if sameGuestKernelOOMVictimAuthority(base, other) {
		t.Fatal("mixed realization token was treated as one Wave21 authority")
	}

	labels = normativeV21GuestVictimLabels()
	labels["main_pid"] = "99"
	if _, err := exactGuestKernelOOMVictimFromSample(labels, "1"); err == nil {
		t.Fatal("MAIN victim whose TGID does not match exact main PID was accepted")
	}
}
