# Nolane Sandbox Freedom Plane / Reality Membrane v8 — Design Specification

**Status:** approved architecture, implementation specification  
**Baseline:** `master` at `e27885a72850dd236c06d780070ad7506437b24a` (Provider Authority Runtime v7)  
**Primary objective:** make Nolane Sandbox substantially easier and faster for AI agents to use while preserving the existing rule that capability may grow freely inside a sandboxed world but external authority, trusted persistence, and real-world effects remain host-governed.

---

## 1. Product statement

Nolane Sandbox v8 turns a collection of disposable microVMs into an **agent-native execution realm**.

The AI should not have to reason in terms of Cube control-plane URLs, sandbox IDs, envd access tokens, traffic tokens, VM pause endpoints, process IDs, or provider credentials. Those are realization details owned by the host.

The agent should instead see a compact semantic surface:

- enter or create a realm;
- acquire an execution world;
- run work;
- create sibling worlds;
- publish internal services;
- checkpoint and resume execution state;
- inspect effective capabilities and limits;
- import public/supply-chain material through a governed boundary;
- export or perform real-world effects only through existing typed authority gates.

The security invariant remains:

> **Unbounded capability creation; bounded authority, promotion, truth, persistence, and reality effects.**

V8 adds a usability invariant:

> **The agent operates on stable semantic identities and desired outcomes; host/runtime realization details remain hidden and replaceable.**

---

## 2. Design sources and lessons

V8 deliberately borrows architectural lessons from two sibling Nolane systems without coupling Nolane Sandbox to their implementations.

### 2.1 Nolane Habitat lessons

Habitat demonstrates that agent UX improves when a large internal system is projected as a compact, task-oriented interface rather than exposing every primitive. Relevant principles adopted here are:

- compact high-level agent operations over a richer canonical internal protocol;
- explicit effective capability reporting instead of optimistic feature claims;
- durable checkpoints that bind real revision/provider state rather than trusting narrative memory;
- stable session/task identities across retries and resumptions;
- typed execution receipts and bounded output instead of treating raw stdout as the only control surface;
- fail-closed behavior when the active provider cannot prove a requested containment property.

V8 does **not** copy Habitat's semantic project memory, source authority, or cognition stack into Sandbox. Habitat understands projects; Sandbox executes hostile work. Their trust domains remain separate.

### 2.2 Nolane Compute lessons

Nolane Compute demonstrates a useful separation between semantic workload/capacity identity and concrete process realization. Relevant principles adopted here are:

- stable logical identity while PID/VM realization may change;
- desired state separated from provider apply;
- lease/revision fencing around mutable resources;
- idempotency bound to semantic request payloads;
- independent read-back before claiming an effect completed;
- ambiguous outcomes reconciled rather than blindly replayed;
- resource admission distinct from kernel/hypervisor enforcement;
- provider capability claims are explicit and fail-honest.

V8 intentionally implements only a small local realm fabric. It does not duplicate Nolane Compute's distributed scheduler, federation, autoscaling, or remote-host control plane. A future integration may let Nolane Compute place Realm worlds through a provider adapter.

---

## 3. Scope

V8 is one coherent milestone with four host-owned components:

1. **Freedom Realm** — stable realm/world/service identities and desired state.
2. **Agent Experience Plane** — compact agent-facing session/runtime API.
3. **Local Capacity Fabric** — reservations, leases, warm-world reuse, realization revision fencing, and fail-honest capability reporting.
4. **Reality Membrane** — explicit crossing rules between internal realm activity and public/external reality.

The implementation stays under `NolaneWorld/**` plus v8-specific CI/docs. It must not modify CubeSandbox's hypervisor, RustVMM/KVM, CubeNet, CubeEgress, CubeCoW, guest kernel, or upstream security core merely to satisfy v8.

---

## 4. Non-negotiable invariants

### NF-001 — Freedom is internal

Creating a process, compiler, service, language runtime, database, child agent, local package registry, internal protocol, or sibling world inside a Realm cannot mint external authority.

### NF-002 — Semantic identity outranks realization identity

Agent-visible identities are `realm://`, `world://`, and `service://` identities. Cube sandbox IDs, PIDs, envd tokens, traffic tokens, and host handles are host-private realization state.

### NF-003 — Reality authority is never inherited from capability

A world gaining more software capability does not gain a stronger Network Class, delegation grant, provider adapter, credential, export right, or promotion right.

### NF-004 — No guest credential possession

Cube API keys, provider credentials, broker secret handles, envd access tokens, and traffic access tokens may not be returned through the agent-facing v8 API or persisted in Realm records/evidence.

### NF-005 — Checkpoint is execution state, not authority state

Realm/world checkpoints may refer to execution snapshots but never restore old authority epochs, delegation grants, revocation state, trusted capability records, provider receipts, or external-effect history.

### NF-006 — Effective capabilities are observed claims

A requested feature is not a capability claim. `CapabilityReport` may claim execution, snapshot, network confinement, internal service connectivity, or resource enforcement only when the active substrate/provider has the required evidence.

### NF-007 — Unknown outcome is not success

If create/destroy/checkpoint/network mutation/exec has an ambiguous result, the fabric records uncertainty and requires read-back/reconciliation where available. It may not manufacture success from a timeout.

### NF-008 — Resource accounting is not enforcement

A reservation proves Nolane accounting only. CPU/memory/disk enforcement is claimed separately only when the active substrate proves it.

### NF-009 — Public read is not authenticated authority

N1/N2 public/supply-chain access may be available under explicit Realm policy without credentials. Authenticated reads and all writes remain behind v6/v7-style delegated authority or later typed provider adapters.

### NF-010 — Internal network freedom cannot become inbound public reachability

Internal Realm service publication is not public exposure. A `service://` registration is reachable only inside its Realm unless a future independently governed ingress adapter explicitly exports it.

### NF-011 — Agent ergonomics cannot bypass the Trust Kernel

High-level operations are projections over existing authority/control/substrate primitives. Convenience APIs may narrow or compose authority but never bypass epoch validation, lifecycle fencing, artifact gates, delegation, or effect journals.

### NF-012 — Release claims are evidence-profile specific

Passing deterministic v8 tests means the implementation satisfies the defined v8 contract profile. It is not a blanket claim that the product is unescapable or that live KVM/network behavior was verified without a matching live evidence artifact.

---

## 5. Target architecture

```text
                         REALITY
       public network / GitHub / host / credentials / humans
                            ^
                            |
                  +--------------------+
                  |  REALITY MEMBRANE  |
                  |--------------------|
                  | N1/N2 public read  |
                  | artifact export    |
                  | v6 delegation      |
                  | v7 typed providers |
                  +---------+----------+
                            |
                  no ambient authority
                            |
============================================================
                    NOLANE FREEDOM REALM

      +------------------------------------------------+
      |              Agent Experience Plane            |
      | enter | acquire | exec | spawn | checkpoint    |
      | resume | service | capabilities | release      |
      +-----------------------+------------------------+
                              |
      +-----------------------v------------------------+
      |               Local Capacity Fabric            |
      | reservations | leases | warm pool | revisions |
      | desired state | reconciliation | observations  |
      +-----------------------+------------------------+
                              |
      +-----------------------v------------------------+
      |                  Realm Mesh                    |
      | world://A        world://B        world://C    |
      | root/processes   root/processes   root/process |
      |      \              |              /           |
      |       +---------- service:// mesh --------+     |
      +-----------------------+------------------------+
                              |
      +-----------------------v------------------------+
      |       existing Nolane control + Cube adapter   |
      +------------------------------------------------+
```

The Realm is a logical trust/execution scope. It is not itself a new hypervisor object.

---

## 6. Freedom Realm model

### 6.1 Identities

Introduce:

```go
type RealmID string
type WorldID string
type ServiceID string
type SessionID string
```

Canonical external text forms:

- `realm://<opaque-id>`
- `world://<opaque-id>`
- `service://<realm>/<name>`
- `session://<opaque-id>`

Opaque IDs are generated/validated by the host. Agent-supplied display names are metadata and never host paths or authority identifiers.

### 6.2 Realm desired state

A Realm record contains only host-safe semantic state:

```go
type RealmSpec struct {
    ID              RealmID
    MaxWorlds       uint32
    DefaultLease    time.Duration
    NetworkProfile  NetworkProfile
    ResourceBudget  ResourceBudget
}
```

The initial v8 implementation is local/single-host. `MaxWorlds` and budgets are admission controls, not claims of kernel enforcement.

### 6.3 World record

A world has:

- semantic WorldID;
- current realization revision;
- lifecycle state;
- lease generation and expiry;
- optional current snapshot reference;
- effective capability report digest;
- host-private substrate handle;
- host-private guest connection material.

The agent never receives the last two fields.

### 6.4 Internal services

A Realm service registration binds:

- `ServiceID`;
- owning WorldID + exact realization revision;
- internal protocol (`tcp`, `udp`, or `http` in v8);
- guest port;
- health/readiness status;
- generation.

Registration is semantic metadata. It does not itself modify public ingress or grant egress authority.

A world restart/replacement increments realization revision and stales prior service registrations until they are observed ready again.

---

## 7. Agent Experience Plane

The agent-facing API should be intentionally small. The initial Go service interface is conceptually:

```go
type Runtime interface {
    Enter(ctx context.Context, req EnterRequest) (Session, error)
    Acquire(ctx context.Context, req AcquireRequest) (WorldLease, error)
    Exec(ctx context.Context, req ExecRequest) (ExecReceipt, error)
    Spawn(ctx context.Context, req SpawnRequest) (WorldLease, error)
    Checkpoint(ctx context.Context, req CheckpointRequest) (CheckpointReceipt, error)
    Resume(ctx context.Context, req ResumeRequest) (WorldLease, error)
    RegisterService(ctx context.Context, req ServiceRequest) (ServiceReceipt, error)
    Capabilities(ctx context.Context, req CapabilityRequest) (CapabilityReport, error)
    Release(ctx context.Context, req ReleaseRequest) error
}
```

### 7.1 Enter

`Enter` establishes a session bound to exact Realm revision and policy digest. It returns no privileged host material.

### 7.2 Acquire

`Acquire` accepts semantic requirements, not a Cube sandbox ID. Example requirements:

- execution shell required;
- snapshot required;
- public-read profile required;
- minimum accounting budget.

The fabric selects or creates a realization and returns a lease over `world://...`.

### 7.3 Exec

`Exec` in v8 is a bounded process operation over an acquired world.

Request fields include:

- SessionID;
- WorldID;
- exact lease generation;
- semantic action ID/idempotency key;
- command/argv contract;
- timeout;
- maximum output bytes.

Response includes:

- stable receipt ID;
- world realization revision;
- exit code;
- bounded stdout/stderr;
- truncation flags;
- duration;
- execution observation digest.

Raw Cube connect tokens are never present.

V8 does not expose a generic host shell. The command runs inside the Realm world.

### 7.4 Spawn

`Spawn` creates a sibling world inside the same Realm subject to Realm admission/budget. A child world receives internal Realm membership but no external grants by inheritance.

### 7.5 Checkpoint / Resume

A checkpoint binds:

- RealmID and Realm revision;
- WorldID and realization revision;
- current authority epoch reference;
- substrate snapshot identity held host-side;
- effective capability report digest;
- service generations;
- policy digest.

Resume always revalidates current authority/policy and advances through the existing rollback/clone lifecycle rules. The snapshot cannot restore stale authority.

### 7.6 Capabilities

`CapabilityReport` is the agent's discovery surface. It reports only verified/observed facts such as:

- process execution available;
- snapshot/rollback available;
- internal service registration available;
- network profile in effect;
- public read available or denied;
- public inbound denied;
- filesystem/process/network isolation claim and evidence family;
- accounting budgets;
- kernel resource enforcement: proven / unproven;
- live-substrate evidence commit/profile where available.

Unsupported or unproven features are explicit, not omitted optimistically.

---

## 8. Local Capacity Fabric

### 8.1 Why it exists

The current `SandboxSubstrate` is intentionally narrow and maps lifecycle actions directly to a realization. That is the right trust boundary, but it is too low-level for fluid agent use. V8 adds a host-owned fabric above it rather than widening the agent's authority over Cube.

### 8.2 Reservation

An acquire/spawn request first obtains an accounting reservation containing:

- reservation ID;
- RealmID;
- requested resource units;
- exact capacity-observation revision;
- expiry;
- semantic request digest.

Same operation ID + same digest is idempotent. Same operation ID + changed request is a hard conflict.

### 8.3 World leases

Agent use is fenced by a lease generation. Pause/resume/replacement/recovery may advance the generation. A stale lease cannot execute after a newer realization has been admitted.

### 8.4 Warm pool

To make sandbox usage materially smoother, v8 may retain a bounded pool of **unassigned, authority-empty** warm realizations.

Hard rules:

- no user/project secrets;
- no delegation grants;
- no previous world identity;
- no trusted-state inheritance;
- no agent files unless the realization is explicitly checkpoint-owned by that same world;
- assignment creates a fresh WorldID/authority context;
- returning a realization to the pool requires verified reset/destroy-and-recreate semantics; if reset cannot be proven, destroy it.

A warm pool is a latency optimization, never a trust shortcut.

### 8.5 Realization observations

The fabric distinguishes:

`requested -> creating -> observed-ready -> leased -> paused -> terminal`

Provider acknowledgement does not automatically equal `observed-ready` when independent read-back is available.

### 8.6 Future Compute integration boundary

The fabric exposes an internal provider interface so a future Nolane Compute adapter can place capacity without changing the agent API. V8 ships only the local Cube-backed provider.

---

## 9. Guest execution boundary

The current Cube adapter already has lifecycle operations and a host-side `GuestSession` for live execution. V8 should formalize a narrow execution interface rather than letting callers depend on Cube-specific token/session details.

Conceptual interface:

```go
type GuestRuntime interface {
    Connect(ctx context.Context, handle substrate.Handle) (GuestHandle, error)
    Exec(ctx context.Context, guest GuestHandle, req ProcessRequest) (ProcessObservation, error)
    Close(ctx context.Context, guest GuestHandle) error
}
```

`GuestHandle` is host-owned. Its concrete Cube implementation may contain envd/traffic tokens in memory, but the type must not be serializable into agent-facing records.

The initial implementation should reuse the hardened Connect protocol already exercised by the live gauntlet while moving generic bounded execution out of the v5-specific canary path.

---

## 10. Realm networking

### 10.1 Profiles

V8 defines semantic Realm profiles independent of Cube JSON spelling:

- `R0_INTERNAL_ONLY` — no public Internet; no public inbound.
- `R1_PUBLIC_READ` — public unauthenticated read where policy can prove the route is bounded; no public inbound.
- `R2_SUPPLY_CHAIN` — public package/source retrieval appropriate for build environments; no ambient credentials; no public inbound.

Authenticated read/write is not a Realm profile. It is a Reality Membrane effect using existing Network Class/delegation/provider authority semantics.

### 10.2 Internal mesh

World-to-world traffic inside one Realm may be unrestricted by Nolane's semantic policy while still subject to substrate capabilities and Realm isolation from other Realms.

If the current substrate cannot prove isolated internal Realm networking, `CapabilityReport` must mark mesh networking unavailable rather than simulating a security claim. The deterministic implementation may still support service metadata/registry before live network wiring exists.

### 10.3 Service discovery

V8's canonical service identity is stable across world replacement. Concrete IP/port/host mapping is realization metadata. DNS synthesis or proxy-based service discovery may be added behind the same interface once the Cube networking surface is proven.

---

## 11. Reality Membrane

The Reality Membrane is a classification/gateway boundary, not one giant proxy.

### 11.1 Public read lane

N1/N2 traffic may be admitted according to Realm policy when:

- no host/provider credential is attached;
- public inbound remains disabled;
- the active substrate reports the configured egress policy;
- the request does not cross into a typed authenticated provider operation.

Downloaded content remains hostile guest material. It is not trusted simply because the gateway allowed retrieval.

### 11.2 Authenticated/consequential lane

N3–N5 operations remain outside Realm freedom and route through existing v6/v7 authority systems:

`agent intent -> delegation resolver -> typed adapter -> brokered credential -> effect journal -> reconciliation`

V8 does not add generic authenticated HTTP.

### 11.3 Artifact export lane

Realm files do not become trusted host artifacts by path reference alone. Export must go through artifact admission/provenance, with durable general quarantine storage remaining a later production gate unless implemented separately.

### 11.4 Public ingress

V8 does not provide public ingress. Internal `service://` registration is not an export mechanism.

---

## 12. Persistence and recovery

V8 host state must follow the existing append-only/fail-closed persistence style.

Persist at minimum:

- Realm specs and revisions;
- world semantic identity + realization revision/lifecycle;
- lease generations;
- capacity reservations;
- checkpoint metadata references;
- service registrations/generations;
- semantic operation idempotency records.

Never persist:

- Cube API keys;
- envd access tokens;
- traffic access tokens;
- provider credential bytes;
- broker secret bytes.

After restart, a world realization is not assumed live because a record exists. Recovery must reconcile or mark it unavailable/terminal according to available observation capability.

---

## 13. Error model

Stable sentinel families should distinguish:

- invalid semantic request;
- stale Realm revision;
- stale world lease generation;
- admission/resource exhausted;
- capability unavailable/unproven;
- world terminal;
- realization unavailable;
- execution failed;
- execution output exceeded bound;
- operation collision;
- uncertain outcome;
- Reality Membrane denial;
- service registration stale.

Raw provider diagnostics, secrets, Cube tokens, host paths, and uncontrolled HTTP bodies must not appear in agent-facing errors.

---

## 14. Implementation structure

Expected new packages:

```text
NolaneWorld/
  realm/
    model.go
    store.go
    durable.go
    service.go
  fabric/
    capacity.go
    lease.go
    pool.go
    reconciler.go
  agentruntime/
    runtime.go
    capability.go
    exec.go
    checkpoint.go
  membrane/
    profile.go
    gate.go
  substrate/
    guest.go
    cube/
      guest.go
```

Existing packages reused rather than bypassed:

- `world`
- `control`
- `authority`
- `network`
- `artifact`
- `delegation`
- `providerhttp` / typed providers
- `substrate`
- `substrate/cube`
- v4/v5/v6/v7 gauntlet families.

Exact file decomposition may change during implementation if a smaller boundary is clearer, but package responsibilities above are normative.

---

## 15. TDD contract matrix

V8 implementation starts with failing tests for at least the following contracts:

1. agent never receives concrete substrate handle or guest tokens;
2. acquire exact retry is idempotent; changed-payload reuse conflicts;
3. stale lease generation cannot execute;
4. terminal world cannot execute or register a service;
5. spawned sibling world inherits Realm membership but not delegation/provider authority;
6. checkpoint/resume cannot restore stale authority epoch;
7. capability report refuses unproven containment claims;
8. public inbound remains denied in all v8 Realm profiles;
9. N3–N5 operation cannot be expressed as a Realm network profile;
10. service registration stales after realization revision changes;
11. warm-pool reuse requires authority-empty reset proof or destroys the realization;
12. capacity reservation replay is checked before fresh admission;
13. ambiguous create/exec result is not converted to success;
14. persistent Realm store detects malformed/tampered sequence/hash state;
15. persisted state contains no synthetic envd/traffic/provider secret canaries;
16. restart does not trust an unreconciled realization as ready;
17. bounded exec marks/truncates output or rejects overflow according to the declared contract;
18. Reality Membrane leaves existing v6/v7 authenticated-effect path authoritative;
19. v4/v6/v7 canonical evidence digests do not drift as a side effect of v8;
20. live v5 remains `UNAVAILABLE` rather than `PASS` without a real configured substrate.

Property/fuzz tests should target identity parsing, operation digest collisions, malformed persistent records, lease-generation transitions, service-name canonicalization, and capability-report mutation.

---

## 16. Release evidence

Add a new deterministic evidence family rather than mutating old evidence semantics:

`nolane-freedom-v8`

Mandatory scenario classes:

- authority non-inheritance;
- stale lease/revision denial;
- token/secret non-disclosure;
- warm-pool reset fencing;
- checkpoint authority non-rewind;
- network-profile reality boundary;
- service-generation staleness;
- capability fail-honesty;
- persistence tamper/restart;
- idempotency/uncertainty.

CI should generate v8 evidence twice and byte-compare it, then scan the artifact for plaintext/base64/hex encodings of synthetic Cube/provider credentials.

The v8 suite must also retain hash guards for canonical v4/v6/v7 deterministic evidence families.

A later live Realm-Mesh profile may extend v5/live evidence, but deterministic v8 success must not be mislabeled as proof of live inter-VM networking.

---

## 17. Performance and agent-ergonomics acceptance criteria

V8 is not only a security milestone. It must show that the agent-facing path is simpler and avoids needless realization churn.

Deterministic acceptance criteria:

- one semantic `Acquire` call replaces manual create/connect/token handling;
- one semantic `Exec` call replaces direct envd protocol handling for ordinary agent execution;
- a session can checkpoint/resume without the agent storing Cube identifiers;
- `Capabilities` provides one fail-honest discovery response;
- exact retries do not duplicate world creation or reservations;
- warm-pool hit performs no authority inheritance from the previous assignment;
- session/world/service identity remains stable across permitted realization changes.

Live benchmark metrics, when infrastructure is available, should record separately:

- acquire cold latency;
- acquire warm latency;
- first exec latency;
- repeated exec latency;
- checkpoint latency;
- resume latency;
- world spawn latency;
- peak host memory per idle warm realization.

No fixed latency number is claimed in deterministic CI. Performance claims require measured live evidence on a named environment/commit.

---

## 18. Explicit non-goals for v8

V8 does not claim or implement:

- a replacement hypervisor;
- a new guest kernel;
- production KMS/HSM attestation;
- generic authenticated HTTP;
- arbitrary public ingress;
- full Docker/Kubernetes/cloud orchestration;
- distributed consensus for Realm state;
- transparent multi-host migration;
- complete fake Internet/cloud simulation;
- Habitat's semantic project cognition inside Sandbox;
- Nolane Compute's full scheduler/federation stack;
- mathematical proof that KVM/kernel/hardware contains no unknown escape vulnerability.

---

## 19. Security claim after v8

If all deterministic v8 gates pass, Nolane Sandbox may claim:

> For the verified v8 contract profile, agent-visible execution is mediated through stable Realm/World/Service identities, lifecycle and lease fences, fail-honest capability reporting, bounded guest execution, and explicit Reality Membrane classes. Internal capability growth does not itself grant authenticated or consequential real-world authority.

It may not claim:

> Nolane Sandbox is unescapable, every live network path is verified, every external provider is trusted, or every resource budget is kernel-enforced.

Those require separate live/provider/backend evidence.

---

## 20. Implementation order

The implementation plan should follow this dependency order:

1. Realm identities/model + deterministic store contracts.
2. Lease/reservation state machine and idempotency.
3. host-only GuestRuntime interface over Cube guest execution.
4. Agent Runtime `Enter/Acquire/Exec/Release` core.
5. checkpoint/resume with authority non-rewind.
6. capability report.
7. network profile + Reality Membrane classification.
8. service registry/generation semantics.
9. bounded warm pool.
10. persistence/recovery hardening.
11. v8 deterministic gauntlet + CI evidence and secret scans.
12. live performance hooks/metrics without making unverified performance claims.

Each stage must preserve existing v0–v7 tests and evidence-family semantics.
