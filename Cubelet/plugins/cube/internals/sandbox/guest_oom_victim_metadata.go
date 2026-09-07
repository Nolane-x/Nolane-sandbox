// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/containerd/ttrpc"
)

const guestKernelOOMVictimEvidenceMetadataKey = "cube-wave21-guest-oom-evidence"

// guestKernelOOMVictimEvidenceContext constructs the only normative Wave 21
// Task Stats selector. The value is the exact current 32-byte realization
// token encoded as canonical lowercase hexadecimal. No sandbox, generation,
// PID, cgroup, or outcome signal may be substituted for this selector.
func guestKernelOOMVictimEvidenceContext(ctx context.Context, token [32]byte) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("Wave21 evidence selector context is required")
	}
	if !validGuestOOMVictimToken(token) {
		return nil, fmt.Errorf("Wave21 evidence selector token must be non-zero")
	}
	md := ttrpc.MD{}
	md.Set(guestKernelOOMVictimEvidenceMetadataKey, hex.EncodeToString(token[:]))
	return ttrpc.WithMetadata(ctx, md), nil
}
