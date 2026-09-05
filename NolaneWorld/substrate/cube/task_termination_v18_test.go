package cube

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const exactTaskTerminationMetricFixture = `# TYPE cubesandbox_task_outcome_info gauge
cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="18446744073709551615",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T05:00:59.555555555Z"} 1
# TYPE cubesandbox_task_realization_oom_info gauge
cubesandbox_task_realization_oom_info{sandbox_id="sandbox-123",generation="18446744073709551615",cgroup_path="/cube/path",signal="kernel.cgroup.memory.oom_kill",baseline_oom_kills="18446744073709551613",final_oom_kills="18446744073709551615",oom_kills="2",baseline_at="2026-09-05T05:00:00.123456789Z",observed_at="2026-09-05T05:01:00.987654321Z",exited_at="2026-09-05T05:00:59.555555555Z",outcome_source="containerd.task.wait"} 1
`

func observeTaskTerminationFixture(t *testing.T, metrics, sandboxID string) (TaskTerminationEvidence, bool, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != hostResourceMetricsPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, hostResourceMetricsPath)
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(metrics))
	}))
	defer server.Close()

	observer, err := NewTaskTerminationObserver(TaskOutcomeConfig{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewTaskTerminationObserver: %v", err)
	}
	return observer.Observe(context.Background(), ResourceBinding{sandboxID: sandboxID})
}

func TestTaskTerminationObserverCorrelatesExactRealizationOOMProof(t *testing.T) {
	evidence, known, err := observeTaskTerminationFixture(t, exactTaskTerminationMetricFixture, "sandbox-123")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !known {
		t.Fatal("exact task termination evidence returned unknown")
	}
	if evidence.Outcome.Generation != math.MaxUint64 || evidence.Outcome.ExitCode != 137 || evidence.Outcome.Source != TaskOutcomeProofSourceWait {
		t.Fatalf("outcome = %#v", evidence.Outcome)
	}
	if evidence.RealizationOOM == nil {
		t.Fatal("exact realization OOM proof was dropped")
	}
	oom := evidence.RealizationOOM
	if oom.Generation != math.MaxUint64 || oom.BaselineOOMKills != math.MaxUint64-2 || oom.FinalOOMKills != math.MaxUint64 || oom.OOMKills != 2 {
		t.Fatalf("OOM counters = %#v", oom)
	}
	if oom.CGroupPath != "/cube/path" || oom.Signal != "kernel.cgroup.memory.oom_kill" || oom.OutcomeSource != TaskOutcomeProofSourceWait {
		t.Fatalf("OOM provenance = %#v", oom)
	}
	if !oom.ExitedAt.Equal(evidence.Outcome.ExitedAt) {
		t.Fatalf("OOM exit %v != outcome exit %v", oom.ExitedAt, evidence.Outcome.ExitedAt)
	}
	observed, knownOOM := evidence.KernelOOMObservedDuringRealization()
	if !knownOOM || !observed {
		t.Fatalf("KernelOOMObservedDuringRealization = %v,%v, want true,true", observed, knownOOM)
	}
}

func TestTaskTerminationExit137AloneDoesNotBecomeOOMEvidence(t *testing.T) {
	metrics := `cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="9",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T05:00:59.555555555Z"} 1
`
	evidence, known, err := observeTaskTerminationFixture(t, metrics, "sandbox-123")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !known || evidence.Outcome.ExitCode != 137 {
		t.Fatalf("outcome = %#v known=%v", evidence.Outcome, known)
	}
	if evidence.RealizationOOM != nil {
		t.Fatalf("exit 137 fabricated realization OOM evidence: %#v", evidence.RealizationOOM)
	}
	observed, knownOOM := evidence.KernelOOMObservedDuringRealization()
	if observed || knownOOM {
		t.Fatalf("exit 137 helper = %v,%v, want false,false", observed, knownOOM)
	}
}

func TestTaskTerminationExactZeroOOMDeltaIsKnownFalse(t *testing.T) {
	metrics := strings.Replace(exactTaskTerminationMetricFixture,
		`baseline_oom_kills="18446744073709551613",final_oom_kills="18446744073709551615",oom_kills="2"`,
		`baseline_oom_kills="8",final_oom_kills="8",oom_kills="0"`, 1)
	evidence, known, err := observeTaskTerminationFixture(t, metrics, "sandbox-123")
	if err != nil || !known || evidence.RealizationOOM == nil {
		t.Fatalf("Observe = %#v,%v,%v", evidence, known, err)
	}
	observed, knownOOM := evidence.KernelOOMObservedDuringRealization()
	if observed || !knownOOM {
		t.Fatalf("zero-delta helper = %v,%v, want false,true", observed, knownOOM)
	}
}

func TestTaskTerminationRejectsDetachedOrMismatchedRealizationOOMProof(t *testing.T) {
	oomLine := `cubesandbox_task_realization_oom_info{sandbox_id="sandbox-123",generation="9",cgroup_path="/cube/path",signal="kernel.cgroup.memory.oom_kill",baseline_oom_kills="1",final_oom_kills="2",oom_kills="1",baseline_at="2026-09-05T05:00:00Z",observed_at="2026-09-05T05:01:00Z",exited_at="2026-09-05T05:00:59Z",outcome_source="containerd.task.wait"} 1` + "\n"
	if _, _, err := observeTaskTerminationFixture(t, oomLine, "sandbox-123"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
		t.Fatalf("detached OOM proof error = %v, want ErrTaskOutcomeUnavailable", err)
	}

	tests := map[string]string{
		"generation": strings.Replace(exactTaskTerminationMetricFixture, `generation="18446744073709551615",cgroup_path`, `generation="7",cgroup_path`, 1),
		"outcome source": strings.Replace(exactTaskTerminationMetricFixture, `outcome_source="containerd.task.wait"`, `outcome_source="containerd.task.state"`, 1),
		"exit timestamp": strings.Replace(exactTaskTerminationMetricFixture, `exited_at="2026-09-05T05:00:59.555555555Z",outcome_source`, `exited_at="2026-09-05T05:00:58.555555555Z",outcome_source`, 1),
	}
	for name, metrics := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := observeTaskTerminationFixture(t, metrics, "sandbox-123"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
				t.Fatalf("error = %v, want ErrTaskOutcomeUnavailable", err)
			}
		})
	}
}

func TestTaskTerminationRejectsMalformedRealizationOOMProof(t *testing.T) {
	tests := map[string]string{
		"duplicate": exactTaskTerminationMetricFixture + `cubesandbox_task_realization_oom_info{sandbox_id="sandbox-123",generation="18446744073709551615",cgroup_path="/cube/path",signal="kernel.cgroup.memory.oom_kill",baseline_oom_kills="1",final_oom_kills="1",oom_kills="0",baseline_at="2026-09-05T05:00:00Z",observed_at="2026-09-05T05:01:00Z",exited_at="2026-09-05T05:00:59.555555555Z",outcome_source="containerd.task.wait"} 1` + "\n",
		"unsupported signal": strings.Replace(exactTaskTerminationMetricFixture, `signal="kernel.cgroup.memory.oom_kill"`, `signal="invented"`, 1),
		"blank path": strings.Replace(exactTaskTerminationMetricFixture, `cgroup_path="/cube/path"`, `cgroup_path=" "`, 1),
		"counter regression": strings.Replace(exactTaskTerminationMetricFixture, `baseline_oom_kills="18446744073709551613",final_oom_kills="18446744073709551615",oom_kills="2"`, `baseline_oom_kills="9",final_oom_kills="8",oom_kills="0"`, 1),
		"wrong delta": strings.Replace(exactTaskTerminationMetricFixture, `oom_kills="2"`, `oom_kills="3"`, 1),
		"baseline after exit": strings.Replace(exactTaskTerminationMetricFixture, `baseline_at="2026-09-05T05:00:00.123456789Z"`, `baseline_at="2026-09-05T05:01:00Z"`, 1),
		"observation before exit": strings.Replace(exactTaskTerminationMetricFixture, `observed_at="2026-09-05T05:01:00.987654321Z"`, `observed_at="2026-09-05T05:00:00Z"`, 1),
		"non canonical uint": strings.Replace(exactTaskTerminationMetricFixture, `oom_kills="2"`, `oom_kills="02"`, 1),
		"non unit value": strings.Replace(exactTaskTerminationMetricFixture, `outcome_source="containerd.task.wait"} 1`, `outcome_source="containerd.task.wait"} 2`, 1),
		"extra label": strings.Replace(exactTaskTerminationMetricFixture, `outcome_source="containerd.task.wait"} 1`, `outcome_source="containerd.task.wait",victim="main"} 1`, 1),
	}
	for name, metrics := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := observeTaskTerminationFixture(t, metrics, "sandbox-123"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
				t.Fatalf("error = %v, want ErrTaskOutcomeUnavailable", err)
			}
		})
	}
}

func TestTaskTerminationObserverIgnoresOtherSandboxEvidence(t *testing.T) {
	evidence, known, err := observeTaskTerminationFixture(t, exactTaskTerminationMetricFixture, "sandbox-other")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if known || evidence != (TaskTerminationEvidence{}) {
		t.Fatalf("other sandbox = %#v,%v, want zero,false", evidence, known)
	}
}

func TestTaskTerminationTimestampsPreserveNanoseconds(t *testing.T) {
	evidence, known, err := observeTaskTerminationFixture(t, exactTaskTerminationMetricFixture, "sandbox-123")
	if err != nil || !known || evidence.RealizationOOM == nil {
		t.Fatalf("Observe = %#v,%v,%v", evidence, known, err)
	}
	wantBaseline := time.Date(2026, 9, 5, 5, 0, 0, 123456789, time.UTC)
	wantObserved := time.Date(2026, 9, 5, 5, 1, 0, 987654321, time.UTC)
	if !evidence.RealizationOOM.BaselineAt.Equal(wantBaseline) || !evidence.RealizationOOM.ObservedAt.Equal(wantObserved) {
		t.Fatalf("timestamps = %v / %v", evidence.RealizationOOM.BaselineAt, evidence.RealizationOOM.ObservedAt)
	}
}
