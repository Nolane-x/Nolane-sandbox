package cube

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const hostResourceMetricsPath = "/v1/metrics/resource"

var (
	ErrHostResourceUnavailable = errors.New("cube host resource observation unavailable")
	ErrInvalidResourceBinding  = errors.New("cube host resource binding invalid")
)

// ResourceBinding is an opaque identity capability minted from a concrete
// GuestSession. Callers may inspect the sandbox identity, but cannot construct
// or rewrite the binding state outside this package.
type ResourceBinding struct {
	sandboxID string
}

// ResourceBinding binds host observations to the exact sandbox backing this
// guest session. A nil or unrealized session yields an invalid zero binding.
func (s *GuestSession) ResourceBinding() ResourceBinding {
	if s == nil {
		return ResourceBinding{}
	}
	return ResourceBinding{sandboxID: strings.TrimSpace(s.sandboxID)}
}

func (b ResourceBinding) SandboxID() string { return b.sandboxID }

type HostResourceConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

// HostResourceSnapshot is observational host data. It is intentionally not a
// resourceproof.TrustedReport and contains no OOMKilled claim: Cubelet's
// memory_failures_total counter is not equivalent to authoritative task exit
// status.
type HostResourceSnapshot struct {
	SandboxID                  string
	CapturedAt                 time.Time
	CPULimitCores              float64
	CPUThrottledPeriods        uint64
	CPUThrottledUsec           uint64
	MemoryLimitBytes           uint64
	MemoryWorkingSetBytes      uint64
	MemoryFailures             uint64
}

type HostResourceObserver struct {
	endpoint string
	http     *http.Client
	clock    func() time.Time
}

func NewHostResourceObserver(cfg HostResourceConfig) (*HostResourceObserver, error) {
	raw := strings.TrimSpace(cfg.BaseURL)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid Cubelet management endpoint", ErrHostResourceUnavailable)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &HostResourceObserver{
		endpoint: strings.TrimRight(raw, "/") + hostResourceMetricsPath,
		http:     client,
		clock:    time.Now,
	}, nil
}

func (o *HostResourceObserver) Observe(ctx context.Context, binding ResourceBinding) (HostResourceSnapshot, error) {
	if o == nil || o.http == nil || o.clock == nil {
		return HostResourceSnapshot{}, ErrHostResourceUnavailable
	}
	sandboxID := strings.TrimSpace(binding.sandboxID)
	if sandboxID == "" {
		return HostResourceSnapshot{}, ErrInvalidResourceBinding
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint, nil)
	if err != nil {
		return HostResourceSnapshot{}, err
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: %v", ErrHostResourceUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return HostResourceSnapshot{}, fmt.Errorf("%w: metrics HTTP status %d", ErrHostResourceUnavailable, resp.StatusCode)
	}

	values, err := parseHostResourceMetrics(io.LimitReader(resp.Body, 1<<20), sandboxID)
	if err != nil {
		return HostResourceSnapshot{}, err
	}
	cpuLimit, err := positiveFinite(values["cubesandbox_host_sandbox_cpu_limit"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: cpu limit: %v", ErrHostResourceUnavailable, err)
	}
	throttledPeriods, err := exactUint(values["cubesandbox_host_sandbox_cpu_throttled_periods_total"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: throttled periods: %v", ErrHostResourceUnavailable, err)
	}
	throttledUsec, err := exactUint(values["cubesandbox_host_sandbox_cpu_throttled_useconds_total"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: throttled usec: %v", ErrHostResourceUnavailable, err)
	}
	memoryLimit, err := positiveUint(values["cubesandbox_host_sandbox_memory_limit"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: memory limit: %v", ErrHostResourceUnavailable, err)
	}
	workingSet, err := exactUint(values["cubesandbox_host_sandbox_memory_working_set_bytes"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: working set: %v", ErrHostResourceUnavailable, err)
	}
	failures, err := exactUint(values["cubesandbox_host_sandbox_memory_failures_total"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: memory failures: %v", ErrHostResourceUnavailable, err)
	}

	return HostResourceSnapshot{
		SandboxID:             sandboxID,
		CapturedAt:            o.clock().UTC(),
		CPULimitCores:         cpuLimit,
		CPUThrottledPeriods:   throttledPeriods,
		CPUThrottledUsec:      throttledUsec,
		MemoryLimitBytes:      memoryLimit,
		MemoryWorkingSetBytes: workingSet,
		MemoryFailures:        failures,
	}, nil
}

var hostResourceMetricNames = map[string]struct{}{
	"cubesandbox_host_sandbox_cpu_limit":                         {},
	"cubesandbox_host_sandbox_cpu_throttled_periods_total":       {},
	"cubesandbox_host_sandbox_cpu_throttled_useconds_total":      {},
	"cubesandbox_host_sandbox_memory_limit":                      {},
	"cubesandbox_host_sandbox_memory_working_set_bytes":          {},
	"cubesandbox_host_sandbox_memory_failures_total":             {},
}

func parseHostResourceMetrics(r io.Reader, sandboxID string) (map[string]float64, error) {
	values := make(map[string]float64, len(hostResourceMetricNames))
	s := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	s.Buffer(buf, 1<<20)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, labels, ok := splitMetricToken(fields[0])
		if !ok {
			continue
		}
		if _, wanted := hostResourceMetricNames[name]; !wanted || labels["sandbox_id"] != sandboxID {
			continue
		}
		if _, duplicate := values[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate %s for sandbox %q", ErrHostResourceUnavailable, name, sandboxID)
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, fmt.Errorf("%w: invalid %s value", ErrHostResourceUnavailable, name)
		}
		values[name] = v
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHostResourceUnavailable, err)
	}
	for name := range hostResourceMetricNames {
		if _, ok := values[name]; !ok {
			return nil, fmt.Errorf("%w: missing %s for sandbox %q", ErrHostResourceUnavailable, name, sandboxID)
		}
	}
	return values, nil
}

func splitMetricToken(token string) (string, map[string]string, bool) {
	open := strings.IndexByte(token, '{')
	close := strings.LastIndexByte(token, '}')
	if open <= 0 || close != len(token)-1 || close <= open+1 {
		return "", nil, false
	}
	labels := map[string]string{}
	for _, raw := range strings.Split(token[open+1:close], ",") {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return "", nil, false
		}
		key := strings.TrimSpace(parts[0])
		value, err := strconv.Unquote(strings.TrimSpace(parts[1]))
		if err != nil || key == "" {
			return "", nil, false
		}
		labels[key] = value
	}
	return token[:open], labels, true
}

func positiveFinite(v float64) (float64, error) {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("must be finite and positive")
	}
	return v, nil
}

func exactUint(v float64) (uint64, error) {
	if v < 0 || v > math.MaxUint64 || math.Trunc(v) != v {
		return 0, errors.New("must be an exact non-negative integer")
	}
	return uint64(v), nil
}

func positiveUint(v float64) (uint64, error) {
	n, err := exactUint(v)
	if err != nil || n == 0 {
		return 0, errors.New("must be a positive integer")
	}
	return n, nil
}
