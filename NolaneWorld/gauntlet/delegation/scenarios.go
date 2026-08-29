package delegationgauntlet

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	delegation "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

const SyntheticSecret = "SYNTHETIC-V6-SECRET"

var fixedNow = time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

func RunStandard(ctx context.Context, policy gauntlet.Policy) (gauntlet.Report, error) {
	return gauntlet.NewRunner(policy).Run(ctx, Standard())
}

func Standard() []gauntlet.Scenario {
	return []gauntlet.Scenario{
		scenario("authority.v6.action-id-collision", "An action ID cannot be rebound to a different delegated request.", "Execute one action, then reuse its action ID with a different payload.", "The effect ledger rejects the second request before adapter execution.", []string{"collision-denied"}, actionCollision),
		scenario("authority.v6.adapter-selection", "The agent cannot choose the credential-bearing adapter.", "Register both the granted adapter and a different adapter and submit a valid intent.", "Only the adapter bound by the immutable grant executes.", []string{"grant-selected-adapter"}, adapterSelection),
		scenario("authority.v6.expired-delegation", "An expired delegation cannot authorize execution.", "Issue a structurally valid grant whose expiry is in the past.", "The plane denies before adapter execution.", []string{"expiry-denied"}, expiredDelegation),
		scenario("authority.v6.generic-http", "Generic authenticated HTTP cannot become a trusted typed adapter.", "Attempt to register a generic authenticated HTTP adapter.", "The registry rejects the adapter kind.", []string{"generic-http-denied"}, genericHTTP),
		scenario("authority.v6.journal-restart-revocation", "Delegation revocation survives host process restart.", "Issue and revoke a grant, close the journal, then reopen it.", "The recovered grant remains revoked.", []string{"revocation-survived-restart"}, journalRestartRevocation),
		scenario("authority.v6.journal-tamper", "Tampered delegation history is never trusted.", "Modify a persisted grant journal byte before recovery.", "Strict hash-chain replay rejects the journal.", []string{"tamper-denied"}, journalTamper),
		scenario("authority.v6.operation-escalation", "A delegated operation allowlist cannot be widened by the agent.", "Replace an allowed operation with an ungranted administrative operation.", "The plane denies before adapter execution.", []string{"operation-denied"}, operationEscalation),
		scenario("authority.v6.reconcile-absent", "An absent reconciliation result never authorizes automatic retry.", "Create an uncertain action, reconcile it as absent, then retry execution.", "The action remains pending and the adapter is not executed again.", []string{"absent-still-pending"}, reconcileAbsent),
		scenario("authority.v6.reconcile-observed", "Historical reconciliation can close an uncertain effect without re-execution.", "Create an uncertain action, revoke/advance authority, then reconcile the effect as observed.", "Reconcile resolves the pending receipt while Adapter.Execute remains at one call.", []string{"observed-resolved-no-reexecute"}, reconcileObserved),
		scenario("authority.v6.resource-rebinding", "A delegation cannot be rebound to another resource.", "Change the intent resource while reusing the same delegation ID.", "Exact resource scope comparison denies the request before adapter execution.", []string{"resource-denied"}, resourceRebinding),
		scenario("authority.v6.revoked-delegation", "A revoked delegation never becomes active again.", "Revoke the exact grant and attempt execution.", "The plane denies before adapter execution.", []string{"revocation-denied"}, revokedDelegation),
		scenario("authority.v6.secret-evidence-absence", "Agent-visible receipts and errors contain no credential bytes.", "Execute a valid delegated action with a synthetic credential and serialize the receipt.", "Only opaque digests are visible; synthetic secret bytes are absent.", []string{"secret-absent-from-receipt"}, secretEvidenceAbsence),
		scenario("authority.v6.secret-echo", "Provider evidence that echoes a credential fails closed.", "Make a typed adapter return its exact credential bytes as evidence.", "The plane reports a stable secret-leak sentinel and leaves the action uncertain.", []string{"secret-echo-denied"}, secretEcho),
		scenario("authority.v6.stale-epoch", "Guest rollback cannot revive old delegated authority.", "Advance the host authority epoch and submit an intent carrying the old epoch.", "World authority rejects the stale epoch before adapter execution.", []string{"stale-epoch-denied"}, staleEpoch),
		scenario("authority.v6.uncertain-replay", "An uncertain external effect is never automatically executed twice.", "Force a provider error after Adapter.Execute is entered and submit the same action again.", "The durable ledger reports uncertainty and adapter call count remains one.", []string{"uncertain-replay-denied"}, uncertainReplay),
	}
}

func scenario(id, invariant, attack, defense string, markers []string, fn func(context.Context, *gauntlet.Probe) error) gauntlet.Scenario {
	return gauntlet.ScenarioFunc{Definition: gauntlet.ScenarioSpec{ID: id, Invariant: invariant, Attack: attack, ExpectedDefense: defense, Severity: gauntlet.SeverityCritical, RequiredMarkers: markers}, Execute: fn}
}

func resourceRebinding(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(false, delegation.ReconcileUnknown)
	if err != nil { return err }
	defer cleanup()
	if err := rec(p, gauntlet.EventAttack, "resource-rebind-attempt", "intent resource changed while delegation id is reused"); err != nil { return err }
	in.Resource = "repo:Nolane-x/other"
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, delegation.ErrScopeDenied) || adapter.exec.Load() != 0 { return errors.New("resource rebinding was not denied") }
	if err := rec(p, gauntlet.EventBoundary, "exact-grant-scope", "delegation plane compared the immutable grant resource"); err != nil { return err }
	if err := rec(p, gauntlet.EventDenial, "resource-denied", "mismatched resource was rejected before adapter execution"); err != nil { return err }
	return nil
}

func operationEscalation(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(false, delegation.ReconcileUnknown)
	if err != nil { return err }
	defer cleanup()
	if err := rec(p, gauntlet.EventAttack, "operation-escalation-attempt", "intent requested an operation outside the grant allowlist"); err != nil { return err }
	in.Operation = delegation.Operation("admin.delete")
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, delegation.ErrScopeDenied) || adapter.exec.Load() != 0 { return errors.New("operation escalation was not denied") }
	if err := rec(p, gauntlet.EventBoundary, "operation-allowlist", "plane checked canonical grant operations"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "operation-denied", "ungranted operation was denied")
}

func staleEpoch(ctx context.Context, p *gauntlet.Probe) error {
	plane, state, adapter, in, cleanup, err := newPlane(false, delegation.ReconcileUnknown)
	if err != nil { return err }
	defer cleanup()
	if err := rec(p, gauntlet.EventAttack, "stale-epoch-attempt", "host epoch advanced while intent retained old epoch"); err != nil { return err }
	state.AdvanceEpoch()
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, world.ErrStaleEpoch) || adapter.exec.Load() != 0 { return errors.New("stale epoch was not denied") }
	if err := rec(p, gauntlet.EventBoundary, "host-authority-epoch", "host authority state remained outside guest rollback"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "stale-epoch-denied", "old delegation epoch could not execute")
}

func revokedDelegation(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(false, delegation.ReconcileUnknown)
	if err != nil { return err }
	defer cleanup()
	store := planeStore(plane)
	if err := store.Revoke(in.DelegationID); err != nil { return err }
	if err := rec(p, gauntlet.EventAttack, "revoked-grant-attempt", "execution attempted after host revocation"); err != nil { return err }
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, delegation.ErrDelegationRevoked) || adapter.exec.Load() != 0 { return errors.New("revoked delegation executed") }
	if err := rec(p, gauntlet.EventBoundary, "host-revocation-state", "resolver returned monotonic revoked state"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "revocation-denied", "revoked grant was denied")
}

func expiredDelegation(ctx context.Context, p *gauntlet.Probe) error {
	state, _ := world.NewState("v6-world")
	store := delegation.NewMemoryStore()
	g := baseGrant()
	g.IssuedAt = fixedNow.Add(-2 * time.Hour)
	g.ExpiresAt = fixedNow.Add(-time.Hour)
	if err := store.Issue(g); err != nil { return err }
	vault := delegation.NewMemoryVault(); _ = vault.Put(g.SecretHandle, []byte(SyntheticSecret))
	adapter := &testAdapter{kind: g.Adapter}
	reg, _ := delegation.NewRegistry(adapter)
	plane, _ := delegation.NewPlane(state, store, vault, reg, authority.NewMemoryLedger(), func() time.Time { return fixedNow })
	if err := rec(p, gauntlet.EventAttack, "expired-grant-attempt", "execution attempted with a structurally valid but expired grant"); err != nil { return err }
	_, err := plane.Execute(ctx, baseIntent())
	if !errors.Is(err, delegation.ErrDelegationExpired) || adapter.exec.Load() != 0 { return errors.New("expired delegation executed") }
	if err := rec(p, gauntlet.EventBoundary, "host-clock-expiry", "plane compared expiry using its host clock"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "expiry-denied", "expired grant was denied")
}

func adapterSelection(ctx context.Context, p *gauntlet.Probe) error {
	state, _ := world.NewState("v6-world")
	store := delegation.NewMemoryStore(); g := baseGrant(); _ = store.Issue(g)
	vault := delegation.NewMemoryVault(); _ = vault.Put(g.SecretHandle, []byte(SyntheticSecret))
	selected := &testAdapter{kind: g.Adapter, effect: []byte("safe-effect")}
	other := &testAdapter{kind: "email.send", effect: []byte("wrong-effect")}
	reg, _ := delegation.NewRegistry(selected, other)
	plane, _ := delegation.NewPlane(state, store, vault, reg, authority.NewMemoryLedger(), func() time.Time { return fixedNow })
	if err := rec(p, gauntlet.EventAttack, "adapter-confusion-attempt", "a second credential-bearing adapter existed beside the granted adapter"); err != nil { return err }
	if _, err := plane.Execute(ctx, baseIntent()); err != nil { return err }
	if selected.exec.Load() != 1 || other.exec.Load() != 0 { return errors.New("grant did not control adapter selection") }
	if err := rec(p, gauntlet.EventBoundary, "grant-owned-adapter", "adapter kind was resolved from the immutable host grant"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "grant-selected-adapter", "alternate adapter remained uncalled")
}

func genericHTTP(_ context.Context, p *gauntlet.Probe) error {
	if err := rec(p, gauntlet.EventAttack, "generic-http-registration", "attempted to register a generic authenticated HTTP adapter"); err != nil { return err }
	_, err := delegation.NewRegistry(&testAdapter{kind: "authenticated-http"})
	if !errors.Is(err, delegation.ErrGenericAdapter) { return errors.New("generic authenticated transport registered") }
	if err := rec(p, gauntlet.EventBoundary, "typed-adapter-registry", "registry enforced typed adapter kinds"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "generic-http-denied", "generic authenticated HTTP was rejected")
}

func actionCollision(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(false, delegation.ReconcileUnknown)
	if err != nil { return err }; defer cleanup(); adapter.effect = []byte("safe-effect")
	if _, err := plane.Execute(ctx, in); err != nil { return err }
	if err := rec(p, gauntlet.EventAttack, "action-rebind-attempt", "same action id was reused with a different payload"); err != nil { return err }
	in.Payload = []byte("different-payload")
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, authority.ErrActionCollision) || adapter.exec.Load() != 1 { return errors.New("action id rebinding was not contained") }
	if err := rec(p, gauntlet.EventBoundary, "effect-ledger-binding", "ledger bound action id to request digest"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "collision-denied", "second request was denied without provider execution")
}

func uncertainReplay(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(true, delegation.ReconcileUnknown)
	if err != nil { return err }; defer cleanup(); adapter.execErr = errors.New("lost provider response")
	_, _ = plane.Execute(ctx, in)
	if err := rec(p, gauntlet.EventAttack, "uncertain-replay-attempt", "same uncertain action was submitted again"); err != nil { return err }
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, authority.ErrActionUncertain) || adapter.exec.Load() != 1 { return errors.New("uncertain action replayed") }
	if err := rec(p, gauntlet.EventBoundary, "pending-effect-ledger", "pending journal state survived provider uncertainty"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "uncertain-replay-denied", "automatic replay was denied")
}

func reconcileObserved(ctx context.Context, p *gauntlet.Probe) error {
	plane, state, adapter, in, cleanup, err := newPlane(true, delegation.ReconcileObserved)
	if err != nil { return err }; defer cleanup(); adapter.execErr = errors.New("lost provider response"); adapter.reconcileEvidence = []byte("provider-observed-effect")
	_, _ = plane.Execute(ctx, in)
	if err := planeStore(plane).Revoke(in.DelegationID); err != nil { return err }; state.AdvanceEpoch()
	before := adapter.exec.Load()
	if err := rec(p, gauntlet.EventAttack, "historical-reconcile", "reconciliation ran after revocation and epoch advance"); err != nil { return err }
	r, err := plane.Reconcile(ctx, in)
	if err != nil || r.EffectDigest == "" || adapter.exec.Load() != before || adapter.reconcile.Load() != 1 { return errors.New("observed reconciliation did not resolve safely") }
	if _, err := plane.Reconcile(ctx, in); err != nil || adapter.reconcile.Load() != 1 { return errors.New("completed reconciliation was not idempotent") }
	if err := rec(p, gauntlet.EventBoundary, "historical-observation-only", "reconciliation inspected provider state without executing the requested effect"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "observed-resolved-no-reexecute", "pending action closed without a second Execute call")
}

func reconcileAbsent(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(true, delegation.ReconcileAbsent)
	if err != nil { return err }; defer cleanup(); adapter.execErr = errors.New("lost provider response")
	_, _ = plane.Execute(ctx, in)
	if err := rec(p, gauntlet.EventAttack, "absent-reconcile", "provider reconciliation reported that effect was absent"); err != nil { return err }
	_, err = plane.Reconcile(ctx, in)
	if !errors.Is(err, delegation.ErrEffectAbsent) { return errors.New("absent result was accepted as completion") }
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, authority.ErrActionUncertain) || adapter.exec.Load() != 1 { return errors.New("absent result enabled replay") }
	if err := rec(p, gauntlet.EventBoundary, "pending-remains-pending", "absent reconciliation did not mutate pending ledger state"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "absent-still-pending", "automatic retry remained denied")
}

func secretEcho(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(true, delegation.ReconcileUnknown)
	if err != nil { return err }; defer cleanup(); adapter.echo = true
	if err := rec(p, gauntlet.EventAttack, "credential-echo", "adapter attempted to return exact credential bytes as evidence"); err != nil { return err }
	_, err = plane.Execute(ctx, in)
	if !errors.Is(err, delegation.ErrSecretLeak) || bytesInError(err, SyntheticSecret) { return errors.New("secret echo was not sanitized") }
	_, retryErr := plane.Execute(ctx, in)
	if !errors.Is(retryErr, authority.ErrActionUncertain) || adapter.exec.Load() != 1 { return errors.New("secret leak failure did not quarantine action") }
	if err := rec(p, gauntlet.EventBoundary, "secret-echo-filter", "host plane inspected provider evidence before receipt creation"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "secret-echo-denied", "credential-bearing evidence was rejected and action remained uncertain")
}

func secretEvidenceAbsence(ctx context.Context, p *gauntlet.Probe) error {
	plane, _, adapter, in, cleanup, err := newPlane(false, delegation.ReconcileUnknown)
	if err != nil { return err }; defer cleanup(); adapter.effect = []byte("public-effect-proof")
	if err := rec(p, gauntlet.EventAttack, "receipt-secret-scan", "synthetic credential was active while a successful receipt was produced"); err != nil { return err }
	r, err := plane.Execute(ctx, in); if err != nil { return err }
	raw, err := json.Marshal(r); if err != nil { return err }
	if contains(raw, []byte(SyntheticSecret)) { return errors.New("receipt contained credential material") }
	if err := rec(p, gauntlet.EventBoundary, "opaque-secret-handle-digest", "receipt bound only the opaque secret-handle digest"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "secret-absent-from-receipt", "synthetic credential bytes were absent from agent-visible receipt")
}

func journalRestartRevocation(_ context.Context, p *gauntlet.Probe) error {
	path := filepath.Join(os.TempDir(), "nolane-v6-gauntlet-restart.jsonl")
	_ = os.Remove(path); _ = os.Remove(path + ".lock")
	defer os.Remove(path)
	s, err := delegation.OpenJournalStore(path); if err != nil { return err }
	g := baseGrant(); if err := s.Issue(g); err != nil { _ = s.Close(); return err }; if err := s.Revoke(g.ID); err != nil { _ = s.Close(); return err }; if err := s.Close(); err != nil { return err }
	if err := rec(p, gauntlet.EventAttack, "restart-after-revoke", "host grant journal was closed and reopened after revocation"); err != nil { return err }
	s, err = delegation.OpenJournalStore(path); if err != nil { return err }; defer s.Close()
	state, err := s.Lookup(g.ID); if err != nil || !state.Revoked { return errors.New("revocation did not survive restart") }
	if err := rec(p, gauntlet.EventBoundary, "durable-grant-journal", "hash-chained host journal replayed issue and revoke transitions"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "revocation-survived-restart", "recovered grant remained revoked")
}

func journalTamper(_ context.Context, p *gauntlet.Probe) error {
	path := filepath.Join(os.TempDir(), "nolane-v6-gauntlet-tamper.jsonl")
	_ = os.Remove(path); defer os.Remove(path)
	s, err := delegation.OpenJournalStore(path); if err != nil { return err }; if err := s.Issue(baseGrant()); err != nil { _ = s.Close(); return err }; if err := s.Close(); err != nil { return err }
	raw, err := os.ReadFile(path); if err != nil { return err }; if len(raw) < 8 { return errors.New("journal too small") }; raw[len(raw)/2] ^= 1; if err := os.WriteFile(path, raw, 0o600); err != nil { return err }
	if err := rec(p, gauntlet.EventAttack, "journal-byte-tamper", "one persisted journal byte was modified before recovery"); err != nil { return err }
	_, err = delegation.OpenJournalStore(path); if !errors.Is(err, delegation.ErrStoreCorrupt) { return errors.New("tampered journal was trusted") }
	if err := rec(p, gauntlet.EventBoundary, "hash-chain-replay", "recovery recomputed the journal hash chain"); err != nil { return err }
	return rec(p, gauntlet.EventDenial, "tamper-denied", "tampered delegation state failed closed")
}

func baseGrant() delegation.Grant {
	return delegation.Grant{ID: "grant-v6", WorldID: "v6-world", AuthorityEpoch: 1, Adapter: "github.repo.write", Resource: "repo:Nolane-x/Nolane-sandbox", Operations: []delegation.Operation{"contents.write"}, SecretHandle: "handle-v6", IssuedAt: fixedNow.Add(-time.Hour), ExpiresAt: fixedNow.Add(time.Hour)}
}
func baseIntent() delegation.Intent { return delegation.Intent{WorldID: "v6-world", AuthorityEpoch: 1, ActionID: "action-v6", DelegationID: "grant-v6", Operation: "contents.write", Resource: "repo:Nolane-x/Nolane-sandbox", Payload: []byte("patch-v6")} }

func newPlane(durable bool, reconcile delegation.ReconcileState) (*delegation.Plane, *world.State, *testAdapter, delegation.Intent, func(), error) {
	state, err := world.NewState("v6-world"); if err != nil { return nil, nil, nil, delegation.Intent{}, func(){}, err }
	store := delegation.NewMemoryStore(); g := baseGrant(); if err := store.Issue(g); err != nil { return nil, nil, nil, delegation.Intent{}, func(){}, err }
	vault := delegation.NewMemoryVault(); if err := vault.Put(g.SecretHandle, []byte(SyntheticSecret)); err != nil { return nil, nil, nil, delegation.Intent{}, func(){}, err }
	adapter := &testAdapter{kind: g.Adapter, effect: []byte("effect-v6"), reconcileState: reconcile}
	registry, err := delegation.NewRegistry(adapter); if err != nil { return nil, nil, nil, delegation.Intent{}, func(){}, err }
	var ledger authority.Ledger = authority.NewMemoryLedger(); cleanup := func(){}
	if durable {
		path := filepath.Join(os.TempDir(), "nolane-v6-effect-"+string(reconcile)+".jsonl"); _ = os.Remove(path)
		j, err := authority.OpenJournalLedger(path); if err != nil { return nil, nil, nil, delegation.Intent{}, func(){}, err }; ledger = j; cleanup = func(){ _ = j.Close(); _ = os.Remove(path) }
	}
	plane, err := delegation.NewPlane(state, store, vault, registry, ledger, func() time.Time { return fixedNow }); if err != nil { cleanup(); return nil, nil, nil, delegation.Intent{}, func(){}, err }
	stores[plane] = store
	return plane, state, adapter, baseIntent(), cleanup, nil
}

var stores = make(map[*delegation.Plane]*delegation.MemoryStore)
func planeStore(p *delegation.Plane) *delegation.MemoryStore { return stores[p] }

type testAdapter struct { kind delegation.AdapterKind; effect []byte; execErr error; echo bool; reconcileState delegation.ReconcileState; reconcileEvidence []byte; exec atomic.Int32; reconcile atomic.Int32 }
func (a *testAdapter) Kind() delegation.AdapterKind { return a.kind }
func (a *testAdapter) Execute(_ context.Context, _ delegation.AdapterRequest, s delegation.Secret) (delegation.Effect, error) { a.exec.Add(1); if a.execErr != nil { return delegation.Effect{}, a.execErr }; if a.echo { return delegation.Effect{Evidence: s.Bytes()}, nil }; return delegation.Effect{Evidence: append([]byte(nil), a.effect...)}, nil }
func (a *testAdapter) Reconcile(_ context.Context, _ delegation.AdapterRequest, _ delegation.Secret) (delegation.ReconcileResult, error) { a.reconcile.Add(1); return delegation.ReconcileResult{State: a.reconcileState, Evidence: append([]byte(nil), a.reconcileEvidence...)}, nil }

func rec(p *gauntlet.Probe, kind gauntlet.EventKind, marker, detail string) error { return p.Record(kind, marker, detail) }
func contains(haystack, needle []byte) bool { if len(needle)==0 || len(haystack)<len(needle) { return false }; for i:=0; i+len(needle)<=len(haystack); i++ { match:=true; for j:=range needle { if haystack[i+j]!=needle[j] { match=false; break } }; if match { return true } }; return false }
func bytesInError(err error, secret string) bool { return err != nil && contains([]byte(err.Error()), []byte(secret)) }
