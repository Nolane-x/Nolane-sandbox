package resourcemetrics

func newProductionTaskEvidenceService(
	cache *SandboxResourceCache,
	outcomes taskOutcomeProofVisitor,
	oom realizationOOMProofVisitor,
	hostIdentity hostProcessIdentityProofVisitor,
	hostVictims hostKernelOOMVictimProofVisitor,
	guestVictims guestKernelOOMVictimProofVisitor,
) *Service {
	return newServiceWithAllTaskEvidenceAndKernelVictimsAndGuestVictims(
		cache,
		outcomes,
		oom,
		hostIdentity,
		hostVictims,
		guestVictims,
	)
}
