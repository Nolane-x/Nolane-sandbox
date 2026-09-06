// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const (
	guestKernelOOMVictimSource = "guest.kernel.oom.mark_victim.raw_tracepoint"
	maxGuestKernelOOMVictims   = 64
)

type GuestKernelOOMVictimClass string

const (
	GuestKernelOOMVictimClassMain   GuestKernelOOMVictimClass = "MAIN"
	GuestKernelOOMVictimClassMember GuestKernelOOMVictimClass = "MEMBER"
)

type GuestKernelOOMVictimProof struct {
	SandboxID                string
	Generation               uint64
	RealizationTokenHex      string
	GuestBootID              string
	TID                      uint32
	TGID                     uint32
	StartTimeTicks           uint64
	MainPID                  uint32
	MainStartTimeTicks       uint64
	EventBootNS              uint64
	CgroupV2ID               uint64
	Class                    GuestKernelOOMVictimClass
	RealizationStartedBootNS uint64
	OutcomeObservedBootNS    uint64
	Source                   string
}

type guestKernelOOMVictimProofState struct {
	Generation uint64
	Proofs     []GuestKernelOOMVictimProof
}

type guestKernelOOMVictimProofRegistry struct {
	mu        sync.RWMutex
	bySandbox map[string]guestKernelOOMVictimProofState
}

var guestKernelOOMVictimProofRegistries sync.Map // map[*taskOutcomeProofStore]*guestKernelOOMVictimProofRegistry

func registryForGuestKernelOOMVictimProofs(store *taskOutcomeProofStore) *guestKernelOOMVictimProofRegistry {
	if store == nil {
		return nil
	}
	if existing, ok := guestKernelOOMVictimProofRegistries.Load(store); ok {
		return existing.(*guestKernelOOMVictimProofRegistry)
	}
	created := &guestKernelOOMVictimProofRegistry{bySandbox: make(map[string]guestKernelOOMVictimProofState)}
	actual, _ := guestKernelOOMVictimProofRegistries.LoadOrStore(store, created)
	return actual.(*guestKernelOOMVictimProofRegistry)
}

func canonicalGuestOOMVictimTokenHex(value string) bool {
	if len(value) != 64 {
		return false
	}
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != value {
		return false
	}
	for _, b := range raw {
		if b != 0 {
			return true
		}
	}
	return false
}

func validateGuestKernelOOMVictimProof(proof GuestKernelOOMVictimProof) error {
	if proof.SandboxID == "" || strings.TrimSpace(proof.SandboxID) != proof.SandboxID {
		return fmt.Errorf("guest kernel OOM victim sandbox ID is not canonical")
	}
	if proof.Generation == 0 {
		return fmt.Errorf("guest kernel OOM victim generation is required")
	}
	if !canonicalGuestOOMVictimTokenHex(proof.RealizationTokenHex) {
		return fmt.Errorf("guest kernel OOM victim realization token is not canonical")
	}
	bootID, err := uuid.Parse(proof.GuestBootID)
	if err != nil || bootID.String() != proof.GuestBootID {
		return fmt.Errorf("guest kernel OOM victim boot ID is not canonical")
	}
	if proof.TID == 0 || proof.TGID == 0 || proof.StartTimeTicks == 0 {
		return fmt.Errorf("guest kernel OOM victim process identity is incomplete")
	}
	if proof.MainPID == 0 || proof.MainStartTimeTicks == 0 {
		return fmt.Errorf("guest kernel OOM victim main-process authority is incomplete")
	}
	if proof.RealizationStartedBootNS == 0 || proof.OutcomeObservedBootNS == 0 || proof.EventBootNS == 0 {
		return fmt.Errorf("guest kernel OOM victim boot-time evidence is incomplete")
	}
	if proof.OutcomeObservedBootNS < proof.RealizationStartedBootNS || proof.EventBootNS < proof.RealizationStartedBootNS || proof.EventBootNS > proof.OutcomeObservedBootNS {
		return fmt.Errorf("guest kernel OOM victim event is outside the exact realization window")
	}
	if proof.Source != guestKernelOOMVictimSource {
		return fmt.Errorf("guest kernel OOM victim source %q is not authoritative", proof.Source)
	}
	switch proof.Class {
	case GuestKernelOOMVictimClassMain:
		if proof.TGID != proof.MainPID || proof.StartTimeTicks != proof.MainStartTimeTicks {
			return fmt.Errorf("guest MAIN OOM victim proof does not match exact main lifetime")
		}
	case GuestKernelOOMVictimClassMember:
		if proof.CgroupV2ID == 0 {
			return fmt.Errorf("guest MEMBER OOM victim proof requires exact cgroup-v2 identity")
		}
	default:
		return fmt.Errorf("guest kernel OOM victim class %q is invalid", proof.Class)
	}
	return nil
}

func sameGuestKernelOOMVictimProof(a, b GuestKernelOOMVictimProof) bool {
	return a == b
}

func sameGuestKernelOOMVictimAuthority(a, b GuestKernelOOMVictimProof) bool {
	return a.SandboxID == b.SandboxID &&
		a.Generation == b.Generation &&
		a.RealizationTokenHex == b.RealizationTokenHex &&
		a.GuestBootID == b.GuestBootID &&
		a.MainPID == b.MainPID &&
		a.MainStartTimeTicks == b.MainStartTimeTicks &&
		a.RealizationStartedBootNS == b.RealizationStartedBootNS &&
		a.OutcomeObservedBootNS == b.OutcomeObservedBootNS &&
		a.Source == b.Source
}

func normalizedGuestKernelOOMVictimProofs(proofs []GuestKernelOOMVictimProof) ([]GuestKernelOOMVictimProof, error) {
	if len(proofs) == 0 {
		return nil, fmt.Errorf("guest kernel OOM victim proof set is empty")
	}
	if len(proofs) > maxGuestKernelOOMVictims {
		return nil, fmt.Errorf("guest kernel OOM victim proof set exceeds %d victims", maxGuestKernelOOMVictims)
	}
	out := append([]GuestKernelOOMVictimProof(nil), proofs...)
	for index, proof := range out {
		if err := validateGuestKernelOOMVictimProof(proof); err != nil {
			return nil, err
		}
		if index > 0 && !sameGuestKernelOOMVictimAuthority(out[0], proof) {
			return nil, fmt.Errorf("guest kernel OOM victim proof set mixes realization authority")
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EventBootNS != out[j].EventBootNS {
			return out[i].EventBootNS < out[j].EventBootNS
		}
		if out[i].TGID != out[j].TGID {
			return out[i].TGID < out[j].TGID
		}
		if out[i].TID != out[j].TID {
			return out[i].TID < out[j].TID
		}
		if out[i].StartTimeTicks != out[j].StartTimeTicks {
			return out[i].StartTimeTicks < out[j].StartTimeTicks
		}
		if out[i].Class != out[j].Class {
			return out[i].Class < out[j].Class
		}
		return out[i].CgroupV2ID < out[j].CgroupV2ID
	})
	for i := 1; i < len(out); i++ {
		if sameGuestKernelOOMVictimProof(out[i-1], out[i]) {
			return nil, fmt.Errorf("guest kernel OOM victim proof set contains a duplicate")
		}
	}
	return out, nil
}

func sameGuestKernelOOMVictimProofSet(a, b []GuestKernelOOMVictimProof) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameGuestKernelOOMVictimProof(a[i], b[i]) {
			return false
		}
	}
	return true
}

// AcceptGuestKernelOOMVictimProofs is the terminal host-side Wave 21 authority
// gate. Positive guest evidence is accepted only after an exact Wave 17 outcome
// exists for the same current generation and only when the opaque realization
// token still matches that generation. Existing accepted evidence is immutable.
func (s *taskOutcomeProofStore) AcceptGuestKernelOOMVictimProofs(sandboxID string, generation uint64, token [32]byte, proofs []GuestKernelOOMVictimProof) error {
	if s == nil {
		return fmt.Errorf("guest kernel OOM victim proof store is unavailable")
	}
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID || generation == 0 || !validGuestOOMVictimToken(token) {
		return fmt.Errorf("guest kernel OOM victim realization authority is invalid")
	}
	normalized, err := normalizedGuestKernelOOMVictimProofs(proofs)
	if err != nil {
		return err
	}
	tokenHex := hex.EncodeToString(token[:])
	for _, proof := range normalized {
		if proof.SandboxID != sandboxID || proof.Generation != generation || proof.RealizationTokenHex != tokenHex {
			return fmt.Errorf("guest kernel OOM victim proof does not match exact sandbox generation token")
		}
	}

	// Hold the Wave 17 authority read lock through token/proof commit so a new
	// realization or Create fence cannot race this terminal acceptance.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return fmt.Errorf("guest kernel OOM victim generation is not current")
	}
	outcome, ok := s.proofs[sandboxID]
	if !ok || outcome.Generation != generation {
		return fmt.Errorf("guest kernel OOM victim proof requires exact Wave17 outcome")
	}

	tokens := registryForGuestOOMVictimTokens(s)
	tokens.mu.RLock()
	state, ok := tokens.bySandbox[sandboxID]
	tokenMatches := ok && state.Generation == generation && state.Token == token && validGuestOOMVictimToken(state.Token)
	tokens.mu.RUnlock()
	if !tokenMatches {
		return fmt.Errorf("guest kernel OOM victim realization token does not match exact generation")
	}

	registry := registryForGuestKernelOOMVictimProofs(s)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if existing, ok := registry.bySandbox[sandboxID]; ok && existing.Generation == generation {
		if sameGuestKernelOOMVictimProofSet(existing.Proofs, normalized) {
			return nil
		}
		return fmt.Errorf("conflicting terminal guest kernel OOM victim proof set")
	}
	registry.bySandbox[sandboxID] = guestKernelOOMVictimProofState{
		Generation: generation,
		Proofs:     append([]GuestKernelOOMVictimProof(nil), normalized...),
	}
	return nil
}

func (s *taskOutcomeProofStore) listGuestKernelOOMVictimProofs() []GuestKernelOOMVictimProof {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	registry := registryForGuestKernelOOMVictimProofs(s)
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	var out []GuestKernelOOMVictimProof
	for sandboxID, state := range registry.bySandbox {
		generation := s.generations[sandboxID]
		if generation == 0 || generation != state.Generation || (s.fenced != nil && s.fenced[sandboxID]) {
			continue
		}
		out = append(out, state.Proofs...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SandboxID != out[j].SandboxID {
			return out[i].SandboxID < out[j].SandboxID
		}
		if out[i].Generation != out[j].Generation {
			return out[i].Generation < out[j].Generation
		}
		if out[i].EventBootNS != out[j].EventBootNS {
			return out[i].EventBootNS < out[j].EventBootNS
		}
		if out[i].TGID != out[j].TGID {
			return out[i].TGID < out[j].TGID
		}
		return out[i].TID < out[j].TID
	})
	return out
}

func (s *taskOutcomeProofStore) VisitGuestKernelOOMVictimProofs(visit func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string)) {
	if s == nil || visit == nil {
		return
	}
	for _, proof := range s.listGuestKernelOOMVictimProofs() {
		visit(
			proof.SandboxID,
			proof.Generation,
			proof.GuestBootID,
			proof.TID,
			proof.TGID,
			proof.StartTimeTicks,
			proof.EventBootNS,
			proof.CgroupV2ID,
			string(proof.Class),
			proof.RealizationStartedBootNS,
			proof.OutcomeObservedBootNS,
			proof.Source,
		)
	}
}

func (c *controllerLocal) VisitGuestKernelOOMVictimProofs(visit func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string)) {
	if c == nil || visit == nil {
		return
	}
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	store.VisitGuestKernelOOMVictimProofs(visit)
}
