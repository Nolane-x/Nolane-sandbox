package cube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const upstreamHostMetricsFixture = `# HELP cubesandbox_host_sandbox_cpu_limit_cores Configured CPU limit for the host sandbox cgroup in cores.
# TYPE cubesandbox_host_sandbox_cpu_limit_cores gauge
cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="other"} 4
cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5
cubesandbox_host_sandbox_cpu_throttled_periods_total{sandbox_id="sandbox-123"} 7
cubesandbox_host_sandbox_cpu_throttled_seconds_total{sandbox_id="sandbox-123"} 0.0009
cubesandbox_host_sandbox_memory_limit_bytes{sandbox_id="sandbox-123"} 67108864
cubesandbox_host_sandbox_memory_current_bytes{sandbox_id="sandbox-123"} 33554432
cubesandbox_host_sandbox_memory_failures_total{sandbox_id="sandbox-123"} 3
`

func upstreamHostMetricsWithExactCPU() string {
	return strings.Replace(upstreamHostMetricsFixture,
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5`,
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5
cubesandbox_host_sandbox_cpu_limit_quota_microseconds{sandbox_id="sandbox-123"} 25000
cubesandbox_host_sandbox_cpu_limit_period_microseconds{sandbox_id="sandbox-123"} 50000`, 1)
}

func TestV13GuestSessionMintsOpaqueResourceBinding(t *testing.T) {
	session := &GuestSession{sandboxID: "sandbox-123"}
	binding := session.ResourceBinding()
	if got := binding.SandboxID(); got != "sandbox-123" {
		t.Fatalf("binding sandbox id = %q, want sandbox-123", got)
	}
}

func TestV13HostResourceObserverMatchesUpstreamCubeletContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/resource" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(upstreamHostMetricsWithExactCPU()))
	}))
	defer server.Close()

	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHostResourceObserver: %v", err)
	}
	binding := (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding()
	snapshot, err := observer.Observe(context.Background(), binding)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if snapshot.SandboxID != "sandbox-123" || snapshot.CPULimitCores != 0.5 || snapshot.CPUThrottledPeriods != 7 || snapshot.CPUThrottledSeconds != 0.0009 {
		t.Fatalf("unexpected CPU snapshot: %+v", snapshot)
	}
	if snapshot.MemoryLimitBytes != 67108864 || snapshot.MemoryCurrentBytes != 33554432 || snapshot.MemoryFailures != 3 {
		t.Fatalf("unexpected memory snapshot: %+v", snapshot)
	}
}

func TestV13HostResourceObserverRejectsDuplicateMetricForBinding(t *testing.T) {
	metrics := strings.Replace(upstreamHostMetricsWithExactCPU(),
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5`,
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5
cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.6`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(metrics)) }))
	defer server.Close()
	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = observer.Observe(context.Background(), (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding())
	if err == nil {
		t.Fatal("duplicate metric must fail closed")
	}
}

func TestV13HostResourceObserverRejectsMissingSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="other"} 1`))
	}))
	defer server.Close()
	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = observer.Observe(context.Background(), (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding())
	if err == nil {
		t.Fatal("missing exact sandbox metrics must fail closed")
	}
}

func TestV13HostResourceObserverRejectsLegacySyntheticMetricAliases(t *testing.T) {
	legacy := `cubesandbox_host_sandbox_cpu_limit{sandbox_id="sandbox-123"} 0.5
cubesandbox_host_sandbox_cpu_throttled_periods_total{sandbox_id="sandbox-123"} 7
cubesandbox_host_sandbox_cpu_throttled_useconds_total{sandbox_id="sandbox-123"} 900
cubesandbox_host_sandbox_memory_limit{sandbox_id="sandbox-123"} 67108864
cubesandbox_host_sandbox_memory_working_set_bytes{sandbox_id="sandbox-123"} 33554432
cubesandbox_host_sandbox_memory_failures_total{sandbox_id="sandbox-123"} 3
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(legacy)) }))
	defer server.Close()
	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Observe(context.Background(), (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding()); err == nil {
		t.Fatal("synthetic v13 aliases must not satisfy the Cubelet producer contract")
	}
}

func TestV13HostResourceObserverRejectsFractionalIntegerMetric(t *testing.T) {
	metrics := strings.Replace(upstreamHostMetricsWithExactCPU(),
		`cubesandbox_host_sandbox_memory_current_bytes{sandbox_id="sandbox-123"} 33554432`,
		`cubesandbox_host_sandbox_memory_current_bytes{sandbox_id="sandbox-123"} 1.5`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(metrics)) }))
	defer server.Close()
	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Observe(context.Background(), (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding()); err == nil {
		t.Fatal("fractional byte gauge must fail closed")
	}
}

func TestV13HostResourceObserverRejectsNonFiniteSeconds(t *testing.T) {
	metrics := strings.Replace(upstreamHostMetricsWithExactCPU(),
		`cubesandbox_host_sandbox_cpu_throttled_seconds_total{sandbox_id="sandbox-123"} 0.0009`,
		`cubesandbox_host_sandbox_cpu_throttled_seconds_total{sandbox_id="sandbox-123"} +Inf`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(metrics)) }))
	defer server.Close()
	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Observe(context.Background(), (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding()); err == nil {
		t.Fatal("non-finite throttled seconds must fail closed")
	}
}

func TestV13MetricParserRejectsDuplicateLabelKeys(t *testing.T) {
	metrics := strings.Replace(upstreamHostMetricsWithExactCPU(),
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="sandbox-123"} 0.5`,
		`cubesandbox_host_sandbox_cpu_limit_cores{sandbox_id="other",sandbox_id="sandbox-123"} 0.5`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(metrics)) }))
	defer server.Close()
	observer, err := NewHostResourceObserver(HostResourceConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Observe(context.Background(), (&GuestSession{sandboxID: "sandbox-123"}).ResourceBinding()); err == nil {
		t.Fatal("duplicate Prometheus label keys must fail closed")
	}
}
