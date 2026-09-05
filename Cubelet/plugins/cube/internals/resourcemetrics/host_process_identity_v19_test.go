package resourcemetrics

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeHostProcessIdentityVisitor struct {
	visit func(func(string, uint64, uint32, uint64, string, string, string, string, time.Time, time.Time))
}

func (f fakeHostProcessIdentityVisitor) VisitHostProcessIdentityProofs(v func(string, uint64, uint32, uint64, string, string, string, string, time.Time, time.Time)) {
	if f.visit != nil {
		f.visit(v)
	}
}

func TestHostProcessIdentityPrometheusTransportPreservesExactIntegers(t *testing.T) {
	placedAt := time.Date(2026, 9, 5, 6, 0, 0, 123456789, time.UTC)
	boundAt := placedAt.Add(time.Nanosecond)
	proofs := fakeHostProcessIdentityVisitor{visit: func(v func(string, uint64, uint32, uint64, string, string, string, string, time.Time, time.Time)) {
		v(
			"sandbox-a",
			math.MaxUint64,
			math.MaxUint32,
			math.MaxUint64,
			"11111111-2222-3333-8444-555555555555",
			"/cube_sandbox_v1/42",
			"cube-shim-vmm",
			"cubebox.cgroup.add_proc",
			placedAt,
			boundAt,
		)
	}}

	h := newPrometheusHandlerWithAllTaskEvidence(nil, nil, nil, proofs, time.Now)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`cubesandbox_host_process_identity_info{`,
		`sandbox_id="sandbox-a"`,
		`generation="18446744073709551615"`,
		`host_pid="4294967295"`,
		`starttime_ticks="18446744073709551615"`,
		`boot_id="11111111-2222-3333-8444-555555555555"`,
		`cgroup_path="/cube_sandbox_v1/42"`,
		`runtime_role="cube-shim-vmm"`,
		`source="cubebox.cgroup.add_proc"`,
		`placed_at="2026-09-05T06:00:00.123456789Z"`,
		`bound_at="2026-09-05T06:00:00.12345679Z"`,
		`} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in metrics:\n%s", want, body)
		}
	}
}

func TestHostProcessIdentityPrometheusTransportOmitsMalformedEvidence(t *testing.T) {
	proofs := fakeHostProcessIdentityVisitor{visit: func(v func(string, uint64, uint32, uint64, string, string, string, string, time.Time, time.Time)) {
		v("sandbox-a", 1, 0, 99, "bad", "/cube/1", "cube-shim-vmm", "cubebox.cgroup.add_proc", time.Now(), time.Now())
	}}
	h := newPrometheusHandlerWithAllTaskEvidence(nil, nil, nil, proofs, time.Now)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(rr.Body.String(), "cubesandbox_host_process_identity_info") {
		t.Fatalf("malformed identity evidence was transported:\n%s", rr.Body.String())
	}
}

func TestHostProcessIdentityTransportIsIndependentOfResourceCache(t *testing.T) {
	proofs := fakeHostProcessIdentityVisitor{visit: func(v func(string, uint64, uint32, uint64, string, string, string, string, time.Time, time.Time)) {
		now := time.Date(2026, 9, 5, 6, 0, 0, 0, time.UTC)
		v("sandbox-a", 7, 1234, 5678, "11111111-2222-3333-8444-555555555555", "/cube/1", "cube-shim-vmm", "cubebox.cgroup.add_proc", now, now)
	}}
	h := newPrometheusHandlerWithAllTaskEvidence(nil, nil, nil, proofs, time.Now)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rr.Body.String(), "cubesandbox_host_process_identity_info") {
		t.Fatalf("identity metric missing with nil resource cache:\n%s", rr.Body.String())
	}
}
