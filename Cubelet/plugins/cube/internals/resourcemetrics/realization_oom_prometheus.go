package resourcemetrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const realizationOOMSignal = "kernel.cgroup.memory.oom_kill"

var realizationOOMInfo = prometheus.NewDesc(
	"cubesandbox_task_realization_oom_info",
	"Exact kernel cgroup OOM-kill counter evidence bound to one task realization; this does not identify the victim process.",
	[]string{
		"sandbox_id",
		"generation",
		"cgroup_path",
		"signal",
		"baseline_oom_kills",
		"final_oom_kills",
		"oom_kills",
		"baseline_at",
		"observed_at",
		"exited_at",
		"outcome_source",
	},
	nil,
)

type realizationOOMProofVisitor interface {
	VisitRealizationOOMProofs(func(string, uint64, string, uint64, uint64, uint64, time.Time, time.Time, time.Time, string))
}

type realizationOOMPrometheusCollector struct {
	proofs realizationOOMProofVisitor
}

func newServiceWithTaskEvidence(cache *SandboxResourceCache, outcomes taskOutcomeProofVisitor, oom realizationOOMProofVisitor) *Service {
	return newServiceWithAllTaskEvidence(cache, outcomes, oom, nil)
}

func newServiceWithAllTaskEvidence(
	cache *SandboxResourceCache,
	outcomes taskOutcomeProofVisitor,
	oom realizationOOMProofVisitor,
	hostIdentity hostProcessIdentityProofVisitor,
) *Service {
	return &Service{
		SandboxResourceCache: cache,
		handler:              newPrometheusHandlerWithAllTaskEvidence(cache, outcomes, oom, hostIdentity, time.Now),
	}
}

func newPrometheusHandlerWithTaskEvidence(cache *SandboxResourceCache, outcomes taskOutcomeProofVisitor, oom realizationOOMProofVisitor, now func() time.Time) http.Handler {
	return newPrometheusHandlerWithAllTaskEvidence(cache, outcomes, oom, nil, now)
}

func newPrometheusHandlerWithAllTaskEvidence(
	cache *SandboxResourceCache,
	outcomes taskOutcomeProofVisitor,
	oom realizationOOMProofVisitor,
	hostIdentity hostProcessIdentityProofVisitor,
	now func() time.Time,
) http.Handler {
	registry := prometheus.NewRegistry()
	registry.MustRegister(&prometheusCollector{cache: cache, now: now})
	if outcomes != nil {
		registry.MustRegister(&taskOutcomePrometheusCollector{outcomes: outcomes})
	}
	if oom != nil {
		registry.MustRegister(&realizationOOMPrometheusCollector{proofs: oom})
	}
	if hostIdentity != nil {
		registry.MustRegister(&hostProcessIdentityPrometheusCollector{proofs: hostIdentity})
	}
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{MaxRequestsInFlight: maxConcurrentPrometheusScrapes})
}

func (c *realizationOOMPrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- realizationOOMInfo
}

func (c *realizationOOMPrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.proofs == nil {
		return
	}
	c.proofs.VisitRealizationOOMProofs(func(
		sandboxID string,
		generation uint64,
		cgroupPath string,
		baselineOOMKills uint64,
		finalOOMKills uint64,
		oomKills uint64,
		baselineAt time.Time,
		observedAt time.Time,
		exitedAt time.Time,
		outcomeSource string,
	) {
		if !transportableRealizationOOMProof(
			sandboxID,
			generation,
			cgroupPath,
			baselineOOMKills,
			finalOOMKills,
			oomKills,
			baselineAt,
			observedAt,
			exitedAt,
			outcomeSource,
		) {
			return
		}
		ch <- prometheus.MustNewConstMetric(
			realizationOOMInfo,
			prometheus.GaugeValue,
			1,
			sandboxID,
			strconv.FormatUint(generation, 10),
			cgroupPath,
			realizationOOMSignal,
			strconv.FormatUint(baselineOOMKills, 10),
			strconv.FormatUint(finalOOMKills, 10),
			strconv.FormatUint(oomKills, 10),
			baselineAt.UTC().Format(time.RFC3339Nano),
			observedAt.UTC().Format(time.RFC3339Nano),
			exitedAt.UTC().Format(time.RFC3339Nano),
			outcomeSource,
		)
	})
}

func transportableRealizationOOMProof(
	sandboxID string,
	generation uint64,
	cgroupPath string,
	baselineOOMKills uint64,
	finalOOMKills uint64,
	oomKills uint64,
	baselineAt time.Time,
	observedAt time.Time,
	exitedAt time.Time,
	outcomeSource string,
) bool {
	if strings.TrimSpace(sandboxID) == "" || generation == 0 {
		return false
	}
	if strings.TrimSpace(cgroupPath) == "" || strings.TrimSpace(cgroupPath) != cgroupPath {
		return false
	}
	if baselineAt.IsZero() || observedAt.IsZero() || exitedAt.IsZero() {
		return false
	}
	if exitedAt.Before(baselineAt) || observedAt.Before(exitedAt) {
		return false
	}
	if finalOOMKills < baselineOOMKills || oomKills != finalOOMKills-baselineOOMKills {
		return false
	}
	return outcomeSource == taskOutcomeSourceWait || outcomeSource == taskOutcomeSourceState
}
