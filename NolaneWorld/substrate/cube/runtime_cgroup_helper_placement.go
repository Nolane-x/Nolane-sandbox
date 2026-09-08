package cube

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidRuntimeCgroupHelperPlacementAuthority = errors.New("cube: invalid runtime cgroup helper placement authority")

const runtimeCgroupHelperRoot = "/sys/fs/cgroup"
const runtimeCgroupHelperDigestDomain = "nolane.runtime-cgroup-helper-placement.v29\x00"
const runtimeCgroupHelperDigestPrefix = "runtime-cgroup-helper-placement-v29:"

type runtimeCgroupHelperReadFile func(string) ([]byte, error)
type runtimeCgroupHelperWriteFile func(string, []byte) error
type runtimeCgroupHelperLaunch func(context.Context, string, string) (runtimeCgroupHelperChild, error)

type runtimeCgroupHelperChild interface {
	PID() int
	AwaitReady(context.Context, string) error
	Release(string) error
	AwaitDone(context.Context, string) error
	Wait() (int, error)
	Kill() error
	Alive() bool
}

type runtimeCgroupHelperExecutorSeal struct{}

var currentRuntimeCgroupHelperExecutorSeal = &runtimeCgroupHelperExecutorSeal{}

type RuntimeCgroupHelperExecutor struct {
	root             string
	executable       string
	executableDigest string
	nonce            func() ([]byte, error)
	launch           runtimeCgroupHelperLaunch
	readFile         runtimeCgroupHelperReadFile
	writeFile        runtimeCgroupHelperWriteFile
	readStartTime    func(int) (uint64, error)
	clock            func() time.Time
	afterChildExit   func()
	seal             *runtimeCgroupHelperExecutorSeal
}

type runtimeCgroupHelperExecutorTestConfig struct {
	Root             string
	Executable       string
	ExecutableDigest string
	NonceRaw         []byte
	Launch           runtimeCgroupHelperLaunch
	ReadFile         runtimeCgroupHelperReadFile
	WriteFile        runtimeCgroupHelperWriteFile
	Clock            func() time.Time
}

func NewRuntimeCgroupHelperExecutor() (*RuntimeCgroupHelperExecutor, error) {
	executable, digest, err := measureRuntimeCgroupHelperExecutable()
	if err != nil {
		return nil, err
	}
	e := &RuntimeCgroupHelperExecutor{
		root:             runtimeCgroupHelperRoot,
		executable:       executable,
		executableDigest: digest,
		nonce: func() ([]byte, error) {
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return nil, ErrInvalidRuntimeCgroupHelperPlacementAuthority
			}
			return raw, nil
		},
		launch:    launchRuntimeCgroupHelperProcess,
		readFile: os.ReadFile,
		writeFile: func(filename string, data []byte) error {
			f, err := os.OpenFile(filename, os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.Write(data)
			return err
		},
		clock: time.Now,
		seal:  currentRuntimeCgroupHelperExecutorSeal,
	}
	e.readStartTime = procStartTimeReader(e.readFile)
	if !e.valid() {
		return nil, ErrInvalidRuntimeCgroupHelperPlacementAuthority
	}
	return e, nil
}

func newRuntimeCgroupHelperExecutorForTest(cfg runtimeCgroupHelperExecutorTestConfig) *RuntimeCgroupHelperExecutor {
	nonceRaw := append([]byte(nil), cfg.NonceRaw...)
	e := &RuntimeCgroupHelperExecutor{
		root:             cfg.Root,
		executable:       cfg.Executable,
		executableDigest: cfg.ExecutableDigest,
		nonce: func() ([]byte, error) { return append([]byte(nil), nonceRaw...), nil },
		launch:    cfg.Launch,
		readFile:  cfg.ReadFile,
		writeFile: cfg.WriteFile,
		clock:     cfg.Clock,
		seal:      currentRuntimeCgroupHelperExecutorSeal,
	}
	e.readStartTime = procStartTimeReader(e.readFile)
	return e
}

func (e *RuntimeCgroupHelperExecutor) valid() bool {
	if e == nil || e.seal != currentRuntimeCgroupHelperExecutorSeal || e.nonce == nil || e.launch == nil || e.readFile == nil || e.writeFile == nil || e.readStartTime == nil || e.clock == nil {
		return false
	}
	cleanRoot := filepath.Clean(e.root)
	if !filepath.IsAbs(cleanRoot) || cleanRoot == string(filepath.Separator) || cleanRoot != e.root {
		return false
	}
	cleanExecutable := filepath.Clean(e.executable)
	return filepath.IsAbs(cleanExecutable) && cleanExecutable == e.executable && validV29SHA256(e.executableDigest)
}

func measureRuntimeCgroupHelperExecutable() (string, string, error) {
	raw, err := os.Executable()
	if err != nil { return "", "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil { return "", "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	absolute, err := filepath.Abs(resolved)
	if err != nil { return "", "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	absolute = filepath.Clean(absolute)
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() { return "", "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	f, err := os.Open(absolute)
	if err != nil { return "", "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil { return "", "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	return absolute, hex.EncodeToString(h.Sum(nil)), nil
}

type execRuntimeCgroupHelperChild struct {
	cmd *exec.Cmd
	release *os.File
	ackFile *os.File
	ack *bufio.Reader
	mu sync.Mutex
	waited bool
}

func launchRuntimeCgroupHelperProcess(ctx context.Context, executable, nonce string) (runtimeCgroupHelperChild, error) {
	if err := ctx.Err(); err != nil { return nil, err }
	if executable == "" || !filepath.IsAbs(executable) || !canonicalV29NonceHex(nonce) { return nil, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	releaseR, releaseW, err := os.Pipe()
	if err != nil { return nil, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	ackR, ackW, err := os.Pipe()
	if err != nil { releaseR.Close(); releaseW.Close(); return nil, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	cmd := exec.Command(executable)
	cmd.Env = runtimeCgroupHelperEnvironment(nonce)
	cmd.ExtraFiles = []*os.File{releaseR, ackW}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		releaseR.Close(); releaseW.Close(); ackR.Close(); ackW.Close()
		return nil, ErrInvalidRuntimeCgroupHelperPlacementAuthority
	}
	_ = releaseR.Close(); _ = ackW.Close()
	return &execRuntimeCgroupHelperChild{cmd: cmd, release: releaseW, ackFile: ackR, ack: bufio.NewReaderSize(ackR, runtimeCgroupHelperProtocolLimit+1)}, nil
}

func runtimeCgroupHelperEnvironment(nonce string) []string {
	out := make([]string, 0, len(os.Environ())+2)
	modePrefix := runtimeCgroupHelperModeEnv + "="
	noncePrefix := runtimeCgroupHelperNonceEnv + "="
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, modePrefix) || strings.HasPrefix(entry, noncePrefix) { continue }
		out = append(out, entry)
	}
	return append(out, modePrefix+runtimeCgroupHelperModeParkExit, noncePrefix+nonce)
}

func (c *execRuntimeCgroupHelperChild) PID() int {
	if c == nil || c.cmd == nil || c.cmd.Process == nil { return 0 }
	return c.cmd.Process.Pid
}
func (c *execRuntimeCgroupHelperChild) AwaitReady(ctx context.Context, nonce string) error { return c.awaitRecord(ctx, "READY "+nonce+"\n", false) }
func (c *execRuntimeCgroupHelperChild) Release(nonce string) error {
	if c == nil || c.release == nil || !canonicalV29NonceHex(nonce) { return ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	_, err := io.WriteString(c.release, "GO "+nonce+"\n")
	closeErr := c.release.Close(); c.release = nil
	if err != nil || closeErr != nil { return ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	return nil
}
func (c *execRuntimeCgroupHelperChild) AwaitDone(ctx context.Context, nonce string) error { return c.awaitRecord(ctx, "DONE "+nonce+"\n", true) }
func (c *execRuntimeCgroupHelperChild) awaitRecord(ctx context.Context, expected string, requireEOF bool) error {
	if c == nil || c.ack == nil { return ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	type result struct{ line []byte; err error }
	ch := make(chan result, 1)
	go func(){ line, err := c.ack.ReadSlice('\n'); ch <- result{append([]byte(nil), line...), err} }()
	select {
	case <-ctx.Done(): return ctx.Err()
	case got := <-ch:
		if got.err != nil || len(got.line) > runtimeCgroupHelperProtocolLimit || string(got.line) != expected { return ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	}
	if !requireEOF { return nil }
	eofCh := make(chan error, 1)
	go func(){ _, err := c.ack.ReadByte(); eofCh <- err }()
	select {
	case <-ctx.Done(): return ctx.Err()
	case err := <-eofCh:
		if !errors.Is(err, io.EOF) { return ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	}
	return nil
}
func (c *execRuntimeCgroupHelperChild) Wait() (int, error) {
	if c == nil || c.cmd == nil { return -1, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	c.mu.Lock(); if c.waited { c.mu.Unlock(); return -1, ErrInvalidRuntimeCgroupHelperPlacementAuthority }; c.waited = true; c.mu.Unlock()
	err := c.cmd.Wait()
	if c.ackFile != nil { _ = c.ackFile.Close(); c.ackFile = nil }
	if c.release != nil { _ = c.release.Close(); c.release = nil }
	code := -1; if c.cmd.ProcessState != nil { code = c.cmd.ProcessState.ExitCode() }
	if err != nil { var exitErr *exec.ExitError; if errors.As(err, &exitErr) { return code, nil }; return code, err }
	return code, nil
}
func (c *execRuntimeCgroupHelperChild) Kill() error {
	if c == nil || c.cmd == nil || c.cmd.Process == nil { return nil }
	if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) { return err }
	return nil
}
func (c *execRuntimeCgroupHelperChild) Alive() bool {
	if c == nil || c.cmd == nil || c.cmd.Process == nil { return false }
	c.mu.Lock(); defer c.mu.Unlock(); return !c.waited && c.cmd.ProcessState == nil
}

func procStartTimeReader(readFile runtimeCgroupHelperReadFile) func(int) (uint64, error) {
	return func(pid int) (uint64, error) {
		if readFile == nil || pid <= 0 { return 0, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
		filename := "/proc/" + strconv.Itoa(pid) + "/stat"
		raw, err := readFile(filename)
		if err != nil { return 0, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
		line := strings.TrimSpace(string(raw))
		closeIdx := strings.LastIndex(line, ")"); openIdx := strings.Index(line, "(")
		if openIdx <= 0 || closeIdx <= openIdx || closeIdx+2 > len(line) { return 0, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
		prefix := strings.TrimSpace(line[:openIdx]); parsedPID, err := strconv.Atoi(prefix)
		if err != nil || parsedPID != pid { return 0, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
		rest := strings.Fields(line[closeIdx+1:]); if len(rest) <= 19 { return 0, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
		start, err := parseCanonicalUint(rest[19], 64); if err != nil || start == 0 { return 0, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
		return start, nil
	}
}

type RuntimeCgroupHelperPlacementSnapshot struct {
	RuntimeDigest string `json:"runtime_digest"`
	BeforeReadbackDigest string `json:"before_readback_digest"`
	AfterReadbackDigest string `json:"after_readback_digest"`
	SandboxID string `json:"sandbox_id"`
	Generation uint64 `json:"generation"`
	RuntimeHostPID uint32 `json:"runtime_host_pid"`
	RuntimeStartTimeTicks uint64 `json:"runtime_starttime_ticks"`
	RuntimeBootID string `json:"runtime_boot_id"`
	CGroupPath string `json:"cgroup_path"`
	CPUQuotaMicros int64 `json:"cpu_quota_micros"`
	CPUPeriodMicros uint64 `json:"cpu_period_micros"`
	MemoryLimitBytes uint64 `json:"memory_limit_bytes"`
	HelperPID int `json:"helper_pid"`
	HelperStartTimeTicks uint64 `json:"helper_starttime_ticks"`
	HelperExecutableSHA256 string `json:"helper_executable_sha256"`
	HelperNonceSHA256 string `json:"helper_nonce_sha256"`
	ReadyAt time.Time `json:"ready_at"`
	PlacedAt time.Time `json:"placed_at"`
	ReleasedAt time.Time `json:"released_at"`
	ExitedAt time.Time `json:"exited_at"`
	ExitCode int `json:"exit_code"`
}

type runtimeCgroupHelperPlacementAuthoritySeal struct{}
var currentRuntimeCgroupHelperPlacementAuthoritySeal = &runtimeCgroupHelperPlacementAuthoritySeal{}

type RuntimeCgroupHelperPlacementAuthority struct {
	before RuntimeCgroupReadbackAuthority
	after RuntimeCgroupReadbackAuthority
	snapshot RuntimeCgroupHelperPlacementSnapshot
	digest string
	seal *runtimeCgroupHelperPlacementAuthoritySeal
}
func (a RuntimeCgroupHelperPlacementAuthority) Valid() bool {
	if a.seal != currentRuntimeCgroupHelperPlacementAuthoritySeal || !a.before.Valid() || !a.after.Valid() || !validV29AuthorityDigest(a.digest) { return false }
	before, ok1 := a.before.Snapshot(); after, ok2 := a.after.Snapshot()
	if !ok1 || !ok2 || !sameV29ImmutableReadback(before, after) || !v29SnapshotMatchesReadbacks(a.snapshot, a.before, a.after) { return false }
	expected, err := deriveV29AuthorityDigest(a.snapshot); return err == nil && expected == a.digest
}
func (a RuntimeCgroupHelperPlacementAuthority) Digest() (string, bool) { if !a.Valid(){ return "", false }; return a.digest, true }
func (a RuntimeCgroupHelperPlacementAuthority) Snapshot() (RuntimeCgroupHelperPlacementSnapshot, bool) { if !a.Valid(){ return RuntimeCgroupHelperPlacementSnapshot{}, false }; return a.snapshot, true }

type runtimeCgroupHelperLifecycle struct {
	PID int
	StartTimeTicks uint64
	ExecutableSHA256 string
	NonceSHA256 string
	ReadyAt time.Time
	PlacedAt time.Time
	ReleasedAt time.Time
	ExitedAt time.Time
	ExitCode int
}

func ValidateRuntimeCgroupHelperPlacementAuthority(
	ctx context.Context,
	controller *realm.Controller,
	realization realm.RealizationAuthority,
	runtimeAuthority RealmResourceRuntimeAuthority,
	epochObserver *RealizationEpochObserver,
	client *Client,
	runtimeObserver *RuntimeRealizationObserver,
	cgroupObserver *RuntimeCgroupReadbackObserver,
	executor *RuntimeCgroupHelperExecutor,
) (RuntimeCgroupHelperPlacementAuthority, error) {
	if err := ctx.Err(); err != nil { return RuntimeCgroupHelperPlacementAuthority{}, err }
	if !runtimeAuthority.Valid() || !executor.valid() { return RuntimeCgroupHelperPlacementAuthority{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	before, err := ValidateRuntimeCgroupReadbackAuthority(ctx, controller, realization, runtimeAuthority, epochObserver, client, runtimeObserver, cgroupObserver)
	if err != nil { return RuntimeCgroupHelperPlacementAuthority{}, err }
	beforeSnapshot, ok := before.Snapshot(); if !ok { return RuntimeCgroupHelperPlacementAuthority{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	lifecycle, err := executor.execute(ctx, beforeSnapshot); if err != nil { return RuntimeCgroupHelperPlacementAuthority{}, err }
	if executor.afterChildExit != nil { executor.afterChildExit() }
	after, err := ValidateRuntimeCgroupReadbackAuthority(ctx, controller, realization, runtimeAuthority, epochObserver, client, runtimeObserver, cgroupObserver)
	if err != nil { return RuntimeCgroupHelperPlacementAuthority{}, err }
	afterSnapshot, ok := after.Snapshot(); if !ok || !sameV29ImmutableReadback(beforeSnapshot, afterSnapshot) { return RuntimeCgroupHelperPlacementAuthority{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	if afterSnapshot.NrThrottled < beforeSnapshot.NrThrottled || afterSnapshot.ThrottledUsec < beforeSnapshot.ThrottledUsec || afterSnapshot.OOMEvents < beforeSnapshot.OOMEvents || afterSnapshot.OOMKillEvents < beforeSnapshot.OOMKillEvents { return RuntimeCgroupHelperPlacementAuthority{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	beforeDigest, ok1 := before.ReadbackDigest(); afterDigest, ok2 := after.ReadbackDigest(); if !ok1 || !ok2 { return RuntimeCgroupHelperPlacementAuthority{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	snapshot := RuntimeCgroupHelperPlacementSnapshot{
		RuntimeDigest: beforeSnapshot.RuntimeDigest, BeforeReadbackDigest: beforeDigest, AfterReadbackDigest: afterDigest,
		SandboxID: beforeSnapshot.SandboxID, Generation: beforeSnapshot.Generation, RuntimeHostPID: beforeSnapshot.HostPID,
		RuntimeStartTimeTicks: beforeSnapshot.StartTimeTicks, RuntimeBootID: beforeSnapshot.BootID, CGroupPath: beforeSnapshot.CGroupPath,
		CPUQuotaMicros: beforeSnapshot.CPUQuotaMicros, CPUPeriodMicros: beforeSnapshot.CPUPeriodMicros, MemoryLimitBytes: beforeSnapshot.MemoryLimitBytes,
		HelperPID: lifecycle.PID, HelperStartTimeTicks: lifecycle.StartTimeTicks, HelperExecutableSHA256: lifecycle.ExecutableSHA256,
		HelperNonceSHA256: lifecycle.NonceSHA256, ReadyAt: lifecycle.ReadyAt, PlacedAt: lifecycle.PlacedAt, ReleasedAt: lifecycle.ReleasedAt,
		ExitedAt: lifecycle.ExitedAt, ExitCode: lifecycle.ExitCode,
	}
	digest, err := deriveV29AuthorityDigest(snapshot); if err != nil { return RuntimeCgroupHelperPlacementAuthority{}, err }
	authority := RuntimeCgroupHelperPlacementAuthority{before: before, after: after, snapshot: snapshot, digest: digest, seal: currentRuntimeCgroupHelperPlacementAuthoritySeal}
	if !authority.Valid() { return RuntimeCgroupHelperPlacementAuthority{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	return authority, nil
}

func (e *RuntimeCgroupHelperExecutor) execute(ctx context.Context, binding RuntimeCgroupReadbackSnapshot) (runtimeCgroupHelperLifecycle, error) {
	if err := ctx.Err(); err != nil { return runtimeCgroupHelperLifecycle{}, err }
	if !e.valid() || binding.CGroupPath == "" || binding.SandboxID == "" { return runtimeCgroupHelperLifecycle{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	nonceRaw, err := e.nonce(); if err != nil || len(nonceRaw) != 32 { return runtimeCgroupHelperLifecycle{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	nonce := hex.EncodeToString(nonceRaw); if !canonicalV29NonceHex(nonce) { return runtimeCgroupHelperLifecycle{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	nonceDigestRaw := sha256.Sum256(nonceRaw); nonceDigest := hex.EncodeToString(nonceDigestRaw[:])
	child, err := e.launch(ctx, e.executable, nonce)
	if err != nil || child == nil { if err != nil { return runtimeCgroupHelperLifecycle{}, err }; return runtimeCgroupHelperLifecycle{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	waited := false
	cleanup := func(){ if waited { return }; if child.Alive(){ _ = child.Kill() }; _, _ = child.Wait(); waited = true }
	fail := func(err error)(runtimeCgroupHelperLifecycle,error){ cleanup(); if ctxErr := ctx.Err(); ctxErr != nil { return runtimeCgroupHelperLifecycle{}, ctxErr }; if err == nil { err = ErrInvalidRuntimeCgroupHelperPlacementAuthority }; return runtimeCgroupHelperLifecycle{}, err }
	if err := child.AwaitReady(ctx, nonce); err != nil { return fail(err) }
	readyAt := e.clock().UTC(); pid := child.PID()
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) || !child.Alive() { return fail(ErrInvalidRuntimeCgroupHelperPlacementAuthority) }
	startBefore, err := e.readStartTime(pid); if err != nil || startBefore == 0 { return fail(ErrInvalidRuntimeCgroupHelperPlacementAuthority) }
	target, err := runtimeCgroupHelperTargetPath(e.root, binding.CGroupPath); if err != nil { return fail(err) }
	procsPath := filepath.Join(target, "cgroup.procs")
	if err := e.writeFile(procsPath, []byte(strconv.Itoa(pid)+"\n")); err != nil { return fail(ErrInvalidRuntimeCgroupHelperPlacementAuthority) }
	procs, err := e.readFile(procsPath); if err != nil || !containsExactCgroupPID(string(procs), uint32(pid)) { return fail(ErrInvalidRuntimeCgroupHelperPlacementAuthority) }
	startAfter, err := e.readStartTime(pid); if err != nil || startAfter != startBefore || !child.Alive() { return fail(ErrInvalidRuntimeCgroupHelperPlacementAuthority) }
	placedAt := e.clock().UTC(); if err := child.Release(nonce); err != nil { return fail(err) }
	releasedAt := e.clock().UTC(); if err := child.AwaitDone(ctx, nonce); err != nil { return fail(err) }
	exitCode, waitErr := child.Wait(); waited = true; exitedAt := e.clock().UTC()
	if waitErr != nil || exitCode != 0 { if ctxErr := ctx.Err(); ctxErr != nil { return runtimeCgroupHelperLifecycle{}, ctxErr }; return runtimeCgroupHelperLifecycle{}, ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	return runtimeCgroupHelperLifecycle{PID: pid, StartTimeTicks: startBefore, ExecutableSHA256: e.executableDigest, NonceSHA256: nonceDigest, ReadyAt: readyAt, PlacedAt: placedAt, ReleasedAt: releasedAt, ExitedAt: exitedAt, ExitCode: exitCode}, nil
}

func runtimeCgroupHelperTargetPath(root, cgroupPath string) (string, error) {
	if root == "" || cgroupPath == "" || !filepath.IsAbs(root) || !path.IsAbs(cgroupPath) || cgroupPath == "/" || path.Clean(cgroupPath) != cgroupPath { return "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	cleanRoot := filepath.Clean(root); if cleanRoot != root || cleanRoot == string(filepath.Separator) { return "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	rel := strings.TrimPrefix(cgroupPath, "/"); if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") { return "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	target := filepath.Clean(filepath.Join(cleanRoot, filepath.FromSlash(rel))); if target == cleanRoot || !strings.HasPrefix(target, cleanRoot+string(filepath.Separator)) { return "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	return target, nil
}
func sameV29ImmutableReadback(a, b RuntimeCgroupReadbackSnapshot) bool {
	return a.RuntimeDigest == b.RuntimeDigest && a.SandboxID == b.SandboxID && a.Generation == b.Generation && a.HostPID == b.HostPID && a.StartTimeTicks == b.StartTimeTicks && a.BootID == b.BootID && a.CGroupPath == b.CGroupPath && a.CPUQuotaMicros == b.CPUQuotaMicros && a.CPUPeriodMicros == b.CPUPeriodMicros && a.MemoryLimitBytes == b.MemoryLimitBytes
}
func v29SnapshotMatchesReadbacks(snapshot RuntimeCgroupHelperPlacementSnapshot, before, after RuntimeCgroupReadbackAuthority) bool {
	beforeSnapshot, ok1 := before.Snapshot(); afterSnapshot, ok2 := after.Snapshot(); beforeDigest, ok3 := before.ReadbackDigest(); afterDigest, ok4 := after.ReadbackDigest()
	if !ok1 || !ok2 || !ok3 || !ok4 || !sameV29ImmutableReadback(beforeSnapshot, afterSnapshot) { return false }
	return snapshot.RuntimeDigest == beforeSnapshot.RuntimeDigest && snapshot.BeforeReadbackDigest == beforeDigest && snapshot.AfterReadbackDigest == afterDigest && snapshot.SandboxID == beforeSnapshot.SandboxID && snapshot.Generation == beforeSnapshot.Generation && snapshot.RuntimeHostPID == beforeSnapshot.HostPID && snapshot.RuntimeStartTimeTicks == beforeSnapshot.StartTimeTicks && snapshot.RuntimeBootID == beforeSnapshot.BootID && snapshot.CGroupPath == beforeSnapshot.CGroupPath && snapshot.CPUQuotaMicros == beforeSnapshot.CPUQuotaMicros && snapshot.CPUPeriodMicros == beforeSnapshot.CPUPeriodMicros && snapshot.MemoryLimitBytes == beforeSnapshot.MemoryLimitBytes && validV29PlacementSnapshot(snapshot)
}
func validV29PlacementSnapshot(s RuntimeCgroupHelperPlacementSnapshot) bool {
	if !strings.HasPrefix(s.RuntimeDigest, "runtime-realization-v27:") || !validRuntimeCgroupReadbackDigest(s.BeforeReadbackDigest) || !validRuntimeCgroupReadbackDigest(s.AfterReadbackDigest) { return false }
	if s.SandboxID == "" || strings.TrimSpace(s.SandboxID) != s.SandboxID || s.Generation == 0 || s.RuntimeHostPID == 0 || s.RuntimeStartTimeTicks == 0 || !canonicalLowerUUID(s.RuntimeBootID) { return false }
	if s.CGroupPath == "" || path.Clean(s.CGroupPath) != s.CGroupPath || s.CGroupPath == "/" || s.CPUQuotaMicros <= 0 || s.CPUPeriodMicros == 0 || s.MemoryLimitBytes == 0 { return false }
	if s.HelperPID <= 0 || s.HelperStartTimeTicks == 0 || !validV29SHA256(s.HelperExecutableSHA256) || !validV29SHA256(s.HelperNonceSHA256) || s.ExitCode != 0 { return false }
	if s.ReadyAt.IsZero() || s.PlacedAt.IsZero() || s.ReleasedAt.IsZero() || s.ExitedAt.IsZero() { return false }
	return !s.PlacedAt.Before(s.ReadyAt) && !s.ReleasedAt.Before(s.PlacedAt) && !s.ExitedAt.Before(s.ReleasedAt)
}
func deriveV29AuthorityDigest(snapshot RuntimeCgroupHelperPlacementSnapshot) (string, error) {
	if !validV29PlacementSnapshot(snapshot) { return "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	raw, err := json.Marshal(snapshot); if err != nil { return "", ErrInvalidRuntimeCgroupHelperPlacementAuthority }
	h := sha256.New(); _, _ = h.Write([]byte(runtimeCgroupHelperDigestDomain)); _, _ = h.Write(raw)
	return runtimeCgroupHelperDigestPrefix + hex.EncodeToString(h.Sum(nil)), nil
}
func validV29AuthorityDigest(raw string) bool { if !strings.HasPrefix(raw, runtimeCgroupHelperDigestPrefix){ return false }; return validV29SHA256(strings.TrimPrefix(raw, runtimeCgroupHelperDigestPrefix)) }
func validV29SHA256(raw string) bool { if len(raw) != 64 || raw != strings.ToLower(raw){ return false }; decoded, err := hex.DecodeString(raw); return err == nil && len(decoded) == sha256.Size }
