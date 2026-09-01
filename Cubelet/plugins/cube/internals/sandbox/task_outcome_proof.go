// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"fmt"
	"sync"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TaskOutcomeProofSource string

const (
	TaskOutcomeProofSourceWait  TaskOutcomeProofSource = "containerd.task.wait"
	TaskOutcomeProofSourceState TaskOutcomeProofSource = "containerd.task.state"
)

type TaskOutcomeProof struct {
	SandboxID  string
	Generation uint64
	ExitCode   uint32
	ExitedAt   time.Time
	Source     TaskOutcomeProofSource
}

type TaskOutcomeProofProvider interface {
	TaskOutcomeProof(sandboxID string) (TaskOutcomeProof, bool)
}

type taskOutcomeCandidate struct {
	SandboxID string
	ExitCode  uint32
	ExitedAt  time.Time
	Source    TaskOutcomeProofSource
}

type taskOutcomeProofStore struct {
	mu          sync.RWMutex
	generations map[string]uint64
	proofs      map[string]TaskOutcomeProof
	fenced      map[string]bool
}

func newTaskOutcomeProofStore() *taskOutcomeProofStore {
	return &taskOutcomeProofStore{
		generations: make(map[string]uint64),
		proofs:      make(map[string]TaskOutcomeProof),
		fenced:      make(map[string]bool),
	}
}

// Clear removes all accepted authority for sandboxID and fences recovery until
// BeginRealization is called. This is the Create boundary: a late response from
// the previous runtime realization must not be adopted as a fresh proof.
func (s *taskOutcomeProofStore) Clear(sandboxID string) {
	if s == nil || sandboxID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fenced == nil {
		s.fenced = make(map[string]bool)
	}
	delete(s.proofs, sandboxID)
	delete(s.generations, sandboxID)
	s.fenced[sandboxID] = true
}

func (s *taskOutcomeProofStore) BeginRealization(sandboxID string) uint64 {
	if s == nil || sandboxID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generations == nil {
		s.generations = make(map[string]uint64)
	}
	if s.proofs == nil {
		s.proofs = make(map[string]TaskOutcomeProof)
	}
	if s.fenced == nil {
		s.fenced = make(map[string]bool)
	}
	s.generations[sandboxID]++
	delete(s.proofs, sandboxID)
	delete(s.fenced, sandboxID)
	return s.generations[sandboxID]
}

// RecoverRealization establishes a controller-local generation from a fresh
// authoritative runtime observation after controller restart. It never crosses
// an explicit Create fence. Existing active generations are returned unchanged.
func (s *taskOutcomeProofStore) RecoverRealization(sandboxID string) (uint64, bool) {
	if s == nil || sandboxID == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fenced != nil && s.fenced[sandboxID] {
		return 0, false
	}
	if s.generations == nil {
		s.generations = make(map[string]uint64)
	}
	if generation := s.generations[sandboxID]; generation != 0 {
		return generation, true
	}
	if s.proofs == nil {
		s.proofs = make(map[string]TaskOutcomeProof)
	}
	s.generations[sandboxID] = 1
	return 1, true
}

func (s *taskOutcomeProofStore) Record(candidate taskOutcomeCandidate) (TaskOutcomeProof, error) {
	if s == nil {
		return TaskOutcomeProof{}, fmt.Errorf("task outcome proof store is unavailable")
	}
	if candidate.SandboxID == "" {
		return TaskOutcomeProof{}, fmt.Errorf("task outcome proof sandbox ID is required")
	}
	if candidate.ExitedAt.IsZero() {
		return TaskOutcomeProof{}, fmt.Errorf("task outcome proof runtime exit timestamp is required")
	}
	if candidate.Source != TaskOutcomeProofSourceWait && candidate.Source != TaskOutcomeProofSourceState {
		return TaskOutcomeProof{}, fmt.Errorf("task outcome proof source %q is not authoritative", candidate.Source)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	generation := s.generations[candidate.SandboxID]
	if generation == 0 {
		return TaskOutcomeProof{}, fmt.Errorf("task outcome proof has no active realization for sandbox %s", candidate.SandboxID)
	}
	if s.proofs == nil {
		s.proofs = make(map[string]TaskOutcomeProof)
	}

	if existing, ok := s.proofs[candidate.SandboxID]; ok {
		if existing.Generation != generation {
			return TaskOutcomeProof{}, fmt.Errorf("task outcome proof generation conflict for sandbox %s", candidate.SandboxID)
		}
		if existing.ExitCode != candidate.ExitCode || !existing.ExitedAt.Equal(candidate.ExitedAt) {
			return TaskOutcomeProof{}, fmt.Errorf("conflicting exact task outcome for sandbox %s generation %d", candidate.SandboxID, generation)
		}
		return existing, nil
	}

	proof := TaskOutcomeProof{
		SandboxID:  candidate.SandboxID,
		Generation: generation,
		ExitCode:   candidate.ExitCode,
		ExitedAt:   candidate.ExitedAt.UTC(),
		Source:     candidate.Source,
	}
	s.proofs[candidate.SandboxID] = proof
	return proof, nil
}

func (s *taskOutcomeProofStore) Get(sandboxID string) (TaskOutcomeProof, bool) {
	if s == nil || sandboxID == "" {
		return TaskOutcomeProof{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	proof, ok := s.proofs[sandboxID]
	return proof, ok
}

func outcomeCandidateFromWait(sandboxID string, resp *task.WaitResponse) (taskOutcomeCandidate, error) {
	if resp == nil {
		return taskOutcomeCandidate{}, fmt.Errorf("task wait response for sandbox %s is nil", sandboxID)
	}
	exitedAt, err := authoritativeRuntimeTimestamp(sandboxID, "wait", resp.GetExitedAt())
	if err != nil {
		return taskOutcomeCandidate{}, err
	}
	return taskOutcomeCandidate{
		SandboxID: sandboxID,
		ExitCode:  resp.GetExitStatus(),
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceWait,
	}, nil
}

func outcomeCandidateFromState(sandboxID string, resp *task.StateResponse) (taskOutcomeCandidate, bool, error) {
	if resp == nil {
		return taskOutcomeCandidate{}, false, fmt.Errorf("task state response for sandbox %s is nil", sandboxID)
	}
	if resp.GetStatus() != tasktypes.Status_STOPPED {
		return taskOutcomeCandidate{}, false, nil
	}
	exitedAt, err := authoritativeRuntimeTimestamp(sandboxID, "state", resp.GetExitedAt())
	if err != nil {
		return taskOutcomeCandidate{}, false, err
	}
	return taskOutcomeCandidate{
		SandboxID: sandboxID,
		ExitCode:  resp.GetExitStatus(),
		ExitedAt:  exitedAt,
		Source:    TaskOutcomeProofSourceState,
	}, true, nil
}

func authoritativeRuntimeTimestamp(sandboxID, source string, ts *timestamppb.Timestamp) (time.Time, error) {
	if ts == nil {
		return time.Time{}, fmt.Errorf("task %s response for sandbox %s has no runtime exit timestamp", source, sandboxID)
	}
	if err := ts.CheckValid(); err != nil {
		return time.Time{}, fmt.Errorf("task %s response for sandbox %s has invalid runtime exit timestamp: %w", source, sandboxID, err)
	}
	exitedAt := ts.AsTime().UTC()
	if exitedAt.IsZero() {
		return time.Time{}, fmt.Errorf("task %s response for sandbox %s has zero runtime exit timestamp", source, sandboxID)
	}
	return exitedAt, nil
}
