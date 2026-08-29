# Nolane Sandbox Runtime Integration v1 Design

**Status:** Implementation spec

**Date:** 2026-08-29

**Repository:** `Nolane-x/Nolane-sandbox`

**Depends on:** `2026-08-29-nolane-sandbox-world-design.md`

## 1. Goal

Runtime Integration v1 connects the Trust Kernel to the CubeSandbox control plane without moving trust state into the guest or modifying CubeSandbox security-critical internals.

This milestone adds four capabilities:

1. host-owned world lifecycle management with terminal authority revocation;
2. a fail-closed CubeAPI substrate adapter;
3. a fresh-world capability validation and promotion pipeline;
4. a crash-recoverable external-effect journal that refuses unsafe replay after uncertain failures.

The governing invariant remains:

> **Unbounded capability creation; bounded authority, promotion, truth, and persistence.**

## 2. Non-goals

This milestone does not claim full production readiness. It does not yet provide:

- KMS-backed production credentials;
- a durable authority-epoch database across host loss;
- a durable capability registry across host loss;
- a live Cube/KVM adversarial gauntlet in default GitHub-hosted CI;
- raw TCP/UDP/DNS egress-bypass proof;
- cross-node replicated effect storage;
- generic external-service authority adapters.

Those remain release gates.

## 3. Terminal world authority

A destroyed world is not merely at a newer epoch. It is terminally revoked.

`world.State.Close()` MUST:

- linearize against any in-flight `WithEpoch` callback;
- advance the epoch exactly once;
- mark the state closed;
- make all future `ValidateEpoch` and `WithEpoch` calls return `ErrClosedWorld`;
- remain closed after repeated close calls;
- prevent later `AdvanceEpoch` calls from reopening or changing terminal state.

This closes a subtle hole in which a caller that learned the post-destroy epoch could otherwise submit fresh authority requests after the execution world no longer existed.

## 4. World Manager

`control.Manager` is the host-side owner of the mapping:

```text
WorldID -> AuthorityState + opaque SubstrateHandle
```

The guest cannot mutate this map.

### 4.1 Create

Create reserves the `WorldID`, creates the substrate world, and exposes its authority state only after substrate creation succeeds.

A concurrent observer MUST NOT obtain an authority state for a world whose substrate creation has not completed.

### 4.2 Rollback

Rollback sequence is:

```text
lock world lifecycle
-> advance host authority epoch
-> invoke substrate rollback
-> keep advanced epoch even if rollback fails
```

The epoch MUST advance before the execution state is restored.

### 4.3 Destroy

Destroy sequence is:

```text
lock world lifecycle
-> terminally close authority
-> invoke substrate destroy
```

If substrate destruction fails, authority remains closed. A later retry may attempt substrate destruction again but MUST NOT reopen authority.

### 4.4 Clone

A clone receives a new `WorldID` and a fresh authority state beginning at epoch 1.

Execution state may derive from a snapshot. Authority state never does.

## 5. CubeAPI adapter

`substrate/cube.Client` implements `substrate.SandboxSubstrate` using CubeAPI HTTP endpoints.

The adapter is a host component. Its API key never enters guest state.

### 5.1 Base URL policy

The client accepts:

- `https://...` for remote CubeAPI;
- `http://localhost`, `http://127.0.0.0/8`, or loopback IPv6 for local development.

It rejects:

- remote cleartext HTTP;
- URL userinfo;
- non-root base paths;
- query parameters;
- fragments;
- unsupported schemes.

### 5.2 Redirect policy

Redirect following is disabled.

This prevents an untrusted or misconfigured CubeAPI endpoint from redirecting a request containing host credentials to another origin.

### 5.3 Response limits

All responses are bounded by `MaxResponseBytes`.

Oversized responses fail closed with `ErrResponseTooLarge`.

### 5.4 World creation defaults

Nolane-created worlds use:

```text
allow_internet_access = false
network.allowPublicTraffic = false
metadata["nolane.world.id"] = WorldID
```

More permissive network access is a later explicit policy transition, not a creation default.

### 5.5 Snapshot and clone mapping

Cube snapshots are opaque Nolane `substrate.Snapshot` values.

Clone is implemented by creating a new sandbox from the snapshot template and binding a new `WorldID`. The source handle is validated but no trust data is copied.

## 6. Capability Forge

`forge.Forge` turns untrusted guest bytes into a candidate, validates them in a fresh world, tears that world down, and only then requests registry promotion.

### 6.1 Candidate admission

Before a validator world is created, both capability content and manifest must pass `artifact.Gate`.

The host computes exact SHA-256 digests. The agent does not supply trusted digest values.

### 6.2 Fresh validation world

The validator world is created with `SandboxSubstrate.Create`.

It MUST NOT be created by cloning the originating world.

A fresh host-generated validator identity MUST differ from the origin identity.

### 6.3 Evidence

Validator output is an evidence report byte string.

The host computes `VerificationDigest` from exact evidence bytes. A validator cannot provide an unrelated trusted digest string.

Empty evidence is invalid.

### 6.4 Teardown-before-promotion

Promotion order is strictly:

```text
validate
-> destroy validator world
-> require successful teardown
-> promote exact candidate bytes
```

Even successful validation does not permit promotion if teardown fails.

A validator error or panic MUST still trigger teardown and MUST NOT promote the candidate.

## 7. Durable external-effect journal

`authority.JournalLedger` is an append-only JSONL journal with synchronous writes.

It exists to close the crash window between action admission and receipt persistence.

### 7.1 States

Each `(WorldID, ActionID)` transitions through:

```text
absent -> pending -> completed
                 \-> aborted
```

`completed` is immutable.

`aborted` is used only for failures known to occur before any external side effect, currently policy denial and policy evaluation failure.

### 7.2 Pending-before-effect rule

Before invoking the callback that may reach an external service, the journal writes and `fsync`s a `pending` record.

After successful execution it writes and `fsync`s a `completed` record.

### 7.3 Crash semantics

If a process crashes after the external side effect but before `completed` is durable, restart recovers the action as `pending`.

A retry returns:

```text
ErrActionUncertain
```

and does not execute the external action again.

This is intentionally conservative.

### 7.4 Reconciliation

A host-only `Resolve` operation may convert an uncertain action to `completed` when an external authority adapter proves the real-world result.

`Resolve` must bind the exact world ID, action ID, request digest, and receipt.

### 7.5 Single writer

Only one process may open a JournalLedger path at a time.

On supported Unix systems an OS advisory lock is held for the lifetime of the ledger.

A second writer fails with `ErrLedgerLocked`.

Unsupported locking platforms fail closed rather than pretending cross-process safety.

### 7.6 Corruption

Malformed journal records, illegal state transitions, mismatched digests, or invalid receipts fail opening with `ErrLedgerCorrupt`.

The implementation does not skip corrupt tail data.

## 8. Trust boundaries

The following values remain host-owned:

- `WorldID`;
- authority epoch and closed state;
- CubeAPI credential;
- JournalLedger file;
- capability registry;
- verifier identity;
- promotion decision.

The guest may control:

- generated capability bytes;
- manifest bytes;
- local guest filesystem;
- packages and processes;
- model output.

Guest-controlled values never become trusted solely because they survived a snapshot or local test.

## 9. Required tests

This milestone requires executable contracts for:

- close linearization against in-flight authority;
- terminal rejection after close;
- rollback epoch advance before substrate call;
- failed rollback never rewinds epoch;
- destroy closes authority before substrate call;
- failed destroy never reopens authority;
- clone starts at epoch 1;
- authority state hidden until create finishes;
- remote HTTP CubeAPI rejection;
- redirect rejection;
- credentialed/decorated base URL rejection;
- response size limit;
- snapshot/rollback/clone wire contract;
- fresh validator world, never clone;
- artifact rejection before validator start;
- validation failure teardown;
- empty evidence rejection;
- validator panic teardown;
- teardown failure blocks promotion;
- journal restart idempotency;
- uncertain execution never re-executes;
- policy denial does not poison an action;
- host reconciliation;
- collision rejection;
- corruption rejection;
- single-writer lock.

## 10. CI gate

The existing `Nolane World Check` workflow remains the default gate and must execute:

```text
go test ./...
go test -race ./...
go vet ./...
```

inside `NolaneWorld/`.

## 11. Remaining production gates

After Runtime Integration v1, the next security milestones are:

1. durable world authority state with crash-safe monotonic epochs;
2. durable capability and artifact provenance registry;
3. typed authority adapters with remote idempotency/reconciliation;
4. live Cube-backed stale-snapshot tests;
5. egress bypass gauntlet for TCP, UDP, DNS, metadata, and cross-sandbox traffic;
6. hostile artifact corpus and archive quarantine;
7. KMS/secret-broker integration;
8. upstream-delta scanner for security-sensitive Cube paths.

No release may claim “no escape” or “production complete” until those gates are executed.
