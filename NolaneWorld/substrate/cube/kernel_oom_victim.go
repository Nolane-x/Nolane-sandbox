package cube

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
)

const (
	hostKernelOOMVictimMetric = "cubesandbox_host_kernel_oom_victim_info"
	hostKernelOOMVictimSource = "kernel.oom.mark_victim.raw_tracepoint"
)

// HostKernelOOMVictimProof proves only that Linux emitted mark_oom_victim for
// the exact host process identity already bound to the realization. VictimTID
// may differ from HostPID because the marked task_struct may be a non-leader
// thread. This is not proof that OOM caused process exit or killed a guest task.
type HostKernelOOMVictimProof struct {
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

// HostKernelOOMVictimMarked reports positive-only Wave 20 evidence. Absence is
// unknown because collector and ring-buffer loss are possible; Wave 20 never
// exposes a known-negative victim result.
func (e TaskTerminationEvidence) HostKernelOOMVictimMarked() (marked bool, known bool) {
	if e.HostKernelOOMVictim == nil {
		return false, false
	}
	return true, true
}

func isHostKernelOOMVictimMetricToken(token string) bool {
	return token == hostKernelOOMVictimMetric || strings.HasPrefix(token, hostKernelOOMVictimMetric+"{")
}

func exactHostKernelOOMVictimFromSample(labels map[string]string, rawValue string) (HostKernelOOMVictimProof, error) {
	if len(labels) != 11 {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: host kernel OOM victim metric must contain exactly eleven labels", ErrTaskOutcomeUnavailable)
	}
	for _, key := range []string{
		"sandbox_id",
		"generation",
		"boot_id",
		"host_pid",
		"victim_tid",
		"starttime_ticks",
		"cgroup_path",
		"event_boot_time_ns",
		"cgroup_v2_id",
		"cgroup_v2_correlated",
		"source",
	} {
		if _, ok := labels[key]; !ok {
			return HostKernelOOMVictimProof{}, fmt.Errorf("%w: host kernel OOM victim metric missing %s", ErrTaskOutcomeUnavailable, key)
		}
	}

	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value != 1 {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: host kernel OOM victim metric value must be exactly one", ErrTaskOutcomeUnavailable)
	}

	sandboxID := labels["sandbox_id"]
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim sandbox identity", ErrTaskOutcomeUnavailable)
	}
	generation, err := parseCanonicalUint(labels["generation"], 64)
	if err != nil || generation == 0 {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim generation", ErrTaskOutcomeUnavailable)
	}
	bootID := labels["boot_id"]
	if !canonicalLowerUUID(bootID) {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim boot ID", ErrTaskOutcomeUnavailable)
	}
	hostPID, err := parseCanonicalUint(labels["host_pid"], 32)
	if err != nil || hostPID == 0 {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim host PID", ErrTaskOutcomeUnavailable)
	}
	victimTID, err := parseCanonicalUint(labels["victim_tid"], 32)
	if err != nil || victimTID == 0 {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim TID", ErrTaskOutcomeUnavailable)
	}
	startTimeTicks, err := parseCanonicalUint(labels["starttime_ticks"], 64)
	if err != nil || startTimeTicks == 0 {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim starttime", ErrTaskOutcomeUnavailable)
	}
	cgroupPath := labels["cgroup_path"]
	if cgroupPath == "" || strings.TrimSpace(cgroupPath) != cgroupPath || !path.IsAbs(cgroupPath) || path.Clean(cgroupPath) != cgroupPath || cgroupPath == "/" {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim cgroup path", ErrTaskOutcomeUnavailable)
	}
	eventBootTimeNS, err := parseCanonicalUint(labels["event_boot_time_ns"], 64)
	if err != nil || eventBootTimeNS == 0 {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim event time", ErrTaskOutcomeUnavailable)
	}

	var cgroupV2Correlated bool
	switch labels["cgroup_v2_correlated"] {
	case "true":
		cgroupV2Correlated = true
	case "false":
		cgroupV2Correlated = false
	default:
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: invalid host kernel OOM victim cgroup correlation flag", ErrTaskOutcomeUnavailable)
	}
	var cgroupV2ID uint64
	if cgroupV2Correlated {
		cgroupV2ID, err = parseCanonicalUint(labels["cgroup_v2_id"], 64)
		if err != nil || cgroupV2ID == 0 {
			return HostKernelOOMVictimProof{}, fmt.Errorf("%w: correlated host kernel OOM victim requires exact cgroup-v2 ID", ErrTaskOutcomeUnavailable)
		}
	} else if labels["cgroup_v2_id"] != "" {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: uncorrelated host kernel OOM victim must not carry cgroup-v2 ID", ErrTaskOutcomeUnavailable)
	}
	if labels["source"] != hostKernelOOMVictimSource {
		return HostKernelOOMVictimProof{}, fmt.Errorf("%w: unsupported host kernel OOM victim source %q", ErrTaskOutcomeUnavailable, labels["source"])
	}

	return HostKernelOOMVictimProof{
		SandboxID:          sandboxID,
		Generation:         generation,
		BootID:             bootID,
		HostPID:            uint32(hostPID),
		VictimTID:          uint32(victimTID),
		StartTimeTicks:     startTimeTicks,
		CGroupPath:         cgroupPath,
		EventBootTimeNS:    eventBootTimeNS,
		CgroupV2ID:         cgroupV2ID,
		CgroupV2Correlated: cgroupV2Correlated,
		Source:             hostKernelOOMVictimSource,
	}, nil
}

func correlateHostKernelOOMVictim(
	outcome TaskOutcomeProof,
	oom *RealizationOOMProof,
	identity HostSandboxProcessIdentityProof,
	victim HostKernelOOMVictimProof,
) error {
	if victim.SandboxID != outcome.SandboxID || victim.Generation != outcome.Generation {
		return fmt.Errorf("%w: host kernel OOM victim proof does not match exact task outcome", ErrTaskOutcomeUnavailable)
	}
	if victim.SandboxID != identity.SandboxID ||
		victim.Generation != identity.Generation ||
		victim.HostPID != identity.HostPID ||
		victim.StartTimeTicks != identity.StartTimeTicks ||
		victim.BootID != identity.BootID ||
		victim.CGroupPath != identity.CGroupPath {
		return fmt.Errorf("%w: host kernel OOM victim proof does not match exact host process identity", ErrTaskOutcomeUnavailable)
	}
	if oom != nil && victim.CGroupPath != oom.CGroupPath {
		return fmt.Errorf("%w: host kernel OOM victim cgroup does not match realization OOM proof", ErrTaskOutcomeUnavailable)
	}
	return nil
}
