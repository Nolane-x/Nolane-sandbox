package cube

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func v30ProtocolEnv(nonce, burnMicros string) func(string) string {
	return func(key string) string {
		switch key {
		case runtimeCgroupHelperModeEnv:
			return runtimeCPUInterventionMode
		case runtimeCgroupHelperNonceEnv:
			return nonce
		case runtimeCPUInterventionBurnMicrosEnv:
			return burnMicros
		default:
			return ""
		}
	}
}

func TestV30HelperStartBurnExitProtocol(t *testing.T) {
	nonce := strings.Repeat("ab", 32)
	release := strings.NewReader(
		helperProtocolRecord("START", nonce) +
			helperProtocolRecord("EXIT", nonce),
	)
	var ack bytes.Buffer
	burnCalls := 0
	var burnDuration time.Duration

	code := runInternalCPUInterventionHelper(
		v30ProtocolEnv(nonce, "400000"),
		release,
		&ack,
		func(d time.Duration) error {
			burnCalls++
			burnDuration = d
			return nil
		},
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if burnCalls != 1 {
		t.Fatalf("burn calls = %d, want 1", burnCalls)
	}
	if burnDuration != 400*time.Millisecond {
		t.Fatalf("burn duration = %s, want 400ms", burnDuration)
	}
	want := helperProtocolRecord("READY", nonce) +
		helperProtocolRecord("BURN_DONE", nonce) +
		helperProtocolRecord("DONE", nonce)
	if ack.String() != want {
		t.Fatalf("ack = %q, want %q", ack.String(), want)
	}
}

func TestV30HelperRejectsExitBeforeStartWithoutBurn(t *testing.T) {
	nonce := strings.Repeat("cd", 32)
	release := strings.NewReader(helperProtocolRecord("EXIT", nonce))
	var ack bytes.Buffer
	burnCalls := 0

	code := runInternalCPUInterventionHelper(
		v30ProtocolEnv(nonce, "400000"),
		release,
		&ack,
		func(time.Duration) error {
			burnCalls++
			return nil
		},
	)
	if code == 0 {
		t.Fatal("unexpected success for EXIT before START")
	}
	if burnCalls != 0 {
		t.Fatalf("burn calls = %d, want 0 before START", burnCalls)
	}
	if ack.String() != helperProtocolRecord("READY", nonce) {
		t.Fatalf("ack = %q, want READY only", ack.String())
	}
}

func TestV30HelperRejectsWrongNonceAndBurnFailure(t *testing.T) {
	nonce := strings.Repeat("ef", 32)
	wrong := strings.Repeat("12", 32)

	t.Run("wrong start nonce", func(t *testing.T) {
		release := strings.NewReader(helperProtocolRecord("START", wrong))
		var ack bytes.Buffer
		burnCalls := 0
		code := runInternalCPUInterventionHelper(
			v30ProtocolEnv(nonce, "400000"),
			release,
			&ack,
			func(time.Duration) error {
				burnCalls++
				return nil
			},
		)
		if code == 0 || burnCalls != 0 {
			t.Fatalf("code=%d burnCalls=%d, want non-zero/0", code, burnCalls)
		}
	})

	t.Run("burn failure", func(t *testing.T) {
		release := strings.NewReader(helperProtocolRecord("START", nonce))
		var ack bytes.Buffer
		code := runInternalCPUInterventionHelper(
			v30ProtocolEnv(nonce, "400000"),
			release,
			&ack,
			func(time.Duration) error { return errors.New("burn failed") },
		)
		if code == 0 {
			t.Fatal("unexpected success after burn failure")
		}
		if ack.String() != helperProtocolRecord("READY", nonce) {
			t.Fatalf("ack = %q, want READY only", ack.String())
		}
	})
}

func TestV30HelperRejectsMalformedBurnDuration(t *testing.T) {
	nonce := strings.Repeat("34", 32)
	for _, burn := range []string{"", "0", "-1", "+4", "04", "400000x"} {
		t.Run(burn, func(t *testing.T) {
			var ack bytes.Buffer
			code := runInternalCPUInterventionHelper(
				v30ProtocolEnv(nonce, burn),
				strings.NewReader(helperProtocolRecord("START", nonce)),
				&ack,
				func(time.Duration) error { return nil },
			)
			if code == 0 {
				t.Fatalf("unexpected success for burn duration %q", burn)
			}
		})
	}
}

func TestV30HelperKeepsWave29ParkExitSemantics(t *testing.T) {
	nonce := strings.Repeat("56", 32)
	var ack bytes.Buffer
	code := runInternalCgroupHelper(
		func(key string) string {
			switch key {
			case runtimeCgroupHelperModeEnv:
				return runtimeCgroupHelperModeParkExit
			case runtimeCgroupHelperNonceEnv:
				return nonce
			default:
				return ""
			}
		},
		strings.NewReader(helperProtocolRecord("GO", nonce)),
		&ack,
	)
	if code != 0 {
		t.Fatalf("Wave29 helper code = %d, want 0", code)
	}
	want := helperProtocolRecord("READY", nonce) + helperProtocolRecord("DONE", nonce)
	if ack.String() != want {
		t.Fatalf("Wave29 ack = %q, want %q", ack.String(), want)
	}
}
