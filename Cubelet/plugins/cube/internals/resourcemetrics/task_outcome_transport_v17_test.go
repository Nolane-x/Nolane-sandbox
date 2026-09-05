package resourcemetrics

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	cubesandbox "github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/sandbox"
)

type fakeTaskOutcomeProofLister struct {
	proofs []cubesandbox.TaskOutcomeProof
}

func (f fakeTaskOutcomeProofLister) VisitTaskOutcomeProofs(visit func(sandboxID string, generation uint64, exitCode uint32, exitedAt time.Time, source string)) {
	for _, proof := range f.proofs {
		visit(proof.SandboxID, proof.Generation, proof.ExitCode, proof.ExitedAt, string(proof.Source))
	}
}

func TestTaskOutcomeTransportExportsExactInfoWithoutResourceCache(t *testing.T) {
	exitedAt := time.Date(2026, 9, 5, 4, 5, 6, 123456789, time.UTC)
	lister := fakeTaskOutcomeProofLister{proofs: []cubesandbox.TaskOutcomeProof{{
		SandboxID:  "sandbox-a",
		Generation: math.MaxUint64,
		ExitCode:   137,
		ExitedAt:   exitedAt,
		Source:     cubesandbox.TaskOutcomeProofSourceWait,
	}}}

	handler := newPrometheusHandlerWithTaskOutcomes(nil, lister, func() time.Time {
		return time.Date(2026, 9, 5, 4, 6, 0, 0, time.UTC)
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics/resource", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()

	for _, want := range []string{
		"cubesandbox_task_outcome_info",
		`sandbox_id="sandbox-a"`,
		`generation="` + strconv.FormatUint(math.MaxUint64, 10) + `"`,
		`source="containerd.task.wait"`,
		`exit_code="137"`,
		`exited_at="2026-09-05T04:05:06.123456789Z"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("task-outcome exposition missing %q:\n%s", want, body)
		}
	}
}

func TestTaskOutcomeTransportOmitsUnknownProof(t *testing.T) {
	handler := newPrometheusHandlerWithTaskOutcomes(nil, fakeTaskOutcomeProofLister{}, time.Now)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics/resource", nil))
	if strings.Contains(recorder.Body.String(), "cubesandbox_task_outcome_info") {
		t.Fatalf("unknown proof emitted task-outcome info metric:\n%s", recorder.Body.String())
	}
}
