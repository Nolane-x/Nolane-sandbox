package kernelvictim

import (
	"strings"
	"testing"

	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
)

func syntheticV20TaskLayout() *btf.Struct {
	i32 := &btf.Int{Name: "int", Size: 4, Encoding: btf.Signed}
	u64 := &btf.Int{Name: "u64", Size: 8, Encoding: btf.Unsigned}
	knID := &btf.Union{Name: "kernfs_node_id", Size: 8, Members: []btf.Member{{Name: "id", Type: u64}}}
	kn := &btf.Struct{Name: "kernfs_node", Size: 128, Members: []btf.Member{{Name: "id", Type: knID, Offset: btf.Bits(8 * 8)}}}
	cg := &btf.Struct{Name: "cgroup", Size: 128, Members: []btf.Member{{Name: "kn", Type: &btf.Pointer{Target: kn}, Offset: btf.Bits(24 * 8)}}}
	css := &btf.Struct{Name: "css_set", Size: 128, Members: []btf.Member{{Name: "dfl_cgrp", Type: &btf.Pointer{Target: cg}, Offset: btf.Bits(16 * 8)}}}
	return &btf.Struct{Name: "task_struct", Size: 512, Members: []btf.Member{
		{Name: "pid", Type: i32, Offset: btf.Bits(12 * 8)},
		{Name: "tgid", Type: i32, Offset: btf.Bits(28 * 8)},
		{Name: "start_boottime", Type: u64, Offset: btf.Bits(104 * 8)},
		{Name: "cgroups", Type: &btf.Pointer{Target: css}, Offset: btf.Bits(160 * 8)},
	}}
}

func TestV20ResolveLayoutUsesBTFAndCurrentKernfsUnionID(t *testing.T) {
	got, err := resolveLayoutFromTask(syntheticV20TaskLayout())
	if err != nil {
		t.Fatal(err)
	}
	if got.PIDOffset != 12 || got.TGIDOffset != 28 || got.StartBootTimeOffset != 104 {
		t.Fatalf("wrong task offsets: %+v", got)
	}
	if got.Cgroup == nil {
		t.Fatal("current kernfs union ID layout was downgraded")
	}
	if got.Cgroup.TaskCgroupsOffset != 160 || got.Cgroup.DefaultCgroupOffset != 16 || got.Cgroup.KernfsNodeOffset != 24 || got.Cgroup.KernfsIDOffset != 8 {
		t.Fatalf("wrong cgroup chain: %+v", got.Cgroup)
	}
}

func TestV20ResolveLayoutRejectsRequiredBitfield(t *testing.T) {
	task := syntheticV20TaskLayout()
	task.Members[0].BitfieldSize = 1
	if _, err := resolveLayoutFromTask(task); err == nil {
		t.Fatal("pid bitfield accepted")
	}
}

func TestV20ResolveLayoutDowngradesOnlyOptionalCgroupChain(t *testing.T) {
	task := syntheticV20TaskLayout()
	task.Members[3].Type = &btf.Int{Name: "bad", Size: 8, Encoding: btf.Unsigned}
	got, err := resolveLayoutFromTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cgroup != nil {
		t.Fatalf("broken optional cgroup chain accepted: %+v", got.Cgroup)
	}
}

func TestV20DynamicProgramContainsRequiredKernelHelpersAndBTFOffsets(t *testing.T) {
	layout := Layout{PIDOffset: 12, TGIDOffset: 28, StartBootTimeOffset: 104}
	insns, err := buildInstructions(layout, 123)
	if err != nil {
		t.Fatal(err)
	}
	helpers := map[asm.BuiltinFunc]bool{}
	constants := map[int64]bool{}
	for _, ins := range insns {
		constants[ins.Constant] = true
		if ins.IsBuiltinCall() {
			helpers[asm.BuiltinFunc(ins.Constant)] = true
		}
	}
	for _, helper := range []asm.BuiltinFunc{asm.FnProbeReadKernel, asm.FnKtimeGetBootNs, asm.FnRingbufReserve, asm.FnRingbufSubmit} {
		if !helpers[helper] {
			t.Fatalf("missing helper %s", helper)
		}
	}
	for _, off := range []int64{12, 28, 104} {
		if !constants[off] {
			t.Fatalf("BTF offset %d not represented in dynamic program", off)
		}
	}
	if strings.Contains(insns.String(), "sandbox-a") {
		t.Fatal("BPF program contains sandbox policy")
	}
}
