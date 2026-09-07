// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"crypto/rand"
	"fmt"
)

// RealizationEpoch is Cubelet-local identity for one current sandbox
// realization. The random token prevents a cleared sandbox whose numeric
// generation later resets from aliasing an earlier realization.
type RealizationEpoch struct {
	SandboxID  string
	Generation uint64
	Token      [32]byte
}

func secureRealizationEpochToken() ([32]byte, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return [32]byte{}, fmt.Errorf("mint realization epoch token: %w", err)
	}
	if token == ([32]byte{}) {
		return [32]byte{}, fmt.Errorf("mint realization epoch token: secure random source returned zero token")
	}
	return token, nil
}

// BeginRealizationEpoch advances the Wave17 realization generation and binds a
// fresh non-zero epoch token to that exact generation in the same store lock.
// Token minting happens before lifecycle mutation so a mint failure cannot
// create partial Wave24 authority.
func (s *taskOutcomeProofStore) BeginRealizationEpoch(sandboxID string) (RealizationEpoch, error) {
	if s == nil {
		return RealizationEpoch{}, fmt.Errorf("realization epoch store is unavailable")
	}
	if sandboxID == "" {
		return RealizationEpoch{}, fmt.Errorf("realization epoch sandbox ID is required")
	}

	token, err := secureRealizationEpochToken()
	if err != nil {
		return RealizationEpoch{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	clearGuestKernelOOMVictimLifecycleState(s, sandboxID)
	if s.generations == nil {
		s.generations = make(map[string]uint64)
	}
	if s.proofs == nil {
		s.proofs = make(map[string]TaskOutcomeProof)
	}
	if s.fenced == nil {
		s.fenced = make(map[string]bool)
	}
	if s.realizationEpochs == nil {
		s.realizationEpochs = make(map[string]RealizationEpoch)
	}
	if s.oomBaselines == nil {
		s.oomBaselines = make(map[string]realizationOOMBaseline)
	}
	if s.oomProofs == nil {
		s.oomProofs = make(map[string]RealizationOOMProof)
	}
	if s.oomFinalized == nil {
		s.oomFinalized = make(map[string]uint64)
	}
	if s.hostProcessLifetimes == nil {
		s.hostProcessLifetimes = make(map[string]*hostProcessLifetime)
	}
	if s.hostProcessPlacements == nil {
		s.hostProcessPlacements = make(map[string]HostProcessPlacementProof)
	}
	if s.hostProcessBindings == nil {
		s.hostProcessBindings = make(map[string]HostProcessRealizationBinding)
	}
	if s.victimWindows == nil {
		s.victimWindows = make(map[string]victimWindow)
	}
	if s.kernelVictimProofs == nil {
		s.kernelVictimProofs = make(map[string]HostProcessKernelOOMVictimProof)
	}
	if s.hostProcessLifetimes[sandboxID] == nil {
		s.hostProcessLifetimes[sandboxID] = &hostProcessLifetime{}
	}

	s.generations[sandboxID]++
	generation := s.generations[sandboxID]
	delete(s.proofs, sandboxID)
	delete(s.oomBaselines, sandboxID)
	delete(s.oomProofs, sandboxID)
	delete(s.oomFinalized, sandboxID)
	delete(s.hostProcessBindings, sandboxID)
	delete(s.victimWindows, sandboxID)
	delete(s.kernelVictimProofs, sandboxID)
	delete(s.fenced, sandboxID)

	epoch := RealizationEpoch{
		SandboxID:  sandboxID,
		Generation: generation,
		Token:      token,
	}
	s.realizationEpochs[sandboxID] = epoch
	return epoch, nil
}

// CurrentRealizationEpoch returns only authority that is still exactly bound to
// the current generation. Recovered generations intentionally have no epoch:
// Wave24 does not reconstruct epoch authority after producer restart.
func (s *taskOutcomeProofStore) CurrentRealizationEpoch(sandboxID string) (RealizationEpoch, bool) {
	if s == nil || sandboxID == "" {
		return RealizationEpoch{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	epoch, ok := s.realizationEpochs[sandboxID]
	if !ok || epoch.SandboxID != sandboxID || epoch.Generation == 0 || epoch.Token == ([32]byte{}) {
		return RealizationEpoch{}, false
	}
	if s.fenced != nil && s.fenced[sandboxID] {
		return RealizationEpoch{}, false
	}
	if s.generations[sandboxID] != epoch.Generation {
		return RealizationEpoch{}, false
	}
	return epoch, true
}

// IsCurrentRealizationEpoch validates an exact descriptive tuple without
// mutating lifecycle state or promoting stale authority.
func (s *taskOutcomeProofStore) IsCurrentRealizationEpoch(sandboxID string, generation uint64, token [32]byte) bool {
	if generation == 0 || token == ([32]byte{}) {
		return false
	}
	epoch, ok := s.CurrentRealizationEpoch(sandboxID)
	if !ok {
		return false
	}
	return epoch.Generation == generation && epoch.Token == token
}
