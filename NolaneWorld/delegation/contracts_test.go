package delegation

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var fixedNow = time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

func validGrant() Grant {
	return Grant{
		ID:             ID("grant-1"),
		WorldID:        world.ID("world-1"),
		AuthorityEpoch: 1,
		Adapter:        AdapterKind("github.repo.write"),
		Resource:       "repo:Nolane-x/Nolane-sandbox",
		Operations:     []Operation{"issue.create", "contents.write", "contents.write"},
		SecretHandle:   SecretHandle("secret-handle-1"),
		IssuedAt:       fixedNow,
		ExpiresAt:      fixedNow.Add(time.Hour),
	}
}

func validIntent() Intent {
	return Intent{
		WorldID:        world.ID("world-1"),
		AuthorityEpoch: 1,
		ActionID:       "action-1",
		DelegationID:   ID("grant-1"),
		Operation:      Operation("contents.write"),
		Resource:       "repo:Nolane-x/Nolane-sandbox",
		Payload:        []byte("patch-v1"),
	}
}

func TestMemoryStoreCanonicalizesGrantAndRejectsRebinding(t *testing.T) {
	store := NewMemoryStore()
	g := validGrant()
	if err := store.Issue(g); err != nil {
		t.Fatal(err)
	}
	state, err := store.Lookup(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Revoked {
		t.Fatal("new grant is revoked")
	}
	want := []Operation{"contents.write", "issue.create"}
	if len(state.Grant.Operations) != len(want) || state.Grant.Operations[0] != want[0] || state.Grant.Operations[1] != want[1] {
		t.Fatalf("operations=%v", state.Grant.Operations)
	}
	if err := store.Issue(g); err != nil {
		t.Fatalf("exact retry should be idempotent: %v", err)
	}
	rebound := g
	rebound.Resource = "repo:Nolane-x/other"
	if err := store.Issue(rebound); !errors.Is(err, ErrGrantCollision) {
		t.Fatalf("err=%v", err)
	}
}

func TestRevokeIsMonotonicAndGrantRemainsQueryable(t *testing.T) {
	store := NewMemoryStore()
	g := validGrant()
	if err := store.Issue(g); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(g.ID); err != nil {
		t.Fatal(err)
	}
	state, err := store.Lookup(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Revoked {
		t.Fatal("grant resurrected")
	}
	if err := store.Revoke(g.ID); !errors.Is(err, ErrAlreadyRevoked) {
		t.Fatalf("err=%v", err)
	}
}

func TestGrantDigestBindsEveryTrustBearingField(t *testing.T) {
	base := validGrant()
	baseDigest, err := GrantDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*Grant){
		func(g *Grant) { g.WorldID = "other" },
		func(g *Grant) { g.AuthorityEpoch++ },
		func(g *Grant) { g.Adapter = "email.send" },
		func(g *Grant) { g.Resource += ":other" },
		func(g *Grant) { g.Operations = []Operation{"issue.create"} },
		func(g *Grant) { g.SecretHandle = "other-handle" },
		func(g *Grant) { g.IssuedAt = g.IssuedAt.Add(time.Second) },
		func(g *Grant) { g.ExpiresAt = g.ExpiresAt.Add(time.Second) },
	}
	for i, mutate := range mutations {
		g := base
		g.Operations = append([]Operation(nil), base.Operations...)
		mutate(&g)
		d, err := GrantDigest(g)
		if err != nil {
			t.Fatalf("mutation %d: %v", i, err)
		}
		if d == baseDigest {
			t.Fatalf("mutation %d did not change digest", i)
		}
	}
}

func TestRegistryRejectsGenericAuthenticatedTransportAndDuplicates(t *testing.T) {
	if _, err := NewRegistry(fakeAdapter{kind: "generic-http"}); !errors.Is(err, ErrGenericAdapter) {
		t.Fatalf("err=%v", err)
	}
	if _, err := NewRegistry(fakeAdapter{kind: "github.repo.write"}, fakeAdapter{kind: "github.repo.write"}); !errors.Is(err, ErrAdapterCollision) {
		t.Fatalf("err=%v", err)
	}
}

func TestMemoryVaultDoesNotExposeStoredSliceAndWipesLease(t *testing.T) {
	vault := NewMemoryVault()
	material := []byte("TOP-SECRET-V6")
	if err := vault.Put(SecretHandle("h1"), material); err != nil {
		t.Fatal(err)
	}
	material[0] = 'X'
	var lease []byte
	if err := vault.Use(context.Background(), SecretHandle("h1"), func(secret Secret) error {
		lease = secret.Bytes()
		if string(lease) != "TOP-SECRET-V6" {
			t.Fatalf("secret=%q", lease)
		}
		lease[0] = 'Y'
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := vault.Use(context.Background(), SecretHandle("h1"), func(secret Secret) error {
		if string(secret.Bytes()) != "TOP-SECRET-V6" {
			t.Fatal("vault storage mutated through lease")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPlaneRejectsScopeRebindingEscalationRevocationExpiryAndStaleEpoch(t *testing.T) {
	secret := []byte("SYNTHETIC-V6-SECRET")
	cases := []struct {
		name   string
		mutate func(*Intent, *Grant, *MemoryStore, *world.State)
		want   error
	}{
		{"resource-rebind", func(in *Intent, _ *Grant, _ *MemoryStore, _ *world.State) { in.Resource = "repo:Nolane-x/other" }, ErrScopeDenied},
		{"operation-escalation", func(in *Intent, _ *Grant, _ *MemoryStore, _ *world.State) { in.Operation = "admin.delete" }, ErrScopeDenied},
		{"revoked", func(_ *Intent, g *Grant, s *MemoryStore, _ *world.State) { _ = s.Revoke(g.ID) }, ErrDelegationRevoked},
		{"expired", func(_ *Intent, g *Grant, _ *MemoryStore, _ *world.State) { g.ExpiresAt = fixedNow.Add(-time.Second) }, ErrDelegationExpired},
		{"stale", func(_ *Intent, _ *Grant, _ *MemoryStore, st *world.State) { st.AdvanceEpoch() }, world.ErrStaleEpoch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, _ := world.NewState("world-1")
			store := NewMemoryStore()
			g := validGrant()
			if tc.name == "expired" {
				g.ExpiresAt = fixedNow.Add(-time.Second)
			}
			if err := store.Issue(g); err != nil {
				t.Fatal(err)
			}
			vault := NewMemoryVault()
			_ = vault.Put(g.SecretHandle, secret)
			adapter := &recordingAdapter{kind: g.Adapter}
			registry, _ := NewRegistry(adapter)
			plane, err := NewPlane(state, store, vault, registry, authority.NewMemoryLedger(), func() time.Time { return fixedNow })
			if err != nil {
				t.Fatal(err)
			}
			in := validIntent()
			tc.mutate(&in, &g, store, state)
			_, err = plane.Execute(context.Background(), in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if adapter.executeCalls.Load() != 0 {
				t.Fatal("adapter executed after denial")
			}
		})
	}
}

func TestPlaneSelectsAdapterFromGrantAndBindsActionID(t *testing.T) {
	state, _ := world.NewState("world-1")
	store := NewMemoryStore()
	g := validGrant()
	_ = store.Issue(g)
	vault := NewMemoryVault()
	_ = vault.Put(g.SecretHandle, []byte("SYNTHETIC-V6-SECRET"))
	selected := &recordingAdapter{kind: g.Adapter, effect: []byte("effect")}
	other := &recordingAdapter{kind: "email.send", effect: []byte("wrong")}
	registry, _ := NewRegistry(selected, other)
	plane, _ := NewPlane(state, store, vault, registry, authority.NewMemoryLedger(), func() time.Time { return fixedNow })

	in := validIntent()
	r, err := plane.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if selected.executeCalls.Load() != 1 || other.executeCalls.Load() != 0 {
		t.Fatalf("selected=%d other=%d", selected.executeCalls.Load(), other.executeCalls.Load())
	}
	if r.GrantDigest == "" || r.SecretHandleDigest == "" || r.RequestDigest == "" || r.EffectDigest == "" {
		t.Fatalf("receipt=%+v", r)
	}

	rebound := in
	rebound.Payload = []byte("different")
	if _, err := plane.Execute(context.Background(), rebound); !errors.Is(err, authority.ErrActionCollision) {
		t.Fatalf("err=%v", err)
	}
	if selected.executeCalls.Load() != 1 {
		t.Fatal("collision re-executed adapter")
	}
}

func TestAdapterErrorsAndSecretEchoAreSanitizedAndBecomeUncertain(t *testing.T) {
	for _, tc := range []struct {
		name    string
		adapter *recordingAdapter
		want    error
	}{
		{"raw-error", &recordingAdapter{kind: "github.repo.write", executeErr: errors.New("provider says SYNTHETIC-V6-SECRET")}, ErrAdapterFailure},
		{"secret-echo", &recordingAdapter{kind: "github.repo.write", echoSecret: true}, ErrSecretLeak},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, _ := world.NewState("world-1")
			store := NewMemoryStore()
			g := validGrant()
			_ = store.Issue(g)
			vault := NewMemoryVault()
			_ = vault.Put(g.SecretHandle, []byte("SYNTHETIC-V6-SECRET"))
			registry, _ := NewRegistry(tc.adapter)
			ledger, err := authority.OpenJournalLedger(filepath.Join(t.TempDir(), "effects.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			plane, _ := NewPlane(state, store, vault, registry, ledger, func() time.Time { return fixedNow })
			in := validIntent()
			_, err = plane.Execute(context.Background(), in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if strings.Contains(err.Error(), "SYNTHETIC-V6-SECRET") {
				t.Fatalf("secret leaked in error: %v", err)
			}
			_, err = plane.Execute(context.Background(), in)
			if !errors.Is(err, authority.ErrActionUncertain) {
				t.Fatalf("retry err=%v", err)
			}
			if tc.adapter.executeCalls.Load() != 1 {
				t.Fatalf("execute calls=%d", tc.adapter.executeCalls.Load())
			}
		})
	}
}

func TestReconcileObservedResolvesPendingWithoutExecuteAndAbsentDoesNotReplay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state ReconcileState
		want  error
	}{
		{"observed", ReconcileObserved, nil},
		{"absent", ReconcileAbsent, ErrEffectAbsent},
		{"unknown", ReconcileUnknown, authority.ErrActionUncertain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, _ := world.NewState("world-1")
			store := NewMemoryStore()
			g := validGrant()
			_ = store.Issue(g)
			vault := NewMemoryVault()
			_ = vault.Put(g.SecretHandle, []byte("SYNTHETIC-V6-SECRET"))
			adapter := &recordingAdapter{kind: g.Adapter, executeErr: errors.New("lost response"), reconcileState: tc.state, reconcileEvidence: []byte("provider-observation")}
			registry, _ := NewRegistry(adapter)
			ledger, err := authority.OpenJournalLedger(filepath.Join(t.TempDir(), "effects.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			plane, _ := NewPlane(state, store, vault, registry, ledger, func() time.Time { return fixedNow })
			in := validIntent()
			_, _ = plane.Execute(context.Background(), in)
			if err := store.Revoke(g.ID); err != nil {
				t.Fatal(err)
			}
			state.AdvanceEpoch()
			before := adapter.executeCalls.Load()
			r, err := plane.Reconcile(context.Background(), in)
			if tc.want == nil && err != nil {
				t.Fatal(err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if adapter.executeCalls.Load() != before {
				t.Fatal("reconcile called Execute")
			}
			if adapter.reconcileCalls.Load() != 1 {
				t.Fatalf("reconcile calls=%d", adapter.reconcileCalls.Load())
			}
			if tc.state == ReconcileObserved {
				if r.EffectDigest == "" {
					t.Fatalf("receipt=%+v", r)
				}
				if _, err := plane.Reconcile(context.Background(), in); err != nil {
					t.Fatalf("completed reconciliation retry: %v", err)
				}
			} else {
				if _, err := plane.Execute(context.Background(), in); !errors.Is(err, world.ErrStaleEpoch) && !errors.Is(err, authority.ErrActionUncertain) {
					t.Fatalf("unsafe retry err=%v", err)
				}
			}
		})
	}
}

type fakeAdapter struct{ kind AdapterKind }

func (a fakeAdapter) Kind() AdapterKind { return a.kind }
func (a fakeAdapter) Execute(context.Context, AdapterRequest, Secret) (Effect, error) {
	return Effect{Evidence: []byte("ok")}, nil
}
func (a fakeAdapter) Reconcile(context.Context, AdapterRequest, Secret) (ReconcileResult, error) {
	return ReconcileResult{State: ReconcileUnknown}, nil
}

type recordingAdapter struct {
	kind              AdapterKind
	effect            []byte
	executeErr        error
	echoSecret        bool
	reconcileState    ReconcileState
	reconcileEvidence []byte
	executeCalls      atomic.Int32
	reconcileCalls    atomic.Int32
}

func (a *recordingAdapter) Kind() AdapterKind { return a.kind }
func (a *recordingAdapter) Execute(_ context.Context, _ AdapterRequest, secret Secret) (Effect, error) {
	a.executeCalls.Add(1)
	if a.executeErr != nil {
		return Effect{}, a.executeErr
	}
	if a.echoSecret {
		return Effect{Evidence: secret.Bytes()}, nil
	}
	return Effect{Evidence: append([]byte(nil), a.effect...)}, nil
}
func (a *recordingAdapter) Reconcile(_ context.Context, _ AdapterRequest, secret Secret) (ReconcileResult, error) {
	a.reconcileCalls.Add(1)
	if bytes.Equal(a.reconcileEvidence, []byte("echo-secret")) {
		return ReconcileResult{State: a.reconcileState, Evidence: secret.Bytes()}, nil
	}
	return ReconcileResult{State: a.reconcileState, Evidence: append([]byte(nil), a.reconcileEvidence...)}, nil
}
