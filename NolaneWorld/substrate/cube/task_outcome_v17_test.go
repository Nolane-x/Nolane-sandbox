package cube

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const exactTaskOutcomeMetricFixture = `# HELP cubesandbox_task_outcome_info Exact containerd task-outcome proof accepted by the Cube sandbox controller.
# TYPE cubesandbox_task_outcome_info gauge
cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="18446744073709551615",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T04:05:06.123456789Z"} 1
`

func observeTaskOutcomeFixture(t *testing.T, metrics, sandboxID string) (TaskOutcomeProof, bool, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != hostResourceMetricsPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, hostResourceMetricsPath)
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(metrics))
	}))
	defer server.Close()

	observer, err := NewTaskOutcomeObserver(TaskOutcomeConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewTaskOutcomeObserver: %v", err)
	}
	return observer.Observe(t.Context(), ResourceBinding{sandboxID: sandboxID})
}

func TestTaskOutcomeObserverPreservesExactProofBeyondBinary64Range(t *testing.T) {
	proof, ok, err := observeTaskOutcomeFixture(t, exactTaskOutcomeMetricFixture, "sandbox-123")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !ok {
		t.Fatal("Observe returned unknown for exact task-outcome metric")
	}
	if proof.SandboxID != "sandbox-123" {
		t.Fatalf("SandboxID = %q", proof.SandboxID)
	}
	if proof.Generation != math.MaxUint64 {
		t.Fatalf("Generation = %d, want %d", proof.Generation, uint64(math.MaxUint64))
	}
	if proof.ExitCode != 137 {
		t.Fatalf("ExitCode = %d, want 137", proof.ExitCode)
	}
	if proof.Source != TaskOutcomeProofSourceWait {
		t.Fatalf("Source = %q, want %q", proof.Source, TaskOutcomeProofSourceWait)
	}
	wantTime := time.Date(2026, 9, 5, 4, 5, 6, 123456789, time.UTC)
	if !proof.ExitedAt.Equal(wantTime) || proof.ExitedAt.Location() != time.UTC {
		t.Fatalf("ExitedAt = %v, want exact UTC %v", proof.ExitedAt, wantTime)
	}
}

func TestTaskOutcomeObserverKeepsMissingProofUnknown(t *testing.T) {
	proof, ok, err := observeTaskOutcomeFixture(t, "# no task outcome\n", "sandbox-123")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ok || proof != (TaskOutcomeProof{}) {
		t.Fatalf("missing proof = (%#v, %v), want zero,false", proof, ok)
	}
}

func TestTaskOutcomeObserverIgnoresOtherSandboxProof(t *testing.T) {
	proof, ok, err := observeTaskOutcomeFixture(t, exactTaskOutcomeMetricFixture, "sandbox-other")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ok || proof != (TaskOutcomeProof{}) {
		t.Fatalf("other-sandbox proof = (%#v, %v), want zero,false", proof, ok)
	}
}

func TestTaskOutcomeObserverRejectsDuplicateMatchingProof(t *testing.T) {
	metrics := exactTaskOutcomeMetricFixture + `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="7",source="containerd.task.state",exit_code="0",exited_at="2026-09-05T05:00:00Z"} 1
`
	_, _, err := observeTaskOutcomeFixture(t, metrics, "sandbox-123")
	if !errors.Is(err, ErrTaskOutcomeUnavailable) {
		t.Fatalf("duplicate proof error = %v, want ErrTaskOutcomeUnavailable", err)
	}
}

func TestTaskOutcomeObserverRejectsMalformedMatchingProof(t *testing.T) {
	tests := map[string]string{
		"missing generation": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T04:05:06Z"} 1` + "\n",
		"extra label": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="1",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T04:05:06Z",reason="invented"} 1` + "\n",
		"zero generation": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="0",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T04:05:06Z"} 1` + "\n",
		"signed generation": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="+1",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T04:05:06Z"} 1` + "\n",
		"exit overflow": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="1",source="containerd.task.wait",exit_code="4294967296",exited_at="2026-09-05T04:05:06Z"} 1` + "\n",
		"unsupported source": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="1",source="cubebox.status",exit_code="137",exited_at="2026-09-05T04:05:06Z"} 1` + "\n",
		"invalid timestamp": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="1",source="containerd.task.wait",exit_code="137",exited_at="not-a-time"} 1` + "\n",
		"zero timestamp": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="1",source="containerd.task.wait",exit_code="137",exited_at="0001-01-01T00:00:00Z"} 1` + "\n",
		"non-unit value": `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="1",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T04:05:06Z"} 2` + "\n",
	}

	for name, metrics := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := observeTaskOutcomeFixture(t, metrics, "sandbox-123")
			if !errors.Is(err, ErrTaskOutcomeUnavailable) {
				t.Fatalf("error = %v, want ErrTaskOutcomeUnavailable", err)
			}
		})
	}
}
