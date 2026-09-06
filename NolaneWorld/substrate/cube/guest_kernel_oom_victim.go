package cube

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	guestKernelOOMVictimMetric = "cubesandbox_guest_kernel_oom_victim_info"
	guestKernelOOMVictimSource = "guest.kernel.oom.mark_victim.raw_tracepoint"
)

type GuestKernelOOMVictimClass string

const (
	GuestKernelOOMVictimMain   GuestKernelOOMVictimClass = "MAIN"
	GuestKernelOOMVictimMember GuestKernelOOMVictimClass = "MEMBER"
)

// GuestKernelOOMVictimProof proves only that the guest Linux kernel emitted
// mark_oom_victim for one exact guest process lifetime belonging to the exact
// realization. MAIN is stronger process-identity correlation than MEMBER, but
// neither class proves OOM caused the task's terminal outcome.
type GuestKernelOOMVictimProof struct {
	SandboxID                string
	Generation               uint64
	GuestBootID              string
	TID                      uint32
	TGID                     uint32
	StartTimeTicks           uint64
	EventBootNS              uint64
	CgroupV2ID               uint64
	VictimClass              GuestKernelOOMVictimClass
	RealizationStartedBootNS uint64
	OutcomeObservedBootNS    uint64
	Source                   string
}

func isGuestKernelOOMVictimMetricToken(token string) bool {
	return token == guestKernelOOMVictimMetric || strings.HasPrefix(token, guestKernelOOMVictimMetric+"{")
}

func exactGuestKernelOOMVictimFromSample(labels map[string]string, rawValue string) (GuestKernelOOMVictimProof, error) {
	if len(labels) != 12 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: guest kernel OOM victim metric must contain exactly twelve labels", ErrTaskOutcomeUnavailable)
	}
	for _, key := range []string{
		"sandbox_id",
		"generation",
		"guest_boot_id",
		"tid",
		"tgid",
		"starttime_ticks",
		"event_boot_ns",
		"cgroup_v2_id",
		"victim_class",
		"realization_started_boot_ns",
		"outcome_observed_boot_ns",
		"source",
	} {
		if _, ok := labels[key]; !ok {
			return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: guest kernel OOM victim metric missing %s", ErrTaskOutcomeUnavailable, key)
		}
	}

	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value != 1 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: guest kernel OOM victim metric value must be exactly one", ErrTaskOutcomeUnavailable)
	}

	sandboxID := labels["sandbox_id"]
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim sandbox identity", ErrTaskOutcomeUnavailable)
	}
	generation, err := parseCanonicalUint(labels["generation"], 64)
	if err != nil || generation == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim generation", ErrTaskOutcomeUnavailable)
	}
	guestBootID := labels["guest_boot_id"]
	if !canonicalLowerUUID(guestBootID) {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim boot ID", ErrTaskOutcomeUnavailable)
	}
	tid, err := parseCanonicalUint(labels["tid"], 32)
	if err != nil || tid == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim TID", ErrTaskOutcomeUnavailable)
	}
	tgid, err := parseCanonicalUint(labels["tgid"], 32)
	if err != nil || tgid == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim TGID", ErrTaskOutcomeUnavailable)
	}
	startTimeTicks, err := parseCanonicalUint(labels["starttime_ticks"], 64)
	if err != nil || startTimeTicks == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim starttime", ErrTaskOutcomeUnavailable)
	}
	eventBootNS, err := parseCanonicalUint(labels["event_boot_ns"], 64)
	if err != nil || eventBootNS == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim event time", ErrTaskOutcomeUnavailable)
	}
	startedBootNS, err := parseCanonicalUint(labels["realization_started_boot_ns"], 64)
	if err != nil || startedBootNS == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest realization start time", ErrTaskOutcomeUnavailable)
	}
	outcomeBootNS, err := parseCanonicalUint(labels["outcome_observed_boot_ns"], 64)
	if err != nil || outcomeBootNS == 0 || outcomeBootNS < startedBootNS {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest realization outcome observation time", ErrTaskOutcomeUnavailable)
	}
	if eventBootNS < startedBootNS || eventBootNS > outcomeBootNS {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: guest kernel OOM victim event is outside exact realization window", ErrTaskOutcomeUnavailable)
	}

	var cgroupV2ID uint64
	if labels["cgroup_v2_id"] != "" {
		cgroupV2ID, err = parseCanonicalUint(labels["cgroup_v2_id"], 64)
		if err != nil || cgroupV2ID == 0 {
			return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim cgroup-v2 ID", ErrTaskOutcomeUnavailable)
		}
	}

	victimClass := GuestKernelOOMVictimClass(labels["victim_class"])
	switch victimClass {
	case GuestKernelOOMVictimMain:
		// MAIN identity comes from exact TGID + lifetime correlation. Exact
		// cgroup identity is optional additional provenance for this class.
	case GuestKernelOOMVictimMember:
		if cgroupV2ID == 0 {
			return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: MEMBER guest kernel OOM victim requires exact cgroup-v2 identity", ErrTaskOutcomeUnavailable)
		}
	default:
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim class %q", ErrTaskOutcomeUnavailable, labels["victim_class"])
	}
	if labels["source"] != guestKernelOOMVictimSource {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: unsupported guest kernel OOM victim source %q", ErrTaskOutcomeUnavailable, labels["source"])
	}

	return GuestKernelOOMVictimProof{
		SandboxID:                sandboxID,
		Generation:               generation,
		GuestBootID:              guestBootID,
		TID:                      uint32(tid),
		TGID:                     uint32(tgid),
		StartTimeTicks:           startTimeTicks,
		EventBootNS:              eventBootNS,
		CgroupV2ID:               cgroupV2ID,
		VictimClass:              victimClass,
		RealizationStartedBootNS: startedBootNS,
		OutcomeObservedBootNS:    outcomeBootNS,
		Source:                   guestKernelOOMVictimSource,
	}, nil
}

func correlateGuestKernelOOMVictim(outcome TaskOutcomeProof, victim GuestKernelOOMVictimProof) error {
	if victim.SandboxID != outcome.SandboxID || victim.Generation != outcome.Generation {
		return fmt.Errorf("%w: guest kernel OOM victim proof does not match exact task outcome", ErrTaskOutcomeUnavailable)
	}
	return nil
}
