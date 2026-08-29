# Nolane Sandbox Trust Kernel

`NolaneWorld` is the host-owned trust/evolution boundary for **Nolane Sandbox**.

The execution substrate remains CubeSandbox. Nolane does not fork the hypervisor, KVM/RustVMM integration, CubeEgress, CubeNet, or CubeCoW for ordinary agent behavior. Instead it defines the rules that let an untrusted AI build arbitrary software capabilities inside disposable worlds without being able to mint external authority, certify itself, or turn execution snapshots into trusted state.

## Implemented

### Trust Kernel v0

- compiled 12-law constitution;
- world identity and monotonic authority epochs;
- typed authority intents and fail-closed policy decisions;
- exact duplicate/collision semantics for effect receipts;
- candidate → independent promotion → trusted registry;
- exact content and manifest binding;
- artifact admission/provenance receipts;
- explicit network authority classes `N0` through `N5`;
- narrow `SandboxSubstrate` interface.

### Runtime Integration v1

- terminal world authority revocation;
- host-owned World Manager for create/pause/resume/snapshot/rollback/destroy/clone;
- rollback epoch advance before execution-state restoration;
- fail-closed CubeAPI adapter with HTTPS/loopback policy, no redirects, bounded responses, no public network by default;
- Capability Forge using fresh validator worlds rather than origin clones;
- host-hashed validation evidence and teardown-before-promotion;
- validator panic/failure cleanup;
- crash-recoverable append-only effect JournalLedger;
- uncertain-outcome replay denial and host reconciliation;
- single-writer OS locking for the durable effect journal.

### Persistence v2

- host-only `AuthorityControl` separated from broker-facing authority reads/execution;
- per-world append-only authority journals with SHA-256 hash chains;
- epoch advances and terminal close fsynced before becoming visible in memory;
- hashed authority filenames so raw WorldID values never become host paths;
- strict replay that rejects malformed, unknown, out-of-order, identity-mismatched, or hash-mismatched records;
- lifetime single-writer authority file locks;
- hash-chained lifecycle catalog for `creating -> ready -> terminal -> destroyed`;
- persistent World Manager recovery;
- incomplete create/clone recovery terminally revokes authority and reports a possible orphan instead of guessing;
- Manager shutdown serializes against in-flight lifecycle mutations.

### Capability Persistence v3

- `capability.Store` decouples Forge from in-memory registry implementation;
- every promotion binds exact content, manifest, and validation-evidence bytes;
- `DurableRegistry` stores all trusted material in SHA-256 content-addressed storage;
- promotion trust is an append-only, fsynced, hash-chained journal;
- exact retry across restart is idempotent and never appends a second promotion;
- candidate-ID and name/version collision rules survive restart;
- recovery verifies every promotion record and every referenced CAS blob before trusting registry state;
- missing, non-regular, tampered, malformed, or hash-mismatched state fails closed;
- orphan CAS blobs alone never create trusted capability records;
- single-writer OS locking prevents split-brain local registry writers;
- Forge persists the exact validator evidence only after clean validator teardown.

## Security model

Assume the model, agent, guest root, downloaded packages, webpages, generated code, snapshots, and exported files are hostile.

The Trust Kernel owns authority state outside the guest. A guest snapshot is execution state only; it is never a source of truth for authority epochs, external-effect history, promotion receipts, or trusted capability records.

The central invariant is:

> **Unbounded capability creation; bounded authority, promotion, truth, and persistence.**

A second invariant is intentionally conservative:

> **When the real-world outcome is uncertain, do not execute it again automatically.**

## What is not production-complete yet

Capability Persistence v3 still does not claim a perfect sandbox or complete production boundary. The local journals are single-host crash-recovery primitives, not distributed consensus and not protection against rollback of the entire host storage device. Remaining release gates include durable general artifact-receipt/quarantine storage, KMS/secret brokering, typed external adapters with reconciliation, live Cube/KVM stale-snapshot tests, egress bypass gauntlets, and hostile artifact corpus testing.

See:

- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-world-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-runtime-integration-v1-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-persistence-v2-design.md`

## Verify

```bash
cd NolaneWorld
go test ./...
go test -race ./...
go vet ./...
```
