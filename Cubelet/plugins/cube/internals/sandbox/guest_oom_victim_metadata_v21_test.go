// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/containerd/ttrpc"
)

func TestV21EvidenceContextCarriesOnlyExactTokenMetadata(t *testing.T) {
	token := v21Token(0x7a)
	ctx, err := guestKernelOOMVictimEvidenceContext(context.Background(), token)
	if err != nil {
		t.Fatalf("guestKernelOOMVictimEvidenceContext: %v", err)
	}

	md, ok := ttrpc.GetMetadata(ctx)
	if !ok {
		t.Fatal("Wave21 evidence context omitted ttrpc metadata")
	}
	if len(md) != 1 {
		t.Fatalf("Wave21 evidence metadata keys = %d, want exactly 1", len(md))
	}
	values, ok := md.Get(guestKernelOOMVictimEvidenceMetadataKey)
	if !ok || len(values) != 1 {
		t.Fatalf("Wave21 selector values = %v, want exactly one value", values)
	}
	if want := hex.EncodeToString(token[:]); values[0] != want {
		t.Fatalf("Wave21 selector = %q, want %q", values[0], want)
	}
}

func TestV21EvidenceContextRejectsZeroToken(t *testing.T) {
	if _, err := guestKernelOOMVictimEvidenceContext(context.Background(), [32]byte{}); err == nil {
		t.Fatal("all-zero Wave21 token must not create evidence selector context")
	}
}
