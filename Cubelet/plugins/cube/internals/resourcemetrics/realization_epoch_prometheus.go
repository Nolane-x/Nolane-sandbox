package resourcemetrics

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var realizationEpochInfo = prometheus.NewDesc(
	"cubesandbox_realization_epoch_info",
	"Current Cubelet-local realization epoch identity for one exact sandbox generation.",
	[]string{"sandbox_id", "generation", "token"},
	nil,
)

type realizationEpochVisitor interface {
	VisitRealizationEpochs(func(string, uint64, [32]byte))
}

type realizationEpochPrometheusCollector struct {
	epochs realizationEpochVisitor
}

func (c *realizationEpochPrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- realizationEpochInfo
}

func (c *realizationEpochPrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.epochs == nil {
		return
	}
	c.epochs.VisitRealizationEpochs(func(sandboxID string, generation uint64, token [32]byte) {
		if !transportableRealizationEpoch(sandboxID, generation, token) {
			return
		}
		ch <- prometheus.MustNewConstMetric(
			realizationEpochInfo,
			prometheus.GaugeValue,
			1,
			sandboxID,
			strconv.FormatUint(generation, 10),
			hex.EncodeToString(token[:]),
		)
	})
}

func transportableRealizationEpoch(sandboxID string, generation uint64, token [32]byte) bool {
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID || generation == 0 {
		return false
	}
	return token != ([32]byte{})
}

// newProductionTaskEvidenceServiceWithRealizationEpochs is the Wave24
// production composition root. It keeps all pre-Wave24 evidence on the same
// trusted management registry and adds exactly one collector for current epoch
// authority instead of creating a parallel endpoint or trust boundary.
func newProductionTaskEvidenceServiceWithRealizationEpochs(
	cache *SandboxResourceCache,
	outcomes taskOutcomeProofVisitor,
	oom realizationOOMProofVisitor,
	hostIdentity hostProcessIdentityProofVisitor,
	hostVictims hostKernelOOMVictimProofVisitor,
	guestVictims guestKernelOOMVictimProofVisitor,
	epochs realizationEpochVisitor,
) *Service {
	return &Service{
		SandboxResourceCache: cache,
		handler: newPrometheusHandlerWithAllTaskEvidenceAndRealizationEpochs(
			cache,
			outcomes,
			oom,
			hostIdentity,
			hostVictims,
			guestVictims,
			epochs,
			time.Now,
		),
	}
}

func newPrometheusHandlerWithAllTaskEvidenceAndRealizationEpochs(
	cache *SandboxResourceCache,
	outcomes taskOutcomeProofVisitor,
	oom realizationOOMProofVisitor,
	hostIdentity hostProcessIdentityProofVisitor,
	hostVictims hostKernelOOMVictimProofVisitor,
	guestVictims guestKernelOOMVictimProofVisitor,
	epochs realizationEpochVisitor,
	now func() time.Time,
) http.Handler {
	if now == nil {
		now = time.Now
	}
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
	if hostVictims != nil {
		registry.MustRegister(&hostKernelOOMVictimPrometheusCollector{proofs: hostVictims})
	}
	if guestVictims != nil {
		registry.MustRegister(&guestKernelOOMVictimPrometheusCollector{proofs: guestVictims})
	}
	if epochs != nil {
		registry.MustRegister(&realizationEpochPrometheusCollector{epochs: epochs})
	}
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{MaxRequestsInFlight: maxConcurrentPrometheusScrapes})
}
