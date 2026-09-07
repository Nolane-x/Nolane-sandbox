// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

// VisitRealizationEpochs exposes a read-only snapshot of current Wave24
// realization epoch authority to the trusted Cubelet management exporter. The
// callback is invoked after releasing the store lock so exporter behavior can
// never mutate or deadlock the lifecycle authority.
func (c *controllerLocal) VisitRealizationEpochs(visit func(string, uint64, [32]byte)) {
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	store.VisitRealizationEpochs(visit)
}

func (s *taskOutcomeProofStore) VisitRealizationEpochs(visit func(string, uint64, [32]byte)) {
	if s == nil || visit == nil {
		return
	}

	s.mu.RLock()
	epochs := make([]RealizationEpoch, 0, len(s.realizationEpochs))
	for sandboxID, epoch := range s.realizationEpochs {
		if sandboxID == "" || epoch.SandboxID != sandboxID || epoch.Generation == 0 || epoch.Token == ([32]byte{}) {
			continue
		}
		if s.fenced != nil && s.fenced[sandboxID] {
			continue
		}
		if s.generations[sandboxID] != epoch.Generation {
			continue
		}
		epochs = append(epochs, epoch)
	}
	s.mu.RUnlock()

	for _, epoch := range epochs {
		visit(epoch.SandboxID, epoch.Generation, epoch.Token)
	}
}
