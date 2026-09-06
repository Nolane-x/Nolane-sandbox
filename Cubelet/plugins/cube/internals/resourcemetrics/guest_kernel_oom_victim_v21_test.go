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
	visit func(func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string))
}

func (f fakeV21GuestVictimVisitor) VisitGuestKernelOOMVictimProofs(v func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string)) {
	if f.visit != nil {
		f.visit(v)
	}
}

func TestV21PrometheusPreservesExactGuestVictimProof(t *testing.T) {
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string)) {
		v(
			"sandbox-a",
			math.MaxUint64,
			"11111111-2222-3333-4444-555555555555",
			math.MaxUint32,
			4247,
			math.MaxUint64,
			math.MaxUint64,
			math.MaxUint64,
			"MAIN",
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
		`tid="4294967295"`,
		`tgid="4247"`,
		`starttime_ticks="18446744073709551615"`,
		`event_boot_ns="18446744073709551615"`,
		`cgroup_v2_id="18446744073709551615"`,
		`victim_class="MAIN"`,
		`outcome_observed_boot_ns="18446744073709551615"`,
		`source="guest.kernel.oom.mark_victim.raw_tracepoint"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestV21PrometheusMainUnknownCgroupUsesEmptyID(t *testing.T) {
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string)) {
		v("sandbox-a", 7, "11111111-2222-3333-4444-555555555555", 42, 42, 99, 150, 0, "MAIN", 100, 200, "guest.kernel.oom.mark_victim.raw_tracepoint")
	}}
	h := newPrometheusHandlerWithGuestKernelVictims(nil, visitor, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rr.Body.String(), `cgroup_v2_id=""`) {
		t.Fatalf("unknown MAIN cgroup must use empty label: %s", rr.Body.String())
	}
}
