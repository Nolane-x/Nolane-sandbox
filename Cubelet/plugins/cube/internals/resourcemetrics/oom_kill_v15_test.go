package resourcemetrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cubeboxstore "github.com/tencentcloud/CubeSandbox/Cubelet/pkg/store/cubebox"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/cgroup/handle"
)

func TestV15HostBaselinePreservesAndNormalizesOOMKillEvidence(t *testing.T) {
	usage := handle.UsageSnapshot{
		MemoryOOMKillsKnown: true,
		MemoryOOMKillsTotal: 9,
	}
	baseline := hostMetricsBaselineFromUsage("/cube/pool/7", handle.UsageSnapshot{
		MemoryOOMKillsKnown: true,
		MemoryOOMKillsTotal: 4,
	})

	require.True(t, baseline.MemoryOOMKillsKnown)
	require.Equal(t, uint64(4), baseline.MemoryOOMKillsTotal)

	normalized, err := normalizeHostSandboxUsage(usage, baseline)
	require.NoError(t, err)
	require.True(t, normalized.MemoryOOMKillsKnown)
	require.Equal(t, uint64(5), normalized.MemoryOOMKillsTotal)
}

func TestV15HostNormalizationFailsClosedOnOOMEvidencePresenceDrift(t *testing.T) {
	_, err := normalizeHostSandboxUsage(handle.UsageSnapshot{
		MemoryOOMKillsKnown: false,
	}, cubeboxstore.HostMetricsBaseline{
		CGroupPath:          "/cube/pool/7",
		MemoryOOMKillsKnown: true,
		MemoryOOMKillsTotal: 4,
	})
	require.Error(t, err)
}

func TestV15HostNormalizationFailsClosedOnOOMCounterRegression(t *testing.T) {
	_, err := normalizeHostSandboxUsage(handle.UsageSnapshot{
		MemoryOOMKillsKnown: true,
		MemoryOOMKillsTotal: 3,
	}, cubeboxstore.HostMetricsBaseline{
		CGroupPath:          "/cube/pool/7",
		MemoryOOMKillsKnown: true,
		MemoryOOMKillsTotal: 4,
	})
	require.Error(t, err)
}

func TestV15PrometheusExportsKnownOOMKillsAndOmitsUnknownEvidence(t *testing.T) {
	host := &HostSandboxSampler{
		config: HostSandboxSamplerConfig{StaleAfter: time.Minute},
		latest: map[string]HostSandboxLatest{
			"known": {
				SandboxID:    "known",
				CollectedAt:  time.Unix(110, 0).UTC(),
				Availability: HostSandboxAvailable,
				Snapshot: &HostSandboxSnapshot{
					SandboxID:            "known",
					MemoryOOMKillsKnown: true,
					MemoryOOMKillsTotal: 4,
				},
			},
			"unknown": {
				SandboxID:    "unknown",
				CollectedAt:  time.Unix(110, 0).UTC(),
				Availability: HostSandboxAvailable,
				Snapshot: &HostSandboxSnapshot{
					SandboxID:            "unknown",
					MemoryOOMKillsKnown: false,
				},
			},
		},
	}
	cache, err := NewSandboxResourceCache(nil, host, ResourceScopeHostSandbox)
	require.NoError(t, err)
	handler := newPrometheusHandler(cache, func() time.Time { return time.Unix(120, 0).UTC() })
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics/resource", nil))
	body := recorder.Body.String()

	require.Contains(t, body, `cubesandbox_host_sandbox_memory_oom_kills_total{sandbox_id="known"} 4`)
	require.NotContains(t, body, `cubesandbox_host_sandbox_memory_oom_kills_total{sandbox_id="unknown"}`)
}
