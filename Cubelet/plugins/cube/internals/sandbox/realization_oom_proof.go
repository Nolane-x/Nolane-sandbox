// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"sort"
	"strings"
	"time"
)

const realizationOOMSignal = "kernel.cgroup.memory.oom_kill"

type realizationOOMSnapshot struct {
	CGroupPath    string
	OOMKillsKnown bool
	OOMKillsTotal uint64
	CapturedAt    time.Time
}

type realizationOOMBaseline struct {
	SandboxID  string
	Generation uint64
	CGroupPath string
	OOMKills   uint64
	CapturedAt time.Time
}

// RealizationOOMProof proves only that the kernel cgroup OOM-kill counter
// changed during one controller-local task realization. It does not identify
// the killed process and must not be interpreted as a main-task OOM cause.
type RealizationOOMProof struct {
	SandboxID        string
	Generation       uint64
	CGroupPath       string
	BaselineOOMKills uint64
	FinalOOMKills    uint64
	OOMKills         uint64
	BaselineAt       time.Time
	ObservedAt       time.Time
	ExitedAt         time.Time
	OutcomeSource    TaskOutcomeProofSource
}

func (s *taskOutcomeProofStore) RecordRealizationOOMBaseline(sandboxID string, generation uint64, snapshot realizationOOMSnapshot) bool {
	if s == nil || sandboxID == "" || generation == 0 || !validRealizationOOMSnapshot(snapshot) {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fenced != nil && s.fenced[sandboxID] {
		return false
	}
	if s.generations[sandboxID] != generation {
		return false
	}
	if s.oomFinalized != nil && s.oomFinalized[sandboxID] == generation {
		return false
	}
	if s.oomBaselines == nil {
		s.oomBaselines = make(map[string]realizationOOMBaseline)
	}

	candidate := realizationOOMBaseline{
		SandboxID:  sandboxID,
		Generation: generation,
		CGroupPath: snapshot.CGroupPath,
		OOMKills:   snapshot.OOMKillsTotal,
		CapturedAt: snapshot.CapturedAt.UTC(),
	}
	if existing, ok := s.oomBaselines[sandboxID]; ok {
		return existing.Generation == candidate.Generation &&
			existing.CGroupPath == candidate.CGroupPath &&
			existing.OOMKills == candidate.OOMKills &&
			existing.CapturedAt.Equal(candidate.CapturedAt)
	}
	s.oomBaselines[sandboxID] = candidate
	return true
}

func (s *taskOutcomeProofStore) FinalizeRealizationOOM(outcome TaskOutcomeProof, snapshot realizationOOMSnapshot) (RealizationOOMProof, bool) {
	if s == nil || outcome.SandboxID == "" || outcome.Generation == 0 {
		return RealizationOOMProof{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fenced != nil && s.fenced[outcome.SandboxID] {
		return RealizationOOMProof{}, false
	}
	if s.generations[outcome.SandboxID] != outcome.Generation {
		return RealizationOOMProof{}, false
	}
	accepted, ok := s.proofs[outcome.SandboxID]
	if !ok || !sameTaskOutcomeProof(accepted, outcome) {
		return RealizationOOMProof{}, false
	}

	if s.oomFinalized == nil {
		s.oomFinalized = make(map[string]uint64)
	}
	if s.oomFinalized[outcome.SandboxID] == outcome.Generation {
		proof, ok := s.oomProofs[outcome.SandboxID]
		if !ok || proof.Generation != outcome.Generation {
			return RealizationOOMProof{}, false
		}
		return proof, true
	}

	// The first finalization attempt for an accepted generation is terminal.
	// Mark it before validating the snapshot so later calls cannot repair an
	// unknown window using evidence sampled after the trusted exit boundary.
	s.oomFinalized[outcome.SandboxID] = outcome.Generation

	baseline, ok := s.oomBaselines[outcome.SandboxID]
	if !ok || baseline.Generation != outcome.Generation || !validRealizationOOMSnapshot(snapshot) {
		return RealizationOOMProof{}, false
	}
	if baseline.CGroupPath != snapshot.CGroupPath || snapshot.OOMKillsTotal < baseline.OOMKills {
		return RealizationOOMProof{}, false
	}
	if outcome.Source != TaskOutcomeProofSourceWait && outcome.Source != TaskOutcomeProofSourceState {
		return RealizationOOMProof{}, false
	}
	baselineAt := baseline.CapturedAt.UTC()
	exitedAt := outcome.ExitedAt.UTC()
	observedAt := snapshot.CapturedAt.UTC()
	if exitedAt.Before(baselineAt) || observedAt.Before(exitedAt) {
		return RealizationOOMProof{}, false
	}

	proof := RealizationOOMProof{
		SandboxID:        outcome.SandboxID,
		Generation:       outcome.Generation,
		CGroupPath:       baseline.CGroupPath,
		BaselineOOMKills: baseline.OOMKills,
		FinalOOMKills:    snapshot.OOMKillsTotal,
		OOMKills:         snapshot.OOMKillsTotal - baseline.OOMKills,
		BaselineAt:       baselineAt,
		ObservedAt:       observedAt,
		ExitedAt:         exitedAt,
		OutcomeSource:    outcome.Source,
	}
	if s.oomProofs == nil {
		s.oomProofs = make(map[string]RealizationOOMProof)
	}
	s.oomProofs[outcome.SandboxID] = proof
	return proof, true
}

func (s *taskOutcomeProofStore) ListRealizationOOMProofs() []RealizationOOMProof {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	proofs := make([]RealizationOOMProof, 0, len(s.oomProofs))
	for _, proof := range s.oomProofs {
		proofs = append(proofs, proof)
	}
	s.mu.RUnlock()

	sort.Slice(proofs, func(i, j int) bool {
		if proofs[i].SandboxID != proofs[j].SandboxID {
			return proofs[i].SandboxID < proofs[j].SandboxID
		}
		return proofs[i].Generation < proofs[j].Generation
	})
	return proofs
}

func validRealizationOOMSnapshot(snapshot realizationOOMSnapshot) bool {
	return snapshot.OOMKillsKnown &&
		strings.TrimSpace(snapshot.CGroupPath) != "" &&
		strings.TrimSpace(snapshot.CGroupPath) == snapshot.CGroupPath &&
		!snapshot.CapturedAt.IsZero()
}

func sameTaskOutcomeProof(a, b TaskOutcomeProof) bool {
	return a.SandboxID == b.SandboxID &&
		a.Generation == b.Generation &&
		a.ExitCode == b.ExitCode &&
		a.Source == b.Source &&
		a.ExitedAt.Equal(b.ExitedAt)
}

func (c *controllerLocal) SetRealizationOOMSnapshotReader(reader func(context.Context, string) (string, bool, uint64, time.Time, error)) {
	if c == nil {
		return
	}
	c.realizationOOMSnapshotMu.Lock()
	c.realizationOOMSnapshotReader = reader
	c.realizationOOMSnapshotMu.Unlock()
}

func (c *controllerLocal) realizationOOMReader() func(context.Context, string) (string, bool, uint64, time.Time, error) {
	if c == nil {
		return nil
	}
	c.realizationOOMSnapshotMu.RLock()
	reader := c.realizationOOMSnapshotReader
	c.realizationOOMSnapshotMu.RUnlock()
	return reader
}

func (c *controllerLocal) captureRealizationOOMBaseline(ctx context.Context, sandboxID string, generation uint64) {
	reader := c.realizationOOMReader()
	if reader == nil || generation == 0 {
		return
	}
	path, known, total, capturedAt, err := reader(ctx, sandboxID)
	if err != nil {
		return
	}
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	store.RecordRealizationOOMBaseline(sandboxID, generation, realizationOOMSnapshot{
		CGroupPath:    path,
		OOMKillsKnown: known,
		OOMKillsTotal: total,
		CapturedAt:    capturedAt,
	})
}

func (c *controllerLocal) finalizeRealizationOOM(ctx context.Context, outcome TaskOutcomeProof) {
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	reader := c.realizationOOMReader()
	if reader == nil {
		store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{})
		return
	}
	path, known, total, capturedAt, err := reader(ctx, outcome.SandboxID)
	if err != nil {
		store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{})
		return
	}
	store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{
		CGroupPath:    path,
		OOMKillsKnown: known,
		OOMKillsTotal: total,
		CapturedAt:    capturedAt,
	})
}

func (c *controllerLocal) ListRealizationOOMProofs() []RealizationOOMProof {
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return nil
	}
	return store.ListRealizationOOMProofs()
}

// VisitRealizationOOMProofs is package-neutral so management exporters can
// consume exact evidence without importing the sandbox package.
func (c *controllerLocal) VisitRealizationOOMProofs(visit func(string, uint64, string, uint64, uint64, uint64, time.Time, time.Time, time.Time, string)) {
	if visit == nil {
		return
	}
	for _, proof := range c.ListRealizationOOMProofs() {
		visit(
			proof.SandboxID,
			proof.Generation,
			proof.CGroupPath,
			proof.BaselineOOMKills,
			proof.FinalOOMKills,
			proof.OOMKills,
			proof.BaselineAt,
			proof.ObservedAt,
			proof.ExitedAt,
			string(proof.OutcomeSource),
		)
	}
}
