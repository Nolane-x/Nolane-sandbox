package realm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const v31FrozenLegacyPolicyDigest = "ff2a38e09715c134c0590d2c286afb4d2d19480eebf04acdcea2a057723c55b1"

func v31Spec(limit uint64) Spec {
	spec := validSpec()
	spec.ID = ID("realm://wave31")
	spec.RuntimeCPULimitMilliCPU = limit
	return spec
}

func seedV31MemoryRealm(t *testing.T, limit uint64) (*MemoryStore, *Controller, RealmRecord) {
	t.Helper()
	store := NewMemoryStore()
	ctl, err := NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ctl.Create(context.Background(), v31Spec(limit))
	if err != nil {
		t.Fatal(err)
	}
	return store, ctl, rec
}

func TestV31LegacySpecOmitsRuntimeCPULimit(t *testing.T) {
	spec := validSpec()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("runtime_cpu_limit_millicpu")) {
		t.Fatalf("legacy spec emitted Wave31 field: %s", raw)
	}
}

func TestV31LegacyPolicyDigestFrozen(t *testing.T) {
	got, err := PolicyDigest(validSpec(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got != v31FrozenLegacyPolicyDigest {
		t.Fatalf("legacy PolicyDigest drift: got=%q want=%q", got, v31FrozenLegacyPolicyDigest)
	}
}

func TestV31PositiveLimitAppearsAndChangesPolicyDigest(t *testing.T) {
	legacy := validSpec()
	withLimit := legacy
	withLimit.RuntimeCPULimitMilliCPU = 500
	raw, err := json.Marshal(withLimit)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"runtime_cpu_limit_millicpu":500`)) {
		t.Fatalf("positive runtime CPU limit missing from canonical JSON: %s", raw)
	}
	before, err := PolicyDigest(legacy, 7)
	if err != nil {
		t.Fatal(err)
	}
	after, err := PolicyDigest(withLimit, 7)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("positive runtime CPU limit did not change PolicyDigest")
	}
}

func TestV31AccountingCPUUnitsDoNotMintAuthority(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 0)
	if rec.Spec.ResourceBudget.CPUUnits == 0 {
		t.Fatal("fixture must retain a positive accounting CPU budget")
	}
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if !errors.Is(err, ErrRuntimeCPUPolicyUnavailable) {
		t.Fatalf("accounting CPUUnits minted authority=%+v err=%v, want unavailable", authority, err)
	}
}

func TestV31RuntimeLimitIndependentOfAccountingCPUUnits(t *testing.T) {
	store := NewMemoryStore()
	ctl, err := NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	spec := v31Spec(375)
	spec.ResourceBudget.CPUUnits = 97
	rec, err := ctl.Create(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := authority.Binding()
	if !ok || binding.LimitMilliCPU != 375 {
		t.Fatalf("binding=%+v ok=%v, want exact 375 milliCPU", binding, ok)
	}
}

func TestV31CurrentRuntimeCPUPolicyAuthorityMemoryStore(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := authority.Binding()
	if !ok {
		t.Fatal("fresh authority did not expose a valid descriptive binding")
	}
	wantPolicy, err := PolicyDigest(rec.Spec, rec.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if binding.RealmID != rec.Spec.ID || binding.RealmRevision != rec.Revision || binding.PolicyDigest != wantPolicy || binding.LimitMilliCPU != 500 {
		t.Fatalf("unexpected binding: %+v", binding)
	}
	if !strings.HasPrefix(binding.Digest, "realm-runtime-cpu-policy-v31:") || len(binding.Digest) != len("realm-runtime-cpu-policy-v31:")+64 {
		t.Fatalf("unexpected Wave31 digest %q", binding.Digest)
	}
	validated, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	if validated != binding {
		t.Fatalf("validated binding=%+v want=%+v", validated, binding)
	}
}

func TestV31CurrentRuntimeCPUPolicyAuthorityDurableStore(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := OpenDurableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ctl, err := NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ctl.Create(ctx, v31Spec(625))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenDurableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedCtl, err := NewController(reopened)
	if err != nil {
		t.Fatal(err)
	}
	recovered, ok := reopened.Realm(rec.Spec.ID)
	if !ok || recovered.Spec.RuntimeCPULimitMilliCPU != 625 {
		t.Fatalf("recovered realm=%+v ok=%v", recovered, ok)
	}
	authority, err := reopenedCtl.CurrentRuntimeCPUPolicyAuthority(ctx, rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := authority.Binding()
	if !ok || binding.LimitMilliCPU != 625 {
		t.Fatalf("durable authority binding=%+v ok=%v", binding, ok)
	}
}

func TestV31AuthorityDigestDeterministic(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 750)
	first, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstBinding, ok := first.Binding()
	if !ok {
		t.Fatal("first binding invalid")
	}
	secondBinding, ok := second.Binding()
	if !ok {
		t.Fatal("second binding invalid")
	}
	if firstBinding != secondBinding {
		t.Fatalf("same authoritative state produced different bindings: first=%+v second=%+v", firstBinding, secondBinding)
	}
}

func TestV31ZeroAuthorityInvalid(t *testing.T) {
	_, ctl, _ := seedV31MemoryRealm(t, 500)
	var zero RuntimeCPUPolicyAuthority
	if _, ok := zero.Binding(); ok {
		t.Fatal("zero authority exposed a valid binding")
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), zero); !errors.Is(err, ErrInvalidRuntimeCPUPolicyAuthority) {
		t.Fatalf("zero authority err=%v, want invalid", err)
	}
}

func TestV31InternalDigestTamperInvalid(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	authority.binding.Digest = "realm-runtime-cpu-policy-v31:" + strings.Repeat("0", 64)
	if _, ok := authority.Binding(); ok {
		t.Fatal("digest-tampered authority remained valid")
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrInvalidRuntimeCPUPolicyAuthority) {
		t.Fatalf("tampered digest err=%v, want invalid", err)
	}
}

func TestV31InternalBindingTamperInvalid(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	authority.binding.LimitMilliCPU++
	if _, ok := authority.Binding(); ok {
		t.Fatal("binding-tampered authority remained valid")
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrInvalidRuntimeCPUPolicyAuthority) {
		t.Fatalf("tampered binding err=%v, want invalid", err)
	}
}

func TestV31RevisionUpdateMakesAuthorityStale(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	unchangedLimit := rec.Spec
	unchangedLimit.MaxWorlds++
	if _, err := ctl.Update(context.Background(), rec.Spec.ID, rec.Revision, unchangedLimit); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrStaleRuntimeCPUPolicyAuthority) {
		t.Fatalf("revision drift err=%v, want stale", err)
	}
}

func TestV31LimitChangeMakesAuthorityStale(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := rec.Spec
	changed.RuntimeCPULimitMilliCPU = 900
	if _, err := ctl.Update(context.Background(), rec.Spec.ID, rec.Revision, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrStaleRuntimeCPUPolicyAuthority) {
		t.Fatalf("limit drift err=%v, want stale", err)
	}
}

func TestV31LimitClearMakesAuthorityStale(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	cleared := rec.Spec
	cleared.RuntimeCPULimitMilliCPU = 0
	if _, err := ctl.Update(context.Background(), rec.Spec.ID, rec.Revision, cleared); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrStaleRuntimeCPUPolicyAuthority) {
		t.Fatalf("cleared limit err=%v, want stale", err)
	}
}

func TestV31UnrelatedPolicyChangeMakesAuthorityStale(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := rec.Spec
	changed.ResourceBudget.MemoryMiB++
	if _, err := ctl.Update(context.Background(), rec.Spec.ID, rec.Revision, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrStaleRuntimeCPUPolicyAuthority) {
		t.Fatalf("unrelated policy drift err=%v, want stale", err)
	}
}

func TestV31RealmCloseMakesAuthorityStale(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctl.Close(context.Background(), rec.Spec.ID, rec.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(context.Background(), authority); !errors.Is(err, ErrStaleRuntimeCPUPolicyAuthority) {
		t.Fatalf("closed realm err=%v, want stale", err)
	}
}

func TestV31ContextCancellation(t *testing.T) {
	_, ctl, rec := seedV31MemoryRealm(t, 500)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ctl.CurrentRuntimeCPUPolicyAuthority(ctx, rec.Spec.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("mint canceled err=%v", err)
	}
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), rec.Spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(ctx, authority); !errors.Is(err, context.Canceled) {
		t.Fatalf("validate canceled err=%v", err)
	}
}
