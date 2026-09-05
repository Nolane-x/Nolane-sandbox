package resourcemetrics

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	hostProcessRuntimeRoleCubeShimVMM = "cube-shim-vmm"
	hostProcessSourceCubeBoxAddProc   = "cubebox.cgroup.add_proc"
)

var hostProcessIdentityInfo = prometheus.NewDesc(
	"cubesandbox_host_process_identity_info",
	"Exact PID-reuse-resistant host CubeShim/VMM process identity bound to one task realization; this does not identify an OOM victim or guest process.",
	[]string{
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
	},
	nil,
)

type hostProcessIdentityProofVisitor interface {
	VisitHostProcessIdentityProofs(func(string, uint64, uint32, uint64, string, string, string, string, time.Time, time.Time))
}

type hostProcessIdentityPrometheusCollector struct {
	proofs hostProcessIdentityProofVisitor
}

func (c *hostProcessIdentityPrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- hostProcessIdentityInfo
}

func (c *hostProcessIdentityPrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.proofs == nil {
		return
	}
	c.proofs.VisitHostProcessIdentityProofs(func(
		sandboxID string,
		generation uint64,
		hostPID uint32,
		startTimeTicks uint64,
		bootID string,
		cgroupPath string,
		runtimeRole string,
		source string,
		placedAt time.Time,
		boundAt time.Time,
	) {
		if !transportableHostProcessIdentityProof(
			sandboxID,
			generation,
			hostPID,
			startTimeTicks,
			bootID,
			cgroupPath,
			runtimeRole,
			source,
			placedAt,
			boundAt,
		) {
			return
		}
		ch <- prometheus.MustNewConstMetric(
			hostProcessIdentityInfo,
			prometheus.GaugeValue,
			1,
			sandboxID,
			strconv.FormatUint(generation, 10),
			strconv.FormatUint(uint64(hostPID), 10),
			strconv.FormatUint(startTimeTicks, 10),
			bootID,
			cgroupPath,
			runtimeRole,
			source,
			placedAt.UTC().Format(time.RFC3339Nano),
			boundAt.UTC().Format(time.RFC3339Nano),
		)
	})
}

func transportableHostProcessIdentityProof(
	sandboxID string,
	generation uint64,
	hostPID uint32,
	startTimeTicks uint64,
	bootID string,
	cgroupPath string,
	runtimeRole string,
	source string,
	placedAt time.Time,
	boundAt time.Time,
) bool {
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID || generation == 0 || hostPID == 0 || startTimeTicks == 0 {
		return false
	}
	parsedBootID, err := uuid.Parse(bootID)
	if err != nil || parsedBootID.String() != bootID {
		return false
	}
	if cgroupPath == "" || strings.TrimSpace(cgroupPath) != cgroupPath || !path.IsAbs(cgroupPath) || path.Clean(cgroupPath) != cgroupPath || cgroupPath == "/" {
		return false
	}
	if runtimeRole != hostProcessRuntimeRoleCubeShimVMM || source != hostProcessSourceCubeBoxAddProc {
		return false
	}
	if placedAt.IsZero() || boundAt.IsZero() || boundAt.Before(placedAt) {
		return false
	}
	return true
}
