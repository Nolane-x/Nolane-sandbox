package resourcemetrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestV14PrometheusExportsExactHostCPUQuotaAndPeriod(t *testing.T) {
	host := &HostSandboxSampler{
		config: HostSandboxSamplerConfig{StaleAfter: time.Minute},
		latest: map[string]HostSandboxLatest{
			"sandbox": {
				SandboxID:    "sandbox",
				CollectedAt:  time.Unix(111, 0).UTC(),
				Availability: HostSandboxAvailable,
				Snapshot: &HostSandboxSnapshot{
					Timestamp:        time.Unix(111, 0).UTC(),
					SandboxID:        "sandbox",
					CPULimitQuotaUS:  150000,
					CPULimitPeriodUS: 100000,
				},
			},
		},
	}
	cache, err := NewSandboxResourceCache(nil, host, ResourceScopeHostSandbox)
	require.NoError(t, err)
	handler := newPrometheusHandler(cache, func() time.Time { return time.Unix(120, 0).UTC() })
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics/resource", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()

	require.Contains(t, body, `cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox"} 1.5`)
	require.Contains(t, body, `cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="sandbox"} 150000`)
	require.Contains(t, body, `cubesandbox_host_sandbox_cpu_limit_period_microseconds{sandbox_id="sandbox"} 100000`)
}

func TestV14PrometheusOmitsExactHostCPUReadbackWhenLimitIsUnlimited(t *testing.T) {
	host := &HostSandboxSampler{
		config: HostSandboxSamplerConfig{StaleAfter: time.Minute},
		latest: map[string]HostSandboxLatest{
			"unlimited": {
				SandboxID:    "unlimited",
				CollectedAt:  time.Unix(111, 0).UTC(),
				Availability: HostSandboxAvailable,
				Snapshot: &HostSandboxSnapshot{
					Timestamp:         time.Unix(111, 0).UTC(),
					SandboxID:         "unlimited",
					CPULimitPeriodUS:  100000,
					CPULimitUnlimited: true,
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

	require.NotContains(t, body, `cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="unlimited"}`)
	require.NotContains(t, body, `cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="unlimited"}`)
	require.NotContains(t, body, `cubesandbox_host_sandbox_cpu_limit_period_microseconds{sandbox_id="unlimited"}`)
}
