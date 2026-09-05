package cube

import (
	"reflect"
	"strings"
	"testing"
)

const v20BaseOutcome = `cubesandbox_task_outcome_info{sandbox_id="sandbox-a",generation="7",source="containerd.task.wait",exit_code="137",exited_at="2026-09-05T08:00:00Z"} 1`
const v20HostIdentity = `cubesandbox_host_process_identity_info{sandbox_id="sandbox-a",generation="7",host_pid="4242",starttime_ticks="1234",boot_id="11111111-2222-3333-4444-555555555555",cgroup_path="/cube_sandbox_v1/42",runtime_role="cube-shim-vmm",source="cubebox.cgroup.add_proc",placed_at="2026-09-05T07:59:00Z",bound_at="2026-09-05T07:59:01Z"} 1`
const v20Victim = `cubesandbox_host_kernel_oom_victim_info{sandbox_id="sandbox-a",generation="7",boot_id="11111111-2222-3333-4444-555555555555",host_pid="4242",victim_tid="4247",starttime_ticks="1234",cgroup_path="/cube_sandbox_v1/42",event_boot_time_ns="20000000000",cgroup_v2_id="88",cgroup_v2_correlated="true",source="kernel.oom.mark_victim.raw_tracepoint"} 1`

func TestV20NolaneWorldAcceptsExactHostVictimProof(t *testing.T) {
	input := strings.Join([]string{v20BaseOutcome, v20HostIdentity, v20Victim}, "\n") + "\n"
	evidence, ok, err := parseTaskTerminationMetrics(strings.NewReader(input), "sandbox-a")
	if err != nil { t.Fatal(err) }
	if !ok || evidence.HostKernelOOMVictim == nil { t.Fatalf("victim proof missing: %+v", evidence) }
	if evidence.HostKernelOOMVictim.HostPID != 4242 || evidence.HostKernelOOMVictim.VictimTID != 4247 { t.Fatalf("TID/TGID provenance lost: %+v", evidence.HostKernelOOMVictim) }
	marked, known := evidence.HostKernelOOMVictimMarked()
	if !marked || !known { t.Fatalf("got marked=%v known=%v", marked, known) }
}

func TestV20NolaneWorldAbsenceIsUnknownNotFalse(t *testing.T) {
	input := strings.Join([]string{v20BaseOutcome, v20HostIdentity}, "\n") + "\n"
	evidence, ok, err := parseTaskTerminationMetrics(strings.NewReader(input), "sandbox-a")
	if err != nil || !ok { t.Fatalf("parse: ok=%v err=%v", ok, err) }
	marked, known := evidence.HostKernelOOMVictimMarked()
	if marked || known { t.Fatalf("absence became known-negative: %v %v", marked, known) }
}

func TestV20NolaneWorldRejectsDetachedAndMismatchedVictimProofs(t *testing.T) {
	cases := []string{
		strings.Join([]string{v20BaseOutcome, v20Victim}, "\n"),
		strings.Join([]string{v20BaseOutcome, v20HostIdentity, v20Victim, v20Victim}, "\n"),
		strings.Join([]string{v20BaseOutcome, v20HostIdentity, strings.Replace(v20Victim, `host_pid="4242"`, `host_pid="4243"`, 1)}, "\n"),
		strings.Join([]string{v20BaseOutcome, v20HostIdentity, strings.Replace(v20Victim, `starttime_ticks="1234"`, `starttime_ticks="1235"`, 1)}, "\n"),
		strings.Join([]string{v20BaseOutcome, v20HostIdentity, strings.Replace(v20Victim, `cgroup_v2_correlated="true"`, `cgroup_v2_correlated="false"`, 1)}, "\n"),
	}
	for i, input := range cases {
		if _, _, err := parseTaskTerminationMetrics(strings.NewReader(input+"\n"), "sandbox-a"); err == nil { t.Fatalf("case %d unexpectedly accepted", i) }
	}
}

func TestV20EvidenceShapesDoNotExposeOOMKilledCausality(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(TaskTerminationEvidence{}), reflect.TypeOf(HostKernelOOMVictimProof{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			if strings.Contains(name, "oomkilled") || strings.Contains(name, "guestoom") || strings.Contains(name, "taskoom") || strings.Contains(name, "applicationoom") {
				t.Fatalf("forbidden kill-causality field %s.%s", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}
