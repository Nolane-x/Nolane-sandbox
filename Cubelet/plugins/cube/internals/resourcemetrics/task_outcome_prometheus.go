package resourcemetrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	cubesandbox "github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/sandbox"
)

var taskOutcomeInfo = prometheus.NewDesc(
	"cubesandbox_task_outcome_info",
	"Exact containerd task-outcome proof accepted by the Cube sandbox controller.",
	[]string{"sandbox_id", "generation", "source", "exit_code", "exited_at"},
	nil,
)

type taskOutcomePrometheusCollector struct {
	outcomes cubesandbox.TaskOutcomeProofLister
}

func NewServiceWithTaskOutcomes(cache *SandboxResourceCache, outcomes cubesandbox.TaskOutcomeProofLister) *Service {
	return &Service{
		SandboxResourceCache: cache,
		handler:              NewPrometheusHandlerWithTaskOutcomes(cache, outcomes),
	}
}

func NewPrometheusHandlerWithTaskOutcomes(cache *SandboxResourceCache, outcomes cubesandbox.TaskOutcomeProofLister) http.Handler {
	return newPrometheusHandlerWithTaskOutcomes(cache, outcomes, time.Now)
}

func newPrometheusHandlerWithTaskOutcomes(cache *SandboxResourceCache, outcomes cubesandbox.TaskOutcomeProofLister, now func() time.Time) http.Handler {
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
	for _, proof := range c.outcomes.ListTaskOutcomeProofs() {
		if !transportableTaskOutcomeProof(proof) {
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			taskOutcomeInfo,
			prometheus.GaugeValue,
			1,
			proof.SandboxID,
			strconv.FormatUint(proof.Generation, 10),
			string(proof.Source),
			strconv.FormatUint(uint64(proof.ExitCode), 10),
			proof.ExitedAt.UTC().Format(time.RFC3339Nano),
		)
	}
}

func transportableTaskOutcomeProof(proof cubesandbox.TaskOutcomeProof) bool {
	if strings.TrimSpace(proof.SandboxID) == "" || proof.Generation == 0 || proof.ExitedAt.IsZero() {
		return false
	}
	return proof.Source == cubesandbox.TaskOutcomeProofSourceWait || proof.Source == cubesandbox.TaskOutcomeProofSourceState
}
