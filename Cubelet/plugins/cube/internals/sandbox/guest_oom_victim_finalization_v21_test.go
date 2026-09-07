// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/ttrpc"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictimbridge"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const v21EvidenceTypeURLForTest = "io.cubesandbox.v1.GuestOOMVictimEvidenceSet"

type v21FinalizationRuntimeService struct {
	exitedAt   time.Time
	statsCalls int
	selectors  []string
	payload    []byte
}

func (f *v21FinalizationRuntimeService) Wait(context.Context, *task.WaitRequest) (*task.WaitResponse, error) {
	return &task.WaitResponse{ExitStatus: 137, ExitedAt: timestamppb.New(f.exitedAt)}, nil
}

func (f *v21FinalizationRuntimeService) State(context.Context, *task.StateRequest) (*task.StateResponse, error) {
	return &task.StateResponse{
		Status:     tasktypes.Status_STOPPED,
		ExitStatus: 137,
		ExitedAt:   timestamppb.New(f.exitedAt),
	}, nil
}

func (f *v21FinalizationRuntimeService) Stats(ctx context.Context, req *task.StatsRequest) (*task.StatsResponse, error) {
	if req == nil || req.ID == "" {
		return nil, errors.New("Wave21 Stats request omitted exact task ID")
	}
	f.statsCalls++
	md, ok := ttrpc.GetMetadata(ctx)
	if !ok {
		return nil, errors.New("Wave21 Stats request omitted ttrpc metadata")
	}
	values, ok := md.Get(guestKernelOOMVictimEvidenceMetadataKey)
	if !ok || len(values) != 1 {
		return nil, errors.New("Wave21 Stats request omitted exact selector")
	}
	f.selectors = append(f.selectors, values[0])
	return &task.StatsResponse{Stats: &anypb.Any{
		TypeUrl: v21EvidenceTypeURLForTest,
		Value:   append([]byte(nil), f.payload...),
	}}, nil
}

func v21WireVarint(dst []byte, field protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, field, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}

func v21WireBytes(dst []byte, field protowire.Number, value []byte) []byte {
	dst = protowire.AppendTag(dst, field, protowire.BytesType)
	return protowire.AppendBytes(dst, value)
}

func v21EvidencePayload(sandboxID string, token [32]byte) []byte {
	record := make([]byte, 0, 256)
	record = v21WireVarint(record, 1, 1)
	record = v21WireBytes(record, 2, []byte(sandboxID))
	record = v21WireBytes(record, 3, token[:])
	record = v21WireBytes(record, 4, []byte("11111111-2222-3333-4444-555555555555"))
	record = v21WireVarint(record, 5, 42)
	record = v21WireVarint(record, 6, 42)
	record = v21WireVarint(record, 7, 9001)
	record = v21WireVarint(record, 8, 150)
	record = v21WireVarint(record, 10, 42)
	record = v21WireVarint(record, 11, 9001)
	record = v21WireVarint(record, 12, 1)
	record = v21WireVarint(record, 13, 100)
	record = v21WireVarint(record, 14, 200)
	record = v21WireBytes(record, 15, []byte(guestKernelOOMVictimSource))

	payload := make([]byte, 0, len(record)+8)
	return v21WireBytes(payload, 1, record)
}

func TestV21WaitPerformsOneExactTerminalEvidenceQuery(t *testing.T) {
	const sandboxID = "sandbox-v21-finalization"
	var token [32]byte
	for i := range token {
		token[i] = byte(0x41 + i)
	}
	service := &v21FinalizationRuntimeService{
		exitedAt: time.Unix(1_725_100_021, 987_654_321).UTC(),
		payload:  v21EvidencePayload(sandboxID, token),
	}
	controller := taskOutcomeControllerWithService(service)
	controller.guestOOMVictimTokenGenerator = func() ([32]byte, error) { return token, nil }
	guestvictimbridge.Clear(sandboxID)
	t.Cleanup(func() { guestvictimbridge.Clear(sandboxID) })

	if _, err := controller.Start(context.Background(), sandboxID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	binding, ok := guestvictimbridge.ClaimStartBinding(sandboxID)
	if !ok {
		t.Fatal("live Wave21 binding was not claimable")
	}
	if !guestvictimbridge.MarkBound(binding) {
		t.Fatal("live Wave21 binding could not be marked bound")
	}

	if _, err := controller.Wait(context.Background(), sandboxID); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	if service.statsCalls != 1 {
		t.Fatalf("Wave21 terminal Stats calls = %d, want 1", service.statsCalls)
	}
	if len(service.selectors) != 1 || service.selectors[0] != hex.EncodeToString(token[:]) {
		t.Fatalf("Wave21 selector = %v, want exact token", service.selectors)
	}
	proofs := controller.ensureTaskOutcomeProofStore().listGuestKernelOOMVictimProofs()
	if len(proofs) != 1 {
		t.Fatalf("accepted Wave21 proofs = %d, want 1", len(proofs))
	}
	if proofs[0].SandboxID != sandboxID || proofs[0].Generation != 1 || proofs[0].RealizationTokenHex != hex.EncodeToString(token[:]) {
		t.Fatalf("accepted Wave21 authority mismatch: %+v", proofs[0])
	}

	// Later terminal observation must never repair/re-query the same realization.
	if _, err := controller.Wait(context.Background(), sandboxID); err != nil {
		t.Fatalf("second Wait: %v", err)
	}
	if service.statsCalls != 1 {
		t.Fatalf("Wave21 terminal Stats calls after duplicate Wait = %d, want 1", service.statsCalls)
	}
}

func TestV21WaitDoesNotQueryEvidenceWhenPreStartBindDidNotSucceed(t *testing.T) {
	const sandboxID = "sandbox-v21-unbound"
	var token [32]byte
	token[0] = 0x55
	service := &v21FinalizationRuntimeService{
		exitedAt: time.Unix(1_725_100_022, 0).UTC(),
		payload:  v21EvidencePayload(sandboxID, token),
	}
	controller := taskOutcomeControllerWithService(service)
	controller.guestOOMVictimTokenGenerator = func() ([32]byte, error) { return token, nil }
	guestvictimbridge.Clear(sandboxID)
	t.Cleanup(func() { guestvictimbridge.Clear(sandboxID) })

	if _, err := controller.Start(context.Background(), sandboxID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	binding, ok := guestvictimbridge.ClaimStartBinding(sandboxID)
	if !ok {
		t.Fatal("Wave21 binding was not claimable")
	}
	if !guestvictimbridge.MarkUnavailable(binding) {
		t.Fatal("Wave21 binding could not be marked unavailable")
	}

	if _, err := controller.Wait(context.Background(), sandboxID); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if service.statsCalls != 0 {
		t.Fatalf("unbound Wave21 realization queried Stats %d times, want 0", service.statsCalls)
	}
	if proofs := controller.ensureTaskOutcomeProofStore().listGuestKernelOOMVictimProofs(); len(proofs) != 0 {
		t.Fatalf("unbound Wave21 realization accepted %d proofs", len(proofs))
	}
}
