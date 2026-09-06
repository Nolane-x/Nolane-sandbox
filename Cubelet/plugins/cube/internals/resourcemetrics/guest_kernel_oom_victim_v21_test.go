// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package resourcemetrics

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeV21GuestVictimVisitor struct {
	visit func(func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string))
}

func (f fakeV21GuestVictimVisitor) VisitGuestKernelOOMVictimAuthorityProofs(v func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string)) {
	if f.visit != nil {
		f.visit(v)
	}
}

func TestV21PrometheusPreservesExactGuestVictimProof(t *testing.T) {
	token := strings.Repeat("71", 32)
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string)) {
		v(
			"sandbox-a",
			math.MaxUint64,
			token,
			"11111111-2222-3333-4444-555555555555",
			math.MaxUint32,
			4247,
			math.MaxUint64,
			4247,
			math.MaxUint64,
			"main",
			math.MaxUint64,
			0,
			1,
			math.MaxUint64,
			"guest.kernel.oom.mark_victim.raw_tracepoint",
		)
	}}
	h := newPrometheusHandlerWithGuestKernelVictims(nil, visitor, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()
	for _, want := range []string{
		"cubesandbox_guest_kernel_oom_victim_info",
		`generation="18446744073709551615"`,
		`realization_token="` + token + `"`,
		`victim_tid="4294967295"`,
		`victim_tgid="4247"`,
		`victim_starttime_ticks="18446744073709551615"`,
		`main_pid="4247"`,
		`main_starttime_ticks="18446744073709551615"`,
		`scope="main"`,
		`event_boot_time_ns="18446744073709551615"`,
		`cgroup_v2_id=""`,
		`outcome_observed_boot_ns="18446744073709551615"`,
		`source="guest.kernel.oom.mark_victim.raw_tracepoint"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	for _, forbidden := range []string{`victim_class=`, ` tid=`, ` tgid=`, ` starttime_ticks=`, `event_boot_ns=`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("non-normative Wave21 label %q leaked in %s", forbidden, body)
		}
	}
}

func TestV21PrometheusMainUnknownCgroupUsesEmptyID(t *testing.T) {
	token := strings.Repeat("71", 32)
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string)) {
		v("sandbox-a", 7, token, "11111111-2222-3333-4444-555555555555", 42, 42, 99, 42, 99, "main", 150, 0, 100, 200, "guest.kernel.oom.mark_victim.raw_tracepoint")
	}}
	h := newPrometheusHandlerWithGuestKernelVictims(nil, visitor, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rr.Body.String(), `cgroup_v2_id=""`) {
		t.Fatalf("unknown MAIN cgroup must use empty label: %s", rr.Body.String())
	}
}

func TestV21PrometheusRejectsMainLifetimeMismatch(t *testing.T) {
	token := strings.Repeat("71", 32)
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string)) {
		v("sandbox-a", 7, token, "11111111-2222-3333-4444-555555555555", 42, 42, 99, 42, 100, "main", 150, 0, 100, 200, "guest.kernel.oom.mark_victim.raw_tracepoint")
	}}
	h := newPrometheusHandlerWithGuestKernelVictims(nil, visitor, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(rr.Body.String(), "cubesandbox_guest_kernel_oom_victim_info{") {
		t.Fatalf("mismatched MAIN lifetime must fail closed: %s", rr.Body.String())
	}
}
