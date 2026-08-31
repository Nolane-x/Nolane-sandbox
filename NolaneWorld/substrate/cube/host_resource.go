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
const cpuLimitRatioTolerance = 1e-12

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
// status. CPUThrottledSeconds is the producer's cumulative seconds counter;
// MemoryCurrentBytes is current cgroup charge, not a working-set estimate.
// CPULimitQuotaUS and CPULimitPeriodUS are exact producer readback scalars;
// retaining them does not itself prove that the configured limit was enforced.
type HostResourceSnapshot struct {
	SandboxID           string
	CapturedAt          time.Time
	CPULimitCores       float64
	CPULimitQuotaUS     uint64
	CPULimitPeriodUS    uint64
	CPUThrottledPeriods uint64
	CPUThrottledSeconds float64
	MemoryLimitBytes    uint64
	MemoryCurrentBytes  uint64
	MemoryFailures      uint64
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
	cpuLimit, err := positiveFinite(values["cubesandbox_host_sandbox_cpu_limit_cores"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: cpu limit: %v", ErrHostResourceUnavailable, err)
	}
	cpuQuota, err := positiveUint(values["cubesandbox_host_sandbox_cpu_limit_quota_microseconds"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: cpu quota: %v", ErrHostResourceUnavailable, err)
	}
	cpuPeriod, err := positiveUint(values["cubesandbox_host_sandbox_cpu_limit_period_microseconds"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: cpu period: %v", ErrHostResourceUnavailable, err)
	}
	if !cpuLimitRatioMatches(cpuLimit, cpuQuota, cpuPeriod) {
		return HostResourceSnapshot{}, fmt.Errorf("%w: cpu limit cores disagree with exact quota/period", ErrHostResourceUnavailable)
	}
	throttledPeriods, err := exactUint(values["cubesandbox_host_sandbox_cpu_throttled_periods_total"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: throttled periods: %v", ErrHostResourceUnavailable, err)
	}
	throttledSeconds, err := nonNegativeFinite(values["cubesandbox_host_sandbox_cpu_throttled_seconds_total"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: throttled seconds: %v", ErrHostResourceUnavailable, err)
	}
	memoryLimit, err := positiveUint(values["cubesandbox_host_sandbox_memory_limit_bytes"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: memory limit: %v", ErrHostResourceUnavailable, err)
	}
	memoryCurrent, err := exactUint(values["cubesandbox_host_sandbox_memory_current_bytes"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: current memory: %v", ErrHostResourceUnavailable, err)
	}
	failures, err := exactUint(values["cubesandbox_host_sandbox_memory_failures_total"])
	if err != nil {
		return HostResourceSnapshot{}, fmt.Errorf("%w: memory failures: %v", ErrHostResourceUnavailable, err)
	}

	return HostResourceSnapshot{
		SandboxID:           sandboxID,
		CapturedAt:          o.clock().UTC(),
		CPULimitCores:       cpuLimit,
		CPULimitQuotaUS:     cpuQuota,
		CPULimitPeriodUS:    cpuPeriod,
		CPUThrottledPeriods: throttledPeriods,
		CPUThrottledSeconds: throttledSeconds,
		MemoryLimitBytes:    memoryLimit,
		MemoryCurrentBytes:  memoryCurrent,
		MemoryFailures:      failures,
	}, nil
}

var hostResourceMetricNames = map[string]struct{}{
	"cubesandbox_host_sandbox_cpu_limit_cores":                    {},
	"cubesandbox_host_sandbox_cpu_limit_quota_microseconds":       {},
	"cubesandbox_host_sandbox_cpu_limit_period_microseconds":      {},
	"cubesandbox_host_sandbox_cpu_throttled_periods_total":        {},
	"cubesandbox_host_sandbox_cpu_throttled_seconds_total":        {},
	"cubesandbox_host_sandbox_memory_limit_bytes":                 {},
	"cubesandbox_host_sandbox_memory_current_bytes":               {},
	"cubesandbox_host_sandbox_memory_failures_total":              {},
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
		if len(fields) == 0 {
			continue
		}
		name, labels, ok := splitMetricToken(fields[0])
		if _, wanted := hostResourceMetricNames[name]; !wanted {
			continue
		}
		if !ok || len(fields) != 2 {
			return nil, fmt.Errorf("%w: malformed %s metric", ErrHostResourceUnavailable, name)
		}
		if labels["sandbox_id"] != sandboxID {
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
	if open <= 0 {
		return "", nil, false
	}
	name := token[:open]
	close := strings.LastIndexByte(token, '}')
	if close != len(token)-1 || close <= open+1 {
		return name, nil, false
	}
	rawLabels, ok := splitPrometheusLabels(token[open+1 : close])
	if !ok {
		return name, nil, false
	}
	labels := make(map[string]string, len(rawLabels))
	for _, raw := range rawLabels {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return name, nil, false
		}
		key := strings.TrimSpace(parts[0])
		value, err := strconv.Unquote(strings.TrimSpace(parts[1]))
		if err != nil || key == "" {
			return name, nil, false
		}
		if _, duplicate := labels[key]; duplicate {
			return name, nil, false
		}
		labels[key] = value
	}
	return name, labels, true
}

func splitPrometheusLabels(raw string) ([]string, bool) {
	var labels []string
	start := 0
	inQuotes := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; {
		case escaped:
			escaped = false
		case inQuotes && c == '\\':
			escaped = true
		case c == '"':
			inQuotes = !inQuotes
		case c == ',' && !inQuotes:
			if i == start {
				return nil, false
			}
			labels = append(labels, raw[start:i])
			start = i + 1
		}
	}
	if inQuotes || escaped || start >= len(raw) {
		return nil, false
	}
	labels = append(labels, raw[start:])
	return labels, true
}

func positiveFinite(v float64) (float64, error) {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("must be finite and positive")
	}
	return v, nil
}

func nonNegativeFinite(v float64) (float64, error) {
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("must be finite and non-negative")
	}
	return v, nil
}

func exactUint(v float64) (uint64, error) {
	// math.MaxUint64 rounds to 2^64 when converted to float64. Reject that
	// rounded boundary as well; accepting it and converting to uint64 would
	// overflow instead of failing closed.
	if v < 0 || v >= float64(math.MaxUint64) || math.Trunc(v) != v {
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

func cpuLimitRatioMatches(cores float64, quota, period uint64) bool {
	expected := float64(quota) / float64(period)
	scale := math.Max(1, math.Max(math.Abs(cores), math.Abs(expected)))
	return math.Abs(cores-expected) <= cpuLimitRatioTolerance*scale
}
