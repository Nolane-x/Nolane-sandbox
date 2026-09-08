package cube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidRuntimeCgroupReadbackAuthority = errors.New("cube: invalid runtime cgroup readback authority")

const runtimeCgroupReadbackRoot = "/sys/fs/cgroup"

type runtimeCgroupReadFile func(string) ([]byte, error)

type runtimeCgroupReadbackObserverSeal struct{}

var currentRuntimeCgroupReadbackObserverSeal = &runtimeCgroupReadbackObserverSeal{}

// RuntimeCgroupReadbackObserver reads one bounded cgroup-v2 snapshot from the
// exact cgroup path already sealed into Wave27 runtime provenance.
type RuntimeCgroupReadbackObserver struct {
	root     string
	readFile runtimeCgroupReadFile
	seal     *runtimeCgroupReadbackObserverSeal
}

func NewRuntimeCgroupReadbackObserver() *RuntimeCgroupReadbackObserver {
	return &RuntimeCgroupReadbackObserver{
		root:     runtimeCgroupReadbackRoot,
		readFile: os.ReadFile,
		seal:     currentRuntimeCgroupReadbackObserverSeal,
	}
}

func newRuntimeCgroupReadbackObserverForTest(root string, readFile runtimeCgroupReadFile) *RuntimeCgroupReadbackObserver {
	return &RuntimeCgroupReadbackObserver{
		root:     root,
		readFile: readFile,
		seal:     currentRuntimeCgroupReadbackObserverSeal,
	}
}

func (o *RuntimeCgroupReadbackObserver) valid() bool {
	if o == nil || o.seal != currentRuntimeCgroupReadbackObserverSeal || o.readFile == nil {
		return false
	}
	clean := filepath.Clean(o.root)
	return filepath.IsAbs(clean) && clean != string(filepath.Separator) && clean == o.root
}

// RuntimeCgroupReadbackSnapshot is descriptive immutable data projected from a
// sealed Wave28 capability. Counter values describe one read instant only.
type RuntimeCgroupReadbackSnapshot struct {
	RuntimeDigest    string
	SandboxID        string
	Generation       uint64
	HostPID          uint32
	StartTimeTicks   uint64
	BootID           string
	CGroupPath       string
	CPUQuotaMicros   int64
	CPUPeriodMicros  uint64
	NrThrottled      uint64
	ThrottledUsec    uint64
	MemoryLimitBytes uint64
	OOMEvents        uint64
	OOMKillEvents    uint64
}

type runtimeCgroupReadbackAuthoritySeal struct{}

var currentRuntimeCgroupReadbackAuthoritySeal = &runtimeCgroupReadbackAuthoritySeal{}

// RuntimeCgroupReadbackAuthority is Wave28's opaque exact-cgroup readback
// capability. It proves a fresh Wave27 runtime was a member of the cgroup from
// which this finite v2 snapshot was read; it makes no causal load claim.
type RuntimeCgroupReadbackAuthority struct {
	runtime  RealmResourceRuntimeAuthority
	snapshot RuntimeCgroupReadbackSnapshot
	digest   string
	seal     *runtimeCgroupReadbackAuthoritySeal
}

func (a RuntimeCgroupReadbackAuthority) Valid() bool {
	if a.seal != currentRuntimeCgroupReadbackAuthoritySeal || !a.runtime.Valid() || !validRuntimeCgroupReadbackDigest(a.digest) {
		return false
	}
	if !runtimeCgroupSnapshotMatchesAuthority(a.snapshot, a.runtime) {
		return false
	}
	expected, err := deriveRuntimeCgroupReadbackDigest(a.runtime, a.snapshot)
	return err == nil && expected == a.digest
}

func (a RuntimeCgroupReadbackAuthority) RuntimeDigest() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.snapshot.RuntimeDigest, true
}

func (a RuntimeCgroupReadbackAuthority) ReadbackDigest() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.digest, true
}

func (a RuntimeCgroupReadbackAuthority) Snapshot() (RuntimeCgroupReadbackSnapshot, bool) {
	if !a.Valid() {
		return RuntimeCgroupReadbackSnapshot{}, false
	}
	return a.snapshot, true
}

// ValidateRuntimeCgroupReadbackAuthority re-establishes Wave27 freshness on
// both sides of one exact cgroup-v2 read. The concrete sandbox identity and
// target path come only from the sealed Wave27 host process identity.
func ValidateRuntimeCgroupReadbackAuthority(
	ctx context.Context,
	controller *realm.Controller,
	realization realm.RealizationAuthority,
	runtimeAuthority RealmResourceRuntimeAuthority,
	epochObserver *RealizationEpochObserver,
	client *Client,
	runtimeObserver *RuntimeRealizationObserver,
	observer *RuntimeCgroupReadbackObserver,
) (RuntimeCgroupReadbackAuthority, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeCgroupReadbackAuthority{}, err
	}
	if !runtimeAuthority.Valid() || !observer.valid() {
		return RuntimeCgroupReadbackAuthority{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	resource := ResourceBinding{sandboxID: runtimeAuthority.runtime.process.SandboxID}

	freshBefore, err := ValidateRealmResourceRuntimeAuthority(
		ctx,
		controller,
		realization,
		runtimeAuthority.endpoint,
		resource,
		epochObserver,
		client,
		runtimeObserver,
		runtimeAuthority.runtime,
	)
	if err != nil {
		return RuntimeCgroupReadbackAuthority{}, err
	}
	if !sameRealmResourceRuntimeAuthority(freshBefore, runtimeAuthority) {
		return RuntimeCgroupReadbackAuthority{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}

	snapshot, err := observer.observe(ctx, freshBefore)
	if err != nil {
		return RuntimeCgroupReadbackAuthority{}, err
	}

	freshAfter, err := ValidateRealmResourceRuntimeAuthority(
		ctx,
		controller,
		realization,
		runtimeAuthority.endpoint,
		resource,
		epochObserver,
		client,
		runtimeObserver,
		runtimeAuthority.runtime,
	)
	if err != nil {
		return RuntimeCgroupReadbackAuthority{}, err
	}
	if !sameRealmResourceRuntimeAuthority(freshAfter, runtimeAuthority) || !sameRealmResourceRuntimeAuthority(freshAfter, freshBefore) {
		return RuntimeCgroupReadbackAuthority{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	if !runtimeCgroupSnapshotMatchesAuthority(snapshot, freshAfter) {
		return RuntimeCgroupReadbackAuthority{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}

	digest, err := deriveRuntimeCgroupReadbackDigest(freshAfter, snapshot)
	if err != nil {
		return RuntimeCgroupReadbackAuthority{}, err
	}
	return RuntimeCgroupReadbackAuthority{
		runtime:  freshAfter,
		snapshot: snapshot,
		digest:   digest,
		seal:     currentRuntimeCgroupReadbackAuthoritySeal,
	}, nil
}

func sameRealmResourceRuntimeAuthority(a, b RealmResourceRuntimeAuthority) bool {
	return a.Valid() && b.Valid() &&
		a.seal == b.seal &&
		sameRealmResourceProviderEndpointAuthority(a.endpoint, b.endpoint) &&
		sameRuntimeRealizationProof(a.runtime, b.runtime) &&
		a.digest == b.digest
}

func (o *RuntimeCgroupReadbackObserver) observe(ctx context.Context, authority RealmResourceRuntimeAuthority) (RuntimeCgroupReadbackSnapshot, error) {
	if !o.valid() || !authority.Valid() {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	if err := ctx.Err(); err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}

	process := authority.runtime.process
	runtimeDigest, ok := authority.RuntimeDigest()
	if !ok {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	target, err := o.targetPath(process.CGroupPath)
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}

	controllers, err := o.read(ctx, filepath.Join(o.root, "cgroup.controllers"))
	if err != nil || !validCgroupV2Controllers(controllers) {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	procs, err := o.read(ctx, filepath.Join(target, "cgroup.procs"))
	if err != nil || !containsExactCgroupPID(procs, process.HostPID) {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	cpuMax, err := o.read(ctx, filepath.Join(target, "cpu.max"))
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	quota, period, err := parseCgroupV2CPUMax(cpuMax)
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	cpuStat, err := o.read(ctx, filepath.Join(target, "cpu.stat"))
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	cpuStats, err := parseCanonicalCgroupStats(cpuStat)
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	nrThrottled, ok := cpuStats["nr_throttled"]
	if !ok {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	throttledUsec, ok := cpuStats["throttled_usec"]
	if !ok {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}

	memoryMax, err := o.read(ctx, filepath.Join(target, "memory.max"))
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	memoryLimit, err := parseCgroupV2MemoryMax(memoryMax)
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	memoryEvents, err := o.read(ctx, filepath.Join(target, "memory.events"))
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	memoryStats, err := parseCanonicalCgroupStats(memoryEvents)
	if err != nil {
		return RuntimeCgroupReadbackSnapshot{}, err
	}
	oom, ok := memoryStats["oom"]
	if !ok {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	oomKill, ok := memoryStats["oom_kill"]
	if !ok {
		return RuntimeCgroupReadbackSnapshot{}, ErrInvalidRuntimeCgroupReadbackAuthority
	}

	return RuntimeCgroupReadbackSnapshot{
		RuntimeDigest:    runtimeDigest,
		SandboxID:        process.SandboxID,
		Generation:       process.Generation,
		HostPID:          process.HostPID,
		StartTimeTicks:   process.StartTimeTicks,
		BootID:           process.BootID,
		CGroupPath:       process.CGroupPath,
		CPUQuotaMicros:   quota,
		CPUPeriodMicros:  period,
		NrThrottled:      nrThrottled,
		ThrottledUsec:    throttledUsec,
		MemoryLimitBytes: memoryLimit,
		OOMEvents:        oom,
		OOMKillEvents:    oomKill,
	}, nil
}

func (o *RuntimeCgroupReadbackObserver) read(ctx context.Context, filename string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	raw, err := o.readFile(filename)
	if err != nil {
		return "", ErrInvalidRuntimeCgroupReadbackAuthority
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(raw), nil
}

func (o *RuntimeCgroupReadbackObserver) targetPath(cgroupPath string) (string, error) {
	if !o.valid() || cgroupPath == "" || !strings.HasPrefix(cgroupPath, "/") || cgroupPath == "/" || path.Clean(cgroupPath) != cgroupPath {
		return "", ErrInvalidRuntimeCgroupReadbackAuthority
	}
	rel := strings.TrimPrefix(cgroupPath, "/")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", ErrInvalidRuntimeCgroupReadbackAuthority
	}
	root := filepath.Clean(o.root)
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	prefix := root + string(filepath.Separator)
	if target == root || !strings.HasPrefix(target, prefix) {
		return "", ErrInvalidRuntimeCgroupReadbackAuthority
	}
	return target, nil
}

func validCgroupV2Controllers(raw string) bool {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field == "" {
			return false
		}
		if _, duplicate := seen[field]; duplicate {
			return false
		}
		seen[field] = struct{}{}
	}
	_, cpu := seen["cpu"]
	_, memory := seen["memory"]
	return cpu && memory
}

func containsExactCgroupPID(raw string, expected uint32) bool {
	if expected == 0 {
		return false
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	seen := make(map[uint64]struct{})
	found := false
	for _, line := range strings.Split(trimmed, "\n") {
		if line == "" || strings.TrimSpace(line) != line || strings.ContainsAny(line, " \t\r") {
			return false
		}
		pid, ok := parseRuntimeCgroupCanonicalUint(line, false)
		if !ok || pid > uint64(^uint32(0)) {
			return false
		}
		if _, duplicate := seen[pid]; duplicate {
			return false
		}
		seen[pid] = struct{}{}
		if uint32(pid) == expected {
			found = true
		}
	}
	return found
}

func parseCgroupV2CPUMax(raw string) (int64, uint64, error) {
	trimmed := strings.TrimSpace(raw)
	fields := strings.Fields(trimmed)
	if len(fields) != 2 || trimmed != fields[0]+" "+fields[1] || fields[0] == "max" {
		return 0, 0, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	quota, ok := parseRuntimeCgroupCanonicalUint(fields[0], false)
	if !ok || quota > uint64(^uint64(0)>>1) {
		return 0, 0, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	period, ok := parseRuntimeCgroupCanonicalUint(fields[1], false)
	if !ok {
		return 0, 0, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	return int64(quota), period, nil
}

func parseCgroupV2MemoryMax(raw string) (uint64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "max" || strings.ContainsAny(trimmed, " \t\r\n") {
		return 0, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	value, ok := parseRuntimeCgroupCanonicalUint(trimmed, false)
	if !ok {
		return 0, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	return value, nil
}

func parseCanonicalCgroupStats(raw string) (map[string]uint64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, ErrInvalidRuntimeCgroupReadbackAuthority
	}
	stats := make(map[string]uint64)
	for _, line := range strings.Split(trimmed, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || line != fields[0]+" "+fields[1] || fields[0] == "" {
			return nil, ErrInvalidRuntimeCgroupReadbackAuthority
		}
		if _, duplicate := stats[fields[0]]; duplicate {
			return nil, ErrInvalidRuntimeCgroupReadbackAuthority
		}
		value, ok := parseRuntimeCgroupCanonicalUint(fields[1], true)
		if !ok {
			return nil, ErrInvalidRuntimeCgroupReadbackAuthority
		}
		stats[fields[0]] = value
	}
	return stats, nil
}

func parseRuntimeCgroupCanonicalUint(raw string, allowZero bool) (uint64, bool) {
	if raw == "" || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || strconv.FormatUint(value, 10) != raw || (!allowZero && value == 0) {
		return 0, false
	}
	return value, true
}

func runtimeCgroupSnapshotMatchesAuthority(snapshot RuntimeCgroupReadbackSnapshot, authority RealmResourceRuntimeAuthority) bool {
	if !authority.Valid() || !validRuntimeRealizationDigest(snapshot.RuntimeDigest) ||
		snapshot.SandboxID == "" || snapshot.Generation == 0 || snapshot.HostPID == 0 || snapshot.StartTimeTicks == 0 ||
		snapshot.CGroupPath == "" || snapshot.CGroupPath == "/" || snapshot.CPUQuotaMicros <= 0 || snapshot.CPUPeriodMicros == 0 || snapshot.MemoryLimitBytes == 0 {
		return false
	}
	runtimeDigest, ok := authority.RuntimeDigest()
	if !ok || runtimeDigest != snapshot.RuntimeDigest {
		return false
	}
	process := authority.runtime.process
	return snapshot.SandboxID == process.SandboxID &&
		snapshot.Generation == process.Generation &&
		snapshot.HostPID == process.HostPID &&
		snapshot.StartTimeTicks == process.StartTimeTicks &&
		snapshot.BootID == process.BootID &&
		snapshot.CGroupPath == process.CGroupPath
}

func deriveRuntimeCgroupReadbackDigest(authority RealmResourceRuntimeAuthority, snapshot RuntimeCgroupReadbackSnapshot) (string, error) {
	if !runtimeCgroupSnapshotMatchesAuthority(snapshot, authority) {
		return "", ErrInvalidRuntimeCgroupReadbackAuthority
	}
	document := struct {
		RuntimeDigest    string `json:"runtime_digest"`
		SandboxID        string `json:"sandbox_id"`
		Generation       uint64 `json:"generation"`
		HostPID          uint32 `json:"host_pid"`
		StartTimeTicks   uint64 `json:"starttime_ticks"`
		BootID           string `json:"boot_id"`
		CGroupPath       string `json:"cgroup_path"`
		CPUQuotaMicros   int64  `json:"cpu_quota_micros"`
		CPUPeriodMicros  uint64 `json:"cpu_period_micros"`
		NrThrottled      uint64 `json:"nr_throttled"`
		ThrottledUsec    uint64 `json:"throttled_usec"`
		MemoryLimitBytes uint64 `json:"memory_limit_bytes"`
		OOMEvents        uint64 `json:"oom_events"`
		OOMKillEvents    uint64 `json:"oom_kill_events"`
	}{
		RuntimeDigest:    snapshot.RuntimeDigest,
		SandboxID:        snapshot.SandboxID,
		Generation:       snapshot.Generation,
		HostPID:          snapshot.HostPID,
		StartTimeTicks:   snapshot.StartTimeTicks,
		BootID:           snapshot.BootID,
		CGroupPath:       snapshot.CGroupPath,
		CPUQuotaMicros:   snapshot.CPUQuotaMicros,
		CPUPeriodMicros:  snapshot.CPUPeriodMicros,
		NrThrottled:      snapshot.NrThrottled,
		ThrottledUsec:    snapshot.ThrottledUsec,
		MemoryLimitBytes: snapshot.MemoryLimitBytes,
		OOMEvents:        snapshot.OOMEvents,
		OOMKillEvents:    snapshot.OOMKillEvents,
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return "", ErrInvalidRuntimeCgroupReadbackAuthority
	}
	preimage := append([]byte("nolane.runtime-cgroup-readback.v28\x00"), raw...)
	digest := sha256.Sum256(preimage)
	return "runtime-cgroup-readback-v28:" + hex.EncodeToString(digest[:]), nil
}

func validRuntimeCgroupReadbackDigest(raw string) bool {
	const prefix = "runtime-cgroup-readback-v28:"
	if !strings.HasPrefix(raw, prefix) {
		return false
	}
	hexDigest := strings.TrimPrefix(raw, prefix)
	if len(hexDigest) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(hexDigest)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == hexDigest
}
