# Nolane Sandbox Delegated Authority Plane v6 Design

## Status

Approved architecture for implementation on `nolane/delegated-authority-plane-v6`.

## Purpose

V6 turns the existing Nolane rule — **capability may grow, authority may not self-expand** — into an external-action protocol that never requires an agent or guest to receive credential bytes.

The execution world may create arbitrary code. It may ask for a typed external action. It may not choose a credential, choose a generic authenticated transport, mint a delegation, widen a delegation, or certify an external side effect by narration.

V6 is a host-side trust subsystem. It does not modify KVM, RustVMM, CubeEgress, CubeNet, CubeCoW, or CubeAPI security internals.

## Core laws

1. **No credential in agent protocol.** Agent-visible intents contain no secret bytes and no `SecretHandle`.
2. **No adapter selection in agent protocol.** The grant, not the agent, determines the adapter kind.
3. **No generic authenticated HTTP adapter.** Raw/generic authenticated HTTP kinds are rejected at adapter registration.
4. **Exact scope binding.** A delegation binds one world, one authority epoch, one adapter kind, one canonical resource identifier, an explicit operation allowlist, one host-only secret handle, and an expiry.
5. **Immutable grant.** Rebinding a delegation ID to different content is forbidden. Scope changes require a new delegation ID.
6. **Revocation is monotonic.** A revoked delegation never becomes active again. Guest snapshot rollback cannot rewind revocation.
7. **World rollback revokes old delegation authority indirectly.** A delegation is bound to the world authority epoch present at issue time; after an epoch advance, the old grant is stale.
8. **No automatic replay of an uncertain external effect.** If an action is pending/uncertain, repeated execution is denied until explicit reconciliation resolves it.
9. **Reconciliation is observation, not execution.** `Reconcile` may determine `observed`, `absent`, or `unknown`; it must not perform the requested side effect.
10. **Observed reconciliation may close a pending receipt.** `absent` and `unknown` remain non-replayable unless a later host policy explicitly creates a new action.
11. **Credential material is not evidence.** Digests may bind the opaque secret handle, never secret bytes.
12. **Secret-bearing adapter failures are sanitized.** Agent-facing errors are stable sentinels and never wrap adapter/vault error strings.
13. **Secret echo fails closed.** If exact credential bytes appear in adapter evidence, the action is treated as unsafe/uncertain and no success receipt is issued.
14. **Authority receipts bind the delegation.** The request digest includes the delegated intent digest, exact grant digest, and secret-handle digest.
15. **Durable delegation state is host-owned.** Issue/revoke state is fsynced append-only state and is never derived from guest files, snapshots, chat history, or model claims.
16. **No release score can hide a failed authority invariant.** Every registered v6 gauntlet scenario is mandatory.
17. **No-effect must be proven explicitly.** A ledger may discard a pending transition only when trusted host code marks a failure as having occurred before external-effect entry; a bare error sentinel never proves that no effect happened.
18. **Persistent trust records have one canonical representation.** Grant/effect journal replay rejects unknown JSON fields and non-canonical encodings instead of silently interpreting ambiguous bytes.

## Architecture

```text
UNTRUSTED WORLD / AGENT
        |
        | DelegatedIntent
        | world_id, epoch, action_id,
        | delegation_id, operation, resource, payload
        v
+----------------------------+
| Delegated Authority Plane  |
|                            |
|  Authority epoch check     |
|  Delegation resolver       |
|  Exact scope check         |
|  Revocation/expiry check   |
|  Action ledger             |
+-------------+--------------+
              |
              | grant chooses adapter + SecretHandle
              v
+----------------------------+
| Host Vault                 |
| Use(handle, callback)      |
| secret exists only during  |
| trusted adapter call       |
+-------------+--------------+
              |
              v
+----------------------------+
| Typed Adapter Registry     |
| github.repo.write          |
| email.send                 |
| deploy.release             |
| ...                        |
+-------------+--------------+
              |
              v
          REAL WORLD
```

## Packages

### `delegation`

Owns the v6 trust protocol:

- opaque identifiers and grant types;
- validation and deterministic digests;
- read-only grant resolution versus host mutation interfaces;
- in-memory store for unit tests;
- durable append-only `JournalStore` for production host state;
- vault contract and test-only memory vault;
- typed adapter registry;
- delegated authority plane execution and reconciliation;
- stable fail-closed error taxonomy.

### `authority`

V6 hardens the existing effect ledger so delegated external effects have inspectable uncertainty semantics rather than a success-only cache.

```go
type InspectableLedger interface {
    Ledger
    Status(world.ID, actionID, requestDigest string) (ActionStatus, Receipt, error)
}
```

Both `MemoryLedger` and `JournalLedger` report `missing`, `pending`, and `completed`. They install `pending` before entering the effect callback. An unmarked callback failure keeps that action pending so an exact retry returns `ErrActionUncertain` instead of executing again. Only an explicit host `MarkNoEffect(...)` annotation may remove/abort that pending transition. `JournalLedger.Resolve` remains the completion transition for a durable already-pending uncertain action after independent reconciliation.

`Plane` requires an `InspectableLedger` at construction time, preventing accidental use of a ledger that cannot expose uncertainty state.

### `gauntlet/delegation`

A second deterministic evidence family specifically for delegated authority. It does not change Release Gauntlet v4 standard scenarios or its evidence hash.

## Data model

### Agent-visible intent

```go
type Intent struct {
    WorldID        world.ID
    AuthorityEpoch world.Epoch
    ActionID       string
    DelegationID   ID
    Operation      Operation
    Resource       string
    Payload        []byte
}
```

Intentionally absent:

- secret handle;
- adapter kind;
- raw endpoint URL;
- raw authentication headers.

### Host-only grant

```go
type Grant struct {
    ID             ID
    WorldID        world.ID
    AuthorityEpoch world.Epoch
    Adapter        AdapterKind
    Resource       string
    Operations     []Operation
    SecretHandle   SecretHandle
    IssuedAt       time.Time
    ExpiresAt      time.Time
}
```

`Operations` is canonicalized to a sorted unique list before persistence/digesting.

### Adapter request

The plane constructs this from the verified intent and immutable grant:

```go
type AdapterRequest struct {
    WorldID        world.ID
    ActionID       string
    Operation      Operation
    Resource       string
    Payload        []byte
    IdempotencyKey string
}
```

`IdempotencyKey` is deterministic and bound to the action/request digest. Adapters should use it whenever the provider supports idempotency.

## Grant store

The plane receives only:

```go
type Resolver interface {
    Lookup(ID) (GrantState, error)
}
```

Host administration receives a separate mutation interface:

```go
type Controller interface {
    Resolver
    Issue(Grant) error
    Revoke(ID) error
}
```

The agent execution path never receives a `Controller`.

### Durable journal

`JournalStore` uses:

- mode `0600`;
- append-only `issue` and `revoke` records;
- monotonically increasing sequence numbers;
- SHA-256 hash chain over canonical length-prefixed fields;
- fsync before in-memory state mutation becomes visible;
- single-writer lifetime file lock;
- strict replay rejecting malformed JSON, unknown JSON fields, non-canonical JSON encoding, unknown transition kinds, sequence gaps, hash mismatch, duplicate issue, revoke-before-issue, and duplicate revoke.

A grant remains queryable after revocation so historical pending actions can be reconciled against the exact original grant.

The effect `JournalLedger` applies the same fail-closed JSON parser discipline to its persisted `pending`, `aborted`, and `completed` records. Unknown fields or alternate/non-canonical encodings are treated as corruption rather than ignored input.

## Vault boundary

```go
type Vault interface {
    Use(context.Context, SecretHandle, func(Secret) error) error
}
```

`Secret` owns unexported material and exposes `Bytes()` only to trusted host adapter code. `Use` provides a fresh copy and zeroes the working buffer after the callback returns.

The plane never logs, hashes, serializes, returns, or places secret bytes into an error.

The built-in memory vault exists for tests/development only. Production KMS integration is explicitly outside v6 scope.

## Adapter boundary

```go
type Adapter interface {
    Kind() AdapterKind
    Execute(context.Context, AdapterRequest, Secret) (Effect, error)
    Reconcile(context.Context, AdapterRequest, Secret) (ReconcileResult, error)
}
```

The registry rejects:

- empty kinds;
- duplicate kinds;
- obvious generic authenticated transports: `http`, `https`, `raw-http`, `raw_http`, `generic-http`, `generic_http`, `authenticated-http`, `authenticated_http`.

The adapter receives only the already-authorized canonical resource and operation. It cannot ask the plane to widen scope.

## Execution state machine

1. Validate intent fields.
2. Require intent world ID to equal host `AuthorityState.ID()`.
3. Enter `AuthorityState.WithEpoch(intent.AuthorityEpoch, ...)`.
4. Resolve exact delegation ID.
5. Require grant world and grant epoch to equal intent world/epoch.
6. Reject revoked grant.
7. Reject expired grant.
8. Require exact `intent.Resource == grant.Resource`.
9. Require operation membership in grant allowlist.
10. Resolve adapter by **grant adapter kind**.
11. Compute request digest from intent digest + grant digest + secret-handle digest.
12. Enter the inspectable action ledger `ExecuteOnce`, which records `pending` before the effect callback.
13. Vault resolves secret only inside callback.
14. Adapter executes with deterministic idempotency key.
15. A vault/policy failure that trusted host code knows occurred before provider entry is wrapped with `authority.MarkNoEffect(...)`; only that explicit proof may abort/remove `pending` and permit a retry.
16. If adapter execution has been entered and returns an error, return stable `ErrAdapterFailure` without wrapping its text and leave the action pending/uncertain.
17. If returned evidence contains exact non-empty secret bytes, return `ErrSecretLeak`; do not create a success receipt and leave the action pending/uncertain.
18. On success, create `authority.Receipt` with request/effect digests and transition the ledger to completed.
19. Return a v6 receipt whose derived fields bind grant and secret handle digests.

A retry of a pending action in either the memory or durable ledger returns `ErrActionUncertain`; the plane never calls `Adapter.Execute` again automatically.

## Reconciliation state machine

`Reconcile` is host-oriented and may run after the delegation expired, was revoked, or the world epoch advanced. Those changes must not prevent safety observation of a historical pending effect.

1. Validate historical intent.
2. Resolve the exact immutable grant, including revoked grants.
3. Recompute the same request digest.
4. Inspect ledger status.
5. Missing → `ErrNoPendingAction`.
6. Completed → return completed derived receipt without provider execution.
7. Pending → resolve secret and call adapter `Reconcile`.
8. `observed` → secret-echo check → append `JournalLedger.Resolve` completion → return receipt.
9. `absent` → return `ErrEffectAbsent`; leave ledger pending to block automatic replay.
10. `unknown` → return `ErrActionUncertain`; leave ledger pending.

Reconciliation never calls `Adapter.Execute`.

## Error taxonomy

Agent-facing errors are stable values:

- `ErrInvalidGrant`
- `ErrDelegationNotFound`
- `ErrDelegationRevoked`
- `ErrDelegationExpired`
- `ErrScopeDenied`
- `ErrAdapterNotFound`
- `ErrGenericAdapter`
- `ErrSecretUnavailable`
- `ErrSecretLeak`
- `ErrAdapterFailure`
- `ErrReconcileFailure`
- `ErrEffectAbsent`
- `ErrNoPendingAction`
- existing `world.ErrStaleEpoch`
- existing `authority.ErrActionCollision`
- existing `authority.ErrActionUncertain`

Raw adapter/vault error strings are never wrapped into an agent-facing return path.

## V6 adversarial gauntlet

The v6 suite is mandatory and independent from v4. At minimum it proves:

1. resource rebinding denied;
2. operation escalation denied;
3. stale world epoch denied;
4. revoked delegation denied;
5. expired delegation denied;
6. adapter selection cannot be overridden by intent;
7. generic authenticated HTTP registration denied;
8. action-ID rebinding collides;
9. pending action cannot auto-replay;
10. `observed` reconciliation closes pending without re-execution;
11. `absent` reconciliation remains non-replayable;
12. exact secret echo in adapter evidence fails closed;
13. agent-visible evidence does not contain secret bytes;
14. durable grant journal survives restart and revocation remains revoked;
15. tampered grant journal fails closed.

The CLI emits a deterministic JSON report via the existing gauntlet evidence contract but uses a separate suite ID/output artifact so v4 remains byte-stable.

Ledger contract tests additionally prove that a bare policy sentinel is not enough to erase uncertainty, explicit no-effect proof can safely abort, both memory/durable ledgers quarantine unmarked failure, and persisted journals reject unknown/non-canonical JSON.

## CI

`Nolane World Check` continues to run all module tests/race/vet and v4 evidence unchanged.

A new v6 step or workflow additionally:

```bash
go run ./cmd/nolane-authority-gauntlet --out release-evidence/nolane-authority-v6.json
```

It runs the command twice and requires byte-for-byte equality before artifact upload.

No real credentials are required by v6 CI. Test vault credentials are synthetic fixed bytes and must never appear in the report.

## Non-goals

V6 does not:

- add a production KMS implementation;
- add GitHub/AWS/email provider adapters;
- expose generic authenticated HTTP;
- claim distributed consensus for host journals;
- make guest snapshots trusted memory;
- claim live KVM verification without a v5 `LIVE_PASS` artifact.

## Release gate

V6 may merge only when:

- unit tests pass;
- race tests pass;
- vet passes;
- v4 evidence remains byte-stable;
- v6 deterministic evidence verifies and is byte-stable across two executions;
- journal tamper/restart/revocation and canonical-parser tests pass;
- uncertainty tests prove unmarked failures remain pending and only explicit no-effect proof permits retry;
- secret-leak tests prove synthetic secret bytes are absent from receipts, errors, and v6 report;
- diff does not modify Cube security-core implementation;
- human DCO policy remains unforgeable by the agent.
