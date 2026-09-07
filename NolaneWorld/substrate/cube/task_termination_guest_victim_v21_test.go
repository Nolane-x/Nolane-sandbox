// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package cube

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func v21OutcomeLine() string {
	return `cubesandbox_task_outcome_info{sandbox_id="sandbox-a",generation="7",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T05:00:59Z"} 1` + "\n"
}

func v21GuestVictimLine(token, scope string, tid, tgid uint32, start, cgroup uint64) string {
	return fmt.Sprintf(
		`cubesandbox_guest_kernel_oom_victim_info{sandbox_id="sandbox-a",generation="7",realization_token="%s",guest_boot_id="11111111-2222-3333-4444-555555555555",victim_tid="%d",victim_tgid="%d",victim_starttime_ticks="%d",main_pid="41",main_starttime_ticks="9001",scope="%s",event_boot_time_ns="150",cgroup_v2_id="%s",realization_started_boot_ns="100",outcome_observed_boot_ns="200",source="guest.kernel.oom.mark_victim.raw_tracepoint"} 1`+"\n",
		token,
		tid,
		tgid,
		start,
		scope,
		func() string {
			if cgroup == 0 {
				return ""
			}
			return fmt.Sprintf("%d", cgroup)
		}(),
	)
}

func TestV21TaskTerminationFusesMultipleGuestVictimsFromSameScrape(t *testing.T) {
	token := strings.Repeat("71", 32)
	metrics := v21OutcomeLine() +
		v21GuestVictimLine(token, "main", 42, 41, 9001, 0) +
		v21GuestVictimLine(token, "member", 43, 43, 9002, 77)

	evidence, known, err := observeTaskTerminationFixture(t, metrics, "sandbox-a")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !known {
		t.Fatal("exact Wave21 evidence returned unknown")
	}
	proofs := evidence.GuestKernelOOMVictimProofs()
	if len(proofs) != 2 {
		t.Fatalf("guest victim count = %d, want 2", len(proofs))
	}
	if marked, known := evidence.GuestKernelOOMVictimMarked(); !marked || !known {
		t.Fatalf("GuestKernelOOMVictimMarked = %v,%v, want true,true", marked, known)
	}
	if marked, known := evidence.GuestMainKernelOOMVictimMarked(); !marked || !known {
		t.Fatalf("GuestMainKernelOOMVictimMarked = %v,%v, want true,true", marked, known)
	}
}

func TestV21TaskTerminationRejectsMixedGuestVictimAuthority(t *testing.T) {
	metrics := v21OutcomeLine() +
		v21GuestVictimLine(strings.Repeat("71", 32), "main", 42, 41, 9001, 0) +
		v21GuestVictimLine(strings.Repeat("72", 32), "member", 43, 43, 9002, 77)

	if _, _, err := observeTaskTerminationFixture(t, metrics, "sandbox-a"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
		t.Fatalf("mixed token error = %v, want ErrTaskOutcomeUnavailable", err)
	}
}

func TestV21TaskTerminationGuestVictimRequiresExactWave17Outcome(t *testing.T) {
	metrics := v21GuestVictimLine(strings.Repeat("71", 32), "member", 43, 43, 9002, 77)
	if _, _, err := observeTaskTerminationFixture(t, metrics, "sandbox-a"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
		t.Fatalf("detached guest proof error = %v, want ErrTaskOutcomeUnavailable", err)
	}
}

func TestV21TaskTerminationGuestVictimAbsenceRemainsUnknown(t *testing.T) {
	evidence, known, err := observeTaskTerminationFixture(t, v21OutcomeLine(), "sandbox-a")
	if err != nil || !known {
		t.Fatalf("Observe = %#v,%v,%v", evidence, known, err)
	}
	if marked, claimKnown := evidence.GuestKernelOOMVictimMarked(); marked || claimKnown {
		t.Fatalf("GuestKernelOOMVictimMarked = %v,%v, want false,false", marked, claimKnown)
	}
	if marked, claimKnown := evidence.GuestMainKernelOOMVictimMarked(); marked || claimKnown {
		t.Fatalf("GuestMainKernelOOMVictimMarked = %v,%v, want false,false", marked, claimKnown)
	}
}
