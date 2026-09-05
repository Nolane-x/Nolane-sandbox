package resourcemetrics

import (
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

const hostKernelOOMVictimSource = "kernel.oom.mark_victim.raw_tracepoint"

var hostKernelOOMVictimInfo = prometheus.NewDesc(
	"cubesandbox_host_kernel_oom_victim_info",
	"Positive kernel mark_oom_victim provenance for the exact host CubeShim/VMM process bound to one task realization; this does not prove OOM-caused exit or guest-process causality.",
	[]string{
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
	},
	nil,
)

type hostKernelOOMVictimProofVisitor interface {
	VisitHostKernelOOMVictimProofs(func(string, uint64, string, uint32, uint32, uint64, string, uint64, uint64, bool, string))
}

type hostKernelOOMVictimPrometheusCollector struct {
	proofs hostKernelOOMVictimProofVisitor
}

func (c *hostKernelOOMVictimPrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- hostKernelOOMVictimInfo
}

func (c *hostKernelOOMVictimPrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.proofs == nil {
		return
	}
	c.proofs.VisitHostKernelOOMVictimProofs(func(
		sandboxID string,
		generation uint64,
		bootID string,
		hostPID uint32,
		victimTID uint32,
		startTimeTicks uint64,
		cgroupPath string,
		eventBootTimeNS uint64,
		cgroupV2ID uint64,
		cgroupV2Correlated bool,
		source string,
	) {
		if !transportableHostKernelOOMVictimProof(
			sandboxID,
			generation,
			bootID,
			hostPID,
			victimTID,
			startTimeTicks,
			cgroupPath,
			eventBootTimeNS,
			cgroupV2ID,
			cgroupV2Correlated,
			source,
		) {
			return
		}
		cgroupIDLabel := ""
		if cgroupV2Correlated {
			cgroupIDLabel = strconv.FormatUint(cgroupV2ID, 10)
		}
		ch <- prometheus.MustNewConstMetric(
			hostKernelOOMVictimInfo,
			prometheus.GaugeValue,
			1,
			sandboxID,
			strconv.FormatUint(generation, 10),
			bootID,
			strconv.FormatUint(uint64(hostPID), 10),
			strconv.FormatUint(uint64(victimTID), 10),
			strconv.FormatUint(startTimeTicks, 10),
			cgroupPath,
			strconv.FormatUint(eventBootTimeNS, 10),
			cgroupIDLabel,
			strconv.FormatBool(cgroupV2Correlated),
			source,
		)
	})
}

func transportableHostKernelOOMVictimProof(
	sandboxID string,
	generation uint64,
	bootID string,
	hostPID uint32,
	victimTID uint32,
	startTimeTicks uint64,
	cgroupPath string,
	eventBootTimeNS uint64,
	cgroupV2ID uint64,
	cgroupV2Correlated bool,
	source string,
) bool {
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID || generation == 0 || hostPID == 0 || victimTID == 0 || startTimeTicks == 0 || eventBootTimeNS == 0 {
		return false
	}
	parsedBootID, err := uuid.Parse(bootID)
	if err != nil || parsedBootID.String() != bootID {
		return false
	}
	if cgroupPath == "" || strings.TrimSpace(cgroupPath) != cgroupPath || !path.IsAbs(cgroupPath) || path.Clean(cgroupPath) != cgroupPath || cgroupPath == "/" {
		return false
	}
	if source != hostKernelOOMVictimSource {
		return false
	}
	if cgroupV2Correlated {
		return cgroupV2ID != 0
	}
	return cgroupV2ID == 0
}

func newPrometheusHandlerWithKernelVictimEvidence(
	cache *SandboxResourceCache,
	outcomes taskOutcomeProofVisitor,
	oom realizationOOMProofVisitor,
	hostIdentity hostProcessIdentityProofVisitor,
	victims hostKernelOOMVictimProofVisitor,
	now func() time.Time,
) http.Handler {
	return newPrometheusHandlerWithAllTaskEvidenceAndKernelVictims(
		cache,
		outcomes,
		oom,
		hostIdentity,
		victims,
		now,
	)
}
