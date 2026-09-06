package cube

import (
	"encoding/hex"
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
	RealizationTokenHex      string
	GuestBootID              string
	TID                      uint32
	TGID                     uint32
	StartTimeTicks           uint64
	MainPID                  uint32
	MainStartTimeTicks       uint64
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

func canonicalGuestKernelOOMVictimTokenHex(value string) bool {
	if len(value) != 64 {
		return false
	}
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != value {
		return false
	}
	for _, b := range raw {
		if b != 0 {
			return true
		}
	}
	return false
}

func exactGuestKernelOOMVictimFromSample(labels map[string]string, rawValue string) (GuestKernelOOMVictimProof, error) {
	if len(labels) != 15 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: guest kernel OOM victim metric must contain exactly fifteen labels", ErrTaskOutcomeUnavailable)
	}
	for _, key := range []string{
		"sandbox_id",
		"generation",
		"realization_token",
		"guest_boot_id",
		"victim_tid",
		"victim_tgid",
		"victim_starttime_ticks",
		"main_pid",
		"main_starttime_ticks",
		"scope",
		"event_boot_time_ns",
		"cgroup_v2_id",
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
	realizationToken := labels["realization_token"]
	if !canonicalGuestKernelOOMVictimTokenHex(realizationToken) {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim realization token", ErrTaskOutcomeUnavailable)
	}
	guestBootID := labels["guest_boot_id"]
	if !canonicalLowerUUID(guestBootID) {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim boot ID", ErrTaskOutcomeUnavailable)
	}
	victimTID, err := parseCanonicalUint(labels["victim_tid"], 32)
	if err != nil || victimTID == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim TID", ErrTaskOutcomeUnavailable)
	}
	victimTGID, err := parseCanonicalUint(labels["victim_tgid"], 32)
	if err != nil || victimTGID == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim TGID", ErrTaskOutcomeUnavailable)
	}
	victimStartTimeTicks, err := parseCanonicalUint(labels["victim_starttime_ticks"], 64)
	if err != nil || victimStartTimeTicks == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim starttime", ErrTaskOutcomeUnavailable)
	}
	mainPID, err := parseCanonicalUint(labels["main_pid"], 32)
	if err != nil || mainPID == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim main PID", ErrTaskOutcomeUnavailable)
	}
	mainStartTimeTicks, err := parseCanonicalUint(labels["main_starttime_ticks"], 64)
	if err != nil || mainStartTimeTicks == 0 {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim main starttime", ErrTaskOutcomeUnavailable)
	}
	eventBootNS, err := parseCanonicalUint(labels["event_boot_time_ns"], 64)
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

	var victimClass GuestKernelOOMVictimClass
	switch labels["scope"] {
	case "main":
		victimClass = GuestKernelOOMVictimMain
		if uint32(victimTGID) != uint32(mainPID) || victimStartTimeTicks != mainStartTimeTicks {
			return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: MAIN guest kernel OOM victim does not match exact main lifetime", ErrTaskOutcomeUnavailable)
		}
	case "member":
		victimClass = GuestKernelOOMVictimMember
		if cgroupV2ID == 0 {
			return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: MEMBER guest kernel OOM victim requires exact cgroup-v2 identity", ErrTaskOutcomeUnavailable)
		}
	default:
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: invalid guest kernel OOM victim scope %q", ErrTaskOutcomeUnavailable, labels["scope"])
	}
	if labels["source"] != guestKernelOOMVictimSource {
		return GuestKernelOOMVictimProof{}, fmt.Errorf("%w: unsupported guest kernel OOM victim source %q", ErrTaskOutcomeUnavailable, labels["source"])
	}

	return GuestKernelOOMVictimProof{
		SandboxID:                sandboxID,
		Generation:               generation,
		RealizationTokenHex:      realizationToken,
		GuestBootID:              guestBootID,
		TID:                      uint32(victimTID),
		TGID:                     uint32(victimTGID),
		StartTimeTicks:           victimStartTimeTicks,
		MainPID:                  uint32(mainPID),
		MainStartTimeTicks:       mainStartTimeTicks,
		EventBootNS:              eventBootNS,
		CgroupV2ID:               cgroupV2ID,
		VictimClass:              victimClass,
		RealizationStartedBootNS: startedBootNS,
		OutcomeObservedBootNS:    outcomeBootNS,
		Source:                   guestKernelOOMVictimSource,
	}, nil
}

func sameGuestKernelOOMVictimAuthority(a, b GuestKernelOOMVictimProof) bool {
	return a.SandboxID == b.SandboxID &&
		a.Generation == b.Generation &&
		a.RealizationTokenHex == b.RealizationTokenHex &&
		a.GuestBootID == b.GuestBootID &&
		a.MainPID == b.MainPID &&
		a.MainStartTimeTicks == b.MainStartTimeTicks &&
		a.RealizationStartedBootNS == b.RealizationStartedBootNS &&
		a.OutcomeObservedBootNS == b.OutcomeObservedBootNS &&
		a.Source == b.Source
}

func sameGuestKernelOOMVictimProof(a, b GuestKernelOOMVictimProof) bool {
	return a == b
}

func correlateGuestKernelOOMVictim(outcome TaskOutcomeProof, victim GuestKernelOOMVictimProof) error {
	if victim.SandboxID != outcome.SandboxID || victim.Generation != outcome.Generation {
		return fmt.Errorf("%w: guest kernel OOM victim proof does not match exact task outcome", ErrTaskOutcomeUnavailable)
	}
	return nil
}
