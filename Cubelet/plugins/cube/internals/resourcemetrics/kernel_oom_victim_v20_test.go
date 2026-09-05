package resourcemetrics

import (
	"io"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeV20VictimVisitor struct {
	visit func(func(string, uint64, string, uint32, uint32, uint64, string, uint64, uint64, bool, string))
}

func (f fakeV20VictimVisitor) VisitHostKernelOOMVictimProofs(v func(string, uint64, string, uint32, uint32, uint64, string, uint64, uint64, bool, string)) {
	if f.visit != nil { f.visit(v) }
}

func TestV20PrometheusPreservesExactVictimProof(t *testing.T) {
	visitor := fakeV20VictimVisitor{visit: func(v func(string, uint64, string, uint32, uint32, uint64, string, uint64, uint64, bool, string)) {
		v("sandbox-a", math.MaxUint64, "11111111-2222-3333-4444-555555555555", math.MaxUint32, 4247, math.MaxUint64, "/cube_sandbox_v1/42", math.MaxUint64, math.MaxUint64, true, "kernel.oom.mark_victim.raw_tracepoint")
	}}
	h := newPrometheusHandlerWithKernelVictimEvidence(nil, nil, nil, nil, visitor, nil)
	rr := httptest.NewRecorder(); req := httptest.NewRequest("GET", "/metrics", nil); h.ServeHTTP(rr, req)
	body := rr.Body.String()
	for _, want := range []string{
		"cubesandbox_host_kernel_oom_victim_info",
		`generation="18446744073709551615"`,
		`host_pid="4294967295"`,
		`victim_tid="4247"`,
		`starttime_ticks="18446744073709551615"`,
		`event_boot_time_ns="18446744073709551615"`,
		`cgroup_v2_id="18446744073709551615"`,
		`cgroup_v2_correlated="true"`,
	} {
		if !strings.Contains(body, want) { t.Fatalf("missing %q in %s", want, body) }
	}
}

func TestV20PrometheusUnknownCgroupUsesEmptyID(t *testing.T) {
	visitor := fakeV20VictimVisitor{visit: func(v func(string, uint64, string, uint32, uint32, uint64, string, uint64, uint64, bool, string)) {
		v("sandbox-a", 1, "11111111-2222-3333-4444-555555555555", 42, 43, 1234, "/cube_sandbox_v1/42", 99, 0, false, "kernel.oom.mark_victim.raw_tracepoint")
	}}
	h := newPrometheusHandlerWithKernelVictimEvidence(nil, nil, nil, nil, visitor, nil)
	rr := httptest.NewRecorder(); h.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rr.Result().Body)
	if !strings.Contains(string(body), `cgroup_v2_id=""`) || !strings.Contains(string(body), `cgroup_v2_correlated="false"`) { t.Fatalf("unknown cgroup transport is not exact: %s", body) }
}
