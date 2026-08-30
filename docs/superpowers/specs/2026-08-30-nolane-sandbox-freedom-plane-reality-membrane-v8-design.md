# Nolane Sandbox Freedom Plane / Reality Membrane v8 — Design Specification

**Status:** approved architecture, implementation specification  
**Baseline:** `master` at `e27885a72850dd236c06d780070ad7506437b24a` (Provider Authority Runtime v7)  
**Primary objective:** make Nolane Sandbox substantially easier and faster for AI agents to use while preserving the existing rule that capability may grow freely inside a sandboxed world but external authority, trusted persistence, and real-world effects remain host-governed.

---

## 1. Product statement

Nolane Sandbox v8 turns disposable Cube microVM realizations into an **agent-native execution Realm**.

The AI should not have to reason in terms of Cube control-plane URLs, sandbox IDs, envd access tokens, traffic tokens, VM pause endpoints, PIDs, or provider credentials. Those are host-owned realization details.

The agent should instead see a compact semantic surface:

- enter an authorized Realm;
- acquire an execution World;
- execute bounded guest work;
- spawn sibling Worlds within the Realm budget;
- publish internal service metadata;
- checkpoint and resume execution state;
- inspect effective capabilities and limits;
- use public/supply-chain network profiles only where explicitly admitted;
- cross into authenticated or consequential reality only through existing typed authority gates.

Realm creation and policy assignment are host/orchestrator operations. An agent may create additional Worlds only inside an already-authorized Realm and only within its admission envelope.

The security invariant remains:

> **Unbounded capability creation; bounded authority, promotion, truth, persistence, and reality effects.**

V8 adds a usability invariant:

> **The agent operates on stable semantic identities and desired outcomes; host/runtime realization details remain hidden and replaceable.**

---

## 2. Design sources and lessons

V8 borrows architectural lessons from two sibling Nolane systems without importing their implementations or trust domains.

### 2.1 Nolane Habitat lessons

Habitat demonstrates that agent UX improves when a large internal system is projected as a compact, task-oriented interface rather than exposing every primitive. V8 adopts:

- compact high-level agent operations over a richer canonical internal runtime;
- explicit effective capability reporting instead of optimistic feature claims;
- checkpoints bound to real revision/provider state rather than narrative memory;
- stable session identities across retries and resumptions;
- typed execution receipts and bounded output;
- fail-closed behavior when a provider cannot prove a requested property.

V8 does **not** copy Habitat's semantic project memory, source authority, or cognition stack into Sandbox. Habitat understands projects; Sandbox executes hostile work.

### 2.2 Nolane Compute lessons

Nolane Compute demonstrates a useful separation between semantic workload/capacity identity and concrete process realization. V8 adopts:

- stable logical identity while PID/VM realization may change;
- desired state separated from provider apply;
- lease/revision fencing around mutable resources;
- idempotency bound to semantic request payloads;
- independent observation before claiming completion where the substrate supports it;
- ambiguous outcomes reconciled rather than blindly replayed;
- resource admission distinct from kernel/hypervisor enforcement;
- fail-honest provider capability claims.

V8 intentionally implements only a small local Realm fabric. It does not duplicate Nolane Compute's distributed scheduler, federation, autoscaling, or remote-host control plane. A future Compute provider may place Worlds without changing the agent API.

---

## 3. Scope

V8 is one coherent milestone with four host-owned components:

1. **Freedom Realm** — Realm/service/session identities plus existing `world.ID` identities and desired state.
2. **Agent Experience Plane** — a compact agent-facing session/runtime API.
3. **Local Capacity Fabric** — reservations, leases, sanitized warm-start baselines, realization revision fencing, and fail-honest capability reporting.
4. **Reality Membrane** — explicit crossing rules between internal Realm activity and public/external reality.

Implementation remains under `NolaneWorld/**` plus v8-specific CI/docs. V8 must not modify CubeSandbox's hypervisor, RustVMM/KVM, CubeNet, CubeEgress, CubeCoW, guest kernel, or upstream security core merely to satisfy this milestone.

---

## 4. Non-negotiable invariants

### NF-001 — Freedom is internal

Creating a process, compiler, service, language runtime, database, child agent, local package registry, internal protocol, or sibling World inside a Realm cannot mint external authority.

### NF-002 — Semantic identity outranks realization identity

Agent-visible execution identities are Realm ID, existing Nolane `world.ID`, Service ID, and Session ID. Cube sandbox IDs, PIDs, envd tokens, traffic tokens, and substrate handles are host-private realization state.

### NF-003 — Reality authority is never inherited from capability

A World gaining more software capability does not gain a stronger Network Class, delegation grant, provider adapter, credential, export right, or promotion right.

### NF-004 — No agent credential possession

Cube API keys, provider credentials, broker secret handles, envd access tokens, and traffic access tokens may not be returned through the agent-facing v8 API or persisted in Realm records/evidence.

### NF-005 — Checkpoint is execution state, not authority state

Realm/World checkpoints may refer to execution snapshots but never restore old authority epochs, delegation grants, revocation state, trusted capability records, provider receipts, or external-effect history.

### NF-006 — Effective capabilities are observed claims

A requested feature is not a capability claim. `CapabilityReport` may claim execution, snapshot, network confinement, internal mesh reachability, or resource enforcement only when the active substrate/provider supplies the evidence required by that claim.

### NF-007 — Unknown outcome is not success

If create/destroy/checkpoint/network mutation/exec has an ambiguous result, the fabric records uncertainty and requires observation/reconciliation where available. It may not manufacture success from a timeout.

### NF-008 — Resource accounting is not enforcement

A reservation proves Nolane accounting only. CPU/memory/disk enforcement is claimed separately only when the active substrate proves it.

### NF-009 — Public read is not authenticated authority

N1/N2 public/supply-chain access may be available under explicit Realm policy without host/provider credentials. Authenticated reads and all writes remain behind v6/v7-style delegated authority or later typed provider adapters.

### NF-010 — Internal service publication is not public ingress

A `service://` registration is Realm metadata. It cannot itself create public reachability.

### NF-011 — Agent ergonomics cannot bypass the Trust Kernel

High-level operations compose existing authority/control/substrate primitives. Convenience APIs may narrow or compose authority but never bypass epoch validation, lifecycle fencing, artifact gates, delegation, provider routing, or effect journals.

### NF-012 — Release claims are evidence-profile specific

Passing deterministic v8 tests means the implementation satisfies the defined v8 contract profile. It is not a blanket claim that the product is unescapable or that live KVM/network behavior was verified without a matching live artifact.

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
                  | artifact boundary  |
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
      | reservations | leases | baselines | revisions |
      | desired state | reconciliation | observations  |
      +-----------------------+------------------------+
                              |
      +-----------------------v------------------------+
      |                    Realm                       |
      | world A          world B          world C      |
      | root/processes   root/processes   root/process |
      |       service:// semantic registry             |
      +-----------------------+------------------------+
                              |
      +-----------------------v------------------------+
      |       existing Nolane control + Cube adapter   |
      +------------------------------------------------+
```

A Realm is a logical trust/execution scope. It is not a new hypervisor object.

---

## 6. Freedom Realm model

### 6.1 Identities

Introduce only identities not already owned by Nolane World:

```go
type RealmID string
type ServiceID string
type SessionID string
```

World identity remains the existing `world.ID`; v8 must not create a parallel World-ID authority domain.

Canonical text forms are generated and validated by the host. Agent-supplied display names are metadata and never host paths or authority identifiers.

### 6.2 Realm administration

Host-side administration uses a separate interface from the agent runtime:

```go
type RealmController interface {
    Create(ctx context.Context, spec RealmSpec) (RealmRecord, error)
    Update(ctx context.Context, id RealmID, expectedRevision uint64, spec RealmSpec) (RealmRecord, error)
    Close(ctx context.Context, id RealmID, expectedRevision uint64) error
}
```

The agent-facing runtime receives only a Realm it is authorized to enter; it cannot create or broaden Realm policy.

### 6.3 Realm spec

```go
type RealmSpec struct {
    ID             RealmID
    MaxWorlds      uint32
    DefaultLease   time.Duration
    NetworkProfile NetworkProfile
    ResourceBudget ResourceBudget
}
```

`MaxWorlds` and resource budgets are admission controls. They are not kernel-enforcement claims.

### 6.4 World record

A Realm World record binds:

- existing `world.ID`;
- current realization revision;
- lifecycle state;
- lease generation and expiry;
- optional checkpoint reference;
- capability-report digest;
- host-private substrate handle.

Guest connection material is not durable Realm state.

### 6.5 Internal services

A service registration binds:

- Service ID;
- owning `world.ID` + exact realization revision;
- internal protocol (`tcp`, `udp`, or `http` in v8);
- guest port;
- readiness observation state;
- generation.

Registration is semantic metadata. A World restart/replacement increments realization revision and stales prior registrations until observed ready again.

---

## 7. Agent Experience Plane

The agent-facing API is intentionally small:

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

`Enter` establishes a Session bound to an exact Realm revision and policy digest. It returns no privileged host material.

### 7.2 Acquire

`Acquire` accepts semantic requirements, never a Cube sandbox ID. Requirements may include guest execution, snapshot support, and the Realm's admitted network profile.

The fabric selects an approved baseline/provider path, creates a fresh realization with the correct `world.ID`, observes readiness where supported, and returns a lease over the semantic World.

### 7.3 Exec

`Exec` is a bounded process operation inside an acquired World.

The request binds:

- Session ID;
- Realm revision;
- `world.ID`;
- exact lease generation;
- semantic action/idempotency ID;
- command/argv contract;
- timeout;
- maximum output bytes.

The receipt exposes:

- stable receipt ID;
- World realization revision;
- exit code;
- bounded stdout/stderr;
- truncation flags;
- duration;
- observation digest.

Raw Cube connection material is never exposed. V8 does not expose a generic **host** shell; execution is guest-scoped.

### 7.4 Spawn

`Spawn` creates a sibling World inside the same Realm subject to Realm admission. It receives Realm membership but does **not** inherit delegation grants, provider credentials, provider adapter authority, or the parent's authority epoch.

### 7.5 Checkpoint / Resume

A checkpoint binds:

- Realm ID + Realm revision;
- `world.ID` + realization revision;
- authority epoch reference;
- host-side substrate snapshot reference;
- capability-report digest;
- service generations;
- policy digest.

Same-World resume follows existing rollback semantics, including authority advancement before execution-state restoration. Forked resume uses a fresh `world.ID` through clone/create semantics and therefore receives a fresh authority context. Neither path restores old authority state from the snapshot.

### 7.6 Capabilities

`CapabilityReport` is the agent's discovery surface. It reports explicit states such as `verified`, `available-unproven`, `unavailable`, or `not-applicable`, with evidence references where relevant.

Candidate fields include:

- guest process execution;
- snapshot/rollback;
- public-read/supply-chain profile;
- public inbound disabled;
- internal mesh isolation/connectivity;
- filesystem/process/network isolation;
- accounting budget;
- kernel resource enforcement;
- live-substrate evidence profile/commit when available.

Unsupported or unproven properties remain explicit.

---

## 8. Local Capacity Fabric

### 8.1 Purpose

The current `SandboxSubstrate` is intentionally narrow and maps lifecycle actions directly to a realization. V8 adds a fabric **above** it so agents operate on semantic Worlds without gaining more Cube authority.

### 8.2 Capacity observation and reservation

A capacity observation has a monotonic local revision. An acquire/spawn operation first records a reservation containing:

- reservation ID;
- Realm ID;
- requested accounting units;
- exact capacity-observation revision;
- expiry;
- semantic request digest.

Replay is checked before fresh admission: same operation ID + same semantic digest is idempotent; same ID + changed digest is a hard conflict.

### 8.3 World leases

Agent use is fenced by a lease generation. Replacement/recovery/resume may advance generation. A stale lease cannot execute after a newer realization or lease generation is active.

### 8.4 Sanitized warm-start baselines

V8 does **not** pool already-assigned live sandboxes and later rebind them to a different `world.ID`. The current Cube create contract binds Nolane World metadata during creation, so identity rebinding would create an unsafe and unnecessary semantic gap.

The optimization surface is instead a bounded pool/catalog of **identity-free sanitized baselines** such as trusted templates or clean snapshots suitable for creating a fresh sandbox realization.

Rules:

- a baseline contains no user/project secrets or delegation/provider state;
- a baseline is content/identity versioned and host-owned;
- `Acquire` always creates a fresh realization with the target `world.ID`;
- baseline selection cannot change Realm network/authority policy;
- if a candidate baseline cannot be proven sanitized for the configured profile, it is not admitted;
- checkpoint-owned snapshots are never placed in the general baseline catalog.

This preserves startup optimization without cross-World identity inheritance.

### 8.5 Realization state

The fabric distinguishes:

`requested -> creating -> observed-ready -> leased -> paused -> terminal`

A provider acknowledgement does not equal `observed-ready` where independent read-back is available. Ambiguous create/destroy remains uncertain until reconciled or terminally quarantined.

### 8.6 Future Compute integration

The fabric exposes a provider-neutral internal interface for placement/realization. V8 ships only the local Cube-backed provider; a future Nolane Compute adapter may satisfy the same contract.

---

## 9. Guest execution boundary

The current Cube adapter already contains host-side `ConnectGuest` and bounded process-stream parsing for live gauntlet work. V8 formalizes an execution interface so ordinary callers never handle Cube tokens.

Conceptual interface:

```go
type GuestRuntime interface {
    Exec(ctx context.Context, handle substrate.Handle, req ProcessRequest) (ProcessObservation, error)
}
```

The Cube implementation may internally create/cache a guest connection, but its envd/traffic tokens remain private, non-serializable implementation state. No v8 receipt or durable record contains them.

`ProcessRequest` has explicit timeout/output bounds. Parser/transport errors collapse to stable sentinels rather than surfacing raw protocol diagnostics containing sensitive material.

---

## 10. Realm networking

### 10.1 Profiles

V8 defines semantic profiles independent of Cube JSON spelling:

- `R0_INTERNAL_ONLY` — no public Internet; no public inbound.
- `R1_PUBLIC_READ` — public unauthenticated read only where admitted; no public inbound.
- `R2_SUPPLY_CHAIN` — public source/package retrieval appropriate for build environments; no ambient host/provider credentials; no public inbound.

Authenticated read/write is not a Realm profile. It is a Reality Membrane effect using existing Network Class/delegation/provider semantics.

### 10.2 Cube mapping

The local Cube provider maps Realm profiles through the existing typed network update surface. Mapping is host-owned, exact, and tested. Agent input never supplies raw Cube network JSON.

### 10.3 Internal mesh

V8 separates **service registry semantics** from a live claim of isolated world-to-world networking.

If the current substrate cannot prove same-Realm connectivity plus cross-Realm isolation, `CapabilityReport` marks live mesh networking unavailable/unproven. V8 may still implement service identity/generation metadata. It must not fake a live network claim.

### 10.4 Service discovery

`service://` identity is stable while concrete host/IP/port realization may change. DNS synthesis or proxy-based discovery can later implement the same semantic contract once proven by the Cube substrate.

---

## 11. Reality Membrane

The Reality Membrane is a set of typed crossing rules, not one giant credential proxy.

### 11.1 Public read/supply-chain lane

N1/N2 traffic may be admitted under Realm policy when:

- no host/provider credential is attached;
- public inbound remains disabled;
- the active provider can represent the configured egress policy;
- the traffic is not being used as a disguised typed authenticated provider mutation.

Downloaded content is hostile guest material, not trusted evidence merely because retrieval was allowed.

### 11.2 Authenticated/consequential lane

N3–N5 operations remain outside Realm freedom and route through existing v6/v7 authority systems:

`agent intent -> delegation resolver -> typed adapter -> brokered credential -> effect journal -> reconciliation`

V8 adds no generic authenticated HTTP path.

### 11.3 Artifact boundary

Realm files do not become trusted host artifacts by path reference alone. Export continues through artifact admission/provenance. Durable general artifact quarantine remains a separate production gate unless implemented in a later milestone.

### 11.4 Public ingress

V8 provides no public ingress. Internal service registration is not an export mechanism.

---

## 12. Persistence and recovery

V8 follows existing append-only, hash-chained, fail-closed persistence conventions where durable state is introduced.

Persist at minimum:

- Realm specs/revisions/lifecycle;
- World semantic identity + realization revision/lifecycle;
- lease generations;
- capacity reservations;
- checkpoint metadata references;
- service registrations/generations;
- semantic operation idempotency records;
- admitted baseline identities/digests where the baseline catalog is durable.

Never persist in v8 state:

- Cube API keys;
- envd access tokens;
- traffic access tokens;
- provider credential bytes;
- broker secret bytes.

After restart, a realization is not assumed ready because a row exists. Recovery observes/reconciles it where possible or marks it unavailable/terminal rather than trusting stale narrative state.

---

## 13. Error model

Stable sentinel families distinguish:

- invalid semantic request;
- stale Realm revision;
- stale World lease generation;
- admission/resource exhausted;
- capability unavailable/unproven;
- World terminal;
- realization unavailable;
- execution failed;
- output bound exceeded/truncated;
- operation collision;
- uncertain outcome;
- Reality Membrane denial;
- stale service registration;
- invalid/untrusted warm-start baseline.

Raw provider diagnostics, secrets, Cube tokens, host paths, and uncontrolled HTTP bodies must not appear in agent-facing errors.

---

## 14. Expected implementation structure

```text
NolaneWorld/
  realm/
    model.go
    controller.go
    store.go
    durable.go
    service.go
  fabric/
    capacity.go
    lease.go
    baseline.go
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

Existing packages are reused rather than bypassed:

- `world`
- `control`
- `authority`
- `network`
- `artifact`
- `delegation`
- typed provider packages
- `substrate`
- `substrate/cube`
- v4/v5/v6/v7 evidence families.

Exact file decomposition may be smaller where clearer, but package responsibilities are normative.

---

## 15. TDD contract matrix

Implementation begins with failing tests for at least these contracts:

1. agent projections never expose substrate handles, sandbox IDs, envd tokens, or traffic tokens;
2. agent cannot create/update Realm policy through `Runtime`;
3. acquire exact retry is idempotent; changed-payload reuse conflicts;
4. stale lease generation cannot execute;
5. terminal World cannot execute or register a service;
6. spawned sibling inherits Realm membership but no delegated/provider authority;
7. checkpoint/resume cannot restore a stale authority epoch;
8. capability report refuses unproven containment/mesh/enforcement claims;
9. all v8 Realm profiles keep public inbound disabled;
10. N3–N5 authority cannot be represented as a Realm network profile;
11. service registration stales after realization revision changes;
12. a general warm-start baseline cannot contain World identity or checkpoint ownership;
13. acquire from a baseline still invokes fresh `Create(..., world.ID)` semantics;
14. capacity replay is recognized before fresh admission;
15. ambiguous create/exec is not converted to success;
16. durable Realm state rejects malformed/tampered sequence/hash state;
17. persisted v8 state contains no synthetic secret/token canaries;
18. restart does not trust an unreconciled realization as ready;
19. bounded exec reports truncation/failure exactly according to contract;
20. Reality Membrane leaves v6/v7 authenticated-effect authority intact;
21. v4/v6/v7 canonical evidence families do not drift;
22. live v5 remains `UNAVAILABLE`, never `PASS`, without real configured substrate.

Property/fuzz tests target identity parsing, operation digest collision/rebinding, malformed durable records, lease transitions, service-name canonicalization, baseline metadata, and capability-report mutation.

---

## 16. Release evidence

Add a new deterministic evidence family rather than mutating old semantics:

`nolane-freedom-v8`

Mandatory scenario classes:

- authority non-inheritance;
- stale lease/revision denial;
- token/secret non-disclosure;
- baseline identity isolation;
- checkpoint authority non-rewind;
- network-profile Reality boundary;
- service-generation staleness;
- capability fail-honesty;
- persistence tamper/restart;
- idempotency/uncertainty.

CI generates v8 evidence twice and byte-compares it, then scans for plaintext/base64/hex forms of synthetic Cube/provider credentials. It also retains hash guards for canonical v4/v6/v7 deterministic evidence families.

A future live Realm-Mesh profile may extend live evidence, but deterministic v8 success must not be mislabeled as proof of live inter-VM networking.

---

## 17. Performance and agent-ergonomics acceptance criteria

V8 must demonstrate simpler agent control without weakening trust boundaries.

Deterministic acceptance criteria:

- one semantic `Acquire` replaces manual create/connect/token handling;
- one semantic `Exec` replaces direct envd protocol handling for ordinary guest work;
- a Session can checkpoint/resume without storing Cube identifiers;
- `Capabilities` provides one fail-honest discovery response;
- exact retries do not duplicate World creation or reservations;
- baseline startup never inherits a previous World identity or authority;
- Realm/World/Service identity is stable across permitted realization changes.

Live benchmark metrics, when infrastructure exists, are recorded separately:

- cold acquire latency;
- baseline-assisted acquire latency;
- first exec latency;
- repeated exec latency;
- checkpoint latency;
- resume latency;
- sibling spawn latency;
- host memory/disk cost of admitted baselines.

Deterministic CI claims no fixed latency number. Performance claims require measured live evidence on a named commit/environment.

---

## 18. Explicit non-goals

V8 does not claim or implement:

- a replacement hypervisor or guest kernel;
- production KMS/HSM attestation;
- generic authenticated HTTP;
- arbitrary public ingress;
- full Docker/Kubernetes/cloud orchestration;
- distributed consensus for Realm state;
- transparent multi-host migration;
- complete fake Internet/cloud simulation;
- Habitat's semantic project cognition inside Sandbox;
- Nolane Compute's full scheduler/federation stack;
- live Realm-mesh connectivity unless separately proven;
- mathematical proof that KVM/kernel/hardware contains no unknown escape vulnerability.

---

## 19. Security claim after v8

If all deterministic v8 gates pass, Nolane Sandbox may claim:

> For the verified v8 contract profile, agent-visible execution is mediated through stable Realm/World/Service identities, host-controlled Realm policy, lifecycle and lease fences, fail-honest capability reporting, bounded guest execution, sanitized identity-free startup baselines, and explicit Reality Membrane classes. Internal capability growth does not itself grant authenticated or consequential real-world authority.

It may not claim:

> Nolane Sandbox is unescapable, every live network path is verified, every external provider is trusted, or every resource budget is kernel-enforced.

---

## 20. Implementation order

The implementation plan follows this dependency order:

1. Realm IDs/model/controller + durable store contracts.
2. capacity observation/reservation + lease state machine and idempotency.
3. host-only GuestRuntime over Cube guest execution.
4. Agent Runtime `Enter/Acquire/Exec/Release` core.
5. checkpoint/resume with authority non-rewind.
6. fail-honest capability report.
7. Realm network profiles + Reality Membrane classification/mapping.
8. service registry/generation semantics.
9. sanitized baseline catalog and startup selection.
10. restart/reconciliation hardening.
11. v8 deterministic gauntlet + CI evidence and credential scans.
12. live performance hooks/metrics without unverified performance claims.

Each stage must preserve existing v0–v7 tests and evidence-family semantics.
