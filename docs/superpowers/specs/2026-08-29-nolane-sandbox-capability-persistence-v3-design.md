# Nolane Sandbox Capability Persistence v3 Design

**Status:** Implementation specification

**Date:** 2026-08-29

**Depends on:** Trust Kernel v0, Runtime Integration v1, Persistence v2

## 1. Goal

A promoted capability is trusted only while its exact implementation, manifest, independent validation evidence, and promotion provenance remain cryptographically bound and recoverable outside every guest world.

The invariant is:

> **A receipt without its exact referenced bytes is not trusted state.**

V3 makes capability promotion crash-recoverable and tamper-evident without turning guest snapshots into trusted memory.

## 2. Store boundary

Forge depends on `capability.Store`, not the concrete in-memory Registry. The store exposes promotion and metadata lookup. `Registry` remains the deterministic in-memory implementation; `DurableRegistry` is the persistent implementation.

`PromotionRequest` carries exact `Content`, `Manifest`, and `VerificationEvidence` bytes plus their declared SHA-256 digests. Promotion rejects missing evidence or any digest mismatch. Forge copies and passes the exact evidence returned by the fresh validator.

Broker/agent code cannot directly mutate durable promotion indexes. Only the Promotion Gate calls Store.Promote after validator teardown succeeds.

## 3. Content-addressed trusted material

DurableRegistry owns a host-only root containing:

```text
promotions.jsonl
blobs/sha256/<first-two-hex>/<full-sha256>
```

Names, versions, CandidateID, verifier identity, or WorldID are never used as blob paths. Blob identity is exact SHA-256 only.

For a new promotion, content, manifest and evidence blobs are created with exclusive-create semantics, written, fsynced, closed, and their directory synced before the promotion journal is appended. Existing blobs are never overwritten; they are re-hashed and must already equal the requested digest.

A crash after blob persistence but before journal append may leave orphan blobs. Orphan blobs carry zero trust and are ignored because only a valid promotion journal record creates a trusted registry entry.

## 4. Promotion journal

Each JSONL record contains:

```text
version
sequence
exact Candidate metadata
exact PromotionReceipt
previous_hash
record_hash
```

The record hash uses a domain separator and length-prefixed canonical fields, including candidate creation time, verifier identity, validation digest, promotion time and previous hash.

A promotion journal record is fsynced before it becomes visible in the registry's in-memory indexes.

## 5. Recovery

Opening DurableRegistry strictly replays the entire journal and verifies:

- supported record version;
- contiguous sequence and previous-hash chain;
- exact record hash;
- complete candidate identity and bounded metadata;
- candidate digest recomputation;
- capability ID recomputation;
- candidate/receipt identity equivalence;
- independent verifier identity;
- content, manifest and evidence digest syntax;
- immutable candidate-ID uniqueness;
- immutable name/version uniqueness;
- existence, regular-file type and exact SHA-256 of every referenced blob.

Any ambiguity fails startup closed. DurableRegistry does not reconstruct missing trusted bytes from an agent request during recovery.

## 6. Idempotency and collision semantics

An exact retry of the same candidate after restart returns the original receipt and does not append another promotion record. Reusing a CandidateID with changed candidate metadata, verifier, or verification digest is denied. Reusing the same capability name/version with another candidate or material is denied.

These rules are reconstructed from durable journal state, not guest memory.

## 7. Local writer ownership

The promotion journal is advisory-locked for the lifetime of DurableRegistry. A second writer is rejected. Unsupported locking platforms fail closed. V3 is a single-host writer primitive, not distributed consensus.

## 8. Forge ordering

The trusted path is:

```text
origin bytes
-> Artifact Gate admission
-> fresh validator world Create
-> validation
-> exact evidence bytes returned
-> validator world Destroy succeeds
-> DurableRegistry persists content/manifest/evidence CAS blobs
-> DurableRegistry fsyncs promotion journal
-> trusted capability becomes visible
```

Validator failure, panic, empty evidence, or teardown failure never reaches promotion.

## 9. Required executable contracts

Tests prove:

- exact evidence bytes are mandatory and digest-bound;
- content/manifest/evidence survive DurableRegistry reopen exactly;
- second writer is rejected;
- journal byte tamper and malformed tail are rejected;
- missing or tampered trusted blob makes recovery fail closed;
- orphan CAS blob alone creates no trusted capability;
- exact retry after restart returns same receipt without new journal entry;
- name/version and CandidateID rebinding remain rejected after restart;
- Forge using DurableRegistry persists validation evidence only after validator teardown;
- unit, race and vet suites remain green.

## 10. Non-goals / next gates

V3 does not provide distributed consensus, KMS, general artifact quarantine persistence, live hypervisor escape testing, or external-service authority adapters. The next trust milestones are durable general artifact quarantine/receipts, typed KMS-backed authority adapters with reconciliation, and the live Cube/KVM adversarial gauntlet.
