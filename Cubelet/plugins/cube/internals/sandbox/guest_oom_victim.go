// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"fmt"
	"sync"
)

type guestOOMVictimTokenState struct {
	Generation uint64
	Token      [32]byte
}

type guestOOMVictimTokenRegistryState struct {
	mu        sync.RWMutex
	bySandbox map[string]guestOOMVictimTokenState
}

// Wave 21 token authority is scoped to the taskOutcomeProofStore instance.
// The Wave 17 generation map remains the only generation authority; this
// registry never invents, increments, recovers, or persists a generation.
var guestOOMVictimTokenRegistries sync.Map // map[*taskOutcomeProofStore]*guestOOMVictimTokenRegistryState

func registryForGuestOOMVictimTokens(store *taskOutcomeProofStore) *guestOOMVictimTokenRegistryState {
	if store == nil {
		return nil
	}
	if existing, ok := guestOOMVictimTokenRegistries.Load(store); ok {
		return existing.(*guestOOMVictimTokenRegistryState)
	}
	created := &guestOOMVictimTokenRegistryState{bySandbox: make(map[string]guestOOMVictimTokenState)}
	actual, _ := guestOOMVictimTokenRegistries.LoadOrStore(store, created)
	return actual.(*guestOOMVictimTokenRegistryState)
}

func validGuestOOMVictimToken(token [32]byte) bool {
	for _, b := range token {
		if b != 0 {
			return true
		}
	}
	return false
}

// BeginGuestOOMVictimRealization attaches an opaque Wave 21 nonce only to an
// already-open exact Wave 17 generation. It cannot create generation authority.
func (s *taskOutcomeProofStore) BeginGuestOOMVictimRealization(sandboxID string, generation uint64, token [32]byte) error {
	if s == nil {
		return fmt.Errorf("guest OOM victim token store is unavailable")
	}
	if sandboxID == "" {
		return fmt.Errorf("guest OOM victim sandbox ID is required")
	}
	if generation == 0 {
		return fmt.Errorf("guest OOM victim generation is required")
	}
	if !validGuestOOMVictimToken(token) {
		return fmt.Errorf("guest OOM victim realization token must be non-zero")
	}

	// Keep the Wave 17 generation check stable until the token write commits.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return fmt.Errorf("guest OOM victim generation %d is not current for sandbox %s", generation, sandboxID)
	}

	registry := registryForGuestOOMVictimTokens(s)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.bySandbox[sandboxID] = guestOOMVictimTokenState{Generation: generation, Token: token}
	return nil
}

// GuestOOMVictimToken returns authority only while the requested generation is
// still the store's exact current Wave 17 generation. Stale registry entries are
// therefore inert across BeginRealization and Create fences.
func (s *taskOutcomeProofStore) GuestOOMVictimToken(sandboxID string, generation uint64) ([32]byte, bool) {
	if s == nil || sandboxID == "" || generation == 0 {
		return [32]byte{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return [32]byte{}, false
	}

	registry := registryForGuestOOMVictimTokens(s)
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	state, ok := registry.bySandbox[sandboxID]
	if !ok || state.Generation != generation || !validGuestOOMVictimToken(state.Token) {
		return [32]byte{}, false
	}
	return state.Token, true
}
