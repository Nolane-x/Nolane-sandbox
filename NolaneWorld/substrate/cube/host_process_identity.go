package cube

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	hostSandboxProcessIdentityMetric           = "cubesandbox_host_process_identity_info"
	HostSandboxProcessRuntimeRoleCubeShimVMM  = "cube-shim-vmm"
	HostSandboxProcessIdentitySourceCubeBoxAddProc = "cubebox.cgroup.add_proc"
)

// HostSandboxProcessIdentityProof is provenance for the exact host CubeShim/VMM
// process identity bound to one controller-local task realization. HostPID and
// CGroupPath are data only; NolaneWorld never treats them as executable host
// authority. This proof does not identify an OOM victim or guest process.
type HostSandboxProcessIdentityProof struct {
	SandboxID      string
	Generation     uint64
	HostPID        uint32
	StartTimeTicks uint64
	BootID         string
	CGroupPath     string
	RuntimeRole    string
	Source         string
	PlacedAt       time.Time
	BoundAt        time.Time
}

func isHostSandboxProcessIdentityMetricToken(token string) bool {
	return token == hostSandboxProcessIdentityMetric || strings.HasPrefix(token, hostSandboxProcessIdentityMetric+"{")
}

func exactHostSandboxProcessIdentityFromSample(labels map[string]string, rawValue string) (HostSandboxProcessIdentityProof, error) {
	if len(labels) != 10 {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: host process identity metric must contain exactly ten labels", ErrTaskOutcomeUnavailable)
	}
	for _, key := range []string{
		"sandbox_id",
		"generation",
		"host_pid",
		"starttime_ticks",
		"boot_id",
		"cgroup_path",
		"runtime_role",
		"source",
		"placed_at",
		"bound_at",
	} {
		if _, ok := labels[key]; !ok {
			return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: host process identity metric missing %s", ErrTaskOutcomeUnavailable, key)
		}
	}

	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value != 1 {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: host process identity metric value must be exactly one", ErrTaskOutcomeUnavailable)
	}

	sandboxID := labels["sandbox_id"]
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process identity sandbox identity", ErrTaskOutcomeUnavailable)
	}
	generation, err := parseCanonicalUint(labels["generation"], 64)
	if err != nil || generation == 0 {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process identity generation", ErrTaskOutcomeUnavailable)
	}
	hostPID, err := parseCanonicalUint(labels["host_pid"], 32)
	if err != nil || hostPID == 0 {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process identity PID", ErrTaskOutcomeUnavailable)
	}
	startTimeTicks, err := parseCanonicalUint(labels["starttime_ticks"], 64)
	if err != nil || startTimeTicks == 0 {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process starttime", ErrTaskOutcomeUnavailable)
	}
	bootID := labels["boot_id"]
	if !canonicalLowerUUID(bootID) {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host boot ID", ErrTaskOutcomeUnavailable)
	}
	cgroupPath := labels["cgroup_path"]
	if cgroupPath == "" || strings.TrimSpace(cgroupPath) != cgroupPath || !path.IsAbs(cgroupPath) || path.Clean(cgroupPath) != cgroupPath || cgroupPath == "/" {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process cgroup path", ErrTaskOutcomeUnavailable)
	}
	if labels["runtime_role"] != HostSandboxProcessRuntimeRoleCubeShimVMM {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: unsupported host process runtime role %q", ErrTaskOutcomeUnavailable, labels["runtime_role"])
	}
	if labels["source"] != HostSandboxProcessIdentitySourceCubeBoxAddProc {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: unsupported host process identity source %q", ErrTaskOutcomeUnavailable, labels["source"])
	}
	placedAt, err := parseCanonicalUTCTimestamp(labels["placed_at"])
	if err != nil {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process placement timestamp", ErrTaskOutcomeUnavailable)
	}
	boundAt, err := parseCanonicalUTCTimestamp(labels["bound_at"])
	if err != nil {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: invalid host process binding timestamp", ErrTaskOutcomeUnavailable)
	}
	if boundAt.Before(placedAt) {
		return HostSandboxProcessIdentityProof{}, fmt.Errorf("%w: host process binding predates placement", ErrTaskOutcomeUnavailable)
	}

	return HostSandboxProcessIdentityProof{
		SandboxID:      sandboxID,
		Generation:     generation,
		HostPID:        uint32(hostPID),
		StartTimeTicks: startTimeTicks,
		BootID:         bootID,
		CGroupPath:     cgroupPath,
		RuntimeRole:    HostSandboxProcessRuntimeRoleCubeShimVMM,
		Source:         HostSandboxProcessIdentitySourceCubeBoxAddProc,
		PlacedAt:       placedAt,
		BoundAt:        boundAt,
	}, nil
}

func canonicalLowerUUID(raw string) bool {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		c := raw[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func correlateHostSandboxProcessIdentity(outcome TaskOutcomeProof, oom *RealizationOOMProof, identity HostSandboxProcessIdentityProof) error {
	if identity.SandboxID != outcome.SandboxID || identity.Generation != outcome.Generation {
		return fmt.Errorf("%w: host process identity proof does not match exact task outcome", ErrTaskOutcomeUnavailable)
	}
	if oom != nil && identity.CGroupPath != oom.CGroupPath {
		return fmt.Errorf("%w: host process identity cgroup does not match realization OOM proof", ErrTaskOutcomeUnavailable)
	}
	return nil
}
