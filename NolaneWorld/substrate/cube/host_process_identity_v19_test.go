package cube

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

const exactHostProcessIdentityMetricFixture = `# TYPE cubesandbox_task_outcome_info gauge
cubesandbox_task_outcome_info{sandbox_id="sandbox-123",generation="18446744073709551615",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T05:00:59.555555555Z"} 1
# TYPE cubesandbox_task_realization_oom_info gauge
cubesandbox_task_realization_oom_info{sandbox_id="sandbox-123",generation="18446744073709551615",cgroup_path="/cube/path",signal="kernel.cgroup.memory.oom_kill",baseline_oom_kills="8",final_oom_kills="9",oom_kills="1",baseline_at="2026-09-05T05:00:00.123456789Z",observed_at="2026-09-05T05:01:00.987654321Z",exited_at="2026-09-05T05:00:59.555555555Z",outcome_source="containerd.task.wait"} 1
# TYPE cubesandbox_host_process_identity_info gauge
cubesandbox_host_process_identity_info{sandbox_id="sandbox-123",generation="18446744073709551615",host_pid="4294967295",starttime_ticks="18446744073709551615",boot_id="11111111-2222-3333-8444-555555555555",cgroup_path="/cube/path",runtime_role="cube-shim-vmm",source="cubebox.cgroup.add_proc",placed_at="2026-09-05T04:59:59.123456789Z",bound_at="2026-09-05T05:00:00.223456789Z"} 1
`

func TestTaskTerminationCorrelatesExactHostProcessIdentity(t *testing.T) {
	evidence, known, err := observeTaskTerminationFixture(t, exactHostProcessIdentityMetricFixture, "sandbox-123")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !known {
		t.Fatal("exact task termination evidence returned unknown")
	}
	if evidence.HostProcessIdentity == nil {
		t.Fatal("exact host-process identity proof was dropped")
	}
	proof := evidence.HostProcessIdentity
	if proof.SandboxID != "sandbox-123" || proof.Generation != math.MaxUint64 {
		t.Fatalf("identity scope = %#v", proof)
	}
	if proof.HostPID != math.MaxUint32 || proof.StartTimeTicks != math.MaxUint64 {
		t.Fatalf("identity integer fields = %#v", proof)
	}
	if proof.BootID != "11111111-2222-3333-8444-555555555555" || proof.CGroupPath != "/cube/path" {
		t.Fatalf("identity provenance = %#v", proof)
	}
	if proof.RuntimeRole != HostSandboxProcessRuntimeRoleCubeShimVMM || proof.Source != HostSandboxProcessIdentitySourceCubeBoxAddProc {
		t.Fatalf("identity source = %#v", proof)
	}
	wantPlaced := time.Date(2026, 9, 5, 4, 59, 59, 123456789, time.UTC)
	wantBound := time.Date(2026, 9, 5, 5, 0, 0, 223456789, time.UTC)
	if !proof.PlacedAt.Equal(wantPlaced) || !proof.BoundAt.Equal(wantBound) {
		t.Fatalf("identity timestamps = %v / %v", proof.PlacedAt, proof.BoundAt)
	}
}

func TestTaskTerminationAllowsMissingHostProcessIdentity(t *testing.T) {
	evidence, known, err := observeTaskTerminationFixture(t, exactTaskTerminationMetricFixture, "sandbox-123")
	if err != nil || !known {
		t.Fatalf("Observe = %#v,%v,%v", evidence, known, err)
	}
	if evidence.HostProcessIdentity != nil {
		t.Fatalf("missing identity fabricated proof: %#v", evidence.HostProcessIdentity)
	}
}

func TestTaskTerminationRejectsDetachedOrMismatchedHostProcessIdentity(t *testing.T) {
	identityOnly := `cubesandbox_host_process_identity_info{sandbox_id="sandbox-123",generation="9",host_pid="1234",starttime_ticks="5678",boot_id="11111111-2222-3333-8444-555555555555",cgroup_path="/cube/path",runtime_role="cube-shim-vmm",source="cubebox.cgroup.add_proc",placed_at="2026-09-05T04:59:59Z",bound_at="2026-09-05T05:00:00Z"} 1` + "\n"
	if _, _, err := observeTaskTerminationFixture(t, identityOnly, "sandbox-123"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
		t.Fatalf("detached identity error = %v, want ErrTaskOutcomeUnavailable", err)
	}

	tests := map[string]string{
		"generation": strings.Replace(exactHostProcessIdentityMetricFixture, `generation="18446744073709551615",host_pid`, `generation="7",host_pid`, 1),
		"cgroup": strings.Replace(exactHostProcessIdentityMetricFixture, `cgroup_path="/cube/path",runtime_role="cube-shim-vmm"`, `cgroup_path="/cube/other",runtime_role="cube-shim-vmm"`, 1),
	}
	for name, metrics := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := observeTaskTerminationFixture(t, metrics, "sandbox-123"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
				t.Fatalf("error = %v, want ErrTaskOutcomeUnavailable", err)
			}
		})
	}
}

func TestTaskTerminationRejectsMalformedHostProcessIdentity(t *testing.T) {
	tests := map[string]string{
		"duplicate": exactHostProcessIdentityMetricFixture + `cubesandbox_host_process_identity_info{sandbox_id="sandbox-123",generation="18446744073709551615",host_pid="1234",starttime_ticks="5678",boot_id="11111111-2222-3333-8444-555555555555",cgroup_path="/cube/path",runtime_role="cube-shim-vmm",source="cubebox.cgroup.add_proc",placed_at="2026-09-05T04:59:59Z",bound_at="2026-09-05T05:00:00Z"} 1` + "\n",
		"zero pid": strings.Replace(exactHostProcessIdentityMetricFixture, `host_pid="4294967295"`, `host_pid="0"`, 1),
		"signed pid": strings.Replace(exactHostProcessIdentityMetricFixture, `host_pid="4294967295"`, `host_pid="+7"`, 1),
		"pid overflow": strings.Replace(exactHostProcessIdentityMetricFixture, `host_pid="4294967295"`, `host_pid="4294967296"`, 1),
		"leading-zero starttime": strings.Replace(exactHostProcessIdentityMetricFixture, `starttime_ticks="18446744073709551615"`, `starttime_ticks="099"`, 1),
		"uppercase boot id": strings.Replace(exactHostProcessIdentityMetricFixture, `boot_id="11111111-2222-3333-8444-555555555555"`, `boot_id="AAAAAAAA-BBBB-CCCC-8DDD-EEEEEEEEEEEE"`, 1),
		"bad boot id": strings.Replace(exactHostProcessIdentityMetricFixture, `boot_id="11111111-2222-3333-8444-555555555555"`, `boot_id="not-a-uuid"`, 1),
		"noncanonical cgroup": strings.Replace(exactHostProcessIdentityMetricFixture, `cgroup_path="/cube/path",runtime_role`, `cgroup_path="/cube/../path",runtime_role`, 1),
		"blank cgroup": strings.Replace(exactHostProcessIdentityMetricFixture, `cgroup_path="/cube/path",runtime_role`, `cgroup_path=" ",runtime_role`, 1),
		"unsupported role": strings.Replace(exactHostProcessIdentityMetricFixture, `runtime_role="cube-shim-vmm"`, `runtime_role="guest-main"`, 1),
		"unsupported source": strings.Replace(exactHostProcessIdentityMetricFixture, `source="cubebox.cgroup.add_proc"`, `source="status.pid"`, 1),
		"placed after bound": strings.Replace(exactHostProcessIdentityMetricFixture, `placed_at="2026-09-05T04:59:59.123456789Z"`, `placed_at="2026-09-05T05:00:01Z"`, 1),
		"noncanonical timestamp": strings.Replace(exactHostProcessIdentityMetricFixture, `bound_at="2026-09-05T05:00:00.223456789Z"`, `bound_at="2026-09-05T05:00:00+00:00"`, 1),
		"non unit value": strings.Replace(exactHostProcessIdentityMetricFixture, `bound_at="2026-09-05T05:00:00.223456789Z"} 1`, `bound_at="2026-09-05T05:00:00.223456789Z"} 2`, 1),
		"extra victim label": strings.Replace(exactHostProcessIdentityMetricFixture, `bound_at="2026-09-05T05:00:00.223456789Z"} 1`, `bound_at="2026-09-05T05:00:00.223456789Z",victim="true"} 1`, 1),
	}
	for name, metrics := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := observeTaskTerminationFixture(t, metrics, "sandbox-123"); !errors.Is(err, ErrTaskOutcomeUnavailable) {
				t.Fatalf("error = %v, want ErrTaskOutcomeUnavailable", err)
			}
		})
	}
}

func TestTaskTerminationIgnoresOtherSandboxHostIdentity(t *testing.T) {
	metrics := strings.Replace(exactTaskTerminationMetricFixture, "", "", 1) +
		`cubesandbox_host_process_identity_info{sandbox_id="sandbox-other",generation="1",host_pid="1234",starttime_ticks="5678",boot_id="11111111-2222-3333-8444-555555555555",cgroup_path="/other",runtime_role="cube-shim-vmm",source="cubebox.cgroup.add_proc",placed_at="2026-09-05T04:59:59Z",bound_at="2026-09-05T05:00:00Z"} 1` + "\n"
	evidence, known, err := observeTaskTerminationFixture(t, metrics, "sandbox-123")
	if err != nil || !known {
		t.Fatalf("Observe = %#v,%v,%v", evidence, known, err)
	}
	if evidence.HostProcessIdentity != nil {
		t.Fatalf("other sandbox identity leaked into target: %#v", evidence.HostProcessIdentity)
	}
}

func TestHostProcessIdentityRemainsProvenanceNotOOMVictimClassification(t *testing.T) {
	evidence, known, err := observeTaskTerminationFixture(t, exactHostProcessIdentityMetricFixture, "sandbox-123")
	if err != nil || !known || evidence.HostProcessIdentity == nil || evidence.RealizationOOM == nil {
		t.Fatalf("Observe = %#v,%v,%v", evidence, known, err)
	}
	if evidence.Outcome.ExitCode != 137 {
		t.Fatalf("outcome exit code = %d", evidence.Outcome.ExitCode)
	}
	observed, knownOOM := evidence.KernelOOMObservedDuringRealization()
	if !observed || !knownOOM {
		t.Fatalf("kernel OOM window = %v,%v", observed, knownOOM)
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(*evidence.HostProcessIdentity), reflect.TypeOf(evidence)} {
		for _, forbidden := range []string{"OOMKilled", "OOMVictim", "Victim", "KilledByOOM"} {
			if _, ok := typ.FieldByName(forbidden); ok {
				t.Fatalf("Wave 19 must not expose victim classification field %s on %s", forbidden, typ.Name())
			}
	}
	}
}
