// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictimbridge"
	"google.golang.org/protobuf/encoding/protowire"
)

const guestKernelOOMVictimEvidenceTypeURL = "io.cubesandbox.v1.GuestOOMVictimEvidenceSet"

type guestKernelOOMVictimFinalizationRegistry struct {
	mu        sync.Mutex
	bySandbox map[string]uint64
}

var guestKernelOOMVictimFinalizationRegistries sync.Map // map[*taskOutcomeProofStore]*guestKernelOOMVictimFinalizationRegistry

func registryForGuestKernelOOMVictimFinalization(store *taskOutcomeProofStore) *guestKernelOOMVictimFinalizationRegistry {
	if existing, ok := guestKernelOOMVictimFinalizationRegistries.Load(store); ok {
		return existing.(*guestKernelOOMVictimFinalizationRegistry)
	}
	created := &guestKernelOOMVictimFinalizationRegistry{bySandbox: make(map[string]uint64)}
	actual, _ := guestKernelOOMVictimFinalizationRegistries.LoadOrStore(store, created)
	return actual.(*guestKernelOOMVictimFinalizationRegistry)
}

// clearGuestKernelOOMVictimLifecycleState is called while the Wave17 store
// write lock is held. It fences every external Wave21 registry so a numeric
// generation reused after Create cannot resurrect old authority.
func clearGuestKernelOOMVictimLifecycleState(store *taskOutcomeProofStore, sandboxID string) {
	if store == nil || sandboxID == "" {
		return
	}
	if existing, ok := guestOOMVictimTokenRegistries.Load(store); ok {
		registry := existing.(*guestOOMVictimTokenRegistryState)
		registry.mu.Lock()
		delete(registry.bySandbox, sandboxID)
		registry.mu.Unlock()
	}
	if existing, ok := guestKernelOOMVictimProofRegistries.Load(store); ok {
		registry := existing.(*guestKernelOOMVictimProofRegistry)
		registry.mu.Lock()
		delete(registry.bySandbox, sandboxID)
		registry.mu.Unlock()
	}
	if existing, ok := guestKernelOOMVictimFinalizationRegistries.Load(store); ok {
		registry := existing.(*guestKernelOOMVictimFinalizationRegistry)
		registry.mu.Lock()
		delete(registry.bySandbox, sandboxID)
		registry.mu.Unlock()
	}
}

// claimGuestKernelOOMVictimFinalization closes the one terminal-attempt latch
// before any RPC is issued. Failure or absence after this point stays unknown;
// later Wait/Status observations cannot repair the same generation.
func (s *taskOutcomeProofStore) claimGuestKernelOOMVictimFinalization(sandboxID string, generation uint64) bool {
	if s == nil || sandboxID == "" || generation == 0 {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return false
	}
	outcome, ok := s.proofs[sandboxID]
	if !ok || outcome.Generation != generation {
		return false
	}
	registry := registryForGuestKernelOOMVictimFinalization(s)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.bySandbox[sandboxID] == generation {
		return false
	}
	registry.bySandbox[sandboxID] = generation
	return true
}

func (c *controllerLocal) finalizeGuestKernelOOMVictim(ctx context.Context, svc taskStatsService, outcome TaskOutcomeProof) {
	store := c.ensureTaskOutcomeProofStore()
	if store == nil || svc == nil || !store.claimGuestKernelOOMVictimFinalization(outcome.SandboxID, outcome.Generation) {
		return
	}

	token, ok := store.GuestOOMVictimToken(outcome.SandboxID, outcome.Generation)
	if !ok {
		return
	}
	binding := guestvictimbridge.StartBinding{
		SandboxID:  outcome.SandboxID,
		Generation: outcome.Generation,
		Token:      token,
	}
	if !guestvictimbridge.IsBound(binding) {
		return
	}

	evidenceCtx, err := guestKernelOOMVictimEvidenceContext(ctx, token)
	if err != nil {
		return
	}
	resp, err := svc.Stats(evidenceCtx, &task.StatsRequest{ID: outcome.SandboxID})
	if err != nil || resp == nil || resp.GetStats() == nil {
		return
	}
	stats := resp.GetStats()
	proofs, err := decodeGuestKernelOOMVictimEvidence(
		outcome.SandboxID,
		outcome.Generation,
		token,
		stats.GetTypeUrl(),
		stats.GetValue(),
	)
	if err != nil {
		return
	}
	_ = store.AcceptGuestKernelOOMVictimProofs(outcome.SandboxID, outcome.Generation, token, proofs)
}

func decodeGuestKernelOOMVictimEvidence(sandboxID string, generation uint64, token [32]byte, typeURL string, payload []byte) ([]GuestKernelOOMVictimProof, error) {
	if typeURL != guestKernelOOMVictimEvidenceTypeURL {
		return nil, fmt.Errorf("guest kernel OOM victim evidence type URL is not authoritative")
	}
	if sandboxID == "" || generation == 0 || !validGuestOOMVictimToken(token) || len(payload) == 0 {
		return nil, fmt.Errorf("guest kernel OOM victim evidence selector is invalid")
	}

	var proofs []GuestKernelOOMVictimProof
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return nil, fmt.Errorf("guest kernel OOM victim evidence set has malformed tag")
		}
		payload = payload[n:]
		if number != 1 || wireType != protowire.BytesType {
			return nil, fmt.Errorf("guest kernel OOM victim evidence set contains unknown field")
		}
		record, n := protowire.ConsumeBytes(payload)
		if n < 0 {
			return nil, fmt.Errorf("guest kernel OOM victim evidence set has malformed record")
		}
		payload = payload[n:]
		proof, err := decodeGuestKernelOOMVictimRecord(sandboxID, generation, token, record)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, proof)
		if len(proofs) > maxGuestKernelOOMVictims {
			return nil, fmt.Errorf("guest kernel OOM victim evidence exceeds %d records", maxGuestKernelOOMVictims)
		}
	}
	return normalizedGuestKernelOOMVictimProofs(proofs)
}

func decodeGuestKernelOOMVictimRecord(sandboxID string, generation uint64, token [32]byte, data []byte) (GuestKernelOOMVictimProof, error) {
	proof := GuestKernelOOMVictimProof{Generation: generation}
	seen := make(map[protowire.Number]bool, 15)
	var version uint64

	for len(data) > 0 {
		number, wireType, n := protowire.ConsumeTag(data)
		if n < 0 || number < 1 || number > 15 || seen[number] {
			return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim record has malformed or duplicate field")
		}
		seen[number] = true
		data = data[n:]

		switch number {
		case 1, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14:
			if wireType != protowire.VarintType {
				return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim record has wrong wire type")
			}
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim record has malformed varint")
			}
			data = data[consumed:]
			switch number {
			case 1:
				version = value
			case 5:
				if value > uint64(^uint32(0)) {
					return GuestKernelOOMVictimProof{}, fmt.Errorf("guest victim tid overflows uint32")
				}
				proof.TID = uint32(value)
			case 6:
				if value > uint64(^uint32(0)) {
					return GuestKernelOOMVictimProof{}, fmt.Errorf("guest victim tgid overflows uint32")
				}
				proof.TGID = uint32(value)
			case 7:
				proof.StartTimeTicks = value
			case 8:
				proof.EventBootNS = value
			case 9:
				proof.CgroupV2ID = value
			case 10:
				if value > uint64(^uint32(0)) {
					return GuestKernelOOMVictimProof{}, fmt.Errorf("guest main pid overflows uint32")
				}
				proof.MainPID = uint32(value)
			case 11:
				proof.MainStartTimeTicks = value
			case 12:
				switch value {
				case 1:
					proof.Class = GuestKernelOOMVictimClassMain
				case 2:
					proof.Class = GuestKernelOOMVictimClassMember
				default:
					return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim scope is invalid")
				}
			case 13:
				proof.RealizationStartedBootNS = value
			case 14:
				proof.OutcomeObservedBootNS = value
			}

		case 2, 3, 4, 15:
			if wireType != protowire.BytesType {
				return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim record has wrong wire type")
			}
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim record has malformed bytes")
			}
			data = data[consumed:]
			switch number {
			case 2:
				proof.SandboxID = string(value)
			case 3:
				if len(value) != len(token) || !bytes.Equal(value, token[:]) {
					return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim token does not match exact selector")
				}
				proof.RealizationTokenHex = hex.EncodeToString(value)
			case 4:
				proof.GuestBootID = string(value)
			case 15:
				proof.Source = string(value)
			}
		default:
			return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim record contains unknown field")
		}
	}

	if version != 1 || proof.SandboxID != sandboxID || !seen[3] {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("guest kernel OOM victim record authority does not match exact request")
	}
	if err := validateGuestKernelOOMVictimProof(proof); err != nil {
		return GuestKernelOOMVictimProof{}, err
	}
	return proof, nil
}
