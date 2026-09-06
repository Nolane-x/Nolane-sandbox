// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package resourcemetrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV21PrometheusPreservesNormativeAuthorityLabels(t *testing.T) {
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string)) {
		v(
			"sandbox-authority",
			7,
			"11111111-2222-3333-4444-555555555555",
			42,
			41,
			9001,
			150,
			77,
			"MAIN",
			100,
			200,
			"guest.kernel.oom.mark_victim.raw_tracepoint",
		)
	}}

	h := newPrometheusHandlerWithGuestKernelVictims(nil, visitor, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	for _, want := range []string{
		`realization_token="`,
		`main_pid="`,
		`main_starttime_ticks="`,
		`scope="main"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Wave21 metric dropped normative authority label %q: %s", want, body)
		}
	}
	if strings.Contains(body, `victim_class=`) {
		t.Fatalf("Wave21 metric retained non-normative victim_class label: %s", body)
	}
}
