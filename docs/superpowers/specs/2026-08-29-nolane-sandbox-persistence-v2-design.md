# Nolane Sandbox Persistence v2 Design

**Status:** Implementation spec

**Date:** 2026-08-29

**Depends on:** Runtime Integration v1

## 1. Goal

Persistence v2 makes host-owned authority and world lifecycle truth survive Trust Plane process restart without trusting guest snapshots or guessing after a partial operation.

The new invariant is:

> **A host restart may reduce availability, but it must never resurrect authority.**

This milestone adds:

- crash-safe durable authority state;
- a durable world lifecycle catalog;
- persistent-manager recovery semantics;
- quarantine of incomplete create/clone operations;
- single-writer ownership and tamper-evident hash chains.

## 2. Authority control interface

The runtime distinguishes read/execute authority from host mutation authority.

`AuthorityState` remains the broker-facing interface.

A new host-only `AuthorityControl` extends it with:

```text
AdvanceAuthority() (Epoch, error)
CloseAuthority() (Epoch, error)
Closed() bool
Release() error
```

Guest code receives neither the controller nor its storage path.

The existing in-memory `State` implements the interface through adapters with no persistence errors.

## 3. Durable AuthorityState

Each world gets a dedicated append-only authority journal in a host-only directory.

The filename is derived from SHA-256 of `WorldID`; raw world IDs are never used as filesystem paths.

### 3.1 Journal records

Each JSONL record contains:

```text
version
sequence
operation: init | advance | close
world_id
epoch
previous_hash
record_hash
```

`record_hash` binds a domain separator, version, sequence, operation, exact world ID, epoch, and previous hash.

### 3.2 Creation

Creation uses exclusive file creation. The initial `init` record for epoch 1 is written and fsynced before the state is returned.

A pre-existing state path is not overwritten.

### 3.3 Advance

`AdvanceAuthority`:

```text
exclusive state lock
-> reject closed state
-> compute next epoch
-> append + fsync advance record
-> update in-memory epoch
-> return new epoch
```

If durable append fails, the in-memory epoch does not change.

### 3.4 Terminal close

`CloseAuthority`:

```text
exclusive state lock
-> if already closed: return stable epoch
-> append + fsync close at epoch+1
-> mark closed
```

All future authority validation is denied.

### 3.5 Recovery

Opening an existing state replays every record and rejects:

- malformed JSON;
- unknown fields;
- unsupported version;
- non-contiguous sequence;
- incorrect world identity;
- illegal operation order;
- epoch jumps/decrements;
- record-hash mismatch;
- previous-hash mismatch;
- data after terminal close.

Corruption fails closed.

### 3.6 Single writer

The authority file is advisory-locked for its lifetime. A second process attempting to open the same world fails.

Unsupported locking platforms fail closed.

## 4. Durable lifecycle catalog

The Trust Plane maintains one append-only host catalog mapping world identity to opaque substrate handle and lifecycle phase.

States are:

```text
absent -> creating -> ready -> terminal -> destroyed
```

`creating -> terminal` is allowed for failed/uncertain creation.

`ready -> terminal` is the only authority-bearing shutdown transition.

No transition out of `terminal` or `destroyed` exists.

Each catalog record is hash-chained and fsynced before the corresponding transition becomes visible in memory.

## 5. Crash ordering

### 5.1 Create

```text
create durable AuthorityState(epoch=1)
-> catalog creating (fsync)
-> substrate Create
-> catalog ready(handle) (fsync)
-> expose world as ready
```

If substrate create fails, authority is terminally closed and catalog transitions to terminal.

If the host crashes while catalog says `creating`, recovery does not guess whether a sandbox was created. It terminally closes authority and leaves a recovery issue indicating a possible orphan execution sandbox.

Because Nolane Cube worlds carry `metadata["nolane.world.id"]`, a later reconciliation worker can locate and destroy such orphans without granting authority.

### 5.2 Rollback

```text
persist authority epoch advance
-> substrate rollback
```

A crash or rollback failure never rewinds the durable epoch.

### 5.3 Destroy

```text
persist terminal authority close
-> catalog terminal (fsync)
-> substrate destroy
-> catalog destroyed (fsync)
```

A crash before substrate destroy may leak compute, but leaked compute has terminally revoked authority.

### 5.4 Clone

Clone follows create ordering for the child. Execution state may derive from a snapshot; child authority begins at fresh epoch 1.

## 6. Persistent Manager

`control.Manager` accepts host-side pluggable state factory and lifecycle catalog.

`NewManager` remains an in-memory convenience constructor.

`NewPersistentManager` uses durable state + durable catalog and first recovers catalog truth.

Recovery rules:

- `ready`: open exact durable authority state; expose only if it is not closed;
- `creating`: close authority, transition catalog to terminal, report `RecoveryIssuePossibleOrphan`;
- `terminal`: ensure authority is closed; keep handle for destroy retry;
- `destroyed`: ensure authority is closed; never expose as active.

Any missing/corrupt authority state for a cataloged world fails manager recovery rather than silently creating a new epoch-1 state.

## 7. Failure policy

Persistence failures are security failures.

- authority epoch cannot be persisted -> rollback/authority mutation is denied;
- terminal close cannot be persisted -> destructive lifecycle action is denied;
- catalog terminal transition cannot persist -> substrate destroy is not invoked;
- catalog ready cannot persist after substrate creation -> authority is immediately terminally closed and recovery reports possible orphan;
- corrupted journal/catalog -> startup fails closed.

## 8. Required tests

Tests must prove:

- epoch survives close/reopen;
- terminal revocation survives close/reopen;
- old epoch stays stale after process restart;
- wrong world identity cannot open another journal;
- hash-chain tamper is detected;
- truncated/malformed tail is rejected;
- single-writer state lock;
- lifecycle catalog legal transitions recover exactly;
- illegal catalog transitions fail;
- catalog single-writer lock;
- persistent rollback advances durable epoch before substrate callback;
- failed rollback keeps advanced epoch after manager restart;
- persistent destroy closes authority before substrate callback;
- incomplete create is quarantined on recovery;
- no cataloged world silently gets a fresh epoch-1 state on missing/corrupt authority storage.

## 9. Non-goals

Persistence v2 does not yet provide distributed consensus or multi-host concurrent writers. It is a single Trust Plane writer foundation. Multi-node operation must place the same interfaces over a transactional/consensus-backed store rather than sharing these local files over unsafe network filesystems.

## 10. Next gates

After Persistence v2 the largest remaining trust gaps are durable capability/provenance storage, KMS-backed authority adapters, and live Cube/KVM adversarial gauntlets.
