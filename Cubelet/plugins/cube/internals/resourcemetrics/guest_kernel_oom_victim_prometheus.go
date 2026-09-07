package resourcemetrics

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

const guestKernelOOMVictimSource = "guest.kernel.oom.mark_victim.raw_tracepoint"

var guestKernelOOMVictimInfo = prometheus.NewDesc(
	"cubesandbox_guest_kernel_oom_victim_info",
	"Positive guest-kernel mark_oom_victim provenance bound to one exact sandbox task realization; this does not prove OOM-caused task exit.",
	[]string{
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
	},
	nil,
)

type guestKernelOOMVictimProofVisitor interface {
	VisitGuestKernelOOMVictimAuthorityProofs(func(string, uint64, string, string, uint32, uint32, uint64, uint32, uint64, string, uint64, uint64, uint64, uint64, string))
}

type guestKernelOOMVictimPrometheusCollector struct {
	proofs guestKernelOOMVictimProofVisitor
}

func (c *guestKernelOOMVictimPrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- guestKernelOOMVictimInfo
}

func (c *guestKernelOOMVictimPrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.proofs == nil {
		return
	}
	c.proofs.VisitGuestKernelOOMVictimAuthorityProofs(func(
		sandboxID string,
		generation uint64,
		realizationToken string,
		guestBootID string,
		victimTID uint32,
		victimTGID uint32,
		victimStarttimeTicks uint64,
		mainPID uint32,
		mainStarttimeTicks uint64,
		scope string,
		eventBootTimeNS uint64,
		cgroupV2ID uint64,
		realizationStartedBootNS uint64,
		outcomeObservedBootNS uint64,
		source string,
	) {
		if !transportableGuestKernelOOMVictimProof(
			sandboxID,
			generation,
			realizationToken,
			guestBootID,
			victimTID,
			victimTGID,
			victimStarttimeTicks,
			mainPID,
			mainStarttimeTicks,
			scope,
			eventBootTimeNS,
			cgroupV2ID,
			realizationStartedBootNS,
			outcomeObservedBootNS,
			source,
		) {
			return
		}

		cgroupIDLabel := ""
		if cgroupV2ID != 0 {
			cgroupIDLabel = strconv.FormatUint(cgroupV2ID, 10)
		}
		ch <- prometheus.MustNewConstMetric(
			guestKernelOOMVictimInfo,
			prometheus.GaugeValue,
			1,
			sandboxID,
			strconv.FormatUint(generation, 10),
			realizationToken,
			guestBootID,
			strconv.FormatUint(uint64(victimTID), 10),
			strconv.FormatUint(uint64(victimTGID), 10),
			strconv.FormatUint(victimStarttimeTicks, 10),
			strconv.FormatUint(uint64(mainPID), 10),
			strconv.FormatUint(mainStarttimeTicks, 10),
			scope,
			strconv.FormatUint(eventBootTimeNS, 10),
			cgroupIDLabel,
			strconv.FormatUint(realizationStartedBootNS, 10),
			strconv.FormatUint(outcomeObservedBootNS, 10),
			source,
		)
	})
}

func canonicalGuestKernelOOMVictimToken(value string) bool {
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

func transportableGuestKernelOOMVictimProof(
	sandboxID string,
	generation uint64,
	realizationToken string,
	guestBootID string,
	victimTID uint32,
	victimTGID uint32,
	victimStarttimeTicks uint64,
	mainPID uint32,
	mainStarttimeTicks uint64,
	scope string,
	eventBootTimeNS uint64,
	cgroupV2ID uint64,
	realizationStartedBootNS uint64,
	outcomeObservedBootNS uint64,
	source string,
) bool {
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID || generation == 0 || !canonicalGuestKernelOOMVictimToken(realizationToken) {
		return false
	}
	parsedBootID, err := uuid.Parse(guestBootID)
	if err != nil || parsedBootID.String() != guestBootID {
		return false
	}
	if victimTID == 0 || victimTGID == 0 || victimStarttimeTicks == 0 || mainPID == 0 || mainStarttimeTicks == 0 || eventBootTimeNS == 0 || realizationStartedBootNS == 0 || outcomeObservedBootNS == 0 {
		return false
	}
	if outcomeObservedBootNS < realizationStartedBootNS || eventBootTimeNS < realizationStartedBootNS || eventBootTimeNS > outcomeObservedBootNS {
		return false
	}
	if source != guestKernelOOMVictimSource {
		return false
	}
	switch scope {
	case "main":
		return victimTGID == mainPID && victimStarttimeTicks == mainStarttimeTicks
	case "member":
		return cgroupV2ID != 0
	default:
		return false
	}
}

func newPrometheusHandlerWithGuestKernelVictims(
	cache *SandboxResourceCache,
	victims guestKernelOOMVictimProofVisitor,
	now func() time.Time,
) http.Handler {
	return newPrometheusHandlerWithAllTaskEvidenceAndKernelVictimsAndGuestVictims(
		cache,
		nil,
		nil,
		nil,
		nil,
		victims,
		now,
	)
}
