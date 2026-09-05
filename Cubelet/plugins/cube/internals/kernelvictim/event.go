// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package kernelvictim

import (
	"encoding/binary"
	"fmt"
)

const (
	EventVersionV1 uint32 = 1
	ProofSource           = "kernel.oom.mark_victim.raw_tracepoint"
	rawVictimEventSize    = 40
)

// RawVictimEvent is the fixed v1 record emitted by the kernel observer.
// PID is the exact victim TID while TGID identifies the process leader.
type RawVictimEvent struct {
	Version         uint32
	Flags           uint32
	PID             uint32
	TGID            uint32
	StartBootTimeNS uint64
	EventBootTimeNS uint64
	CgroupV2ID      uint64
}

// Event is a userspace-normalized positive kernel victim fact. StartTimeTicks
// uses the same /proc-visible lifetime domain as the Wave 19 identity proof.
type Event struct {
	BootID          string
	VictimTID       uint32
	TGID            uint32
	StartTimeTicks  uint64
	EventBootTimeNS uint64
	CgroupV2ID      uint64
}

func DecodeRawVictimEvent(raw []byte) (RawVictimEvent, error) {
	if len(raw) != rawVictimEventSize {
		return RawVictimEvent{}, fmt.Errorf("kernel OOM victim record size %d is not v1 size %d", len(raw), rawVictimEventSize)
	}
	e := RawVictimEvent{
		Version:         binary.LittleEndian.Uint32(raw[0:4]),
		Flags:           binary.LittleEndian.Uint32(raw[4:8]),
		PID:             binary.LittleEndian.Uint32(raw[8:12]),
		TGID:            binary.LittleEndian.Uint32(raw[12:16]),
		StartBootTimeNS: binary.LittleEndian.Uint64(raw[16:24]),
		EventBootTimeNS: binary.LittleEndian.Uint64(raw[24:32]),
		CgroupV2ID:      binary.LittleEndian.Uint64(raw[32:40]),
	}
	if e.Version != EventVersionV1 {
		return RawVictimEvent{}, fmt.Errorf("unsupported kernel OOM victim record version %d", e.Version)
	}
	if e.PID == 0 || e.TGID == 0 {
		return RawVictimEvent{}, fmt.Errorf("kernel OOM victim record has incomplete task identity")
	}
	if e.StartBootTimeNS == 0 || e.EventBootTimeNS == 0 {
		return RawVictimEvent{}, fmt.Errorf("kernel OOM victim record has incomplete boot-time identity")
	}
	if e.EventBootTimeNS < e.StartBootTimeNS {
		return RawVictimEvent{}, fmt.Errorf("kernel OOM victim event predates process start")
	}
	return e, nil
}
