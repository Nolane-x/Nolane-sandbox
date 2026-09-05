package kernelvictim

import (
	"fmt"
	"math"

	"github.com/cilium/ebpf/btf"
)

type Layout struct {
	PIDOffset           int16
	TGIDOffset          int16
	StartBootTimeOffset int16
	Cgroup              *CgroupV2Layout
}

type CgroupV2Layout struct {
	TaskCgroupsOffset    int16
	DefaultCgroupOffset  int16
	KernfsNodeOffset     int16
	KernfsIDOffset       int16
}

func ResolveLayout(spec *btf.Spec) (Layout, error) {
	if spec == nil {
		return Layout{}, fmt.Errorf("kernel BTF spec is required")
	}
	var task *btf.Struct
	if err := spec.TypeByName("task_struct", &task); err != nil {
		return Layout{}, fmt.Errorf("resolve task_struct from kernel BTF: %w", err)
	}
	return resolveLayoutFromTask(task)
}

func resolveLayoutFromTask(task *btf.Struct) (Layout, error) {
	if task == nil {
		return Layout{}, fmt.Errorf("task_struct BTF is required")
	}
	pid, err := requiredIntegerMember(task, "pid", 4, btf.Signed)
	if err != nil {
		return Layout{}, err
	}
	tgid, err := requiredIntegerMember(task, "tgid", 4, btf.Signed)
	if err != nil {
		return Layout{}, err
	}
	start, err := requiredIntegerMember(task, "start_boottime", 8, btf.Unsigned)
	if err != nil {
		return Layout{}, err
	}

	layout := Layout{
		PIDOffset:           pid,
		TGIDOffset:          tgid,
		StartBootTimeOffset: start,
	}
	if cgroup, ok := resolveOptionalCgroupV2Layout(task); ok {
		layout.Cgroup = cgroup
	}
	return layout, nil
}

func requiredIntegerMember(s *btf.Struct, name string, size uint32, encoding btf.IntEncoding) (int16, error) {
	m, ok := namedMember(s, name)
	if !ok {
		return 0, fmt.Errorf("required BTF member %s.%s is missing", s.Name, name)
	}
	if m.BitfieldSize != 0 {
		return 0, fmt.Errorf("required BTF member %s.%s is a bitfield", s.Name, name)
	}
	i, ok := btf.UnderlyingType(m.Type).(*btf.Int)
	if !ok || i.Size != size || i.Encoding != encoding {
		return 0, fmt.Errorf("required BTF member %s.%s has incompatible integer shape", s.Name, name)
	}
	return memberOffset(m)
}

func resolveOptionalCgroupV2Layout(task *btf.Struct) (*CgroupV2Layout, bool) {
	cgroups, ok := namedMember(task, "cgroups")
	if !ok || cgroups.BitfieldSize != 0 {
		return nil, false
	}
	taskCgroupsOffset, err := memberOffset(cgroups)
	if err != nil {
		return nil, false
	}
	css, ok := pointerTargetStruct(cgroups.Type)
	if !ok {
		return nil, false
	}

	dfl, ok := namedMember(css, "dfl_cgrp")
	if !ok || dfl.BitfieldSize != 0 {
		return nil, false
	}
	defaultOffset, err := memberOffset(dfl)
	if err != nil {
		return nil, false
	}
	cgroup, ok := pointerTargetStruct(dfl.Type)
	if !ok {
		return nil, false
	}

	knMember, ok := namedMember(cgroup, "kn")
	if !ok || knMember.BitfieldSize != 0 {
		return nil, false
	}
	knOffset, err := memberOffset(knMember)
	if err != nil {
		return nil, false
	}
	kn, ok := pointerTargetStruct(knMember.Type)
	if !ok {
		return nil, false
	}

	idMember, ok := namedMember(kn, "id")
	if !ok || idMember.BitfieldSize != 0 {
		return nil, false
	}
	idOuter, err := memberOffset(idMember)
	if err != nil {
		return nil, false
	}
	idOffset := int64(idOuter)
	switch idType := btf.UnderlyingType(idMember.Type).(type) {
	case *btf.Int:
		if idType.Size != 8 || idType.Encoding != btf.Unsigned {
			return nil, false
		}
	case *btf.Union:
		inner, ok := namedMember(idType, "id")
		if !ok || inner.BitfieldSize != 0 {
			return nil, false
		}
		i, ok := btf.UnderlyingType(inner.Type).(*btf.Int)
		if !ok || i.Size != 8 || i.Encoding != btf.Unsigned {
			return nil, false
		}
		innerOffset, err := memberOffset(inner)
		if err != nil {
			return nil, false
		}
		idOffset += int64(innerOffset)
	default:
		return nil, false
	}
	if idOffset < 0 || idOffset > math.MaxInt16 {
		return nil, false
	}

	return &CgroupV2Layout{
		TaskCgroupsOffset:   taskCgroupsOffset,
		DefaultCgroupOffset: defaultOffset,
		KernfsNodeOffset:    knOffset,
		KernfsIDOffset:      int16(idOffset),
	}, true
}

func pointerTargetStruct(t btf.Type) (*btf.Struct, bool) {
	p, ok := btf.UnderlyingType(t).(*btf.Pointer)
	if !ok || p == nil || p.Target == nil {
		return nil, false
	}
	s, ok := btf.UnderlyingType(p.Target).(*btf.Struct)
	return s, ok && s != nil
}

type btfMembers interface {
	membersForWave20() []btf.Member
}

func namedMember(t interface{}, name string) (btf.Member, bool) {
	var members []btf.Member
	switch v := t.(type) {
	case *btf.Struct:
		if v == nil {
			return btf.Member{}, false
		}
		members = v.Members
	case *btf.Union:
		if v == nil {
			return btf.Member{}, false
		}
		members = v.Members
	default:
		return btf.Member{}, false
	}
	for _, member := range members {
		if member.Name == name {
			return member, true
		}
	}
	return btf.Member{}, false
}

func memberOffset(member btf.Member) (int16, error) {
	if member.Offset%8 != 0 {
		return 0, fmt.Errorf("BTF member %s has non-byte-aligned offset", member.Name)
	}
	bytes := member.Offset.Bytes()
	if bytes > math.MaxInt16 {
		return 0, fmt.Errorf("BTF member %s offset %d is not representable", member.Name, bytes)
	}
	return int16(bytes), nil
}
