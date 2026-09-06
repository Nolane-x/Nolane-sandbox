package resourcemetrics

import (
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
	},
	nil,
)

type guestKernelOOMVictimProofVisitor interface {
	VisitGuestKernelOOMVictimProofs(func(string, uint64, string, uint32, uint32, uint64, uint64, uint64, string, uint64, uint64, string))
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
	c.proofs.VisitGuestKernelOOMVictimProofs(func(
		sandboxID string,
		generation uint64,
		guestBootID string,
		tid uint32,
		tgid uint32,
		starttimeTicks uint64,
		eventBootNS uint64,
		cgroupV2ID uint64,
		victimClass string,
		realizationStartedBootNS uint64,
		outcomeObservedBootNS uint64,
		source string,
	) {
		if !transportableGuestKernelOOMVictimProof(
			sandboxID,
			generation,
			guestBootID,
			tid,
			tgid,
			starttimeTicks,
			eventBootNS,
			cgroupV2ID,
			victimClass,
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
			guestBootID,
			strconv.FormatUint(uint64(tid), 10),
			strconv.FormatUint(uint64(tgid), 10),
			strconv.FormatUint(starttimeTicks, 10),
			strconv.FormatUint(eventBootNS, 10),
			cgroupIDLabel,
			victimClass,
			strconv.FormatUint(realizationStartedBootNS, 10),
			strconv.FormatUint(outcomeObservedBootNS, 10),
			source,
		)
	})
}

func transportableGuestKernelOOMVictimProof(
	sandboxID string,
	generation uint64,
	guestBootID string,
	tid uint32,
	tgid uint32,
	starttimeTicks uint64,
	eventBootNS uint64,
	cgroupV2ID uint64,
	victimClass string,
	realizationStartedBootNS uint64,
	outcomeObservedBootNS uint64,
	source string,
) bool {
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID || generation == 0 {
		return false
	}
	parsedBootID, err := uuid.Parse(guestBootID)
	if err != nil || parsedBootID.String() != guestBootID {
		return false
	}
	if tid == 0 || tgid == 0 || starttimeTicks == 0 || eventBootNS == 0 || realizationStartedBootNS == 0 || outcomeObservedBootNS == 0 {
		return false
	}
	if eventBootNS < realizationStartedBootNS || eventBootNS > outcomeObservedBootNS {
		return false
	}
	if source != guestKernelOOMVictimSource {
		return false
	}
	switch victimClass {
	case "MAIN":
		return true
	case "MEMBER":
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
