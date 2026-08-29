# Nolane Sandbox Agent-World Architecture

**Status:** Implementation baseline for Trust Kernel v0

**Date:** 2026-08-29

**Repository:** `Nolane-x/Nolane-sandbox`

## 1. Purpose

Nolane Sandbox is an agent execution environment in which an AI may create, install, compile, execute, destroy, repair, and retain software capabilities without receiving a pre-enumerated tool catalog, while the AI cannot grant itself additional real-world authority, trusted persistence, or promotion rights.

The core design principle is:

> **Unbounded capability creation; bounded authority, promotion, truth, and persistence.**

CubeSandbox remains the execution substrate. Nolane-specific trust, authority, evolution, artifact, and recovery semantics are added above the substrate through narrow interfaces. Security-critical upstream primitives are not forked unless a measured gap cannot be solved through an adapter or upstreamable extension.

## 2. Design goals

1. Let an untrusted agent obtain root inside its disposable guest without implying host trust.
2. Let the agent synthesize new tools and workflows from ordinary compute primitives.
3. Keep host credentials and external authority outside the guest.
4. Ensure rollback cannot replay or erase real-world side effects.
5. Ensure a capability cannot certify or promote itself.
6. Ensure execution snapshots are never treated as trusted memory.
7. Make every trust transition observable, typed, attributable, replay-resistant, and testable.
8. Fail closed when policy, identity, ledger state, provenance, or validation is ambiguous.
9. Preserve upstream CubeSandbox security fixes with minimal merge debt.
10. Make the first Trust Kernel small enough to audit exhaustively.

## 3. Non-goals for Trust Kernel v0

Trust Kernel v0 does **not**:

- replace KVM, RustVMM, CubeEgress, CubeCoW, CubeNet, or CubeOps;
- implement a secret manager or KMS;
- implement generic browser automation;
- provide production cloud/Git/mail connectors;
- claim protection from all hypervisor or kernel zero-days;
- treat antivirus scanning as a proof of safety;
- persist arbitrary guest filesystem state as trusted capability memory;
- allow an agent to alter constitution or promotion policy;
- provide an unrestricted authenticated HTTP proxy for consequential writes.

Those are intentionally outside the initial trust core.

## 4. Threat model

### 4.1 Assumed-compromised components

The system MUST remain correct if all of the following are malicious or compromised:

- the model output;
- the agent process;
- every process in the guest;
- guest root;
- packages installed by `apt`, `pip`, `npm`, `cargo`, or equivalent;
- downloaded source repositories;
- webpages, documents, and prompt-injection content;
- agent-authored capabilities;
- stale execution snapshots;
- files exported by the guest.

### 4.2 Protected assets

The Trust Kernel protects:

- host and external-service credentials;
- authority policy;
- authority epochs and revocation state;
- external-effect history;
- trusted capability registry state;
- promotion receipts;
- artifact provenance;
- cross-world isolation identifiers;
- audit history.

### 4.3 Explicitly modeled attacks

The implementation and tests MUST model at least:

- self-escalation of authority;
- confused-deputy use of broker credentials;
- replay of an already executed external action after rollback;
- stale snapshot resurrection after revocation;
- capability self-promotion;
- promotion with mismatched content hash;
- cross-world receipt reuse;
- path-traversal artifact names;
- oversized or empty artifacts;
- malformed/unknown network classes;
- fail-open policy errors;
- duplicate action IDs with altered payloads;
- forged authority epochs;
- mutation of trusted state from guest-owned data.

## 5. Trust zones

```text
REAL WORLD / EXTERNAL SERVICES
          |
          v
+-----------------------------+
| Authority adapters / KMS    |
+-----------------------------+
          |
          v
+===========================================================+
| NOLANE TRUST PLANE                                        |
|                                                           |
| Authority Broker  Effect Ledger  Capability Registry      |
| Promotion Gate    Artifact Gate  Constitution             |
| World Identity    Audit/Receipts Network-class policy     |
+===========================================================+
          |
          v
+-----------------------------+
| CubeSandbox adapter         |
+-----------------------------+
          |
          v
+===========================================================+
| UPSTREAM EXECUTION SUBSTRATE                              |
| CubeAPI/CubeMaster/CubeOps/CubeEgress/CubeNet/CubeCoW     |
| RustVMM/KVM guest                                           |
+===========================================================+
          |
          v
UNTRUSTED AGENT WORLD
```

The guest MUST NOT have a write path to Trust Plane storage or policy.

## 6. Constitution: non-negotiable laws

The following laws are normative and use stable identifiers.

### NS-LAW-001 — No ambient secrets

Raw host credentials, cloud keys, Git tokens, browser session secrets, and KMS material MUST NOT be stored in guest-visible state.

### NS-LAW-002 — Authority cannot self-increase

An agent MAY request an action but MUST NOT mint, widen, or extend its own authority. Every accepted external action is evaluated against host-owned policy.

### NS-LAW-003 — No self-promotion

A world that creates a capability MUST NOT make that capability trusted. Promotion requires an independent gate operating outside the originating world.

### NS-LAW-004 — Rollback does not roll back reality

Guest/snapshot rollback MUST NOT decrement authority epochs, delete effect receipts, resurrect revoked grants, or make a completed external action executable again.

### NS-LAW-005 — Execution state is not trusted memory

A CubeCoW snapshot, volume, or paused world is execution state only. Trusted reusable capability state requires a promotion receipt bound to exact content.

### NS-LAW-006 — Consequential writes are typed intents

External writes above the configured reversible-risk threshold MUST execute through a typed authority intent. Generic authenticated raw HTTP is insufficient authority mediation.

### NS-LAW-007 — Exports are untrusted

Every guest-originating artifact is untrusted until it crosses the Artifact Gate and receives a content-addressed receipt.

### NS-LAW-008 — Exact-content binding

Receipts MUST bind identity, world, epoch where applicable, and a canonical cryptographic digest of the exact protected payload.

### NS-LAW-009 — Monotonic authority epoch

Each world has a host-owned authority epoch. Epoch transitions are monotonic and MUST NOT be guest-controlled or restored from snapshots.

### NS-LAW-010 — Network authority is classified

Network access is represented by explicit classes. Unknown classes fail closed. Authenticated consequential writes cannot silently downgrade to ordinary egress.

### NS-LAW-011 — Host mounts are denied by default

A Nolane world MUST NOT require arbitrary host filesystem mounts. Any future mount capability is an explicit, separately authorized exception.

### NS-LAW-012 — Ambiguity fails closed

Missing identity, malformed input, policy evaluation error, unknown action, stale epoch, duplicate action collision, or validation uncertainty MUST deny promotion/authority rather than silently permit it.

## 7. Core identity model

### 7.1 World identity

Every running world has a `WorldID` generated outside the guest. A world operation is always scoped by `WorldID`.

### 7.2 Authority epoch

`AuthorityEpoch` is a positive monotonic integer owned by the Trust Plane.

It changes when authority-relevant state changes, including revocation or policy replacement. Guest rollback never changes it.

### 7.3 Action identity

Every consequential external request has an `ActionID` unique within a world. The immutable action fingerprint binds:

- world ID;
- authority epoch;
- action ID;
- action kind;
- target;
- canonical payload hash.

A repeated `ActionID` with the same fingerprint returns the previous effect receipt. A repeated `ActionID` with a different fingerprint is denied as a collision/replay attack.

## 8. Authority model

### 8.1 AuthorityIntent

The minimum intent fields are:

```text
WorldID
AuthorityEpoch
ActionID
Kind
Target
Payload
```

`Payload` is opaque to the core broker but is cryptographically bound into the request fingerprint. Policy implementations may parse typed payloads at adapters.

### 8.2 PolicyDecision

A decision is one of:

- `ALLOW`
- `DENY`

There is no implicit allow. Policy failures produce `DENY`.

### 8.3 EffectReceipt

An accepted execution produces an immutable receipt containing:

```text
WorldID
AuthorityEpoch
ActionID
RequestDigest
EffectDigest
CompletedAt
```

The effect ledger is host-owned and survives world destruction and rollback.

### 8.4 Broker execution sequence

```text
validate intent
  -> verify world identity
  -> verify current epoch
  -> canonicalize + hash request
  -> consult effect ledger
     -> exact duplicate: return prior receipt
     -> same ActionID / different digest: deny
  -> evaluate policy (fail closed)
  -> execute typed adapter
  -> hash effect result
  -> append effect receipt atomically
  -> return receipt
```

A production implementation MUST make adapter side effect + durable receipt effectively idempotent. v0 provides the semantic core and a deterministic in-memory ledger used to prove the invariant.

## 9. Network classes

The Trust Kernel defines the following classes:

- `N0_NONE`: no network;
- `N1_PUBLIC_READ`: unauthenticated public reads;
- `N2_PUBLIC_SUPPLY_CHAIN`: public package/source endpoints, subject to separate provenance controls;
- `N3_AUTHENTICATED_READ`: authenticated read-only operations through a credential mediator;
- `N4_REVERSIBLE_WRITE`: typed brokered writes with idempotency/effect receipts;
- `N5_CONSEQUENTIAL_WRITE`: typed brokered actions requiring the strongest configured verification/approval.

Unknown values are invalid.

## 10. Capability evolution model

### 10.1 Candidate

An agent-created capability is always a `Candidate` first. A candidate contains:

```text
CandidateID
OriginWorldID
Name
Version
ContentDigest
ManifestDigest
CreatedAt
```

The candidate is untrusted regardless of local tests performed by the agent.

### 10.2 Promotion

A Promotion Gate operating outside the originating world validates exact candidate content and issues a `PromotionReceipt` containing:

```text
CapabilityID
CandidateID
OriginWorldID
ContentDigest
VerifierID
VerificationDigest
PromotedAt
```

Promotion MUST fail if:

- candidate content changed;
- verifier ID is empty;
- verifier is the originating world identity;
- verification digest is empty;
- capability ID collides with different content.

### 10.3 Registry

Only capabilities with valid promotion receipts enter the trusted registry. The registry is content-addressed and immutable per version. Reusing a name/version with different bytes is rejected.

## 11. Artifact Gate

Every guest export is represented as an `ArtifactEnvelope` with:

```text
WorldID
LogicalName
MediaType
Size
ContentDigest
```

The v0 gate MUST enforce:

- non-empty world ID;
- normalized relative logical names only;
- no absolute paths;
- no `..` traversal;
- no NUL bytes;
- positive size;
- configurable maximum size;
- SHA-256 exact-content digest;
- receipt bound to world + normalized name + digest + size.

The gate does not declare an artifact safe to execute; it declares the exported bytes and provenance well-formed and immutable.

## 12. Trusted persistence versus execution persistence

### Execution persistence

Owned by CubeSandbox:

- pause/resume;
- CubeCoW snapshots;
- cloned worlds;
- volumes.

Execution persistence is treated as untrusted mutable state.

### Trusted persistence

Owned by Nolane Trust Plane:

- effect receipts;
- authority epoch/revocation state;
- candidate records;
- promotion receipts;
- trusted capability records;
- artifact receipts;
- audit history.

Trusted persistence MUST NOT be restored from a guest snapshot.

## 13. Rollback semantics

Given world `W`, authority epoch `E=7`, and completed action `A`:

1. guest snapshot `S` may have been created before `A`;
2. action `A` executes and receipt `R` is recorded outside the guest;
3. guest rolls back to `S`;
4. Trust Plane still records `R` and current epoch >= 7;
5. replaying `A` returns `R` without executing it again;
6. replaying the same `ActionID` with altered target/payload is denied;
7. if host revokes authority and epoch becomes 8, requests from snapshot state claiming epoch 7 are denied.

This invariant is mandatory.

## 14. Substrate abstraction

Nolane-specific code depends on a narrow `SandboxSubstrate` interface rather than internal Cube components.

Required operations for the initial adapter contract:

```text
Create
Destroy
Pause
Resume
Snapshot
Rollback
Clone
```

The interface uses Nolane world identity and opaque substrate handles. Trust data never lives inside the opaque handle.

The first implementation supplies the interface contract and a test double. Wiring to CubeAPI is a later integration milestone after the trust semantics are green.

## 15. Upstream/fork policy

1. Keep upstream CubeSandbox security-critical components unmodified whenever possible.
2. Nolane code lives under a separate top-level module and communicates through adapters.
3. A modification to KVM/RustVMM/hypervisor/network isolation/egress core requires a documented measured gap.
4. Prefer an upstream issue/PR for generic substrate fixes.
5. Never weaken an upstream security default merely to simplify agent behavior.
6. Track the upstream commit from which each Nolane release is built.
7. Upstream merge conflicts in security-sensitive paths block release until reviewed.

## 16. Trust Kernel v0 module layout

```text
NolaneWorld/
  go.mod
  constitution/
  world/
  authority/
  capability/
  artifact/
  network/
  substrate/
```

Each package has one responsibility and no dependency on guest code.

## 17. Error model

Stable sentinel errors are part of the contract. At minimum:

```text
ErrInvalidWorld
ErrInvalidEpoch
ErrStaleEpoch
ErrInvalidAction
ErrActionCollision
ErrDenied
ErrPolicyFailure
ErrExecutionFailure
ErrInvalidCandidate
ErrSelfPromotion
ErrDigestMismatch
ErrCapabilityCollision
ErrInvalidArtifact
ErrArtifactTooLarge
ErrInvalidNetworkClass
```

Security-sensitive callers MUST branch on typed/sentinel errors rather than parsing human text.

## 18. Audit model

Every accepted or denied trust transition SHOULD be representable as an immutable event. Trust Kernel v0 focuses first on deterministic receipts and stable errors; durable hash-chained audit storage is the next persistence milestone.

No model-generated prose is sufficient evidence of an authority execution or capability promotion.

## 19. Testing strategy

### 19.1 Unit contract tests

Tests MUST prove:

- valid authority action executes once;
- exact retry returns the same receipt without second execution;
- altered payload under same ActionID is denied;
- stale epoch is denied;
- policy error denies execution;
- policy deny prevents adapter execution;
- world mismatch cannot reuse a receipt;
- candidate cannot self-promote;
- content mutation invalidates promotion;
- capability version collision is rejected;
- artifact traversal/absolute paths are rejected;
- artifact digest binds exact bytes;
- oversized artifact is rejected;
- network classes reject unknown values;
- constitution law IDs are unique and immutable in the compiled catalog.

### 19.2 Property/fuzz targets

Future fuzzing targets include:

- canonical request hashing;
- artifact path normalization;
- duplicate/collision detection;
- capability identity parsing;
- serialized receipt round trips.

### 19.3 Integration gauntlet

A later Cube-backed test environment MUST attempt:

- raw TCP/UDP/DNS egress bypass;
- cloud metadata access;
- cross-sandbox traffic;
- credential exfiltration;
- stale-snapshot authority replay;
- malicious package install scripts;
- artifact archive traversal;
- malicious capability promotion;
- host mount escape;
- resource exhaustion.

## 20. CI gates for v0

A Nolane change is green only when:

```text
go test ./...
go vet ./...
```

pass inside `NolaneWorld/`.

The repository CI runs this module independently so upstream component build topology remains untouched.

## 21. Release gates

A Trust Kernel release MUST NOT claim production completion until all are true:

1. unit contract suite green;
2. race-sensitive persistence code has race tests when introduced;
3. Cube-backed integration gauntlet exists and is green on supported host kernel;
4. egress bypass tests are green;
5. stale snapshot/revocation test is green;
6. credential boundary test is green;
7. capability promotion runs in a fresh validation world;
8. artifact export quarantine has hostile corpus tests;
9. upstream security delta is reviewed;
10. external-effect ledger has durable crash-recovery semantics.

## 22. Initial implementation boundary

The first implementation SHALL build only the auditable semantic core:

- constitution catalog;
- world identity + monotonic epoch store;
- authority broker + deterministic request digest + in-memory effect ledger;
- capability candidate/promotion registry;
- artifact gate;
- network class parser/validator;
- substrate interface;
- CI workflow.

It SHALL NOT pretend that the in-memory stores are production durable. The purpose of v0 is to make the security invariants executable and regression-testable before wiring real infrastructure.

## 23. Acceptance criteria for this milestone

The milestone is accepted when:

- all code is isolated under `NolaneWorld/` except CI/docs;
- no upstream security core is modified;
- every law in sections 6–13 has a corresponding executable contract or an explicit future-release gate;
- all v0 tests pass;
- `go vet ./...` passes;
- the branch can be merged without changing CubeSandbox runtime behavior when NolaneWorld is unused.
