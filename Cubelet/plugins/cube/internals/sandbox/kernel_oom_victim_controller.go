package sandbox

import (
	"context"

	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/kernelvictim"
)

type kernelVictimSource interface {
	Find(bootID string, tgid uint32, startTimeTicks, minBootNS, maxBootNS uint64) (kernelvictim.Event, bool)
}

func (c *controllerLocal) configureKernelVictimCollector(ctx context.Context) {
	if c == nil {
		return
	}
	c.kernelVictimBootTimeNS = kernelvictim.BootTimeNS
	c.kernelVictimCgroupV2Resolver = kernelvictim.ResolveCgroupV2ID
	source, err := kernelvictim.StartBestEffortCollector(ctx)
	if err != nil {
		// Wave 20 is observational-only. Missing BTF, verifier/helper support,
		// tracepoint access or privilege must never prevent Cubelet startup.
		return
	}
	c.kernelVictimSource = source
}

func (c *controllerLocal) beginKernelVictimWindow(sandboxID string, generation uint64) {
	if c == nil || c.kernelVictimBootTimeNS == nil || generation == 0 {
		return
	}
	bootNS, err := c.kernelVictimBootTimeNS()
	if err != nil || bootNS == 0 {
		return
	}
	if store := c.ensureTaskOutcomeProofStore(); store != nil {
		store.RecordVictimWindowStart(sandboxID, generation, bootNS)
	}
}

func (c *controllerLocal) closeKernelVictimWindow(proof TaskOutcomeProof) {
	if c == nil || c.kernelVictimBootTimeNS == nil || proof.SandboxID == "" || proof.Generation == 0 {
		return
	}
	bootNS, err := c.kernelVictimBootTimeNS()
	if err != nil || bootNS == 0 {
		return
	}
	if store := c.ensureTaskOutcomeProofStore(); store != nil {
		store.RecordVictimOutcomeObserved(proof.SandboxID, proof.Generation, bootNS)
	}
}

func (s *taskOutcomeProofStore) KernelVictimCorrelationContext(sandboxID string, generation uint64) (HostProcessRealizationBinding, victimWindow, bool) {
	if s == nil || sandboxID == "" || generation == 0 {
		return HostProcessRealizationBinding{}, victimWindow{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return HostProcessRealizationBinding{}, victimWindow{}, false
	}
	outcome, outcomeOK := s.proofs[sandboxID]
	binding, bindingOK := s.hostProcessBindings[sandboxID]
	window, windowOK := s.victimWindows[sandboxID]
	if !outcomeOK || outcome.Generation != generation || !bindingOK || binding.Generation != generation || !windowOK || window.Generation != generation {
		return HostProcessRealizationBinding{}, victimWindow{}, false
	}
	if window.StartedBootNS == 0 || window.OutcomeObservedBootNS == 0 || window.OutcomeObservedBootNS < window.StartedBootNS {
		return HostProcessRealizationBinding{}, victimWindow{}, false
	}
	return binding, window, true
}

func (c *controllerLocal) finalizeHostKernelOOMVictim(proof TaskOutcomeProof) {
	if c == nil || c.kernelVictimSource == nil || proof.SandboxID == "" || proof.Generation == 0 {
		return
	}
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	binding, window, ok := store.KernelVictimCorrelationContext(proof.SandboxID, proof.Generation)
	if !ok {
		return
	}
	event, ok := c.kernelVictimSource.Find(
		binding.BootID,
		binding.HostPID,
		binding.StartTimeTicks,
		window.StartedBootNS,
		window.OutcomeObservedBootNS,
	)
	if !ok {
		return
	}
	var expectedCgroupV2ID uint64
	var cgroupKnown bool
	if c.kernelVictimCgroupV2Resolver != nil {
		expectedCgroupV2ID, cgroupKnown = c.kernelVictimCgroupV2Resolver(binding.CGroupPath)
	}
	store.FinalizeHostKernelOOMVictim(proof.SandboxID, event, expectedCgroupV2ID, cgroupKnown)
}
