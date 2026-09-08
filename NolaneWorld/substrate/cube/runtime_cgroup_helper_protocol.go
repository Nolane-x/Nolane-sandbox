package cube

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

const runtimeCgroupHelperModeEnv = "NOLANE_INTERNAL_CGROUP_HELPER"
const runtimeCgroupHelperNonceEnv = "NOLANE_INTERNAL_CGROUP_HELPER_NONCE"
const runtimeCgroupHelperModeParkExit = "park-exit"
const runtimeCgroupHelperReleaseFD = 3
const runtimeCgroupHelperAckFD = 4
const runtimeCgroupHelperProtocolLimit = 96

// MaybeRunInternalCgroupHelper handles the package-owned Wave29 helper mode.
// It exposes no authority and performs no pressure work.
func MaybeRunInternalCgroupHelper() (handled bool, exitCode int) {
	if os.Getenv(runtimeCgroupHelperModeEnv) != runtimeCgroupHelperModeParkExit {
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
