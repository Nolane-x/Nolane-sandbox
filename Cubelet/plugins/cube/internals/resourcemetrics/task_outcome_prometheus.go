package resourcemetrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	taskOutcomeSourceWait  = "containerd.task.wait"
	taskOutcomeSourceState = "containerd.task.state"
)

var taskOutcomeInfo = prometheus.NewDesc(
	"cubesandbox_task_outcome_info",
	"Exact containerd task-outcome proof accepted by the Cube sandbox controller.",
	[]string{"sandbox_id", "generation", "source", "exit_code", "exited_at"},
	nil,
)

type taskOutcomeProofVisitor interface {
	VisitTaskOutcomeProofs(func(sandboxID string, generation uint64, exitCode uint32, exitedAt time.Time, source string))
}

type taskOutcomePrometheusCollector struct {
	outcomes taskOutcomeProofVisitor
}

func newServiceWithTaskOutcomes(cache *SandboxResourceCache, outcomes taskOutcomeProofVisitor) *Service {
	return &Service{
		SandboxResourceCache: cache,
		handler:              newPrometheusHandlerWithTaskOutcomes(cache, outcomes, time.Now),
	}
}

func newPrometheusHandlerWithTaskOutcomes(cache *SandboxResourceCache, outcomes taskOutcomeProofVisitor, now func() time.Time) http.Handler {
	registry := prometheus.NewRegistry()
	registry.MustRegister(&prometheusCollector{cache: cache, now: now})
	if outcomes != nil {
		registry.MustRegister(&taskOutcomePrometheusCollector{outcomes: outcomes})
	}
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{MaxRequestsInFlight: maxConcurrentPrometheusScrapes})
}

func (c *taskOutcomePrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- taskOutcomeInfo
}

func (c *taskOutcomePrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.outcomes == nil {
		return
	}
	c.outcomes.VisitTaskOutcomeProofs(func(sandboxID string, generation uint64, exitCode uint32, exitedAt time.Time, source string) {
		if !transportableTaskOutcomeProof(sandboxID, generation, exitedAt, source) {
			return
		}
		ch <- prometheus.MustNewConstMetric(
			taskOutcomeInfo,
			prometheus.GaugeValue,
			1,
			sandboxID,
			strconv.FormatUint(generation, 10),
			source,
			strconv.FormatUint(uint64(exitCode), 10),
			exitedAt.UTC().Format(time.RFC3339Nano),
		)
	})
}

func transportableTaskOutcomeProof(sandboxID string, generation uint64, exitedAt time.Time, source string) bool {
	if strings.TrimSpace(sandboxID) == "" || generation == 0 || exitedAt.IsZero() {
		return false
	}
	return source == taskOutcomeSourceWait || source == taskOutcomeSourceState
}
