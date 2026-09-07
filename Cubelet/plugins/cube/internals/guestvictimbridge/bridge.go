// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package guestvictimbridge

import (
	"fmt"
	"sync"
)

const maxStartBindings = 4096

type StartBinding struct {
	SandboxID  string
	Generation uint64
	Token      [32]byte
}

type bindingPhase uint8

const (
	bindingPending bindingPhase = iota + 1
	bindingClaimed
	bindingBound
	bindingUnavailable
)

type bindingEntry struct {
	binding StartBinding
	phase   bindingPhase
	seq     uint64
}

var startBindings = struct {
	sync.Mutex
	bySandbox map[string]bindingEntry
	nextSeq   uint64
}{
	bySandbox: make(map[string]bindingEntry),
}

func validBinding(binding StartBinding) bool {
	if binding.SandboxID == "" || binding.Generation == 0 {
		return false
	}
	for _, b := range binding.Token {
		if b != 0 {
			return true
		}
	}
	return false
}

func sameBinding(a, b StartBinding) bool {
	return a == b
}

// PublishStartBinding publishes one exact controller-owned generation token.
// A newer generation may fence an older entry; same-generation conflicts never
// replace the first token.
func PublishStartBinding(binding StartBinding) error {
	if !validBinding(binding) {
		return fmt.Errorf("guest OOM victim start binding is invalid")
	}

	startBindings.Lock()
	defer startBindings.Unlock()
	if existing, ok := startBindings.bySandbox[binding.SandboxID]; ok {
		if existing.binding.Generation > binding.Generation {
			return fmt.Errorf("guest OOM victim start binding generation is stale")
		}
		if existing.binding.Generation == binding.Generation {
			if !sameBinding(existing.binding, binding) {
				return fmt.Errorf("conflicting guest OOM victim token for generation %d", binding.Generation)
			}
			return nil
		}
	}

	if _, exists := startBindings.bySandbox[binding.SandboxID]; !exists && len(startBindings.bySandbox) >= maxStartBindings {
		evictOldestBindingLocked()
	}
	startBindings.nextSeq++
	startBindings.bySandbox[binding.SandboxID] = bindingEntry{
		binding: binding,
		phase:   bindingPending,
		seq:     startBindings.nextSeq,
	}
	return nil
}

// CurrentStartBinding is a read-only view of an unclaimed exact binding.
func CurrentStartBinding(sandboxID string) (StartBinding, bool) {
	if sandboxID == "" {
		return StartBinding{}, false
	}
	startBindings.Lock()
	defer startBindings.Unlock()
	entry, ok := startBindings.bySandbox[sandboxID]
	if !ok || entry.phase != bindingPending {
		return StartBinding{}, false
	}
	return entry.binding, true
}

// ClaimStartBinding atomically reserves the pending binding for the one live
// CubeBox main-task bind attempt. A second claimant cannot observe it.
func ClaimStartBinding(sandboxID string) (StartBinding, bool) {
	if sandboxID == "" {
		return StartBinding{}, false
	}
	startBindings.Lock()
	defer startBindings.Unlock()
	entry, ok := startBindings.bySandbox[sandboxID]
	if !ok || entry.phase != bindingPending {
		return StartBinding{}, false
	}
	entry.phase = bindingClaimed
	startBindings.bySandbox[sandboxID] = entry
	return entry.binding, true
}

// MarkBound terminally records successful delivery only when the exact claimed
// generation/token is still current. Stale callbacks cannot mutate new state.
func MarkBound(binding StartBinding) bool {
	return markTerminal(binding, bindingBound)
}

// IsBound reports only whether this exact generation/token completed the live
// pre-Start bind. It is read-only and never promotes pending/unavailable state.
func IsBound(binding StartBinding) bool {
	if !validBinding(binding) {
		return false
	}
	startBindings.Lock()
	defer startBindings.Unlock()
	entry, ok := startBindings.bySandbox[binding.SandboxID]
	return ok && entry.phase == bindingBound && sameBinding(entry.binding, binding)
}

// MarkUnavailable terminally records a failed delivery only when the exact
// claimed generation/token is still current. Wave 21 remains observational.
func MarkUnavailable(binding StartBinding) bool {
	return markTerminal(binding, bindingUnavailable)
}

func markTerminal(binding StartBinding, phase bindingPhase) bool {
	if !validBinding(binding) {
		return false
	}
	startBindings.Lock()
	defer startBindings.Unlock()
	entry, ok := startBindings.bySandbox[binding.SandboxID]
	if !ok || entry.phase != bindingClaimed || !sameBinding(entry.binding, binding) {
		return false
	}
	entry.phase = phase
	startBindings.bySandbox[binding.SandboxID] = entry
	return true
}

// Clear fences all bridge authority for sandboxID. Controller Create and each
// new Start call this before publishing a fresh generation.
func Clear(sandboxID string) {
	if sandboxID == "" {
		return
	}
	startBindings.Lock()
	delete(startBindings.bySandbox, sandboxID)
	startBindings.Unlock()
}

func evictOldestBindingLocked() {
	var (
		oldestID  string
		oldestSeq uint64
	)
	for sandboxID, entry := range startBindings.bySandbox {
		if oldestID == "" || entry.seq < oldestSeq {
			oldestID = sandboxID
			oldestSeq = entry.seq
		}
	}
	if oldestID != "" {
		delete(startBindings.bySandbox, oldestID)
	}
}
