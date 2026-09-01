package cube

import (
	"strings"
	"testing"
)

const v15OOMHostMetricsFixture = v14ExactCPUHostMetricsFixture + `cubesandbox_host_sandbox_memory_oom_kills_total{sandbox_id="sandbox-123"} 4
`

func TestV15ObserverPreservesExactOOMKillEvidence(t *testing.T) {
	snapshot, err := observeV14Fixture(t, v15OOMHostMetricsFixture)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if snapshot.MemoryOOMKills == nil || *snapshot.MemoryOOMKills != 4 {
		t.Fatalf("MemoryOOMKills = %v, want pointer to 4", snapshot.MemoryOOMKills)
	}
}

func TestV15ObserverKeepsMissingOOMKillEvidenceUnknown(t *testing.T) {
	snapshot, err := observeV14Fixture(t, v14ExactCPUHostMetricsFixture)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if snapshot.MemoryOOMKills != nil {
		t.Fatalf("MemoryOOMKills = %v, want nil when producer evidence is absent", *snapshot.MemoryOOMKills)
	}
}

func TestV15ObserverRejectsDuplicateOOMKillEvidence(t *testing.T) {
	metrics := v15OOMHostMetricsFixture + `cubesandbox_host_sandbox_memory_oom_kills_total{sandbox_id="sandbox-123"} 5
`
	if _, err := observeV14Fixture(t, metrics); err == nil {
		t.Fatal("duplicate OOM-kill evidence must fail closed")
	}
}

func TestV15ObserverRejectsFractionalOOMKillEvidence(t *testing.T) {
	metrics := strings.Replace(v15OOMHostMetricsFixture,
		`cubesandbox_host_sandbox_memory_oom_kills_total{sandbox_id="sandbox-123"} 4`,
		`cubesandbox_host_sandbox_memory_oom_kills_total{sandbox_id="sandbox-123"} 4.5`, 1)
	if _, err := observeV14Fixture(t, metrics); err == nil {
		t.Fatal("fractional OOM-kill evidence must fail closed")
	}
}
