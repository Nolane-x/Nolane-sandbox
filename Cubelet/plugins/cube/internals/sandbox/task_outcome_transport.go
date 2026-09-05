// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import "sort"

// TaskOutcomeProofLister exposes a detached snapshot of currently accepted
// Wave 16 task-outcome proofs. It does not recover or synthesize proof.
type TaskOutcomeProofLister interface {
	ListTaskOutcomeProofs() []TaskOutcomeProof
}

func (s *taskOutcomeProofStore) List() []TaskOutcomeProof {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	proofs := make([]TaskOutcomeProof, 0, len(s.proofs))
	for _, proof := range s.proofs {
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

func (c *controllerLocal) ListTaskOutcomeProofs() []TaskOutcomeProof {
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return nil
	}
	return store.List()
}

var _ TaskOutcomeProofLister = (*controllerLocal)(nil)
