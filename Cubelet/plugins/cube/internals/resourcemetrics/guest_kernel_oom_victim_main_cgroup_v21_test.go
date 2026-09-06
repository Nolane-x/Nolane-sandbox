// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package resourcemetrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV21PrometheusMainMayPreserveAdditionalCgroupAuthority(t *testing.T) {
	token := strings.Repeat("71", 32)
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string)) {
		v("sandbox-a", 7, token, "11111111-2222-3333-4444-555555555555", 42, 41, 9001, 41, 9001, "main", 150, 77, 100, 200, "guest.kernel.oom.mark_victim.raw_tracepoint")
	}}
	h := newPrometheusHandlerWithGuestKernelVictims(nil, visitor, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()
	if !strings.Contains(body, `scope="main"`) || !strings.Contains(body, `cgroup_v2_id="77"`) {
		t.Fatalf("MAIN additional cgroup authority was dropped: %s", body)
	}
}
