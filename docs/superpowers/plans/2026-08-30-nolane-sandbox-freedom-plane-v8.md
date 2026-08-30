# Nolane Sandbox Freedom Plane / Reality Membrane v8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an agent-native Freedom Realm that hides Cube realization details behind stable Realm/World/Service identities, bounded guest execution, lease-fenced local capacity, fail-honest capabilities, and an explicit Reality Membrane without weakening v0-v7 authority semantics.

**Architecture:** Add host-owned `realm`, `fabric`, `agentruntime`, and `membrane` packages above the existing `control.Manager` and `substrate` interfaces. Reuse `world.ID` as the only World identity authority, reuse Manager rollback for authority advancement, and reuse Cube guest execution through a new host-only `substrate.GuestRuntime`; deterministic v8 evidence is a new family and must not mutate v4/v6/v7 evidence.

**Tech Stack:** Go 1.x module under `NolaneWorld`, existing CubeSandbox HTTP/Connect adapter, append-only SHA-256 JSONL persistence conventions, GitHub Actions, existing `gauntlet` deterministic evidence framework.

**Spec:** `docs/superpowers/specs/2026-08-30-nolane-sandbox-freedom-plane-reality-membrane-v8-design.md`

## Global Constraints

- Do not modify CubeSandbox hypervisor, RustVMM/KVM, CubeNet, CubeEgress, CubeCoW, guest kernel, or upstream security core for v8.
- World identity is always existing `world.ID`; do not introduce a parallel World-ID type.
- Agent-facing records never expose `substrate.Handle`, sandbox IDs, Cube API keys, envd tokens, traffic tokens, provider credentials, `SecretHandle`, or raw host paths.
- `Runtime` cannot create/update Realm policy. Realm administration is host-only.
- Realm profiles are only `R0_INTERNAL_ONLY`, `R1_PUBLIC_READ`, and `R2_SUPPLY_CHAIN`; N3-N5 remain v6/v7 delegated/provider effects.
- No public ingress is introduced.
- Same-World rollback must use existing `control.Manager.Rollback`, preserving authority-epoch advancement before execution restoration.
- Capacity accounting must never be labeled kernel enforcement without evidence.
- Sanitized startup optimization uses identity-free baseline/template/snapshot identities; live assigned sandboxes are never rebound to a new World identity.
- Ambiguous create/exec/destroy outcomes are not manufactured into success.
- Existing v4/v6/v7 evidence bytes remain canonical and v5 remains a separate live evidence family.
- AI-generated commits include `Autonomously-by: ChatGPT:GPT-5.6-Sol` and never add `Signed-off-by`.

---

## File Structure

Create focused packages:

- `NolaneWorld/realm/model.go` — semantic IDs, Realm/World/Checkpoint/Service records and validation.
- `NolaneWorld/realm/store.go` — store interfaces and in-memory reference implementation.
- `NolaneWorld/realm/durable.go` + lock files — strict append-only hash-chained Realm state persistence.
- `NolaneWorld/realm/controller.go` — host-only Realm administration.
- `NolaneWorld/realm/service.go` — service-generation semantics.
- `NolaneWorld/fabric/capacity.go` — observations, reservations and idempotency.
- `NolaneWorld/fabric/lease.go` — World lease generation/expiry fencing.
- `NolaneWorld/fabric/baseline.go` — identity-free baseline admission/catalog.
- `NolaneWorld/fabric/fabric.go` — local Cube-backed realization lifecycle above `control.Manager`.
- `NolaneWorld/substrate/guest.go` — provider-neutral host-only guest execution contract.
- `NolaneWorld/substrate/cube/guest.go` — Cube implementation using existing Connect/envd path without exposing tokens.
- `NolaneWorld/agentruntime/runtime.go` — compact agent-facing Enter/Acquire/Spawn/Release facade.
- `NolaneWorld/agentruntime/exec.go` — lease/revision-fenced bounded guest execution and idempotent receipts.
- `NolaneWorld/agentruntime/checkpoint.go` — checkpoint/resume and authority-non-rewind.
- `NolaneWorld/agentruntime/capability.go` — fail-honest capability projection.
- `NolaneWorld/membrane/profile.go` — Realm network profiles and N0-N2 mapping.
- `NolaneWorld/membrane/gate.go` — crossing classification; authenticated/consequential lanes stay v6/v7.
- `NolaneWorld/gauntlet/freedom/*` — deterministic v8 adversarial suite.
- `NolaneWorld/cmd/nolane-freedom-gauntlet/main.go` — deterministic evidence CLI.

---

### Task 1: Realm semantic model, host controller, and strict durable state

**Files:**
- Create: `NolaneWorld/realm/model.go`
- Create: `NolaneWorld/realm/store.go`
- Create: `NolaneWorld/realm/controller.go`
- Create: `NolaneWorld/realm/durable.go`
- Create: `NolaneWorld/realm/durable_lock_unix.go`
- Create: `NolaneWorld/realm/durable_lock_other.go`
- Test: `NolaneWorld/realm/model_test.go`
- Test: `NolaneWorld/realm/durable_test.go`

**Interfaces:**

```go
type ID string
type ServiceID string
type SessionID string
type CheckpointID string

type NetworkProfile string
const (
    R0InternalOnly NetworkProfile = "R0_INTERNAL_ONLY"
    R1PublicRead NetworkProfile = "R1_PUBLIC_READ"
    R2SupplyChain NetworkProfile = "R2_SUPPLY_CHAIN"
)

type ResourceBudget struct { CPUUnits, MemoryMiB, DiskMiB uint64 }
type Spec struct {
    ID ID
    MaxWorlds uint32
    DefaultLease time.Duration
    NetworkProfile NetworkProfile
    ResourceBudget ResourceBudget
}

type RealmRecord struct { Spec Spec; Revision uint64; Closed bool }
type WorldPhase string
const (
    WorldRequested WorldPhase = "requested"
    WorldCreating WorldPhase = "creating"
    WorldObservedReady WorldPhase = "observed-ready"
    WorldLeased WorldPhase = "leased"
    WorldPaused WorldPhase = "paused"
    WorldTerminal WorldPhase = "terminal"
)

type WorldRecord struct {
    RealmID ID
    WorldID world.ID
    RealizationRevision uint64
    Phase WorldPhase
    LeaseGeneration uint64
    LeaseExpiresUnix int64
    Handle substrate.Handle // host-store only; never projected by agentruntime
    CapabilityDigest string
}
```

Store contract:

```go
type Store interface {
    CreateRealm(Spec) (RealmRecord, error)
    UpdateRealm(ID, uint64, Spec) (RealmRecord, error)
    CloseRealm(ID, uint64) error
    Realm(ID) (RealmRecord, bool)
    PutWorld(WorldRecord) error
    World(ID, world.ID) (WorldRecord, bool)
    PutCheckpoint(CheckpointRecord) error
    Checkpoint(CheckpointID) (CheckpointRecord, bool)
    PutService(ServiceRecord) error
    Service(ServiceID) (ServiceRecord, bool)
    RecordOperation(OperationRecord) error
    Operation(ID, string) (OperationRecord, bool)
    Close() error
}
```

Host administration:

```go
type Controller struct { store Store }
func (c *Controller) Create(ctx context.Context, spec Spec) (RealmRecord, error)
func (c *Controller) Update(ctx context.Context, id ID, expected uint64, spec Spec) (RealmRecord, error)
func (c *Controller) Close(ctx context.Context, id ID, expected uint64) error
```

- [ ] **Step 1: Write RED tests for canonical IDs/spec validation and host-only revision fencing.** Tests reject empty/oversized/ambiguous IDs, invalid profiles, `MaxWorlds==0`, negative/zero lease, update with stale revision, and any spec update that changes `Spec.ID`.
- [ ] **Step 2: Run `go test ./realm` and verify failure because Realm types/controller do not exist.**
- [ ] **Step 3: Implement minimal model/store/controller.** In-memory store must clone returned records and perform exact revision checks under a mutex.
- [ ] **Step 4: Add RED durable tests.** Create/reopen state; reject malformed JSON, unknown fields, duplicate sequence, hash mutation, transition after Realm close, wrong Realm/World binding, and non-canonical records; ensure synthetic token strings never appear unless explicitly stored in a forbidden-field test that must be rejected.
- [ ] **Step 5: Implement `OpenDurableStore(root string)` with mode `0700` directory, `0600` journal, fsync-before-index update, SHA-256 predecessor chain, strict single-object JSON decoding with `DisallowUnknownFields`, and lifetime OS lock.**
- [ ] **Step 6: Run `go test ./realm`, `go test -race ./realm`, and `go vet ./realm` until GREEN.**
- [ ] **Step 7: Commit Task 1.**

---

### Task 2: Capacity observations, reservations, leases, and sanitized baselines

**Files:**
- Create: `NolaneWorld/fabric/capacity.go`
- Create: `NolaneWorld/fabric/lease.go`
- Create: `NolaneWorld/fabric/baseline.go`
- Test: `NolaneWorld/fabric/capacity_test.go`
- Test: `NolaneWorld/fabric/lease_test.go`
- Test: `NolaneWorld/fabric/baseline_test.go`

**Interfaces:**

```go
type Observation struct { Revision uint64; Capacity realm.ResourceBudget; ObservedUnix int64 }
type Reservation struct {
    ID string
    RealmID realm.ID
    OperationID string
    RequestDigest string
    ObservationRevision uint64
    Units realm.ResourceBudget
    ExpiresUnix int64
}

type Lease struct {
    RealmID realm.ID
    WorldID world.ID
    Generation uint64
    ExpiresUnix int64
    RealizationRevision uint64
}

type Baseline struct {
    ID string
    Digest string
    TemplateRef string
    NetworkProfile realm.NetworkProfile
    Sanitized bool
    WorldIdentity string
    CheckpointOwner string
}
```

- [ ] **Step 1: RED tests for replay-before-admission.** Same operation ID + same digest returns the original reservation even when capacity is now full; same operation ID + changed digest returns `ErrOperationCollision`.
- [ ] **Step 2: RED tests for lease generation.** Generation zero invalid; stale generation rejected; expiry rejected; generation advance fences previous lease.
- [ ] **Step 3: RED baseline tests.** Reject `Sanitized=false`, any non-empty WorldIdentity/CheckpointOwner, invalid digest/profile/template; exact admitted baseline is immutable; profile selection cannot broaden Realm profile.
- [ ] **Step 4: Implement capacity/lease/baseline components with deterministic request digests and mutex-protected state.** Reservation accounting explicitly reports `EnforcementProven=false`.
- [ ] **Step 5: Run `go test -race ./fabric` and `go vet ./fabric` until GREEN.**
- [ ] **Step 6: Commit Task 2.**

---

### Task 3: Host-only GuestRuntime and hardened Cube bounded execution

**Files:**
- Create: `NolaneWorld/substrate/guest.go`
- Create: `NolaneWorld/substrate/cube/guest.go`
- Modify: `NolaneWorld/substrate/cube/live.go` only to delegate generic process execution helpers without changing v5 wire semantics.
- Test: `NolaneWorld/substrate/cube/guest_test.go`

**Interfaces:**

```go
type ProcessRequest struct {
    Command string
    Timeout time.Duration
    MaxOutputBytes int64
}
type ProcessObservation struct {
    ExitCode int
    Stdout []byte
    Stderr []byte
    StdoutTruncated bool
    StderrTruncated bool
    ObservationDigest string
}
type GuestRuntime interface {
    Exec(context.Context, Handle, ProcessRequest) (ProcessObservation, error)
}
```

- [ ] **Step 1: RED tests prove caller never receives guest tokens.** Use fake Cube control/data endpoints containing token canaries and assert `ProcessObservation`, errors, `%#v`, JSON serialization and observation digest contain none of them.
- [ ] **Step 2: RED tests for command/timeout/output bounds and redirects.** Invalid command/NUL/oversized command/zero or excessive max output fail before guest entry; Connect/envd redirect is never followed.
- [ ] **Step 3: Implement Cube `GuestRuntime.Exec` by `ConnectGuest` + existing Connect stream path, converting output into bounded byte slices and stable sentinels.** The concrete guest session remains private to the call.
- [ ] **Step 4: Preserve `RunCanary`/v5 behavior and run `go test -race ./substrate/...`.**
- [ ] **Step 5: Commit Task 3.**

---

### Task 4: Local Capacity Fabric realization lifecycle

**Files:**
- Create: `NolaneWorld/fabric/fabric.go`
- Create: `NolaneWorld/fabric/reconcile.go`
- Test: `NolaneWorld/fabric/fabric_test.go`

**Interfaces:**

```go
type WorldManager interface {
    Create(context.Context, world.ID) (substrate.Handle, error)
    Snapshot(context.Context, world.ID) (substrate.Snapshot, error)
    Rollback(context.Context, world.ID, substrate.Snapshot) error
    Clone(context.Context, world.ID, substrate.Snapshot, world.ID) (substrate.Handle, error)
    Destroy(context.Context, world.ID) error
    AuthorityState(world.ID) (world.AuthorityState, bool)
}

type Local struct { /* store, manager, reservations, leases, baselines */ }
func (f *Local) Acquire(context.Context, AcquireRequest) (Lease, error)
func (f *Local) Spawn(context.Context, SpawnRequest) (Lease, error)
func (f *Local) Release(context.Context, realm.ID, world.ID, uint64) error
func (f *Local) Handle(realm.ID, world.ID, uint64) (substrate.Handle, uint64, error)
func (f *Local) Checkpoint(context.Context, realm.ID, world.ID, uint64) (realm.CheckpointRecord, error)
func (f *Local) Resume(context.Context, realm.CheckpointID, uint64) (Lease, error)
```

- [ ] **Step 1: RED tests show Acquire uses `Manager.Create(ctx, exactWorldID)` even when a baseline is selected; no live-handle reuse path exists.**
- [ ] **Step 2: RED tests for max-world/resource admission, stale lease, terminal World, exact retry, changed-payload collision and create ambiguity.** A create error after possible provider entry must leave semantic World non-ready/uncertain, not return a Lease.
- [ ] **Step 3: Implement `Acquire/Spawn/Release/Handle` with World records and lease generation.** `Handle` is host-only and must never be reachable from `agentruntime` responses.
- [ ] **Step 4: RED checkpoint tests.** Checkpoint stores snapshot/authority epoch/realization revision; Resume same World calls Manager.Rollback and verifies new authority epoch is strictly greater before issuing a new lease; stale service registrations are invalidated.
- [ ] **Step 5: Implement Checkpoint/Resume using Manager methods; forked resume uses Manager.Clone with a caller-provided fresh `world.ID`.**
- [ ] **Step 6: Run `go test -race ./fabric ./control ./world` and `go vet ./fabric`.**
- [ ] **Step 7: Commit Task 4.**

---

### Task 5: Agent Experience Plane — Enter/Acquire/Exec/Spawn/Checkpoint/Resume/Release

**Files:**
- Create: `NolaneWorld/agentruntime/runtime.go`
- Create: `NolaneWorld/agentruntime/exec.go`
- Create: `NolaneWorld/agentruntime/checkpoint.go`
- Test: `NolaneWorld/agentruntime/runtime_test.go`
- Test: `NolaneWorld/agentruntime/exec_test.go`

**Interfaces:**

```go
type Session struct { ID realm.SessionID; RealmID realm.ID; RealmRevision uint64; PolicyDigest string }
type WorldLease struct { WorldID world.ID; Generation uint64; RealizationRevision uint64; ExpiresUnix int64 }
type ExecRequest struct {
    SessionID realm.SessionID
    RealmRevision uint64
    WorldID world.ID
    LeaseGeneration uint64
    ActionID string
    Command string
    Timeout time.Duration
    MaxOutputBytes int64
}
type ExecReceipt struct {
    ReceiptID string
    WorldID world.ID
    RealizationRevision uint64
    ExitCode int
    Stdout string
    Stderr string
    StdoutTruncated bool
    StderrTruncated bool
    ObservationDigest string
}
```

- [ ] **Step 1: RED test `Runtime` surface has no Realm-create/update method and Session/Lease/Receipt contain no substrate or token fields.** Reflection test fails if field names/types include `Handle`, `Sandbox`, `Token`, `Secret`, `Credential`, or Cube-specific types.
- [ ] **Step 2: RED Enter tests for stale/closed Realm and exact Realm revision binding.**
- [ ] **Step 3: Implement `Enter`, `Acquire`, `Spawn`, and `Release` as narrow projections over store/fabric.**
- [ ] **Step 4: RED Exec tests for stale session revision, stale lease, terminal World, action-ID collision, exact replay and guest error uncertainty.**
- [ ] **Step 5: Implement bounded Exec.** Compute semantic request digest before guest entry; exact completed replay returns prior receipt; changed request conflicts; raw guest/provider errors collapse to stable sentinels.
- [ ] **Step 6: Wire Checkpoint/Resume and assert checkpoint/snapshot/handle/token material never appears in agent responses except opaque `CheckpointID`.**
- [ ] **Step 7: Run `go test -race ./agentruntime ./fabric ./realm` and `go vet ./agentruntime`.**
- [ ] **Step 8: Commit Task 5.**

---

### Task 6: Fail-honest capability report, network profiles, and Reality Membrane

**Files:**
- Create: `NolaneWorld/agentruntime/capability.go`
- Create: `NolaneWorld/membrane/profile.go`
- Create: `NolaneWorld/membrane/gate.go`
- Test: `NolaneWorld/agentruntime/capability_test.go`
- Test: `NolaneWorld/membrane/profile_test.go`
- Test: `NolaneWorld/membrane/gate_test.go`

**Interfaces:**

```go
type ClaimState string
const (
    Verified ClaimState = "verified"
    AvailableUnproven ClaimState = "available-unproven"
    Unavailable ClaimState = "unavailable"
    NotApplicable ClaimState = "not-applicable"
)

type CapabilityReport struct {
    RealmID realm.ID
    RealmRevision uint64
    GuestExec Claim
    SnapshotRollback Claim
    PublicRead Claim
    PublicInbound Claim
    InternalMesh Claim
    FilesystemIsolation Claim
    ProcessIsolation Claim
    NetworkIsolation Claim
    ResourceEnforcement Claim
    AccountingBudget realm.ResourceBudget
    EvidenceDigest string
}
```

Membrane mapping:

```go
func NetworkClass(profile realm.NetworkProfile) network.Class
// R0 -> N0None, R1 -> N1PublicRead, R2 -> N2PublicSupplyChain
func AllowsRealmProfile(class network.Class) bool // true only N0-N2
```

- [ ] **Step 1: RED capability tests ensure requested features are not automatically `verified`; live mesh/resource enforcement remain unavailable/unproven unless explicit attestation is supplied.**
- [ ] **Step 2: RED profile tests ensure every v8 profile maps with public inbound false and N3/N4/N5 cannot be encoded as a Realm profile.**
- [ ] **Step 3: Implement deterministic capability projection and membrane classification.** Evidence digest excludes runtime noise and sensitive realization fields.
- [ ] **Step 4: Add Cube profile mapper using existing typed `UpdateNetwork`; agent never supplies raw `NetworkPolicy`.**
- [ ] **Step 5: Run `go test -race ./agentruntime ./membrane ./network ./substrate/cube`.**
- [ ] **Step 6: Commit Task 6.**

---

### Task 7: Service registry and realization-generation staleness

**Files:**
- Create: `NolaneWorld/realm/service.go`
- Test: `NolaneWorld/realm/service_test.go`
- Modify: `NolaneWorld/agentruntime/runtime.go`
- Test: `NolaneWorld/agentruntime/service_test.go`

**Interfaces:**

```go
type ServiceProtocol string
const (ServiceTCP ServiceProtocol="tcp"; ServiceUDP ServiceProtocol="udp"; ServiceHTTP ServiceProtocol="http")
type ServiceRecord struct {
    ID ServiceID
    RealmID ID
    WorldID world.ID
    RealizationRevision uint64
    Protocol ServiceProtocol
    Port uint16
    Generation uint64
    Ready bool
}
```

- [ ] **Step 1: RED canonical service-name tests reject traversal, percent/URL ambiguity, empty names, ports 0, unsupported protocol, cross-Realm owner.**
- [ ] **Step 2: RED staleness test: realization revision increment makes old registration non-ready/stale and cannot be returned as current.**
- [ ] **Step 3: Implement registry with generation increments and Runtime `RegisterService` projection; no public route or ingress mutation occurs.**
- [ ] **Step 4: Run `go test -race ./realm ./agentruntime`.**
- [ ] **Step 5: Commit Task 7.**

---

### Task 8: Restart/reconciliation and secret-free durable projections

**Files:**
- Modify: `NolaneWorld/realm/durable.go`
- Create: `NolaneWorld/fabric/reconcile_test.go`
- Test: `NolaneWorld/realm/recovery_test.go`

- [ ] **Step 1: RED restart tests persist Realm/World/lease/checkpoint/service/operation state, reopen, and prove no unreconciled realization becomes `observed-ready` merely because its row existed.**
- [ ] **Step 2: RED secret scan writes synthetic Cube/provider/envd/traffic canaries through all public v8 APIs and scans durable bytes for plaintext/base64/hex forms.**
- [ ] **Step 3: Implement recovery projection: active realization records reopen as `creating`/`unavailable` until the host provider reconciler supplies an observation; terminal stays terminal; lease generation never decreases.**
- [ ] **Step 4: Add strict canonical replay checks for every newly added event type and fail on unknown fields/trailing JSON.**
- [ ] **Step 5: Run `go test -race ./realm ./fabric ./agentruntime` and `go vet ./...`.**
- [ ] **Step 6: Commit Task 8.**

---

### Task 9: Freedom Gauntlet v8 deterministic evidence

**Files:**
- Create: `NolaneWorld/gauntlet/freedom/suite.go`
- Create: `NolaneWorld/gauntlet/freedom/scenarios.go`
- Test: `NolaneWorld/gauntlet/freedom/suite_test.go`
- Create: `NolaneWorld/cmd/nolane-freedom-gauntlet/main.go`
- Test: `NolaneWorld/cmd/nolane-freedom-gauntlet/main_test.go`

**Mandatory scenario IDs:**

1. `freedom.authority-noninheritance`
2. `freedom.agent-projection-secret-free`
3. `freedom.realm-policy-host-only`
4. `freedom.acquire-idempotency`
5. `freedom.acquire-collision`
6. `freedom.stale-lease-denial`
7. `freedom.terminal-world-denial`
8. `freedom.checkpoint-authority-nonrewind`
9. `freedom.baseline-identity-isolation`
10. `freedom.baseline-fresh-create`
11. `freedom.profile-no-public-ingress`
12. `freedom.profile-no-n3-n5`
13. `freedom.service-generation-stale`
14. `freedom.capability-fail-honest`
15. `freedom.persistence-tamper`
16. `freedom.restart-no-false-ready`
17. `freedom.exec-bounded-output`
18. `freedom.exec-uncertain-not-success`
19. `freedom.v4-v6-v7-nondrift`
20. `freedom.v5-unavailable-not-pass`

- [ ] **Step 1: RED contract test requires exactly the 20 stable IDs above and an approved report only when every mandatory scenario passes with proof-of-exercise markers.**
- [ ] **Step 2: Implement scenarios by invoking real v8 package boundaries, not hard-coded pass results.**
- [ ] **Step 3: Add determinism test: two suite runs marshal byte-identically.**
- [ ] **Step 4: Add secret-negative test for plaintext/base64/hex synthetic Cube/provider credentials.**
- [ ] **Step 5: Add hash guards for canonical v4/v6/v7 artifacts and verify v5 probe without live config cannot report PASS.**
- [ ] **Step 6: Implement CLI `go run ./cmd/nolane-freedom-gauntlet --out <path>` with verified deterministic JSON.**
- [ ] **Step 7: Run `go test -race ./gauntlet/freedom ./cmd/nolane-freedom-gauntlet` and full `go vet ./...`.**
- [ ] **Step 8: Commit Task 9.**

---

### Task 10: CI, docs, performance hooks, final release verification

**Files:**
- Modify: `.github/workflows/nolane-world-check.yml`
- Modify: `NolaneWorld/README.md`
- Create: `NolaneWorld/agentruntime/metrics.go`
- Test: `NolaneWorld/agentruntime/metrics_test.go`

- [ ] **Step 1: Add non-secret live timing hooks for Acquire/Exec/Checkpoint/Resume/Spawn.** Metrics contain semantic operation labels and durations only; no handles/tokens/credentials.
- [ ] **Step 2: Update CI to generate v8 evidence twice, `cmp` bytes, scan plaintext/base64/hex synthetic credentials, upload `nolane-freedom-v8` artifact, while leaving v4/v6/v7 commands unchanged.**
- [ ] **Step 3: Update README with Freedom Plane v8 capabilities and explicit non-claims: deterministic v8 does not prove live inter-VM mesh, KVM escape impossibility, or kernel resource enforcement.**
- [ ] **Step 4: Run exact final verification:**

```bash
cd NolaneWorld
gofmt -w $(find . -name '*.go' -type f)
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/nolane-gauntlet --out /tmp/nolane-v4.json
go run ./cmd/nolane-authority-gauntlet --out /tmp/nolane-v6.json
go run ./cmd/nolane-provider-gauntlet --out /tmp/nolane-v7.json
go run ./cmd/nolane-freedom-gauntlet --out /tmp/nolane-v8-a.json
go run ./cmd/nolane-freedom-gauntlet --out /tmp/nolane-v8-b.json
cmp /tmp/nolane-v8-a.json /tmp/nolane-v8-b.json
```

- [ ] **Step 5: Compare branch to `master`; confirm changed files are restricted to `NolaneWorld/**`, v8 docs and Nolane workflow, with zero Cube security-core changes.**
- [ ] **Step 6: Open/update PR, wait for exact-head Nolane World/Docs/Format gates, inspect v8 artifact bytes, and do not merge with a technical failure.**
- [ ] **Step 7: DCO remains human-only. If required, compact history to one exact-tree AI-attributed commit so the human need sign only one commit; never synthesize `Signed-off-by`.**
- [ ] **Step 8: Merge with exact head lock after all technical + human gates are satisfied, then verify push-triggered `master` checks and v8 artifact again.**
