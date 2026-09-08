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
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidRuntimeCPUThrottleInterventionAuthority = errors.New("cube: invalid runtime CPU throttle intervention authority")
var ErrRuntimeCPUInterventionUnavailable = errors.New("cube: runtime CPU intervention unavailable")

const runtimeCPUInterventionRoot = "/sys/fs/cgroup"
const runtimeCPUInterventionDigestDomain = "nolane.runtime-cpu-throttle-intervention.v30\x00"
const runtimeCPUInterventionDigestPrefix = "runtime-cpu-throttle-intervention-v30:"
const runtimeCPUInterventionMaxControlMicros = uint64(1_000_000)

type runtimeCPUInterventionLaunch func(context.Context, string, string, uint64) (runtimeCPUInterventionChild, error)
type runtimeCPUInterventionSleep func(context.Context, time.Duration) error

type runtimeCPUInterventionChild interface {
	PID() int
	AwaitReady(context.Context, string) error
	Start(string) error
	AwaitBurnDone(context.Context, string) error
	Exit(string) error
	AwaitDone(context.Context, string) error
	Wait() (int, error)
	Kill() error
	Alive() bool
}

type runtimeCPUInterventionExecutorSeal struct{}

var currentRuntimeCPUInterventionExecutorSeal = &runtimeCPUInterventionExecutorSeal{}

type RuntimeCPUInterventionExecutor struct {
	root                       string
	executable                 string
	executableDigest           string
	nonce                      func() ([]byte, error)
	launch                     runtimeCPUInterventionLaunch
	readFile                   runtimeCgroupHelperReadFile
	writeFile                  runtimeCgroupHelperWriteFile
	readStartTime              func(int) (uint64, error)
	readHelperExecutableDigest func(int) (string, error)
	readSchedstat              func(int) (uint64, error)
	sleep                      runtimeCPUInterventionSleep
	clock                      func() time.Time
	seal                       *runtimeCPUInterventionExecutorSeal
}

type runtimeCPUInterventionExecutorTestConfig struct {
	Root             string
	Executable       string
	ExecutableDigest string
	NonceRaw         []byte
	Launch           runtimeCPUInterventionLaunch
	ReadFile         runtimeCgroupHelperReadFile
	WriteFile        runtimeCgroupHelperWriteFile
	Sleep            runtimeCPUInterventionSleep
	Clock            func() time.Time
}

func NewRuntimeCPUInterventionExecutor() (*RuntimeCPUInterventionExecutor, error) {
	executable, digest, err := measureRuntimeCgroupHelperExecutable()
	if err != nil {
		return nil, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	e := &RuntimeCPUInterventionExecutor{
		root:             runtimeCPUInterventionRoot,
		executable:       executable,
		executableDigest: digest,
		nonce: func() ([]byte, error) {
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return nil, ErrInvalidRuntimeCPUThrottleInterventionAuthority
			}
			return raw, nil
		},
		launch:                     launchRuntimeCPUInterventionProcess,
		readFile:                   os.ReadFile,
		readHelperExecutableDigest: measureRuntimeCgroupHelperProcessExecutable,
		writeFile: func(filename string, data []byte) error {
			f, err := os.OpenFile(filename, os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			defer f.Close()
			n, err := f.Write(data)
			if err != nil || n != len(data) {
				return ErrInvalidRuntimeCPUThrottleInterventionAuthority
			}
			return nil
		},
		sleep: sleepRuntimeCPUIntervention,
		clock: time.Now,
		seal:  currentRuntimeCPUInterventionExecutorSeal,
	}
	e.readStartTime = procStartTimeReader(e.readFile)
	e.readSchedstat = runtimeCPUSchedstatReader(e.readFile)
	if !e.valid() {
		return nil, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return e, nil
}

func newRuntimeCPUInterventionExecutorForTest(cfg runtimeCPUInterventionExecutorTestConfig) *RuntimeCPUInterventionExecutor {
	nonceRaw := append([]byte(nil), cfg.NonceRaw...)
	e := &RuntimeCPUInterventionExecutor{
		root:             cfg.Root,
		executable:       cfg.Executable,
		executableDigest: cfg.ExecutableDigest,
		nonce: func() ([]byte, error) {
			return append([]byte(nil), nonceRaw...), nil
		},
		launch:    cfg.Launch,
		readFile:  cfg.ReadFile,
		writeFile: cfg.WriteFile,
		sleep:     cfg.Sleep,
		clock:     cfg.Clock,
		seal:      currentRuntimeCPUInterventionExecutorSeal,
	}
	e.readStartTime = procStartTimeReader(e.readFile)
	e.readHelperExecutableDigest = func(int) (string, error) { return e.executableDigest, nil }
	e.readSchedstat = runtimeCPUSchedstatReader(e.readFile)
	return e
}

func (e *RuntimeCPUInterventionExecutor) valid() bool {
	if e == nil || e.seal != currentRuntimeCPUInterventionExecutorSeal || e.nonce == nil || e.launch == nil || e.readFile == nil || e.writeFile == nil || e.readStartTime == nil || e.readHelperExecutableDigest == nil || e.readSchedstat == nil || e.sleep == nil || e.clock == nil {
		return false
	}
	cleanRoot := filepath.Clean(e.root)
	if !filepath.IsAbs(cleanRoot) || cleanRoot == string(filepath.Separator) || cleanRoot != e.root {
		return false
	}
	cleanExecutable := filepath.Clean(e.executable)
	return filepath.IsAbs(cleanExecutable) && cleanExecutable == e.executable && validV29SHA256(e.executableDigest)
}

type runtimeCPUInterventionParameters struct {
	controlDuration time.Duration
	burnDuration    time.Duration
	burnMicros      uint64
	minimumCPUNS    uint64
}

func runtimeCPUInterventionParametersFor(snapshot RuntimeCgroupReadbackSnapshot) (runtimeCPUInterventionParameters, error) {
	if snapshot.CPUQuotaMicros <= 0 || snapshot.CPUPeriodMicros == 0 {
		return runtimeCPUInterventionParameters{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	quota := uint64(snapshot.CPUQuotaMicros)
	period := snapshot.CPUPeriodMicros
	if quota >= period {
		return runtimeCPUInterventionParameters{}, ErrRuntimeCPUInterventionUnavailable
	}
	if period > runtimeCPUInterventionMaxControlMicros/2 || period > runtimeCPUInterventionMaxBurnMicros/4 {
		return runtimeCPUInterventionParameters{}, ErrRuntimeCPUInterventionUnavailable
	}
	controlMicros := period * 2
	burnMicros := period * 4
	if controlMicros == 0 || burnMicros == 0 || controlMicros > runtimeCPUInterventionMaxControlMicros || burnMicros > runtimeCPUInterventionMaxBurnMicros {
		return runtimeCPUInterventionParameters{}, ErrRuntimeCPUInterventionUnavailable
	}
	return runtimeCPUInterventionParameters{
		controlDuration: time.Duration(controlMicros) * time.Microsecond,
		burnDuration:    time.Duration(burnMicros) * time.Microsecond,
		burnMicros:      burnMicros,
		minimumCPUNS:    quota * 1000,
	}, nil
}

func sleepRuntimeCPUIntervention(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if duration <= 0 || duration > time.Duration(runtimeCPUInterventionMaxControlMicros)*time.Microsecond {
		return ErrRuntimeCPUInterventionUnavailable
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runtimeCPUSchedstatReader(readFile runtimeCgroupHelperReadFile) func(int) (uint64, error) {
	return func(pid int) (uint64, error) {
		if readFile == nil || pid <= 0 {
			return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
		}
		raw, err := readFile("/proc/" + strconv.Itoa(pid) + "/schedstat")
		if err != nil {
			return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
		}
		return parseRuntimeCPUSchedstat(raw)
	}
}

func parseRuntimeCPUSchedstat(raw []byte) (uint64, error) {
	if len(raw) == 0 || len(raw) > 256 {
		return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	text := string(raw)
	if !strings.HasSuffix(text, "\n") || strings.Count(text, "\n") != 1 {
		return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	line := strings.TrimSuffix(text, "\n")
	fields := strings.Fields(line)
	if len(fields) != 3 || line != strings.Join(fields, " ") {
		return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	runtimeNS, ok := parseRuntimeCgroupCanonicalUint(fields[0], false)
	if !ok {
		return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if _, ok := parseRuntimeCgroupCanonicalUint(fields[1], true); !ok {
		return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if _, ok := parseRuntimeCgroupCanonicalUint(fields[2], true); !ok {
		return 0, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return runtimeNS, nil
}

type execRuntimeCPUInterventionChild struct {
	base *execRuntimeCgroupHelperChild
}

func launchRuntimeCPUInterventionProcess(ctx context.Context, executable, nonce string, burnMicros uint64) (runtimeCPUInterventionChild, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if executable == "" || !filepath.IsAbs(executable) || !canonicalV29NonceHex(nonce) || burnMicros == 0 || burnMicros > runtimeCPUInterventionMaxBurnMicros {
		return nil, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	releaseR, releaseW, err := os.Pipe()
	if err != nil {
		return nil, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	ackR, ackW, err := os.Pipe()
	if err != nil {
		_ = releaseR.Close()
		_ = releaseW.Close()
		return nil, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	cmd := exec.Command(executable)
	cmd.Env = runtimeCPUInterventionEnvironment(nonce, burnMicros)
	cmd.ExtraFiles = []*os.File{releaseR, ackW}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = releaseR.Close()
		_ = releaseW.Close()
		_ = ackR.Close()
		_ = ackW.Close()
		return nil, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	_ = releaseR.Close()
	_ = ackW.Close()
	base := &execRuntimeCgroupHelperChild{
		cmd:     cmd,
		release: releaseW,
		ackFile: ackR,
		ack:     bufio.NewReaderSize(ackR, runtimeCgroupHelperProtocolLimit+1),
	}
	return &execRuntimeCPUInterventionChild{base: base}, nil
}

func runtimeCPUInterventionEnvironment(nonce string, burnMicros uint64) []string {
	out := make([]string, 0, len(os.Environ())+3)
	modePrefix := runtimeCgroupHelperModeEnv + "="
	noncePrefix := runtimeCgroupHelperNonceEnv + "="
	burnPrefix := runtimeCPUInterventionBurnMicrosEnv + "="
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, modePrefix) || strings.HasPrefix(entry, noncePrefix) || strings.HasPrefix(entry, burnPrefix) {
			continue
		}
		out = append(out, entry)
	}
	return append(out,
		modePrefix+runtimeCPUInterventionMode,
		noncePrefix+nonce,
		burnPrefix+strconv.FormatUint(burnMicros, 10),
	)
}

func (c *execRuntimeCPUInterventionChild) PID() int {
	if c == nil || c.base == nil {
		return 0
	}
	return c.base.PID()
}

func (c *execRuntimeCPUInterventionChild) AwaitReady(ctx context.Context, nonce string) error {
	if c == nil || c.base == nil {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if err := c.base.awaitRecord(ctx, helperProtocolRecord("READY", nonce), false); err != nil {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return nil
}

func (c *execRuntimeCPUInterventionChild) Start(nonce string) error {
	return c.send(helperProtocolRecord("START", nonce), false)
}

func (c *execRuntimeCPUInterventionChild) AwaitBurnDone(ctx context.Context, nonce string) error {
	if c == nil || c.base == nil {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if err := c.base.awaitRecord(ctx, helperProtocolRecord("BURN_DONE", nonce), false); err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return nil
}

func (c *execRuntimeCPUInterventionChild) Exit(nonce string) error {
	return c.send(helperProtocolRecord("EXIT", nonce), true)
}

func (c *execRuntimeCPUInterventionChild) AwaitDone(ctx context.Context, nonce string) error {
	if c == nil || c.base == nil {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if err := c.base.awaitRecord(ctx, helperProtocolRecord("DONE", nonce), true); err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return nil
}

func (c *execRuntimeCPUInterventionChild) send(record string, closeAfter bool) error {
	if c == nil || c.base == nil || c.base.release == nil || record == "" || len(record) > runtimeCgroupHelperProtocolLimit {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	n, err := io.WriteString(c.base.release, record)
	if err != nil || n != len(record) {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if closeAfter {
		if err := c.base.release.Close(); err != nil {
			return ErrInvalidRuntimeCPUThrottleInterventionAuthority
		}
		c.base.release = nil
	}
	return nil
}

func (c *execRuntimeCPUInterventionChild) Wait() (int, error) {
	if c == nil || c.base == nil {
		return -1, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return c.base.Wait()
}

func (c *execRuntimeCPUInterventionChild) Kill() error {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.Kill()
}

func (c *execRuntimeCPUInterventionChild) Alive() bool {
	return c != nil && c.base != nil && c.base.Alive()
}

func waitRuntimeCPUInterventionChild(ctx context.Context, child runtimeCPUInterventionChild) (int, error) {
	if child == nil {
		return -1, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	type result struct {
		code int
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		code, err := child.Wait()
		ch <- result{code: code, err: err}
	}()
	select {
	case got := <-ch:
		if err := ctx.Err(); err != nil {
			return got.code, err
		}
		return got.code, got.err
	case <-ctx.Done():
		_ = child.Kill()
		got := <-ch
		return got.code, ctx.Err()
	}
}

type RuntimeCPUThrottleInterventionSnapshot struct {
	RuntimeDigest               string    `json:"runtime_digest"`
	SandboxID                   string    `json:"sandbox_id"`
	Generation                  uint64    `json:"generation"`
	RuntimeHostPID              uint32    `json:"runtime_host_pid"`
	RuntimeStartTimeTicks       uint64    `json:"runtime_starttime_ticks"`
	RuntimeBootID               string    `json:"runtime_boot_id"`
	CGroupPath                  string    `json:"cgroup_path"`
	CPUQuotaMicros              int64     `json:"cpu_quota_micros"`
	CPUPeriodMicros             uint64    `json:"cpu_period_micros"`
	MemoryLimitBytes            uint64    `json:"memory_limit_bytes"`
	HelperPID                   int       `json:"helper_pid"`
	HelperStartTimeTicks        uint64    `json:"helper_starttime_ticks"`
	HelperExecutableSHA256      string    `json:"helper_executable_sha256"`
	HelperNonceSHA256           string    `json:"helper_nonce_sha256"`
	ControlBeforeReadbackDigest string    `json:"control_before_readback_digest"`
	ControlAfterReadbackDigest  string    `json:"control_after_readback_digest"`
	PressureAfterReadbackDigest string    `json:"pressure_after_readback_digest"`
	FinalReadbackDigest         string    `json:"final_readback_digest"`
	ControlBeforeNrThrottled    uint64    `json:"control_before_nr_throttled"`
	ControlAfterNrThrottled     uint64    `json:"control_after_nr_throttled"`
	PressureAfterNrThrottled    uint64    `json:"pressure_after_nr_throttled"`
	ControlBeforeThrottledUsec  uint64    `json:"control_before_throttled_usec"`
	ControlAfterThrottledUsec   uint64    `json:"control_after_throttled_usec"`
	PressureAfterThrottledUsec  uint64    `json:"pressure_after_throttled_usec"`
	HelperSchedstatBeforeNS     uint64    `json:"helper_schedstat_before_ns"`
	HelperSchedstatAfterNS      uint64    `json:"helper_schedstat_after_ns"`
	ReadyAt                     time.Time `json:"ready_at"`
	PlacedAt                    time.Time `json:"placed_at"`
	ControlBeforeAt             time.Time `json:"control_before_at"`
	ControlAfterAt              time.Time `json:"control_after_at"`
	StartedAt                   time.Time `json:"started_at"`
	BurnDoneAt                  time.Time `json:"burn_done_at"`
	PressureAfterAt             time.Time `json:"pressure_after_at"`
	ExitRequestedAt             time.Time `json:"exit_requested_at"`
	DoneAt                      time.Time `json:"done_at"`
	ExitedAt                    time.Time `json:"exited_at"`
	ExitCode                    int       `json:"exit_code"`
}

type runtimeCPUThrottleInterventionAuthoritySeal struct{}

var currentRuntimeCPUThrottleInterventionAuthoritySeal = &runtimeCPUThrottleInterventionAuthoritySeal{}

type RuntimeCPUThrottleInterventionAuthority struct {
	controlBefore RuntimeCgroupReadbackAuthority
	controlAfter  RuntimeCgroupReadbackAuthority
	pressureAfter RuntimeCgroupReadbackAuthority
	final         RuntimeCgroupReadbackAuthority
	snapshot      RuntimeCPUThrottleInterventionSnapshot
	digest        string
	seal          *runtimeCPUThrottleInterventionAuthoritySeal
}

func (a RuntimeCPUThrottleInterventionAuthority) Valid() bool {
	if a.seal != currentRuntimeCPUThrottleInterventionAuthoritySeal || !validV30AuthorityDigest(a.digest) {
		return false
	}
	if !a.controlBefore.Valid() || !a.controlAfter.Valid() || !a.pressureAfter.Valid() || !a.final.Valid() {
		return false
	}
	if !v30SnapshotMatchesReadbacks(a.snapshot, a.controlBefore, a.controlAfter, a.pressureAfter, a.final) {
		return false
	}
	expected, err := deriveV30AuthorityDigest(a.snapshot)
	return err == nil && expected == a.digest
}

func (a RuntimeCPUThrottleInterventionAuthority) Digest() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.digest, true
}

func (a RuntimeCPUThrottleInterventionAuthority) Snapshot() (RuntimeCPUThrottleInterventionSnapshot, bool) {
	if !a.Valid() {
		return RuntimeCPUThrottleInterventionSnapshot{}, false
	}
	return a.snapshot, true
}

type runtimeCPUInterventionResult struct {
	controlBefore    RuntimeCgroupReadbackAuthority
	controlAfter     RuntimeCgroupReadbackAuthority
	pressureAfter    RuntimeCgroupReadbackAuthority
	final            RuntimeCgroupReadbackAuthority
	pid              int
	startTimeTicks   uint64
	executableSHA256 string
	nonceSHA256      string
	schedBeforeNS    uint64
	schedAfterNS     uint64
	readyAt          time.Time
	placedAt         time.Time
	controlBeforeAt  time.Time
	controlAfterAt   time.Time
	startedAt        time.Time
	burnDoneAt       time.Time
	pressureAfterAt  time.Time
	exitRequestedAt  time.Time
	doneAt           time.Time
	exitedAt         time.Time
	exitCode         int
}

type runtimeCPUReadback func(context.Context) (RuntimeCgroupReadbackAuthority, error)

func ValidateRuntimeCPUThrottleInterventionAuthority(
	ctx context.Context,
	controller *realm.Controller,
	realization realm.RealizationAuthority,
	runtimeAuthority RealmResourceRuntimeAuthority,
	epochObserver *RealizationEpochObserver,
	client *Client,
	runtimeObserver *RuntimeRealizationObserver,
	cgroupObserver *RuntimeCgroupReadbackObserver,
	executor *RuntimeCPUInterventionExecutor,
) (RuntimeCPUThrottleInterventionAuthority, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeCPUThrottleInterventionAuthority{}, err
	}
	if !runtimeAuthority.Valid() || executor == nil || !executor.valid() {
		return RuntimeCPUThrottleInterventionAuthority{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	observe := func(callCtx context.Context) (RuntimeCgroupReadbackAuthority, error) {
		return ValidateRuntimeCgroupReadbackAuthority(
			callCtx, controller, realization, runtimeAuthority, epochObserver, client, runtimeObserver, cgroupObserver,
		)
	}
	basis, err := observe(ctx)
	if err != nil {
		return RuntimeCPUThrottleInterventionAuthority{}, err
	}
	basisSnapshot, ok := basis.Snapshot()
	if !ok {
		return RuntimeCPUThrottleInterventionAuthority{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if _, err := runtimeCPUInterventionParametersFor(basisSnapshot); err != nil {
		return RuntimeCPUThrottleInterventionAuthority{}, err
	}
	result, err := executor.execute(ctx, basisSnapshot, observe)
	if err != nil {
		return RuntimeCPUThrottleInterventionAuthority{}, err
	}

	controlBeforeSnapshot, ok1 := result.controlBefore.Snapshot()
	controlAfterSnapshot, ok2 := result.controlAfter.Snapshot()
	pressureAfterSnapshot, ok3 := result.pressureAfter.Snapshot()
	finalSnapshot, ok4 := result.final.Snapshot()
	controlBeforeDigest, ok5 := result.controlBefore.ReadbackDigest()
	controlAfterDigest, ok6 := result.controlAfter.ReadbackDigest()
	pressureAfterDigest, ok7 := result.pressureAfter.ReadbackDigest()
	finalDigest, ok8 := result.final.ReadbackDigest()
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 {
		return RuntimeCPUThrottleInterventionAuthority{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	if !sameV29ImmutableReadback(basisSnapshot, controlBeforeSnapshot) || !sameV29ImmutableReadback(controlBeforeSnapshot, controlAfterSnapshot) || !sameV29ImmutableReadback(controlAfterSnapshot, pressureAfterSnapshot) || !sameV29ImmutableReadback(pressureAfterSnapshot, finalSnapshot) {
		return RuntimeCPUThrottleInterventionAuthority{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}

	snapshot := RuntimeCPUThrottleInterventionSnapshot{
		RuntimeDigest:                      basisSnapshot.RuntimeDigest,
		SandboxID:                          basisSnapshot.SandboxID,
		Generation:                         basisSnapshot.Generation,
		RuntimeHostPID:                     basisSnapshot.HostPID,
		RuntimeStartTimeTicks:              basisSnapshot.StartTimeTicks,
		RuntimeBootID:                      basisSnapshot.BootID,
		CGroupPath:                         basisSnapshot.CGroupPath,
		CPUQuotaMicros:                     basisSnapshot.CPUQuotaMicros,
		CPUPeriodMicros:                    basisSnapshot.CPUPeriodMicros,
		MemoryLimitBytes:                   basisSnapshot.MemoryLimitBytes,
		HelperPID:                          result.pid,
		HelperStartTimeTicks:               result.startTimeTicks,
		HelperExecutableSHA256:             result.executableSHA256,
		HelperNonceSHA256:                  result.nonceSHA256,
		ControlBeforeReadbackDigest:        controlBeforeDigest,
		ControlAfterReadbackDigest:         controlAfterDigest,
		PressureAfterReadbackDigest:        pressureAfterDigest,
		FinalReadbackDigest:                finalDigest,
		ControlBeforeNrThrottled:           controlBeforeSnapshot.NrThrottled,
		ControlAfterNrThrottled:            controlAfterSnapshot.NrThrottled,
		PressureAfterNrThrottled:           pressureAfterSnapshot.NrThrottled,
		ControlBeforeThrottledUsec:         controlBeforeSnapshot.ThrottledUsec,
		ControlAfterThrottledUsec:          controlAfterSnapshot.ThrottledUsec,
		PressureAfterThrottledUsec:         pressureAfterSnapshot.ThrottledUsec,
		HelperSchedstatBeforeNS:            result.schedBeforeNS,
		HelperSchedstatAfterNS:             result.schedAfterNS,
		ReadyAt:                            result.readyAt,
		PlacedAt:                           result.placedAt,
		ControlBeforeAt:                    result.controlBeforeAt,
		ControlAfterAt:                     result.controlAfterAt,
		StartedAt:                          result.startedAt,
		BurnDoneAt:                         result.burnDoneAt,
		PressureAfterAt:                    result.pressureAfterAt,
		ExitRequestedAt:                    result.exitRequestedAt,
		DoneAt:                             result.doneAt,
		ExitedAt:                           result.exitedAt,
		ExitCode:                           result.exitCode,
	}
	digest, err := deriveV30AuthorityDigest(snapshot)
	if err != nil {
		return RuntimeCPUThrottleInterventionAuthority{}, err
	}
	authority := RuntimeCPUThrottleInterventionAuthority{
		controlBefore: result.controlBefore,
		controlAfter:  result.controlAfter,
		pressureAfter: result.pressureAfter,
		final:         result.final,
		snapshot:      snapshot,
		digest:        digest,
		seal:          currentRuntimeCPUThrottleInterventionAuthoritySeal,
	}
	if !authority.Valid() {
		return RuntimeCPUThrottleInterventionAuthority{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return authority, nil
}

func (e *RuntimeCPUInterventionExecutor) execute(
	ctx context.Context,
	basis RuntimeCgroupReadbackSnapshot,
	observe runtimeCPUReadback,
) (runtimeCPUInterventionResult, error) {
	if err := ctx.Err(); err != nil {
		return runtimeCPUInterventionResult{}, err
	}
	if !e.valid() || observe == nil || basis.CGroupPath == "" || basis.SandboxID == "" {
		return runtimeCPUInterventionResult{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	params, err := runtimeCPUInterventionParametersFor(basis)
	if err != nil {
		return runtimeCPUInterventionResult{}, err
	}
	nonceRaw, err := e.nonce()
	if err != nil || len(nonceRaw) != 32 {
		return runtimeCPUInterventionResult{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	nonce := hex.EncodeToString(nonceRaw)
	if !canonicalV29NonceHex(nonce) {
		return runtimeCPUInterventionResult{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	nonceDigestRaw := sha256.Sum256(nonceRaw)
	nonceDigest := hex.EncodeToString(nonceDigestRaw[:])
	child, err := e.launch(ctx, e.executable, nonce, params.burnMicros)
	if err != nil || child == nil {
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return runtimeCPUInterventionResult{}, ctxErr
			}
		}
		return runtimeCPUInterventionResult{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	waited := false
	cleanup := func() {
		if waited {
			return
		}
		if child.Alive() {
			_ = child.Kill()
		}
		_, _ = child.Wait()
		waited = true
	}
	fail := func(cause error) (runtimeCPUInterventionResult, error) {
		cleanup()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return runtimeCPUInterventionResult{}, ctxErr
		}
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
			return runtimeCPUInterventionResult{}, cause
		}
		if errors.Is(cause, ErrRuntimeCPUInterventionUnavailable) {
			return runtimeCPUInterventionResult{}, cause
		}
		return runtimeCPUInterventionResult{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}

	if err := child.AwaitReady(ctx, nonce); err != nil {
		return fail(err)
	}
	readyAt := e.clock().UTC()
	pid := child.PID()
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) || !child.Alive() {
		return fail(ErrInvalidRuntimeCPUThrottleInterventionAuthority)
	}
	startBefore, err := e.readStartTime(pid)
	if err != nil || startBefore == 0 {
		return fail(err)
	}
	executableDigest, err := e.readHelperExecutableDigest(pid)
	if err != nil || executableDigest != e.executableDigest || !validV29SHA256(executableDigest) {
		return fail(err)
	}
	target, err := runtimeCgroupHelperTargetPath(e.root, basis.CGroupPath)
	if err != nil {
		return fail(err)
	}
	procsPath := filepath.Join(target, "cgroup.procs")
	if err := e.writeFile(procsPath, []byte(strconv.Itoa(pid)+"\n")); err != nil {
		return fail(err)
	}
	if err := e.requireHelperIdentity(procsPath, pid, startBefore, executableDigest, child); err != nil {
		return fail(err)
	}
	placedAt := e.clock().UTC()

	controlBefore, err := observe(ctx)
	if err != nil {
		return fail(err)
	}
	controlBeforeSnapshot, ok := controlBefore.Snapshot()
	if !ok || !sameV29ImmutableReadback(basis, controlBeforeSnapshot) {
		return fail(ErrInvalidRuntimeCPUThrottleInterventionAuthority)
	}
	controlBeforeAt := e.clock().UTC()
	if err := e.sleep(ctx, params.controlDuration); err != nil {
		return fail(err)
	}
	if err := e.requireHelperIdentity(procsPath, pid, startBefore, executableDigest, child); err != nil {
		return fail(err)
	}
	schedBefore, err := e.readSchedstat(pid)
	if err != nil || schedBefore == 0 {
		return fail(err)
	}
	controlAfter, err := observe(ctx)
	if err != nil {
		return fail(err)
	}
	controlAfterSnapshot, ok := controlAfter.Snapshot()
	if !ok || !sameV29ImmutableReadback(controlBeforeSnapshot, controlAfterSnapshot) || !nondecreasingV30Counters(controlBeforeSnapshot, controlAfterSnapshot) {
		return fail(ErrInvalidRuntimeCPUThrottleInterventionAuthority)
	}
	if controlAfterSnapshot.NrThrottled != controlBeforeSnapshot.NrThrottled || controlAfterSnapshot.ThrottledUsec != controlBeforeSnapshot.ThrottledUsec {
		return fail(ErrInvalidRuntimeCPUThrottleInterventionAuthority)
	}
	controlAfterAt := e.clock().UTC()
	if err := e.requireHelperIdentity(procsPath, pid, startBefore, executableDigest, child); err != nil {
		return fail(err)
	}

	if err := child.Start(nonce); err != nil {
		return fail(err)
	}
	startedAt := e.clock().UTC()
	if err := child.AwaitBurnDone(ctx, nonce); err != nil {
		return fail(err)
	}
	burnDoneAt := e.clock().UTC()
	if err := e.requireHelperIdentity(procsPath, pid, startBefore, executableDigest, child); err != nil {
		return fail(err)
	}
	schedAfter, err := e.readSchedstat(pid)
	if err != nil || schedAfter < schedBefore || schedAfter-schedBefore < params.minimumCPUNS {
		return fail(err)
	}
	pressureAfter, err := observe(ctx)
	if err != nil {
		return fail(err)
	}
	pressureAfterSnapshot, ok := pressureAfter.Snapshot()
	if !ok || !sameV29ImmutableReadback(controlAfterSnapshot, pressureAfterSnapshot) || !nondecreasingV30Counters(controlAfterSnapshot, pressureAfterSnapshot) {
		return fail(ErrInvalidRuntimeCPUThrottleInterventionAuthority)
	}
	if pressureAfterSnapshot.NrThrottled <= controlAfterSnapshot.NrThrottled || pressureAfterSnapshot.ThrottledUsec <= controlAfterSnapshot.ThrottledUsec {
		return fail(ErrInvalidRuntimeCPUThrottleInterventionAuthority)
	}
	pressureAfterAt := e.clock().UTC()

	if err := child.Exit(nonce); err != nil {
		return fail(err)
	}
	exitRequestedAt := e.clock().UTC()
	if err := child.AwaitDone(ctx, nonce); err != nil {
		return fail(err)
	}
	doneAt := e.clock().UTC()
	exitCode, waitErr := waitRuntimeCPUInterventionChild(ctx, child)
	waited = true
	exitedAt := e.clock().UTC()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return runtimeCPUInterventionResult{}, ctxErr
	}
	if waitErr != nil || exitCode != 0 {
		return runtimeCPUInterventionResult{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	final, err := observe(ctx)
	if err != nil {
		return runtimeCPUInterventionResult{}, err
	}
	finalSnapshot, ok := final.Snapshot()
	if !ok || !sameV29ImmutableReadback(pressureAfterSnapshot, finalSnapshot) || !nondecreasingV30Counters(pressureAfterSnapshot, finalSnapshot) {
		return runtimeCPUInterventionResult{}, ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}

	return runtimeCPUInterventionResult{
		controlBefore:     controlBefore,
		controlAfter:      controlAfter,
		pressureAfter:     pressureAfter,
		final:             final,
		pid:               pid,
		startTimeTicks:    startBefore,
		executableSHA256:  executableDigest,
		nonceSHA256:       nonceDigest,
		schedBeforeNS:     schedBefore,
		schedAfterNS:      schedAfter,
		readyAt:           readyAt,
		placedAt:          placedAt,
		controlBeforeAt:   controlBeforeAt,
		controlAfterAt:    controlAfterAt,
		startedAt:         startedAt,
		burnDoneAt:        burnDoneAt,
		pressureAfterAt:   pressureAfterAt,
		exitRequestedAt:   exitRequestedAt,
		doneAt:            doneAt,
		exitedAt:          exitedAt,
		exitCode:          exitCode,
	}, nil
}

func (e *RuntimeCPUInterventionExecutor) requireHelperIdentity(
	procsPath string,
	pid int,
	startTime uint64,
	executableDigest string,
	child runtimeCPUInterventionChild,
) error {
	if !e.valid() || child == nil || pid <= 0 || startTime == 0 || !validV29SHA256(executableDigest) || !child.Alive() {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	procs, err := e.readFile(procsPath)
	if err != nil || !containsExactCgroupPID(string(procs), uint32(pid)) {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	currentStart, err := e.readStartTime(pid)
	if err != nil || currentStart != startTime {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	currentExecutable, err := e.readHelperExecutableDigest(pid)
	if err != nil || currentExecutable != executableDigest || currentExecutable != e.executableDigest {
		return ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	return nil
}

func nondecreasingV30Counters(before, after RuntimeCgroupReadbackSnapshot) bool {
	return after.NrThrottled >= before.NrThrottled &&
		after.ThrottledUsec >= before.ThrottledUsec &&
		after.OOMEvents >= before.OOMEvents &&
		after.OOMKillEvents >= before.OOMKillEvents
}

func v30SnapshotMatchesReadbacks(
	s RuntimeCPUThrottleInterventionSnapshot,
	controlBefore RuntimeCgroupReadbackAuthority,
	controlAfter RuntimeCgroupReadbackAuthority,
	pressureAfter RuntimeCgroupReadbackAuthority,
	final RuntimeCgroupReadbackAuthority,
) bool {
	cb, ok1 := controlBefore.Snapshot()
	ca, ok2 := controlAfter.Snapshot()
	pa, ok3 := pressureAfter.Snapshot()
	fn, ok4 := final.Snapshot()
	cbd, ok5 := controlBefore.ReadbackDigest()
	cad, ok6 := controlAfter.ReadbackDigest()
	pad, ok7 := pressureAfter.ReadbackDigest()
	fnd, ok8 := final.ReadbackDigest()
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 {
		return false
	}
	if !sameV29ImmutableReadback(cb, ca) || !sameV29ImmutableReadback(ca, pa) || !sameV29ImmutableReadback(pa, fn) {
		return false
	}
	if !nondecreasingV30Counters(cb, ca) || !nondecreasingV30Counters(ca, pa) || !nondecreasingV30Counters(pa, fn) {
		return false
	}
	if cb.NrThrottled != ca.NrThrottled || cb.ThrottledUsec != ca.ThrottledUsec || pa.NrThrottled <= ca.NrThrottled || pa.ThrottledUsec <= ca.ThrottledUsec {
		return false
	}
	return s.RuntimeDigest == cb.RuntimeDigest && s.SandboxID == cb.SandboxID && s.Generation == cb.Generation &&
		s.RuntimeHostPID == cb.HostPID && s.RuntimeStartTimeTicks == cb.StartTimeTicks && s.RuntimeBootID == cb.BootID &&
		s.CGroupPath == cb.CGroupPath && s.CPUQuotaMicros == cb.CPUQuotaMicros && s.CPUPeriodMicros == cb.CPUPeriodMicros &&
		s.MemoryLimitBytes == cb.MemoryLimitBytes && s.ControlBeforeReadbackDigest == cbd && s.ControlAfterReadbackDigest == cad &&
		s.PressureAfterReadbackDigest == pad && s.FinalReadbackDigest == fnd &&
		s.ControlBeforeNrThrottled == cb.NrThrottled && s.ControlAfterNrThrottled == ca.NrThrottled && s.PressureAfterNrThrottled == pa.NrThrottled &&
		s.ControlBeforeThrottledUsec == cb.ThrottledUsec && s.ControlAfterThrottledUsec == ca.ThrottledUsec && s.PressureAfterThrottledUsec == pa.ThrottledUsec &&
		validV30Snapshot(s)
}

func validV30Snapshot(s RuntimeCPUThrottleInterventionSnapshot) bool {
	if !strings.HasPrefix(s.RuntimeDigest, "runtime-realization-v27:") || !validRuntimeCgroupReadbackDigest(s.ControlBeforeReadbackDigest) || !validRuntimeCgroupReadbackDigest(s.ControlAfterReadbackDigest) || !validRuntimeCgroupReadbackDigest(s.PressureAfterReadbackDigest) || !validRuntimeCgroupReadbackDigest(s.FinalReadbackDigest) {
		return false
	}
	if s.SandboxID == "" || strings.TrimSpace(s.SandboxID) != s.SandboxID || s.Generation == 0 || s.RuntimeHostPID == 0 || s.RuntimeStartTimeTicks == 0 || !canonicalLowerUUID(s.RuntimeBootID) {
		return false
	}
	if s.CGroupPath == "" || path.Clean(s.CGroupPath) != s.CGroupPath || s.CGroupPath == "/" || s.CPUQuotaMicros <= 0 || s.CPUPeriodMicros == 0 || uint64(s.CPUQuotaMicros) >= s.CPUPeriodMicros || s.MemoryLimitBytes == 0 {
		return false
	}
	if s.HelperPID <= 0 || s.HelperStartTimeTicks == 0 || !validV29SHA256(s.HelperExecutableSHA256) || !validV29SHA256(s.HelperNonceSHA256) || s.ExitCode != 0 {
		return false
	}
	if s.ControlAfterNrThrottled != s.ControlBeforeNrThrottled || s.ControlAfterThrottledUsec != s.ControlBeforeThrottledUsec || s.PressureAfterNrThrottled <= s.ControlAfterNrThrottled || s.PressureAfterThrottledUsec <= s.ControlAfterThrottledUsec {
		return false
	}
	if s.HelperSchedstatBeforeNS == 0 || s.HelperSchedstatAfterNS < s.HelperSchedstatBeforeNS || s.HelperSchedstatAfterNS-s.HelperSchedstatBeforeNS < uint64(s.CPUQuotaMicros)*1000 {
		return false
	}
	times := []time.Time{s.ReadyAt, s.PlacedAt, s.ControlBeforeAt, s.ControlAfterAt, s.StartedAt, s.BurnDoneAt, s.PressureAfterAt, s.ExitRequestedAt, s.DoneAt, s.ExitedAt}
	for _, ts := range times {
		if ts.IsZero() {
			return false
		}
	}
	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			return false
		}
	}
	return true
}

func deriveV30AuthorityDigest(snapshot RuntimeCPUThrottleInterventionSnapshot) (string, error) {
	if !validV30Snapshot(snapshot) {
		return "", ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", ErrInvalidRuntimeCPUThrottleInterventionAuthority
	}
	h := sha256.New()
	_, _ = h.Write([]byte(runtimeCPUInterventionDigestDomain))
	_, _ = h.Write(raw)
	return runtimeCPUInterventionDigestPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

func validV30AuthorityDigest(raw string) bool {
	if !strings.HasPrefix(raw, runtimeCPUInterventionDigestPrefix) {
		return false
	}
	return validV29SHA256(strings.TrimPrefix(raw, runtimeCPUInterventionDigestPrefix))
}
