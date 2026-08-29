# Nolane Sandbox Persistence v2 Implementation Plan

> **For agentic workers:** Execute with TDD and verification-before-completion. Do not modify CubeSandbox security internals.

**Goal:** Persist authority epochs, terminal revocation, and world lifecycle truth across Trust Plane restart without recreating authority from guest state.

**Architecture:** Use host-only `AuthorityControl`, managed read-only broker views, per-world hash-chained authority journals, a hash-chained lifecycle catalog, and strict persistent-manager recovery. The durable lifecycle `terminal` transition is also a process-local broker fence; substrate destruction requires both terminal lifecycle durability and durable authority close.

**Tech Stack:** Go standard library; JSONL; SHA-256; fsync; advisory OS file locks.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-persistence-v2-design.md`

## Global constraints

- Host restart may reduce availability but must never resurrect authority.
- Raw WorldID is never a filesystem path.
- Unknown/corrupt persistence state fails closed.
- Durable mutation is fsynced before its authoritative in-memory transition.
- One host writer owns each authority file and lifecycle catalog.
- Broker-facing callers never receive `AuthorityControl`.
- Already-issued broker views must fail immediately after terminal fence or Manager shutdown.
- No substrate destroy before terminal lifecycle is durable and authority close succeeds.
- Do not modify KVM, RustVMM, CubeEgress, CubeNet, CubeCoW, hypervisor, or guest-kernel security code.

---

### Task 1: Host authority-control boundary

**Files:** `NolaneWorld/world/identity.go`, `NolaneWorld/world/control_test.go`

- [x] Add `AuthorityControl` with `AdvanceAuthority`, `CloseAuthority`, `Closed`, `Release`.
- [x] Preserve legacy in-memory state behavior.
- [x] Make broker-facing API depend only on `AuthorityState`.

### Task 2: Durable authority journal

**Files:** `NolaneWorld/world/durable.go`, `durable_lock_*.go`, `durable_test.go`

- [x] Hash WorldID into filename.
- [x] Exclusive create and fsynced epoch-1 init.
- [x] Strict JSON replay with unknown-field rejection.
- [x] SHA-256 hash-chain and legal transition verification.
- [x] Fsync advance/close before memory mutation.
- [x] Lifetime single-writer lock; unsupported platforms fail closed.
- [x] Tests for restart, terminal state, stale epoch, tamper, malformed tail, identity/path binding and lock contention.

### Task 3: Lifecycle catalog

**Files:** `NolaneWorld/control/catalog.go`, `catalog_lock_*.go`, `catalog_test.go`

- [x] Implement `creating -> ready -> terminal -> destroyed`, plus `creating -> terminal` quarantine.
- [x] Hash-chain every global lifecycle transition.
- [x] Append + fsync before catalog memory transition.
- [x] Strict replay and single-writer lock.
- [x] Tests for recovery, illegal transitions, tamper, malformed tail and second writer.

### Task 4: Persistent World Manager

**Files:** `NolaneWorld/control/manager.go`, `manager_test.go`, `persistent_manager_test.go`

- [x] Generalize entries to `AuthorityControl` while exposing only managed `AuthorityState` views.
- [x] Add persistent factory/catalog constructor and strict recovery.
- [x] Persist create intent before substrate Create and ready after returned handle.
- [x] Persist epoch advance before rollback callback.
- [x] Quarantine incomplete create/clone recovery.
- [x] Fail recovery on missing/corrupt authority storage.
- [x] Serialize Manager shutdown against in-flight lifecycle operations.
- [x] Add atomic terminal fence for already-issued broker views.
- [x] Persist lifecycle terminal before authority close; if close fails, deny destruction and retry later.
- [x] Require successful durable authority close before substrate destroy.

### Task 5: Documentation and verification

**Files:** `NolaneWorld/README.md`, Persistence v2 spec and plan

- [x] Document exact crash ordering and non-goals.
- [x] Local `go test ./...`.
- [x] Local `go test -race ./...`.
- [x] Local `go vet ./...`.
- [ ] Commit exact implementation tree to `nolane/world-foundation-v0`.
- [ ] Verify GitHub `Nolane World Check` on exact commit.
- [ ] Inspect branch diff and confirm no Cube security-core files changed.

DCO remains a human attestation gate and must not be synthesized by an AI agent.
