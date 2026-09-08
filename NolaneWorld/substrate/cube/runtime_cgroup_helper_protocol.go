package cube

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const runtimeCgroupHelperModeEnv = "NOLANE_INTERNAL_CGROUP_HELPER"
const runtimeCgroupHelperNonceEnv = "NOLANE_INTERNAL_CGROUP_HELPER_NONCE"
const runtimeCgroupHelperModeParkExit = "park-exit"
const runtimeCPUInterventionMode = "cpu-throttle-v30"
const runtimeCPUInterventionBurnMicrosEnv = "NOLANE_INTERNAL_CGROUP_HELPER_BURN_MICROS"
const runtimeCgroupHelperReleaseFD = 3
const runtimeCgroupHelperAckFD = 4
const runtimeCgroupHelperProtocolLimit = 96
const runtimeCPUInterventionMaxBurnMicros = uint64(2_000_000)

// MaybeRunInternalCgroupHelper handles package-owned internal helper modes before
// ordinary command parsing. Wave29 park-exit semantics remain unchanged; Wave30
// adds a separate bounded CPU-intervention protocol and exposes no authority.
func MaybeRunInternalCgroupHelper() (handled bool, exitCode int) {
	mode := os.Getenv(runtimeCgroupHelperModeEnv)
	if mode != runtimeCgroupHelperModeParkExit && mode != runtimeCPUInterventionMode {
		return false, 0
	}
	release := os.NewFile(uintptr(runtimeCgroupHelperReleaseFD), "nolane-cgroup-helper-release")
	ack := os.NewFile(uintptr(runtimeCgroupHelperAckFD), "nolane-cgroup-helper-ack")
	if release == nil || ack == nil {
		if release != nil {
			_ = release.Close()
		}
		if ack != nil {
			_ = ack.Close()
		}
		return true, 2
	}
	defer release.Close()
	defer ack.Close()
	if mode == runtimeCPUInterventionMode {
		return true, runInternalCPUInterventionHelper(os.Getenv, release, ack, runRuntimeCPUInterventionBurn)
	}
	return true, runInternalCgroupHelper(os.Getenv, release, ack)
}

func runInternalCgroupHelper(getenv func(string) string, release io.Reader, ack io.Writer) int {
	if getenv == nil || release == nil || ack == nil {
		return 2
	}
	if getenv(runtimeCgroupHelperModeEnv) != runtimeCgroupHelperModeParkExit {
		return 2
	}
	nonce := getenv(runtimeCgroupHelperNonceEnv)
	if !canonicalV29NonceHex(nonce) {
		return 2
	}
	if _, err := io.WriteString(ack, "READY "+nonce+"\n"); err != nil {
		return 2
	}

	raw, err := io.ReadAll(io.LimitReader(release, runtimeCgroupHelperProtocolLimit+1))
	if err != nil || len(raw) > runtimeCgroupHelperProtocolLimit {
		return 2
	}
	if string(raw) != "GO "+nonce+"\n" {
		return 2
	}
	if _, err := io.WriteString(ack, "DONE "+nonce+"\n"); err != nil {
		return 2
	}
	return 0
}

func runInternalCPUInterventionHelper(
	getenv func(string) string,
	release io.Reader,
	ack io.Writer,
	burn func(time.Duration) error,
) int {
	if getenv == nil || release == nil || ack == nil || burn == nil {
		return 2
	}
	if getenv(runtimeCgroupHelperModeEnv) != runtimeCPUInterventionMode {
		return 2
	}
	nonce := getenv(runtimeCgroupHelperNonceEnv)
	if !canonicalV29NonceHex(nonce) {
		return 2
	}
	burnMicros, ok := parseRuntimeCgroupCanonicalUint(getenv(runtimeCPUInterventionBurnMicrosEnv), false)
	if !ok || burnMicros > runtimeCPUInterventionMaxBurnMicros {
		return 2
	}
	burnDuration := time.Duration(burnMicros) * time.Microsecond
	if burnDuration <= 0 {
		return 2
	}
	if _, err := io.WriteString(ack, helperProtocolRecord("READY", nonce)); err != nil {
		return 2
	}

	reader := bufio.NewReaderSize(release, runtimeCgroupHelperProtocolLimit+1)
	if !readExactCPUInterventionRecord(reader, helperProtocolRecord("START", nonce)) {
		return 2
	}
	if err := burn(burnDuration); err != nil {
		return 2
	}
	if _, err := io.WriteString(ack, helperProtocolRecord("BURN_DONE", nonce)); err != nil {
		return 2
	}
	if !readExactCPUInterventionRecord(reader, helperProtocolRecord("EXIT", nonce)) {
		return 2
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		return 2
	}
	if _, err := io.WriteString(ack, helperProtocolRecord("DONE", nonce)); err != nil {
		return 2
	}
	return 0
}

func readExactCPUInterventionRecord(reader *bufio.Reader, expected string) bool {
	if reader == nil || expected == "" || len(expected) > runtimeCgroupHelperProtocolLimit {
		return false
	}
	line, err := reader.ReadString('\n')
	if err != nil || len(line) > runtimeCgroupHelperProtocolLimit || line != expected {
		return false
	}
	return true
}

func runRuntimeCPUInterventionBurn(duration time.Duration) error {
	if duration <= 0 || duration > time.Duration(runtimeCPUInterventionMaxBurnMicros)*time.Microsecond {
		return fmt.Errorf("cube: invalid Wave30 CPU burn duration")
	}

	// /proc/<pid>/schedstat is task-scoped. Lock the burn goroutine and only
	// proceed when it is the process leader, so the later PID schedstat read is
	// evidence for the exact task that performed the package-owned intervention.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if syscall.Gettid() != os.Getpid() {
		return fmt.Errorf("cube: Wave30 CPU burn is not on process leader")
	}

	deadline := time.Now().Add(duration)
	var accumulator uint64 = 0x9e3779b97f4a7c15
	for time.Now().Before(deadline) {
		for i := 0; i < 16_384; i++ {
			accumulator ^= accumulator << 7
			accumulator ^= accumulator >> 9
			accumulator *= 0xbf58476d1ce4e5b9
		}
	}
	runtime.KeepAlive(accumulator)
	return nil
}

func canonicalV29NonceHex(raw string) bool {
	if len(raw) != 64 || raw != strings.ToLower(raw) {
		return false
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return false
	}
	for _, b := range []byte(raw) {
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')) {
			return false
		}
	}
	return true
}

func helperProtocolRecord(kind, nonce string) string {
	return fmt.Sprintf("%s %s\n", kind, nonce)
}
