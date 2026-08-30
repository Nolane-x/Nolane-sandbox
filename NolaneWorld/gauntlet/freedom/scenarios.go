package freedomgauntlet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/fabric"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	delegationgauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/delegation"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	providergauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/provider"
	v4scenarios "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/scenarios"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/membrane"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/network"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

const (
	v4EvidenceSHA256 = "94ef192c57f2587d34a8340a8bfd8d297782e121c88ad4aa96792e42bf40c6f4"
	v6EvidenceSHA256 = "34705e6ce2128ce884447004257d22fe577ad0b98ef1cf91df0f57ae270148ce"
)

func attack(p *gauntlet.Probe, marker, detail string) error {
	return p.Record(gauntlet.EventAttack, marker, detail)
}
func defend(p *gauntlet.Probe, boundary, denial, detail string) error {
	if err := p.Record(gauntlet.EventBoundary, boundary, detail); err != nil {
		return err
	}
	return p.Record(gauntlet.EventDenial, denial, detail)
}

type simManager struct {
	creates []world.ID
	states  map[world.ID]world.AuthorityControl
	snaps   map[world.ID]substrate.Snapshot
}

func newSimManager() *simManager {
	return &simManager{states: map[world.ID]world.AuthorityControl{}, snaps: map[world.ID]substrate.Snapshot{}}
}
func (m *simManager) Create(_ context.Context, id world.ID) (substrate.Handle, error) {
	m.creates = append(m.creates, id)
	st, err := world.NewState(id)
	if err != nil { return "", err }
	m.states[id] = st
	return substrate.Handle("handle-" + string(id)), nil
}
func (m *simManager) Snapshot(_ context.Context, id world.ID) (substrate.Snapshot, error) {
	if _, ok := m.states[id]; !ok { return "", errors.New("missing world") }
	s := substrate.Snapshot("snapshot-" + string(id))
	m.snaps[id] = s
	return s, nil
}
func (m *simManager) Rollback(_ context.Context, id world.ID, snap substrate.Snapshot) error {
	if m.snaps[id] != snap { return errors.New("wrong snapshot") }
	state, ok := m.states[id]
	if !ok { return errors.New("missing authority state") }
	_, err := state.AdvanceAuthority()
	return err
}
func (m *simManager) Clone(_ context.Context, _ world.ID, _ substrate.Snapshot, child world.ID) (substrate.Handle, error) {
	return m.Create(context.Background(), child)
}
func (m *simManager) Destroy(_ context.Context, id world.ID) error {
	state, ok := m.states[id]
	if !ok { return errors.New("missing authority state") }
	_, err := state.CloseAuthority()
	return err
}
func (m *simManager) AuthorityState(id world.ID) (world.AuthorityState, bool) {
	state, ok := m.states[id]
	return state, ok
}

type simGuest struct {
	calls atomic.Int64
	err   error
}
func (g *simGuest) Exec(_ context.Context, _ substrate.Handle, req substrate.ProcessRequest) (substrate.ProcessObservation, error) {
	g.calls.Add(1)
	if g.err != nil { return substrate.ProcessObservation{}, g.err }
	n := req.MaxOutputBytes
	if n > 4096 { n = 4096 }
	return substrate.ProcessObservation{ExitCode: 0, Stdout: bytes.Repeat([]byte{'x'}, int(n)), StdoutTruncated: true, ObservationDigest: "observation-v8"}, nil
}

type harness struct {
	store   *realm.MemoryStore
	local   *fabric.Local
	manager *simManager
	guest   *simGuest
	runtime *agentruntime.Service
	session agentruntime.Session
	spec    realm.Spec
}

func newHarness(profile realm.NetworkProfile) (*harness, error) {
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID: realm.ID("realm://freedom"), MaxWorlds: 8, DefaultLease: time.Minute, NetworkProfile: profile, ResourceBudget: realm.ResourceBudget{CPUUnits: 8, MemoryMiB: 8192, DiskMiB: 16384}}
	if _, err := store.CreateRealm(spec); err != nil { return nil, err }
	capacity := fabric.NewCapacity()
	capacity.Observe(spec.ResourceBudget)
	baselines := fabric.NewBaselineCatalog()
	if err := baselines.Admit(fabric.Baseline{ID: "clean", Digest: strings.Repeat("a", 64), TemplateRef: "clean-template", NetworkProfile: profile, Sanitized: true}); err != nil { return nil, err }
	manager := newSimManager()
	local, err := fabric.NewLocal(store, manager, capacity, fabric.NewLeaseBook(), baselines)
	if err != nil { return nil, err }
	guest := &simGuest{}
	runtime, err := agentruntime.New(store, local, guest)
	if err != nil { return nil, err }
	session, err := runtime.Enter(context.Background(), agentruntime.EnterRequest{RealmID: spec.ID, ExpectedRevision: 1, PolicyDigest: "freedom-policy-v8"})
	if err != nil { return nil, err }
	return &harness{store: store, local: local, manager: manager, guest: guest, runtime: runtime, session: session, spec: spec}, nil
}

func acquireRequest(h *harness, id world.ID, operation string) fabric.AcquireRequest {
	return fabric.AcquireRequest{RealmID: h.spec.ID, RealmRevision: h.session.RealmRevision, WorldID: id, OperationID: operation, Units: realm.ResourceBudget{CPUUnits: 1, MemoryMiB: 256, DiskMiB: 512}, ExpiresUnix: time.Now().Add(10 * time.Minute).Unix()}
}

func runtimeAcquire(ctx context.Context, h *harness, id world.ID, operation string) (agentruntime.WorldLease, error) {
	r := acquireRequest(h, id, operation)
	return h.runtime.Acquire(ctx, agentruntime.AcquireRequest{SessionID: h.session.ID, RealmRevision: h.session.RealmRevision, WorldID: r.WorldID, OperationID: r.OperationID, Units: r.Units, ExpiresUnix: r.ExpiresUnix})
}

func authorityNoninheritance(_ context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "internal-capability-reality-attempt", "an internal Realm capability attempted to treat authenticated Reality access as inherited authority"); err != nil { return err }
	plan, err := membrane.Classify(network.N3AuthenticatedRead)
	if err != nil { return err }
	if plan.Lane != membrane.LaneDelegatedProvider || !plan.RequiresDelegation || !plan.RequiresTypedProvider || plan.AmbientCredentialsAllowed || plan.PublicInboundAllowed {
		return errors.New("Reality authority was inherited from Realm capability")
	}
	return defend(p, "reality-membrane-authority-boundary", "reality-authority-not-inherited", "N3 remained behind delegated typed-provider authority with no ambient credential or ingress grant")
}

func agentProjectionSecretFree(_ context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "agent-projection-inspection", "the complete agent-facing semantic projection was inspected for substrate handles and credential authority"); err != nil { return err }
	types := []reflect.Type{reflect.TypeOf(agentruntime.Session{}), reflect.TypeOf(agentruntime.WorldLease{}), reflect.TypeOf(agentruntime.ExecReceipt{}), reflect.TypeOf(agentruntime.CheckpointReceipt{}), reflect.TypeOf(agentruntime.ServiceReceipt{}), reflect.TypeOf(agentruntime.CapabilityReport{})}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			text := strings.ToLower(f.Name + " " + f.Type.String())
			for _, forbidden := range []string{"handle", "sandbox", "token", "secret", "credential", "envd", "traffic", "cube"} {
				if strings.Contains(text, forbidden) { return errors.New("agent projection exposes realization authority") }
			}
		}
	}
	raw, _ := json.Marshal(types[0].Name())
	if bytes.Contains(raw, []byte(SyntheticSecret)) { return errors.New("synthetic credential in semantic projection") }
	return defend(p, "semantic-projection-boundary", "agent-projection-secret-free", "agent-facing types remained semantic and contained no realization handle or credential-bearing field")
}

func realmPolicyHostOnly(_ context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "runtime-admin-surface-inspection", "the agent Runtime interface was inspected for Realm policy administration methods"); err != nil { return err }
	typ := reflect.TypeOf((*agentruntime.Runtime)(nil)).Elem()
	required := map[string]bool{"Enter": false, "Acquire": false, "Exec": false, "Spawn": false, "Checkpoint": false, "Resume": false, "RegisterService": false, "Capabilities": false, "Release": false}
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		lower := strings.ToLower(name)
		if strings.Contains(lower, "createrealm") || strings.Contains(lower, "updaterealm") || strings.Contains(lower, "closerealm") { return errors.New("Realm administration escaped into agent runtime") }
		if _, ok := required[name]; ok { required[name] = true }
	}
	for _, ok := range required { if !ok { return errors.New("semantic Runtime surface incomplete") } }
	return defend(p, "host-owned-realm-controller", "realm-policy-host-only", "agent runtime exposed Realm use operations but no Realm create/update/close policy authority")
}

func acquireIdempotency(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	req := acquireRequest(h, world.ID("world-a"), "acquire-idempotent")
	if err := attack(p, "acquire-replay", "an identical completed Acquire request was replayed with the same operation identity"); err != nil { return err }
	first, err := h.local.Acquire(ctx, req); if err != nil { return err }
	second, err := h.local.Acquire(ctx, req); if err != nil { return err }
	if first != second || len(h.manager.creates) != 1 || h.manager.creates[0] != req.WorldID { return errors.New("Acquire replay created or rebound a realization") }
	return defend(p, "fabric-operation-ledger", "acquire-idempotent", "the operation journal returned the original lease and host Create was entered once")
}

func acquireCollision(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	req := acquireRequest(h, world.ID("world-a"), "acquire-collision")
	if _, err := h.local.Acquire(ctx, req); err != nil { return err }
	if err := attack(p, "acquire-operation-rebind", "a completed Acquire operation ID was reused after changing its resource budget"); err != nil { return err }
	changed := req; changed.Units.MemoryMiB++
	_, err = h.local.Acquire(ctx, changed)
	if !errors.Is(err, fabric.ErrOperationCollision) || len(h.manager.creates) != 1 { return errors.New("Acquire collision was not fenced before realization") }
	return defend(p, "fabric-request-digest-binding", "acquire-collision-denied", "the operation ID remained bound to its original canonical request digest")
}

func staleLeaseDenial(_ context.Context, p *gauntlet.Probe) error {
	book := fabric.NewLeaseBook()
	expires := time.Now().Add(time.Minute).Unix()
	first, err := book.Issue(realm.ID("realm://lease"), world.ID("world-a"), 1, expires); if err != nil { return err }
	if err := attack(p, "old-lease-reuse", "a prior lease generation was presented after a newer generation had been issued"); err != nil { return err }
	second, err := book.Issue(first.RealmID, first.WorldID, 2, expires); if err != nil { return err }
	if second.Generation <= first.Generation { return errors.New("lease generation did not advance") }
	if err := book.Validate(first, time.Now().Unix()); !errors.Is(err, fabric.ErrStaleLease) { return errors.New("stale lease remained valid") }
	return defend(p, "lease-generation-fence", "stale-lease-denied", "the LeaseBook accepted only the current generation and realization revision")
}

func terminalWorldDenial(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	lease, err := h.local.Acquire(ctx, acquireRequest(h, world.ID("world-a"), "terminal-acquire")); if err != nil { return err }
	if err := h.local.Release(ctx, lease.RealmID, lease.WorldID, lease.Generation); err != nil { return err }
	if err := attack(p, "terminal-handle-reuse", "a released World's former lease generation requested its host realization handle"); err != nil { return err }
	handle, _, err := h.local.Handle(lease.RealmID, lease.WorldID, lease.Generation)
	if !errors.Is(err, fabric.ErrWorldTerminal) || handle != "" { return errors.New("terminal World exposed a realization") }
	return defend(p, "terminal-world-fence", "terminal-world-denied", "terminal phase prevented host handle recovery through the old lease")
}

func checkpointAuthorityNonrewind(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	lease, err := h.local.Acquire(ctx, acquireRequest(h, world.ID("world-a"), "checkpoint-acquire")); if err != nil { return err }
	cp, err := h.local.Checkpoint(ctx, lease.RealmID, lease.WorldID, lease.Generation); if err != nil { return err }
	if err := attack(p, "checkpoint-rollback-authority-revival", "a guest snapshot rollback attempted to revive the authority state associated with the old lease"); err != nil { return err }
	resumed, err := h.local.Resume(ctx, cp.ID, h.session.RealmRevision); if err != nil { return err }
	state, ok := h.manager.AuthorityState(lease.WorldID); if !ok { return errors.New("authority state missing") }
	if state.CurrentEpoch() <= cp.AuthorityEpoch || resumed.Generation <= lease.Generation || resumed.RealizationRevision <= lease.RealizationRevision { return errors.New("checkpoint resume rewound an authority fence") }
	if err := h.local.ValidateLease(lease, time.Now().Unix()); !errors.Is(err, fabric.ErrStaleLease) { return errors.New("old lease survived checkpoint resume") }
	return defend(p, "host-authority-monotonicity", "checkpoint-authority-advanced", "host epoch, realization revision, and lease generation advanced across rollback")
}

func baselineIdentityIsolation(_ context.Context, p *gauntlet.Probe) error {
	catalog := fabric.NewBaselineCatalog()
	base := fabric.Baseline{ID: "clean", Digest: strings.Repeat("a", 64), TemplateRef: "template-clean", NetworkProfile: realm.R0InternalOnly, Sanitized: true}
	if err := catalog.Admit(base); err != nil { return err }
	if err := attack(p, "identity-bearing-baseline", "baseline admission was attempted with World identity and checkpoint ownership attached"); err != nil { return err }
	badWorld := base; badWorld.ID = "bad-world"; badWorld.WorldIdentity = "world-a"
	badCheckpoint := base; badCheckpoint.ID = "bad-checkpoint"; badCheckpoint.CheckpointOwner = "world-a"
	if !errors.Is(catalog.Admit(badWorld), fabric.ErrInvalidBaseline) || !errors.Is(catalog.Admit(badCheckpoint), fabric.ErrInvalidBaseline) { return errors.New("identity-bearing baseline was admitted") }
	selected, ok := catalog.Select(realm.R0InternalOnly)
	if !ok || selected.WorldIdentity != "" || selected.CheckpointOwner != "" || !selected.Sanitized { return errors.New("selected baseline carries authority identity") }
	return defend(p, "sanitized-baseline-admission", "baseline-identity-denied", "baseline catalog required sanitized identity-free reusable material")
}

func baselineFreshCreate(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	if err := attack(p, "baseline-world-rebinding", "two distinct World IDs requested realization from the same reusable sanitized baseline"); err != nil { return err }
	a, err := h.local.Acquire(ctx, acquireRequest(h, world.ID("world-a"), "baseline-a")); if err != nil { return err }
	b, err := h.local.Acquire(ctx, acquireRequest(h, world.ID("world-b"), "baseline-b")); if err != nil { return err }
	wa, _ := h.store.World(h.spec.ID, a.WorldID); wb, _ := h.store.World(h.spec.ID, b.WorldID)
	if len(h.manager.creates) != 2 || h.manager.creates[0] != a.WorldID || h.manager.creates[1] != b.WorldID || wa.Handle == wb.Handle || wa.BaselineID == "" || wa.BaselineID != wb.BaselineID { return errors.New("baseline reused a realized identity instead of fresh create") }
	return defend(p, "exact-world-create-boundary", "baseline-fresh-create", "the shared sanitized baseline selected configuration only; each World received a fresh exact-ID realization")
}

func profileNoPublicIngress(_ context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "realm-profile-ingress-attempt", "all supported Realm profiles were inspected for hidden raw Internet, public ingress, or ambient credential grants"); err != nil { return err }
	for _, profile := range []realm.NetworkProfile{realm.R0InternalOnly, realm.R1PublicRead, realm.R2SupplyChain} {
		plan, err := membrane.Plan(profile); if err != nil { return err }
		if plan.PublicInboundAllowed || plan.AmbientCredentialsAllowed || plan.RawPublicInternetAllowed { return errors.New("Realm profile granted Reality authority") }
		if (profile == realm.R1PublicRead || profile == realm.R2SupplyChain) && !plan.RequiresRealityGateway { return errors.New("public Realm profile bypassed governed gateway") }
	}
	return defend(p, "profile-to-membrane-boundary", "profile-ingress-denied", "R0-R2 remained fail-closed and R1/R2 required the governed Reality gateway")
}

func profileNoN3N5(_ context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "n3-n5-realm-escalation", "authenticated and consequential Reality classes were presented as if a Realm network profile could grant them"); err != nil { return err }
	for _, class := range []network.Class{network.N3AuthenticatedRead, network.N4ReversibleWrite, network.N5ConsequentialWrite} {
		if membrane.AllowsRealmClass(class) { return errors.New("N3-N5 accepted as Realm class") }
		if _, err := membrane.ProfileForClass(class); !errors.Is(err, membrane.ErrRealityAuthorityRequired) { return errors.New("N3-N5 did not require Reality authority") }
		plan, err := membrane.Classify(class); if err != nil { return err }
		if plan.Lane != membrane.LaneDelegatedProvider || !plan.RequiresDelegation || !plan.RequiresTypedProvider || plan.AmbientCredentialsAllowed || plan.PublicInboundAllowed { return errors.New("N3-N5 escaped delegated provider lane") }
	}
	return defend(p, "typed-reality-authority-boundary", "n3-n5-delegated", "N3-N5 remained outside Realm profiles and required delegated typed-provider authority")
}

func serviceGenerationStale(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	lease, err := h.local.Acquire(ctx, acquireRequest(h, world.ID("world-a"), "service-acquire")); if err != nil { return err }
	registry, err := realm.NewServiceRegistry(h.store); if err != nil { return err }
	first, err := registry.Register(realm.ServiceRequest{RealmID: h.spec.ID, WorldID: lease.WorldID, RealizationRevision: lease.RealizationRevision, Name: "api", Protocol: realm.ServiceHTTP, Port: 8080, Ready: true}); if err != nil { return err }
	if _, ok := registry.Current(first.ID); !ok { return errors.New("initial service was not current") }
	cp, err := h.local.Checkpoint(ctx, lease.RealmID, lease.WorldID, lease.Generation); if err != nil { return err }
	if err := attack(p, "stale-service-generation", "a ready internal service generation was retained while its World realization advanced through checkpoint resume"); err != nil { return err }
	resumed, err := h.local.Resume(ctx, cp.ID, h.session.RealmRevision); if err != nil { return err }
	if _, ok := registry.Current(first.ID); ok { return errors.New("stale service generation remained ready") }
	second, err := registry.Register(realm.ServiceRequest{RealmID: h.spec.ID, WorldID: resumed.WorldID, RealizationRevision: resumed.RealizationRevision, Name: "api", Protocol: realm.ServiceHTTP, Port: 8080, Ready: true}); if err != nil { return err }
	if second.Generation <= first.Generation { return errors.New("service generation did not advance") }
	return defend(p, "service-realization-revision-fence", "stale-service-not-ready", "service readiness was invalidated by realization revision and required a new generation")
}

func capabilityFailHonest(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R1PublicRead); if err != nil { return err }
	if err := attack(p, "boolean-attestation-upgrade", "provider availability and verification booleans were asserted without supplying evidence"); err != nil { return err }
	report, err := h.runtime.Capabilities(ctx, agentruntime.CapabilityRequest{SessionID: h.session.ID, RealmRevision: h.session.RealmRevision, Attestation: agentruntime.ProviderAttestation{GuestExecAvailable: true, GuestExecVerified: true, SnapshotAvailable: true, SnapshotVerified: true, PublicReadAvailable: true, PublicReadVerified: true, PublicInboundDisabled: true, InternalMeshAvailable: true, InternalMeshVerified: true, FilesystemIsolationVerified: true, ProcessIsolationVerified: true, NetworkIsolationVerified: true, ResourceEnforcementAvailable: true, ResourceEnforcementVerified: true}})
	if err != nil { return err }
	claims := []agentruntime.Claim{report.GuestExec, report.SnapshotRollback, report.PublicRead, report.PublicInbound, report.InternalMesh, report.FilesystemIsolation, report.ProcessIsolation, report.NetworkIsolation, report.ResourceEnforcement}
	for _, claim := range claims { if claim.State == agentruntime.Verified { return errors.New("capability was falsely verified without evidence") } }
	if report.GuestExec.State != agentruntime.AvailableUnproven || report.PublicRead.State != agentruntime.AvailableUnproven || report.EvidenceDigest == "" { return errors.New("fail-honest capability projection missing") }
	return defend(p, "evidence-gated-capability-projection", "capability-not-falsely-verified", "availability remained available-unproven and verification-only claims remained unavailable without evidence")
}

func persistenceTamper(_ context.Context, p *gauntlet.Probe) error {
	root, err := os.MkdirTemp("", "nolane-freedom-tamper-"); if err != nil { return err }; defer os.RemoveAll(root)
	store, err := realm.OpenDurableStore(root); if err != nil { return err }
	spec := realm.Spec{ID: realm.ID("realm://tamper"), MaxWorlds: 2, DefaultLease: time.Minute, NetworkProfile: realm.R0InternalOnly, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil { return err }
	if err := store.Close(); err != nil { return err }
	if err := attack(p, "journal-byte-tamper", "a persisted Realm journal record was modified after clean shutdown while retaining parseable JSON"); err != nil { return err }
	path := filepath.Join(root, "realm-state.jsonl")
	raw, err := os.ReadFile(path); if err != nil { return err }
	mutated := bytes.Replace(raw, []byte("realm://tamper"), []byte("realm://tamperx"), 1)
	if bytes.Equal(raw, mutated) { return errors.New("journal mutation did not apply") }
	if err := os.WriteFile(path, mutated, 0o600); err != nil { return err }
	if reopened, err := realm.OpenDurableStore(root); err == nil {
		_ = reopened.Close()
		return errors.New("tampered durable journal was accepted")
	} else if !errors.Is(err, realm.ErrStoreCorrupt) { return err }
	return defend(p, "strict-hash-chain-replay", "persistence-tamper-denied", "durable recovery rejected altered history instead of projecting it as trusted state")
}

func restartNoFalseReady(_ context.Context, p *gauntlet.Probe) error {
	root, err := os.MkdirTemp("", "nolane-freedom-restart-"); if err != nil { return err }; defer os.RemoveAll(root)
	store, err := realm.OpenDurableStore(root); if err != nil { return err }
	spec := realm.Spec{ID: realm.ID("realm://restart"), MaxWorlds: 2, DefaultLease: time.Minute, NetworkProfile: realm.R0InternalOnly, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil { return err }
	wr := realm.WorldRecord{RealmID: spec.ID, WorldID: world.ID("world-a"), RealizationRevision: 7, Phase: realm.WorldLeased, LeaseGeneration: 4, LeaseExpiresUnix: time.Now().Add(time.Hour).Unix(), Handle: substrate.Handle("stale-host-handle")}
	if err := store.PutWorld(wr); err != nil { return err }
	registry, err := realm.NewServiceRegistry(store); if err != nil { return err }
	service, err := registry.Register(realm.ServiceRequest{RealmID: spec.ID, WorldID: wr.WorldID, RealizationRevision: wr.RealizationRevision, Name: "api", Protocol: realm.ServiceHTTP, Port: 8080, Ready: true}); if err != nil { return err }
	if err := store.Close(); err != nil { return err }
	recovered, err := realm.OpenDurableStore(root); if err != nil { return err }
	before, _ := recovered.World(spec.ID, wr.WorldID)
	if before.Handle != wr.Handle || before.Phase != realm.WorldLeased { _ = recovered.Close(); return errors.New("durable replay did not preserve history") }
	if err := attack(p, "post-restart-stale-readiness", "historical leased realization and ready service state were present immediately after durable replay"); err != nil { _ = recovered.Close(); return err }
	capacity := fabric.NewCapacity(); capacity.Observe(spec.ResourceBudget)
	local, err := fabric.NewLocal(recovered, newSimManager(), capacity, fabric.NewLeaseBook(), fabric.NewBaselineCatalog()); if err != nil { _ = recovered.Close(); return err }
	if err := local.FenceRecoveredRealizations(); err != nil { _ = recovered.Close(); return err }
	after, _ := recovered.World(spec.ID, wr.WorldID)
	reg2, _ := realm.NewServiceRegistry(recovered)
	if after.Handle != "" || after.Phase != realm.WorldCreating { _ = recovered.Close(); return errors.New("stale realization survived recovery fence") }
	if _, ok := reg2.Current(service.ID); ok { _ = recovered.Close(); return errors.New("stale service readiness survived recovery fence") }
	if err := recovered.Close(); err != nil { return err }
	replayed, err := realm.OpenDurableStore(root); if err != nil { return err }; defer replayed.Close()
	historical, _ := replayed.World(spec.ID, wr.WorldID)
	historicalService, _ := replayed.Service(service.ID)
	if historical.Handle != wr.Handle || historical.Phase != realm.WorldLeased || !historicalService.Ready { return errors.New("recovery projection rewrote durable history") }
	return defend(p, "fabric-post-replay-recovery-fence", "restart-readiness-fenced", "Fabric invalidated stale realization readiness in memory while durable replay remained historical truth")
}

func execBoundedOutput(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	lease, err := runtimeAcquire(ctx, h, world.ID("world-a"), "exec-bound-acquire"); if err != nil { return err }
	if err := attack(p, "oversized-exec-output-request", "agent execution first used a four-byte observation budget and then requested more than the runtime maximum"); err != nil { return err }
	receipt, err := h.runtime.Exec(ctx, agentruntime.ExecRequest{SessionID: h.session.ID, RealmRevision: h.session.RealmRevision, WorldID: lease.WorldID, LeaseGeneration: lease.Generation, ActionID: "bounded", Command: "printf data", Timeout: time.Second, MaxOutputBytes: 4}); if err != nil { return err }
	if len(receipt.Stdout) != 4 || !receipt.StdoutTruncated || h.guest.calls.Load() != 1 { return errors.New("bounded observation contract not preserved") }
	_, err = h.runtime.Exec(ctx, agentruntime.ExecRequest{SessionID: h.session.ID, RealmRevision: h.session.RealmRevision, WorldID: lease.WorldID, LeaseGeneration: lease.Generation, ActionID: "oversized", Command: "printf data", Timeout: time.Second, MaxOutputBytes: (64 << 20) + 1})
	if !errors.Is(err, agentruntime.ErrInvalidRequest) || h.guest.calls.Load() != 1 { return errors.New("oversized output request reached guest runtime") }
	return defend(p, "process-request-output-budget", "exec-output-bounded", "bounded observation was returned and the oversized request was rejected before a second guest entry")
}

func execUncertainNotSuccess(ctx context.Context, p *gauntlet.Probe) error {
	h, err := newHarness(realm.R0InternalOnly); if err != nil { return err }
	lease, err := runtimeAcquire(ctx, h, world.ID("world-a"), "exec-uncertain-acquire"); if err != nil { return err }
	h.guest.err = errors.New("lost guest result")
	req := agentruntime.ExecRequest{SessionID: h.session.ID, RealmRevision: h.session.RealmRevision, WorldID: lease.WorldID, LeaseGeneration: lease.Generation, ActionID: "uncertain", Command: "do-work", Timeout: time.Second, MaxOutputBytes: 1024}
	if err := attack(p, "uncertain-exec-replay", "guest execution was entered but its trusted result was lost, then the same action was submitted again"); err != nil { return err }
	if _, err := h.runtime.Exec(ctx, req); !errors.Is(err, agentruntime.ErrExecUncertain) { return errors.New("first uncertain execution was not quarantined") }
	if _, err := h.runtime.Exec(ctx, req); !errors.Is(err, agentruntime.ErrExecUncertain) { return errors.New("uncertain execution replay was not denied") }
	if h.guest.calls.Load() != 1 { return errors.New("uncertain execution entered guest more than once") }
	op, ok := h.store.Operation(h.spec.ID, "exec:"+req.ActionID)
	if !ok || op.Status != "uncertain" { return errors.New("uncertain execution was recorded as success") }
	return defend(p, "exec-operation-uncertainty-ledger", "exec-uncertain-quarantined", "the pending action remained uncertain and automatic guest re-entry was denied")
}

func historicalNondrift(ctx context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "historical-evidence-regeneration", "v4, v6, and v7 deterministic evidence suites were regenerated while Freedom Plane v8 code was active"); err != nil { return err }
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 5 * time.Second}
	v4, err := v4scenarios.RunStandard(ctx, policy); if err != nil || !v4.Approved { return errors.New("v4 evidence no longer approved") }
	v4raw, err := gauntlet.MarshalReport(v4); if err != nil { return err }
	if digestBytes(v4raw) != v4EvidenceSHA256 { return errors.New("v4 canonical evidence drifted") }
	v6, err := delegationgauntlet.RunStandard(ctx, policy); if err != nil || !v6.Approved { return errors.New("v6 evidence no longer approved") }
	v6raw, err := gauntlet.MarshalReport(v6); if err != nil { return err }
	if digestBytes(v6raw) != v6EvidenceSHA256 { return errors.New("v6 canonical evidence drifted") }
	v7, err := providergauntlet.RunStandard(ctx, policy); if err != nil || !v7.Approved { return errors.New("v7 evidence no longer approved") }
	v7raw, err := gauntlet.MarshalReport(v7); if err != nil || len(v7raw) == 0 { return errors.New("v7 canonical evidence unavailable") }
	return defend(p, "historical-gauntlet-digest-boundary", "historical-evidence-stable", "v4 and v6 matched pinned canonical hashes and v7 regenerated as fully verified approved evidence")
}

func v5UnavailableNotPass(ctx context.Context, p *gauntlet.Probe) error {
	if err := attack(p, "missing-live-driver", "the live substrate gauntlet was executed without a configured driver or live endpoint"); err != nil { return err }
	report, err := (live.Runner{Mode: live.ModeProbe, Profile: live.ProfileCore}).Run(ctx, nil)
	if err != nil { return err }
	if report.Status != live.StatusUnavailable || report.Approved || report.Reason != live.ReasonConfigMissing { return errors.New("missing live evidence was upgraded into pass") }
	return defend(p, "live-evidence-status-boundary", "unavailable-not-pass", "missing live configuration produced UNAVAILABLE with Approved=false")
}

func digestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
