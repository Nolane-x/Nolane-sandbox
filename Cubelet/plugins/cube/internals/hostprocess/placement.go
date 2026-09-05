// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

// Package hostprocess contains the package-neutral trusted ordering primitive
// used to couple a successful cgroup placement with observational host-process
// evidence. It owns no sandbox lifecycle or realization authority.
package hostprocess

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"
)

// PlacementRecorder records observational evidence after the caller has
// successfully placed a process in the exact cgroup path. Implementations must
// treat recording failures as evidence loss, not execution failure.
type PlacementRecorder interface {
	RecordHostProcessPlacement(context.Context, string, string, uint32, time.Time) error
}

// AddProcAndRecord is the trusted ordering boundary for Wave 19. It validates
// authority inputs, performs AddProc first, and only then gives the recorder a
// chance to capture host-process identity. Recorder failure is deliberately
// observational: a successful cgroup placement remains successful.
func AddProcAndRecord(
	ctx context.Context,
	sandboxID string,
	group string,
	pid uint32,
	addProc func(string, uint64) error,
	recorder PlacementRecorder,
	now func() time.Time,
) error {
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return fmt.Errorf("host process placement sandbox ID is not canonical")
	}
	if err := validateCgroupPath(group); err != nil {
		return err
	}
	if pid == 0 {
		return fmt.Errorf("host process placement PID is required")
	}
	if addProc == nil {
		return fmt.Errorf("host process placement AddProc function is unavailable")
	}

	if err := addProc(group, uint64(pid)); err != nil {
		return err
	}

	if recorder == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	placedAt := now().UTC()
	if placedAt.IsZero() {
		return nil
	}
	_ = recorder.RecordHostProcessPlacement(ctx, sandboxID, group, pid, placedAt)
	return nil
}

func validateCgroupPath(group string) error {
	if group == "" || strings.TrimSpace(group) != group {
		return fmt.Errorf("host process placement cgroup path is not canonical")
	}
	if !path.IsAbs(group) || path.Clean(group) != group || group == "/" {
		return fmt.Errorf("host process placement cgroup path %q is not canonical", group)
	}
	return nil
}
