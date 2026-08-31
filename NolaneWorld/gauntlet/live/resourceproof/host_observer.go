package resourceproof

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

// ErrHostObservationUnavailable means the host-owned observation path could not
// complete strongly enough to mint live resource evidence. It intentionally
// carries no cgroup path, task handle, provider handle, or underlying error.
var ErrHostObservationUnavailable = errors.New("live resource proof: host observation unavailable")

type HostCgroupVersion uint8

const (
	HostCgroupV1 HostCgroupVersion = 1
	HostCgroupV2 HostCgroupVersion = 2

	// Linux cgroup v1 commonly exposes an effectively-unlimited memory limit as
	// LONG_MAX rounded down to a page boundary. Bounded v12 proof rejects that
	// sentinel rather than laundering it into an exact finite readback.
	v1MemoryUnlimitedFloor uint64 = 9223372036854771712
)

type HostFileSource interface {
	ReadFile(string) ([]byte, error)
}

type HostTaskStatus struct {
	ExitCode int
	Reason   string
}

type HostPressureRunner interface {
	RunCPU(context.Context) error
	RunMemory(context.Context, uint64) (HostTaskStatus, error)
}

type HostObserverConfig struct {
	Version    HostCgroupVersion
	CgroupRoot string
	Files      HostFileSource
	Pressure   HostPressureRunner
}

type HostRequestedLimits struct {
	CPUQuotaMicros     int64
	CPUPeriodMicros    uint64
	MemoryBytes        uint64
	MemoryAttemptBytes uint64
}

type hostResourceObserver struct {
	config HostObserverConfig

	// These hooks are deliberately package-private test seams. Production host
	// authority remains the Pressure implementation; callers cannot export these
	// through canonical evidence or the agent runtime surface.
	afterCPU    func()
	afterMemory func()
}

func newHostResourceObserver(config HostObserverConfig) *hostResourceObserver {
	return &hostResourceObserver{config: config}
}

func (o *hostResourceObserver) observe(ctx context.Context, mode live.Mode, binding Binding, requested HostRequestedLimits) (TrustedReport, error) {
	if err := validateHostObserverRequest(o, ctx, mode, binding, requested); err != nil {
		return unavailableHostReport(mode, binding, requested), err
	}

	effectiveQuota, effectivePeriod, err := o.readCPUReadback()
	if err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}
	effectiveMemory, err := o.readMemoryReadback()
	if err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}
	throttleBefore, err := o.readThrottleCounters()
	if err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}
	oomBefore, err := o.readOOMCounter()
	if err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}

	if err := ctx.Err(); err != nil {
		return unavailableHostReport(mode, binding, requested), err
	}
	if err := o.config.Pressure.RunCPU(ctx); err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}
	if o.afterCPU != nil {
		o.afterCPU()
	}
	throttleAfter, err := o.readThrottleCounters()
	if err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}

	if err := ctx.Err(); err != nil {
		return unavailableHostReport(mode, binding, requested), err
	}
	taskStatus, err := o.config.Pressure.RunMemory(ctx, requested.MemoryAttemptBytes)
	if err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}
	if o.afterMemory != nil {
		o.afterMemory()
	}
	oomAfter, err := o.readOOMCounter()
	if err != nil {
		return unavailableHostReport(mode, binding, requested), ErrHostObservationUnavailable
	}

	cpu := CPUObservation{
		Source:                 SourceLiveHost,
		RequestedQuotaMicros:   requested.CPUQuotaMicros,
		RequestedPeriodMicros:  requested.CPUPeriodMicros,
		EffectiveQuotaMicros:   effectiveQuota,
		EffectivePeriodMicros:  effectivePeriod,
		PressureObserved:       true,
		NrThrottledBefore:      throttleBefore.nrThrottled,
		NrThrottledAfter:       throttleAfter.nrThrottled,
		ThrottledUsecBefore:    throttleBefore.throttledUsec,
		ThrottledUsecAfter:     throttleAfter.throttledUsec,
	}
	memory := MemoryObservation{
		Source:              SourceLiveHost,
		RequestedLimitBytes: requested.MemoryBytes,
		EffectiveLimitBytes: effectiveMemory,
		AttemptedBytes:      requested.MemoryAttemptBytes,
		OOMEventsBefore:     oomBefore,
		OOMEventsAfter:      oomAfter,
		ExitCode:            taskStatus.ExitCode,
		ExitReason:          taskStatus.Reason,
	}
	return buildTrustedReport(mode, binding, cpu, memory), nil
}

func validateHostObserverRequest(o *hostResourceObserver, ctx context.Context, mode live.Mode, binding Binding, requested HostRequestedLimits) error {
	if o == nil || o.config.Files == nil || o.config.Pressure == nil || strings.TrimSpace(o.config.CgroupRoot) == "" {
		return ErrHostObservationUnavailable
	}
	if o.config.Version != HostCgroupV1 && o.config.Version != HostCgroupV2 {
		return ErrHostObservationUnavailable
	}
	if mode != live.ModeProbe && mode != live.ModeRequireLive {
		return ErrHostObservationUnavailable
	}
	if !bindingValid(binding) {
		return ErrHostObservationUnavailable
	}
	if requested.CPUQuotaMicros <= 0 || requested.CPUPeriodMicros == 0 || requested.MemoryBytes == 0 || requested.MemoryAttemptBytes == 0 {
		return ErrHostObservationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func unavailableHostReport(mode live.Mode, binding Binding, requested HostRequestedLimits) TrustedReport {
	return buildTrustedReport(mode, binding, CPUObservation{
		Source:                 SourceFixture,
		RequestedQuotaMicros:   requested.CPUQuotaMicros,
		RequestedPeriodMicros:  requested.CPUPeriodMicros,
	}, MemoryObservation{
		Source:              SourceFixture,
		RequestedLimitBytes: requested.MemoryBytes,
		AttemptedBytes:      requested.MemoryAttemptBytes,
	})
}

func (o *hostResourceObserver) readCPUReadback() (int64, uint64, error) {
	switch o.config.Version {
	case HostCgroupV2:
		raw, err := o.readHostFile("cpu.max")
		if err != nil {
			return 0, 0, err
		}
		fields := strings.Fields(raw)
		if len(fields) != 2 || fields[0] == "max" {
			return 0, 0, ErrHostObservationUnavailable
		}
		quota, err := parsePositiveInt64(fields[0])
		if err != nil {
			return 0, 0, err
		}
		period, err := parsePositiveUint64(fields[1])
		if err != nil {
			return 0, 0, err
		}
		return quota, period, nil
	case HostCgroupV1:
		quotaRaw, err := o.readHostFile("cpu.cfs_quota_us")
		if err != nil {
			return 0, 0, err
		}
		periodRaw, err := o.readHostFile("cpu.cfs_period_us")
		if err != nil {
			return 0, 0, err
		}
		quota, err := parsePositiveInt64(singleField(quotaRaw))
		if err != nil {
			return 0, 0, err
		}
		period, err := parsePositiveUint64(singleField(periodRaw))
		if err != nil {
			return 0, 0, err
		}
		return quota, period, nil
	default:
		return 0, 0, ErrHostObservationUnavailable
	}
}

func (o *hostResourceObserver) readMemoryReadback() (uint64, error) {
	var leaf string
	switch o.config.Version {
	case HostCgroupV2:
		leaf = "memory.max"
	case HostCgroupV1:
		leaf = "memory.limit_in_bytes"
	default:
		return 0, ErrHostObservationUnavailable
	}
	raw, err := o.readHostFile(leaf)
	if err != nil {
		return 0, err
	}
	value := singleField(raw)
	if value == "" || value == "max" {
		return 0, ErrHostObservationUnavailable
	}
	limit, err := parsePositiveUint64(value)
	if err != nil {
		return 0, err
	}
	if o.config.Version == HostCgroupV1 && limit >= v1MemoryUnlimitedFloor {
		return 0, ErrHostObservationUnavailable
	}
	return limit, nil
}

type hostThrottleCounters struct {
	nrThrottled  uint64
	throttledUsec uint64
}

func (o *hostResourceObserver) readThrottleCounters() (hostThrottleCounters, error) {
	raw, err := o.readHostFile("cpu.stat")
	if err != nil {
		return hostThrottleCounters{}, err
	}
	stats, err := parseKeyUint64Stats(raw)
	if err != nil {
		return hostThrottleCounters{}, err
	}
	nr, ok := stats["nr_throttled"]
	if !ok {
		return hostThrottleCounters{}, ErrHostObservationUnavailable
	}
	switch o.config.Version {
	case HostCgroupV2:
		usec, ok := stats["throttled_usec"]
		if !ok {
			return hostThrottleCounters{}, ErrHostObservationUnavailable
		}
		return hostThrottleCounters{nrThrottled: nr, throttledUsec: usec}, nil
	case HostCgroupV1:
		nsec, ok := stats["throttled_time"]
		if !ok {
			return hostThrottleCounters{}, ErrHostObservationUnavailable
		}
		return hostThrottleCounters{nrThrottled: nr, throttledUsec: nsec / 1000}, nil
	default:
		return hostThrottleCounters{}, ErrHostObservationUnavailable
	}
}

func (o *hostResourceObserver) readOOMCounter() (uint64, error) {
	switch o.config.Version {
	case HostCgroupV2:
		raw, err := o.readHostFile("memory.events")
		if err != nil {
			return 0, err
		}
		stats, err := parseKeyUint64Stats(raw)
		if err != nil {
			return 0, err
		}
		if count, ok := stats["oom_kill"]; ok {
			return count, nil
		}
		if count, ok := stats["oom"]; ok {
			return count, nil
		}
		return 0, ErrHostObservationUnavailable
	case HostCgroupV1:
		raw, err := o.readHostFile("memory.failcnt")
		if err != nil {
			return 0, err
		}
		return parseUint64(singleField(raw))
	default:
		return 0, ErrHostObservationUnavailable
	}
}

func (o *hostResourceObserver) readHostFile(leaf string) (string, error) {
	root := strings.TrimRight(strings.TrimSpace(o.config.CgroupRoot), "/")
	if root == "" {
		root = "/"
	}
	path := root + "/" + leaf
	if root == "/" {
		path = root + leaf
	}
	raw, err := o.config.Files.ReadFile(path)
	if err != nil {
		return "", ErrHostObservationUnavailable
	}
	return string(raw), nil
}

func parseKeyUint64Stats(raw string) (map[string]uint64, error) {
	stats := make(map[string]uint64)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] == "" {
			return nil, ErrHostObservationUnavailable
		}
		value, err := parseUint64(fields[1])
		if err != nil {
			return nil, err
		}
		if _, duplicate := stats[fields[0]]; duplicate {
			return nil, ErrHostObservationUnavailable
		}
		stats[fields[0]] = value
	}
	if len(stats) == 0 {
		return nil, ErrHostObservationUnavailable
	}
	return stats, nil
}

func singleField(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) != 1 {
		return ""
	}
	return fields[0]
}

func parsePositiveInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, ErrHostObservationUnavailable
	}
	return value, nil
}

func parsePositiveUint64(raw string) (uint64, error) {
	value, err := parseUint64(raw)
	if err != nil || value == 0 {
		return 0, ErrHostObservationUnavailable
	}
	return value, nil
}

func parseUint64(raw string) (uint64, error) {
	if raw == "" || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return 0, ErrHostObservationUnavailable
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, ErrHostObservationUnavailable
	}
	return value, nil
}

// MarshalTrustedReport serializes only a package-owned, self-consistent trusted
// report. Host locators never enter Report, so this surface cannot leak the
// cgroup root or pressure-runner authority.
func MarshalTrustedReport(trusted TrustedReport) ([]byte, error) {
	if err := VerifyTrustedReport(trusted); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(trusted.Report(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
