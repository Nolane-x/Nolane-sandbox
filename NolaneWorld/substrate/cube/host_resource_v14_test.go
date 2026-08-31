package cube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const v14ExactCPUHostMetricsFixture = `cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5
cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="sandbox-123"} 25000
cubesandbox_host_sandbox_cpu_limit_period_microseconds{sandbox_id="sandbox-123"} 50000
cubesandbox_host_sandbox_cpu_throttled_periods_total{sandbox_id="sandbox-123"} 7
cubesandbox_host_sandbox_cpu_throttled_seconds_total{sandbox_id="sandbox-123"} 0.0009
cubesandbox_host_sandbox_memory_limit_bytes{sandbox_id="sandbox-123"} 67108864
cubesandbox_host_sandbox_memory_current_bytes{sandbox_id="sandbox-123"} 33554432
cubesandbox_host_sandbox_memory_failures_total{sandbox_id="sandbox-123"} 3
`

func observeV14Fixture(t *testing.T, metrics string) (HostResourceSnapshot, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/resource" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(metrics))
	}))
	t.Cleanup(server.Close)
	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHostResourceObserver: %v", err)
	}
	return observer.Observe(context.Background(), (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding())
}

func TestV14ObserverPreservesExactCPUQuotaAndPeriodWithoutReconstruction(t *testing.T) {
	snapshot, err := observeV14Fixture(t, v14ExactCPUHostMetricsFixture)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	value := reflect.ValueOf(snapshot)
	quota := value.FieldByName("CPULimitQuotaUS")
	period := value.FieldByName("CPULimitPeriodUS")
	if !quota.IsValid() || !period.IsValid() {
		t.Fatalf("HostResourceSnapshot must expose exact quota/period readback fields: %+v", snapshot)
	}
	if quota.Uint() != 25000 || period.Uint() != 50000 {
		t.Fatalf("exact CPU readback = quota %d period %d, want 25000/50000", quota.Uint(), period.Uint())
	}
}

func TestV14ObserverRejectsCoresOnlyCPUClaim(t *testing.T) {
	if _, err := observeV14Fixture(t, upstreamHostMetricsFixture); err == nil {
		t.Fatal("cpu_limit_cores without exact quota/period must fail closed")
	}
}

func TestV14ObserverRejectsFractionalExactCPUReadback(t *testing.T) {
	metrics := strings.Replace(v14ExactCPUHostMetricsFixture,
		`cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="sandbox-123"} 25000`,
		`cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="sandbox-123"} 25000.5`, 1)
	if _, err := observeV14Fixture(t, metrics); err == nil {
		t.Fatal("fractional CPU quota must fail closed")
	}
}

func TestV14ObserverRejectsOverflowExactCPUReadback(t *testing.T) {
	metrics := strings.Replace(v14ExactCPUHostMetricsFixture,
		`cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="sandbox-123"} 25000`,
		`cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="sandbox-123"} 18446744073709551616`, 1)
	if _, err := observeV14Fixture(t, metrics); err == nil {
		t.Fatal("overflow CPU quota must fail closed")
	}
}

func TestV14ObserverRejectsDerivedCoreMismatch(t *testing.T) {
	metrics := strings.Replace(v14ExactCPUHostMetricsFixture,
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5`,
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.75`, 1)
	if _, err := observeV14Fixture(t, metrics); err == nil {
		t.Fatal("derived cpu_limit_cores must agree with exact quota/period")
	}
}
