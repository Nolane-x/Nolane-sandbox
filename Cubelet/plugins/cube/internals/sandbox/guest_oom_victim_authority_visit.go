// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

// VisitGuestKernelOOMVictimAuthorityProofs exposes the complete accepted Wave21
// proof without reconstructing any authority at the metrics layer. The legacy
// 12-field visitor remains available only for compatibility with older internal
// callers; new Wave21 transport must use this lossless authority visitor.
func (s *taskOutcomeProofStore) VisitGuestKernelOOMVictimAuthorityProofs(visit func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string)) {
	if s == nil || visit == nil {
		return
	}
	for _, proof := range s.listGuestKernelOOMVictimProofs() {
		var scope string
		switch proof.Class {
		case GuestKernelOOMVictimClassMain:
			scope = "main"
		case GuestKernelOOMVictimClassMember:
			scope = "member"
		default:
			continue
		}
		visit(
			proof.SandboxID,
			proof.Generation,
			proof.RealizationTokenHex,
			proof.GuestBootID,
			proof.TID,
			proof.TGID,
			proof.StartTimeTicks,
			proof.MainPID,
			proof.MainStartTimeTicks,
			scope,
			proof.EventBootNS,
			proof.CgroupV2ID,
			proof.RealizationStartedBootNS,
			proof.OutcomeObservedBootNS,
			proof.Source,
		)
	}
}

func (c *controllerLocal) VisitGuestKernelOOMVictimAuthorityProofs(visit func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string)) {
	if c == nil || visit == nil {
		return
	}
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	store.VisitGuestKernelOOMVictimAuthorityProofs(visit)
}
