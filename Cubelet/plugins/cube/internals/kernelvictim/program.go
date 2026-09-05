package kernelvictim

import (
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

const rawVictimRecordSize = 40

func BuildProgramSpec(layout Layout, events *ebpf.Map) (*ebpf.ProgramSpec, error) {
	if events == nil {
		return nil, fmt.Errorf("kernel victim ring buffer map is required")
	}
	insns, err := buildInstructions(layout, events.FD())
	if err != nil {
		return nil, err
	}
	return &ebpf.ProgramSpec{
		Name:         "cube_oom_victim",
		Type:         ebpf.RawTracepoint,
		License:      "GPL",
		Instructions: insns,
	}, nil
}

func buildInstructions(layout Layout, mapFD int) (asm.Instructions, error) {
	if mapFD < 0 {
		return nil, fmt.Errorf("invalid ring buffer map fd %d", mapFD)
	}
	if layout.PIDOffset < 0 || layout.TGIDOffset < 0 || layout.StartBootTimeOffset < 0 {
		return nil, fmt.Errorf("kernel victim BTF offsets must be non-negative")
	}

	insns := asm.Instructions{
		// Raw tracepoint ctx args[0] is the victim task_struct pointer.
		asm.LoadMem(asm.R6, asm.R1, 0, asm.DWord),
		asm.JEq.Imm(asm.R6, 0, "exit"),

		// Required pid -> FP[-4].
		asm.Mov.Reg(asm.R1, asm.RFP),
		asm.Add.Imm(asm.R1, -4),
		asm.Mov.Imm(asm.R2, 4),
		asm.Mov.Reg(asm.R3, asm.R6),
		asm.Add.Imm(asm.R3, int32(layout.PIDOffset)),
		asm.FnProbeReadKernel.Call(),
		asm.JNE.Imm(asm.R0, 0, "exit"),

		// Required tgid -> FP[-8].
		asm.Mov.Reg(asm.R1, asm.RFP),
		asm.Add.Imm(asm.R1, -8),
		asm.Mov.Imm(asm.R2, 4),
		asm.Mov.Reg(asm.R3, asm.R6),
		asm.Add.Imm(asm.R3, int32(layout.TGIDOffset)),
		asm.FnProbeReadKernel.Call(),
		asm.JNE.Imm(asm.R0, 0, "exit"),

		// Required start_boottime -> FP[-16].
		asm.Mov.Reg(asm.R1, asm.RFP),
		asm.Add.Imm(asm.R1, -16),
		asm.Mov.Imm(asm.R2, 8),
		asm.Mov.Reg(asm.R3, asm.R6),
		asm.Add.Imm(asm.R3, int32(layout.StartBootTimeOffset)),
		asm.FnProbeReadKernel.Call(),
		asm.JNE.Imm(asm.R0, 0, "exit"),

		// Optional cgroup-v2 ID defaults to unknown (zero).
		asm.Mov.Imm(asm.R7, 0),
		asm.StoreMem(asm.RFP, -32, asm.R7, asm.DWord),
	}

	if layout.Cgroup != nil {
		cg := layout.Cgroup
		insns = append(insns,
			// task_struct.cgroups -> FP[-24] -> R7
			asm.Mov.Reg(asm.R1, asm.RFP),
			asm.Add.Imm(asm.R1, -24),
			asm.Mov.Imm(asm.R2, 8),
			asm.Mov.Reg(asm.R3, asm.R6),
			asm.Add.Imm(asm.R3, int32(cg.TaskCgroupsOffset)),
			asm.FnProbeReadKernel.Call(),
			asm.JNE.Imm(asm.R0, 0, "event_time"),
			asm.LoadMem(asm.R7, asm.RFP, -24, asm.DWord),
			asm.JEq.Imm(asm.R7, 0, "event_time"),

			// css_set.dfl_cgrp
			asm.Mov.Reg(asm.R1, asm.RFP),
			asm.Add.Imm(asm.R1, -24),
			asm.Mov.Imm(asm.R2, 8),
			asm.Mov.Reg(asm.R3, asm.R7),
			asm.Add.Imm(asm.R3, int32(cg.DefaultCgroupOffset)),
			asm.FnProbeReadKernel.Call(),
			asm.JNE.Imm(asm.R0, 0, "event_time"),
			asm.LoadMem(asm.R7, asm.RFP, -24, asm.DWord),
			asm.JEq.Imm(asm.R7, 0, "event_time"),

			// cgroup.kn
			asm.Mov.Reg(asm.R1, asm.RFP),
			asm.Add.Imm(asm.R1, -24),
			asm.Mov.Imm(asm.R2, 8),
			asm.Mov.Reg(asm.R3, asm.R7),
			asm.Add.Imm(asm.R3, int32(cg.KernfsNodeOffset)),
			asm.FnProbeReadKernel.Call(),
			asm.JNE.Imm(asm.R0, 0, "event_time"),
			asm.LoadMem(asm.R7, asm.RFP, -24, asm.DWord),
			asm.JEq.Imm(asm.R7, 0, "event_time"),

			// kernfs_node.id exact u64 -> FP[-32].
			asm.Mov.Reg(asm.R1, asm.RFP),
			asm.Add.Imm(asm.R1, -32),
			asm.Mov.Imm(asm.R2, 8),
			asm.Mov.Reg(asm.R3, asm.R7),
			asm.Add.Imm(asm.R3, int32(cg.KernfsIDOffset)),
			asm.FnProbeReadKernel.Call(),
			asm.JNE.Imm(asm.R0, 0, "cgroup_unknown"),
			asm.Ja.Label("event_time"),
			asm.Mov.Imm(asm.R7, 0).WithSymbol("cgroup_unknown"),
			asm.StoreMem(asm.RFP, -32, asm.R7, asm.DWord),
		)
	}

	insns = append(insns,
		asm.FnKtimeGetBootNs.Call().WithSymbol("event_time"),
		asm.Mov.Reg(asm.R8, asm.R0),

		// Reserve the exact fixed v1 record.
		asm.LoadMapPtr(asm.R1, mapFD),
		asm.Mov.Imm(asm.R2, rawVictimRecordSize),
		asm.Mov.Imm(asm.R3, 0),
		asm.FnRingbufReserve.Call(),
		asm.JEq.Imm(asm.R0, 0, "exit"),
		asm.Mov.Reg(asm.R9, asm.R0),

		// version + flags
		asm.Mov.Imm(asm.R1, int32(EventVersionV1)),
		asm.StoreMem(asm.R9, 0, asm.R1, asm.Word),
		asm.Mov.Imm(asm.R1, 0),
		asm.StoreMem(asm.R9, 4, asm.R1, asm.Word),

		// pid / tgid
		asm.LoadMem(asm.R1, asm.RFP, -4, asm.Word),
		asm.StoreMem(asm.R9, 8, asm.R1, asm.Word),
		asm.LoadMem(asm.R1, asm.RFP, -8, asm.Word),
		asm.StoreMem(asm.R9, 12, asm.R1, asm.Word),

		// start_boottime / event_boottime / optional cgroup-v2 id
		asm.LoadMem(asm.R1, asm.RFP, -16, asm.DWord),
		asm.StoreMem(asm.R9, 16, asm.R1, asm.DWord),
		asm.StoreMem(asm.R9, 24, asm.R8, asm.DWord),
		asm.LoadMem(asm.R1, asm.RFP, -32, asm.DWord),
		asm.StoreMem(asm.R9, 32, asm.R1, asm.DWord),

		asm.Mov.Reg(asm.R1, asm.R9),
		asm.Mov.Imm(asm.R2, 0),
		asm.FnRingbufSubmit.Call(),
		asm.Mov.Imm(asm.R0, 0).WithSymbol("exit"),
		asm.Return(),
	)
	return insns, nil
}
