package sandbox

import (
	"sort"

	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/kernelvictim"
)

const HostProcessKernelOOMVictimSource = "kernel.oom.mark_victim.raw_tracepoint"

type victimWindow struct {
	Generation            uint64
	StartedBootNS         uint64
	OutcomeObservedBootNS uint64
}

type HostProcessKernelOOMVictimProof struct {
	SandboxID          string
	Generation         uint64
	BootID             string
	HostPID            uint32
	VictimTID          uint32
	StartTimeTicks     uint64
	CGroupPath         string
	EventBootTimeNS    uint64
	CgroupV2ID         uint64
	CgroupV2Correlated bool
	Source             string
}

func (s *taskOutcomeProofStore) ensureVictimMapsLocked() {
	if s.victimWindows == nil {
		s.victimWindows = make(map[string]victimWindow)
	}
	if s.kernelVictimProofs == nil {
		s.kernelVictimProofs = make(map[string]HostProcessKernelOOMVictimProof)
	}
}

func (s *taskOutcomeProofStore) RecordVictimWindowStart(sandboxID string, generation, startedBootNS uint64) bool {
	if s == nil || sandboxID == "" || generation == 0 || startedBootNS == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureVictimMapsLocked()
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return false
	}
	if existing, ok := s.victimWindows[sandboxID]; ok {
		return existing.Generation == generation && existing.StartedBootNS == startedBootNS
	}
	s.victimWindows[sandboxID] = victimWindow{Generation: generation, StartedBootNS: startedBootNS}
	return true
}

func (s *taskOutcomeProofStore) RecordVictimOutcomeObserved(sandboxID string, generation, observedBootNS uint64) bool {
	if s == nil || sandboxID == "" || generation == 0 || observedBootNS == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureVictimMapsLocked()
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return false
	}
	window, ok := s.victimWindows[sandboxID]
	if !ok || window.Generation != generation || window.StartedBootNS == 0 || observedBootNS < window.StartedBootNS {
		return false
	}
	if window.OutcomeObservedBootNS != 0 {
		return window.OutcomeObservedBootNS == observedBootNS
	}
	window.OutcomeObservedBootNS = observedBootNS
	s.victimWindows[sandboxID] = window
	return true
}

func (s *taskOutcomeProofStore) VictimWindow(sandboxID string) (victimWindow, bool) {
	if s == nil || sandboxID == "" {
		return victimWindow{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	window, ok := s.victimWindows[sandboxID]
	return window, ok
}

func (s *taskOutcomeProofStore) FinalizeHostKernelOOMVictim(
	sandboxID string,
	event kernelvictim.Event,
	expectedCgroupV2ID uint64,
	cgroupResolverKnown bool,
) (HostProcessKernelOOMVictimProof, bool) {
	if s == nil || sandboxID == "" {
		return HostProcessKernelOOMVictimProof{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureVictimMapsLocked()

	generation := s.generations[sandboxID]
	if generation == 0 || (s.fenced != nil && s.fenced[sandboxID]) {
		return HostProcessKernelOOMVictimProof{}, false
	}
	outcome, ok := s.proofs[sandboxID]
	if !ok || outcome.Generation != generation {
		return HostProcessKernelOOMVictimProof{}, false
	}
	binding, ok := s.hostProcessBindings[sandboxID]
	if !ok || binding.Generation != generation || validateHostProcessCgroupPath(binding.CGroupPath) != nil {
		return HostProcessKernelOOMVictimProof{}, false
	}
	window, ok := s.victimWindows[sandboxID]
	if !ok || window.Generation != generation || window.StartedBootNS == 0 || window.OutcomeObservedBootNS == 0 {
		return HostProcessKernelOOMVictimProof{}, false
	}
	if event.BootID == "" || event.BootID != binding.BootID || event.TGID == 0 || event.TGID != binding.HostPID || event.VictimTID == 0 {
		return HostProcessKernelOOMVictimProof{}, false
	}
	if event.StartTimeTicks == 0 || event.StartTimeTicks != binding.StartTimeTicks || event.EventBootTimeNS < window.StartedBootNS || event.EventBootTimeNS > window.OutcomeObservedBootNS {
		return HostProcessKernelOOMVictimProof{}, false
	}
	if oom, exists := s.oomProofs[sandboxID]; exists {
		if oom.Generation != generation || oom.CGroupPath != binding.CGroupPath {
			return HostProcessKernelOOMVictimProof{}, false
		}
	}

	proof := HostProcessKernelOOMVictimProof{
		SandboxID:      sandboxID,
		Generation:     generation,
		BootID:         binding.BootID,
		HostPID:        binding.HostPID,
		VictimTID:      event.VictimTID,
		StartTimeTicks: binding.StartTimeTicks,
		CGroupPath:     binding.CGroupPath,
		EventBootTimeNS: event.EventBootTimeNS,
		Source:         HostProcessKernelOOMVictimSource,
	}
	if cgroupResolverKnown {
		if expectedCgroupV2ID == 0 || event.CgroupV2ID == 0 || event.CgroupV2ID != expectedCgroupV2ID {
			return HostProcessKernelOOMVictimProof{}, false
		}
		proof.CgroupV2ID = event.CgroupV2ID
		proof.CgroupV2Correlated = true
	}

	if existing, exists := s.kernelVictimProofs[sandboxID]; exists {
		if sameHostKernelVictimProof(existing, proof) {
			return existing, true
		}
		return HostProcessKernelOOMVictimProof{}, false
	}
	s.kernelVictimProofs[sandboxID] = proof
	return proof, true
}

func sameHostKernelVictimProof(a, b HostProcessKernelOOMVictimProof) bool {
	return a.SandboxID == b.SandboxID &&
		a.Generation == b.Generation &&
		a.BootID == b.BootID &&
		a.HostPID == b.HostPID &&
		a.VictimTID == b.VictimTID &&
		a.StartTimeTicks == b.StartTimeTicks &&
		a.CGroupPath == b.CGroupPath &&
		a.EventBootTimeNS == b.EventBootTimeNS &&
		a.CgroupV2ID == b.CgroupV2ID &&
		a.CgroupV2Correlated == b.CgroupV2Correlated &&
		a.Source == b.Source
}

func (s *taskOutcomeProofStore) ListHostKernelOOMVictimProofs() []HostProcessKernelOOMVictimProof {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	proofs := make([]HostProcessKernelOOMVictimProof, 0, len(s.kernelVictimProofs))
	for _, proof := range s.kernelVictimProofs {
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

func (c *controllerLocal) VisitHostKernelOOMVictimProofs(
	visit func(string, uint64, string, uint32, uint32, uint64, string, uint64, uint64, bool, string),
) {
	if c == nil || visit == nil {
		return
	}
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	for _, proof := range store.ListHostKernelOOMVictimProofs() {
		visit(
			proof.SandboxID,
			proof.Generation,
			proof.BootID,
			proof.HostPID,
			proof.VictimTID,
			proof.StartTimeTicks,
			proof.CGroupPath,
			proof.EventBootTimeNS,
			proof.CgroupV2ID,
			proof.CgroupV2Correlated,
			proof.Source,
		)
	}
}
