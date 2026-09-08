package cube

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func mintV27Wave26Authority(t *testing.T, f v26BridgeFixture) RealmResourceProviderEndpointAuthority {
	t.Helper()
	auth, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25, f.resource,
		f.epochObserver, f.client, f.endpoint,
	)
	if err != nil {
		t.Fatalf("mint Wave26 authority: %v", err)
	}
	return auth
}

func observeV27RuntimeForWave26(t *testing.T, sandboxID, token string) (*RuntimeRealizationObserver, *v27MutableMetrics, RuntimeRealizationProof) {
	t.Helper()
	body := v27RuntimeMetrics(
		sandboxID,
		1,
		token,
		777,
		9001,
		"33333333-3333-4333-8333-333333333333",
	)
	observer, mutable := newV27Observer(t, body)
	proof, err := observer.Observe(context.Background(), v27Binding(sandboxID))
	if err != nil {
		t.Fatalf("observe runtime realization: %v", err)
	}
	return observer, mutable, proof
}

func validateV27(t *testing.T, f v26BridgeFixture, wave26 RealmResourceProviderEndpointAuthority, observer *RuntimeRealizationObserver, proof RuntimeRealizationProof) (RealmResourceRuntimeAuthority, error) {
	t.Helper()
	return ValidateRealmResourceRuntimeAuthority(
		context.Background(),
		f.controller,
		f.realization,
		wave26,
		f.resource,
		f.epochObserver,
		f.client,
		observer,
		proof,
	)
}

func TestV27ExactFreshWave26AndRuntimeMintRealmResourceRuntimeAuthority(t *testing.T) {
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	observer, _, proof := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("81", 32))

	auth, err := validateV27(t, f, wave26, observer, proof)
	if err != nil {
		t.Fatalf("ValidateRealmResourceRuntimeAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("exact Wave27 runtime authority is invalid")
	}
	if sandboxID, ok := auth.SandboxID(); !ok || sandboxID != "sandbox-v26" {
		t.Fatalf("SandboxID=(%q,%v)", sandboxID, ok)
	}
	digest, ok := auth.RuntimeDigest()
	if !ok || !strings.HasPrefix(digest, "runtime-realization-v27:") {
		t.Fatalf("RuntimeDigest=(%q,%v)", digest, ok)
	}
	raw := strings.TrimPrefix(digest, "runtime-realization-v27:")
	if len(raw) != 64 {
		t.Fatalf("runtime digest hex length=%d, want 64", len(raw))
	}
	if decoded, err := hex.DecodeString(raw); err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != raw {
		t.Fatalf("runtime digest is not canonical lowercase SHA-256: %q err=%v", raw, err)
	}

	expected, ok := f.realization.Binding()
	if !ok {
		t.Fatal("fixture realization authority invalid")
	}
	projected, ok := auth.RealizationBinding()
	if !ok || projected != expected {
		t.Fatalf("RealizationBinding=(%+v,%v), want %+v", projected, ok, expected)
	}
}

func TestV27RuntimeDigestIsDeterministicForSameFreshAuthority(t *testing.T) {
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	observer, _, proof := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("81", 32))

	first, err := validateV27(t, f, wave26, observer, proof)
	if err != nil {
		t.Fatal(err)
	}
	second, err := validateV27(t, f, wave26, observer, proof)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest, ok1 := first.RuntimeDigest()
	secondDigest, ok2 := second.RuntimeDigest()
	if !ok1 || !ok2 || firstDigest != secondDigest {
		t.Fatalf("nondeterministic runtime digest: %q %q", firstDigest, secondDigest)
	}
}

func TestV27RuntimeEpochMustMatchWave26Epoch(t *testing.T) {
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	observer, _, proof := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("82", 32))

	_, err := validateV27(t, f, wave26, observer, proof)
	if !errors.Is(err, ErrInvalidRealmResourceRuntimeAuthority) {
		t.Fatalf("epoch mismatch error=%v, want ErrInvalidRealmResourceRuntimeAuthority", err)
	}
}

func TestV27CrossObserverRuntimeProofCannotMintRealmResourceRuntimeAuthority(t *testing.T) {
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	observerA, _, proof := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("81", 32))
	observerB, _, _ := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("81", 32))
	if observerA == observerB {
		t.Fatal("observers unexpectedly alias")
	}

	_, err := validateV27(t, f, wave26, observerB, proof)
	if !errors.Is(err, ErrInvalidRuntimeRealizationProof) {
		t.Fatalf("cross-observer error=%v, want ErrInvalidRuntimeRealizationProof", err)
	}
}

func TestV27RuntimeProcessReplacementMakesOldProofStale(t *testing.T) {
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	observer, mutable, proof := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("81", 32))
	mutable.body = v27RuntimeMetrics(
		"sandbox-v26",
		1,
		strings.Repeat("81", 32),
		778,
		9002,
		"33333333-3333-4333-8333-333333333333",
	)

	_, err := validateV27(t, f, wave26, observer, proof)
	if !errors.Is(err, ErrStaleRuntimeRealizationProof) {
		t.Fatalf("runtime replacement error=%v, want ErrStaleRuntimeRealizationProof", err)
	}
}

func TestV27ReconstructsWave26ProviderFreshnessBeforeMint(t *testing.T) {
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	observer, _, proof := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("81", 32))
	f.providerState.SetIncarnation("8f9619ff-8b86-4e2e-aad7-5bf05e8a0f4d")

	_, err := validateV27(t, f, wave26, observer, proof)
	if !errors.Is(err, ErrStaleProviderIncarnationProof) {
		t.Fatalf("stale provider error=%v, want ErrStaleProviderIncarnationProof", err)
	}
}

func TestV27WrongResourceCannotMintRealmResourceRuntimeAuthority(t *testing.T) {
	f := newV26BridgeFixture(t)
	wave26 := mintV27Wave26Authority(t, f)
	observer, _, proof := observeV27RuntimeForWave26(t, "sandbox-v26", strings.Repeat("81", 32))
	f.resource = ResourceBinding{sandboxID: "sandbox-other"}

	_, err := validateV27(t, f, wave26, observer, proof)
	if err == nil {
		t.Fatal("wrong resource minted Wave27 authority")
	}
}
