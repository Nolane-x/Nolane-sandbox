package resourcemetrics

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV21ProductionTaskEvidenceServiceRegistersGuestVictims(t *testing.T) {
	visitor := fakeV21GuestVictimVisitor{visit: func(v func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string)) {
		v(
			"sandbox-production",
			math.MaxUint64,
			"11111111-2222-3333-4444-555555555555",
			42,
			41,
			9001,
			150,
			77,
			"MEMBER",
			100,
			200,
			"guest.kernel.oom.mark_victim.raw_tracepoint",
		)
	}}

	service := newProductionTaskEvidenceService(nil, nil, nil, nil, nil, visitor)
	rr := httptest.NewRecorder()
	service.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "cubesandbox_guest_kernel_oom_victim_info") || !strings.Contains(body, `sandbox_id="sandbox-production"`) {
		t.Fatalf("production evidence service omitted Wave21 metric: %s", body)
	}
}
