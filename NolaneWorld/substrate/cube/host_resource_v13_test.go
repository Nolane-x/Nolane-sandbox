package cube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV13GuestSessionMintsOpaqueResourceBinding(t *testing.T) {
	session := &GuestSession{sandboxID: "sandbox-123"}
	binding := session.ResourceBinding()
	if got := binding.SandboxID(); got != "sandbox-123" {
		t.Fatalf("binding sandbox id = %q, want sandbox-123", got)
	}
}

func TestV13HostResourceObserverSelectsExactSandbox(t *testing.T) {
	metrics := `# HELP cubesandbox_host_sandbox_cpu_limit CPU limit
# TYPE cubesandbox_host_sandbox_cpu_limit gauge
cubesandbox_host_sandbox_cpu_limit{sandbox_id="other"} 4
cubesandbox_host_sandbox_cpu_limit{sandbox_id="sandbox-123"} 0.5
cubesandbox_host_sandbox_cpu_throttled_periods_total{sandbox_id="sandbox-123"} 7
cubesandbox_host_sandbox_cpu_throttled_useconds_total{sandbox_id="sandbox-123"} 900
cubesandbox_host_sandbox_memory_limit{sandbox_id="sandbox-123"} 67108864
cubesandbox_host_sandbox_memory_working_set_bytes{sandbox_id="sandbox-123"} 33554432
cubesandbox_host_sandbox_memory_failures_total{sandbox_id="sandbox-123"} 3
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/resource" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(metrics))
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
	if snapshot.SandboxID != "sandbox-123" || snapshot.CPULimitCores != 0.5 || snapshot.CPUThrottledPeriods != 7 || snapshot.CPUThrottledUsec != 900 {
		t.Fatalf("unexpected CPU snapshot: %+v", snapshot)
	}
	if snapshot.MemoryLimitBytes != 67108864 || snapshot.MemoryWorkingSetBytes != 33554432 || snapshot.MemoryFailures != 3 {
		t.Fatalf("unexpected memory snapshot: %+v", snapshot)
	}
}

func TestV13HostResourceObserverRejectsDuplicateMetricForBinding(t *testing.T) {
	metrics := `cubesandbox_host_sandbox_cpu_limit{sandbox_id="sandbox-123"} 0.5
cubesandbox_host_sandbox_cpu_limit{sandbox_id="sandbox-123"} 0.6
cubesandbox_host_sandbox_cpu_throttled_periods_total{sandbox_id="sandbox-123"} 1
cubesandbox_host_sandbox_cpu_throttled_useconds_total{sandbox_id="sandbox-123"} 2
cubesandbox_host_sandbox_memory_limit{sandbox_id="sandbox-123"} 1024
cubesandbox_host_sandbox_memory_working_set_bytes{sandbox_id="sandbox-123"} 512
cubesandbox_host_sandbox_memory_failures_total{sandbox_id="sandbox-123"} 0
`
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
		_, _ = w.Write([]byte(`cubesandbox_host_sandbox_cpu_limit{sandbox_id="other"} 1`))
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
