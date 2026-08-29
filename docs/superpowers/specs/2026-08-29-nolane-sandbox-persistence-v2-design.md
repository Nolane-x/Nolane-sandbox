# Nolane Sandbox Persistence v2 Design

**Status:** Implemented milestone specification

**Date:** 2026-08-29

**Depends on:** Runtime Integration v1

## 1. Goal

Persistence v2 makes host-owned authority and world lifecycle truth survive Trust Plane process restart without trusting guest snapshots or guessing after a partial operation.

The invariant is:

> **A host restart may reduce availability, but it must never resurrect authority.**

This milestone adds crash-safe durable authority state, a durable lifecycle catalog, persistent-manager recovery, quarantine of incomplete create/clone operations, single-writer ownership, tamper-evident hash chains, and a broker-facing terminal fence.

## 2. Authority interfaces and ownership

The runtime strictly separates read/execute authority from host mutation authority.

`AuthorityState` is broker-facing and exposes only identity, epoch observation/validation, and `WithEpoch` execution linearization.

`AuthorityControl` is host-only and extends `AuthorityState` with:

```text
AdvanceAuthority() (Epoch, error)
CloseAuthority() (Epoch, error)
Closed() bool
Release() error
```

Guest code and authority adapters never receive an `AuthorityControl`.

`Manager.AuthorityState` returns a managed read-only view rather than the underlying mutable controller. The view is bound to a manager-owned atomic terminal fence. Once the lifecycle is terminal, all already-issued views deny authority immediately. This prevents a retained broker pointer from bypassing a later terminal transition, including the case where the durable authority-close write itself temporarily fails.

The existing in-memory `State` implements `AuthorityControl` while preserving legacy `AdvanceEpoch` and `Close` behavior.

## 3. Durable authority state

Each world gets a dedicated append-only authority journal in a host-only directory. The filename is SHA-256 of exact `WorldID`; raw IDs are never host filesystem paths.

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

`record_hash` binds a domain separator, version, sequence, operation, exact world identity, epoch, and previous hash.

### 3.1 Creation

Creation uses exclusive file creation. Epoch 1 `init` is written and fsynced before the state is returned. The containing directory is synced so the file's existence is durable. Existing state is never overwritten.

### 3.2 Advance

`AdvanceAuthority` serializes on the state lock, rejects a closed/released state, appends and fsyncs `epoch+1`, then changes the in-memory epoch. A failed durable append leaves the old in-memory epoch unchanged.

### 3.3 Terminal close

`CloseAuthority` appends and fsyncs `close(epoch+1)` before marking the authority controller closed. Repeated close is idempotent. All future validation is denied.

### 3.4 Recovery

Opening an existing state strictly replays every record and rejects malformed JSON, unknown fields, unsupported version, non-contiguous sequence, wrong world identity, illegal operation order, epoch jumps/decrements, record-hash mismatch, previous-hash mismatch, and data after terminal close. Ambiguity is corruption and fails closed.

### 3.5 Single writer

The authority file is advisory-locked for the object's lifetime. A second writer fails. Platforms where locking cannot be implemented by this milestone fail closed rather than silently degrade.

## 4. Durable lifecycle catalog

The Trust Plane owns an append-only catalog mapping world identity to opaque substrate handle and lifecycle phase:

```text
absent -> creating -> ready -> terminal -> destroyed
              \-------> terminal
```

`creating -> terminal` is the quarantine path for failed or uncertain creation. There is no transition out of `terminal` or `destroyed`.

Each catalog record binds version, global sequence, exact WorldID, opaque handle, phase, previous hash, and record hash. Every transition is appended and fsynced before becoming visible in catalog memory. The catalog is single-writer locked and strict-replayed on startup.

## 5. Crash ordering

### 5.1 Create

```text
create durable authority(epoch=1)
-> catalog creating (fsync)
-> substrate Create
-> catalog ready(handle) (fsync)
-> expose managed authority view
```

If substrate creation fails, authority is terminally closed and catalog transitions to terminal. If the host crashes while catalog says `creating`, recovery never guesses whether a sandbox exists: it closes authority, transitions terminal, and reports `RecoveryIssuePossibleOrphan`.

### 5.2 Rollback

```text
persist authority epoch advance
-> substrate rollback
```

Rollback failure or process crash can leak an incremented epoch but can never restore an older one.

### 5.3 Destroy

Persistent teardown uses two independent fences:

```text
catalog terminal (fsync)
-> set manager atomic terminal fence
-> durable authority close (fsync)
-> substrate destroy
-> catalog destroyed (fsync)
```

The catalog terminal transition is deliberately first. It is the recoverable lifecycle fact and the manager fence immediately invalidates every broker-facing authority view already issued in the process. If durable authority close then fails, destruction is denied and the world remains terminal; recovery retries the authority close before the world can ever be considered active or before substrate destruction proceeds.

For the in-memory manager, authority close occurs before setting terminal/fence because there is no durable catalog boundary to recover.

### 5.4 Clone

Execution may derive from a source snapshot; authority never does. The child receives a newly created authority state at epoch 1 and follows the same `creating -> ready` persistence sequence as Create.

## 6. Persistent Manager

`control.Manager` is parameterized by `AuthorityFactory` and `LifecycleCatalog`. `NewManager` keeps in-memory behavior; `NewPersistentManager` performs strict recovery.

Recovery rules:

- `ready`: open exact durable authority. If unexpectedly already closed, transition catalog terminal and fence it.
- `creating`: close authority, transition terminal, fence it, report possible orphan.
- `terminal`: ensure authority is durably closed, fence it, keep handle for destroy retry.
- `destroyed`: ensure authority is closed, fence it, never expose it as active.

Missing/corrupt authority storage for any cataloged world fails startup instead of minting a replacement epoch-1 state.

Manager shutdown serializes against lifecycle operations, releases writer locks, closes the catalog, and makes already-issued managed authority views fail closed.

## 7. Failure policy

Persistence failures are security failures.

- authority epoch cannot be fsynced -> rollback is denied;
- lifecycle terminal cannot be fsynced -> no terminal transition is assumed and no substrate destroy is attempted;
- lifecycle terminal is durable but authority close fails -> existing managed views are fenced immediately, substrate destroy is denied, retry/recovery must close authority first;
- catalog `ready` cannot persist after substrate creation -> capability is never exposed as ready; authority is closed and the execution is quarantined before any cleanup attempt;
- incomplete create/clone after crash -> terminal/quarantine, never fresh authority;
- corrupted journal/catalog -> startup fails closed.

## 8. Required executable contracts

Tests prove at minimum:

- authority epoch survives release/reopen;
- terminal revocation survives restart;
- old epoch stays stale after restart;
- raw WorldID cannot become a host path;
- wrong identity/tamper/malformed tail is rejected;
- second authority/catalog writer is rejected;
- lifecycle legal transitions recover exactly and illegal ones fail;
- persistent rollback advances durable epoch before substrate callback;
- failed rollback keeps the advanced epoch after restart;
- failed destroy keeps terminal authority after restart;
- incomplete create is quarantined on recovery;
- missing authority storage for a cataloged world fails recovery;
- ready-persistence failure cannot trigger destruction before durable terminal quarantine;
- manager shutdown waits for in-flight lifecycle mutation;
- broker-facing authority does not implement `AuthorityControl`;
- an already-issued broker view is denied immediately after durable terminal lifecycle even if the authority close write fails;
- an already-issued view is denied after Manager shutdown.

## 9. Non-goals

Persistence v2 is single-host crash recovery, not distributed consensus. These local files are not safe shared-state primitives for multi-host concurrent writers and do not protect against rollback of the entire trusted host filesystem/device. A multi-node control plane must place the same interfaces over transactional/consensus-backed trusted storage.

## 10. Remaining release gates

The largest remaining trust gaps after v2 are durable capability/provenance storage, KMS-backed authority adapters, typed external-action reconciliation, hostile artifact corpus testing, and live Cube/KVM adversarial gauntlets for egress, metadata, cross-world traffic, stale snapshots, mounts, and resource exhaustion.
