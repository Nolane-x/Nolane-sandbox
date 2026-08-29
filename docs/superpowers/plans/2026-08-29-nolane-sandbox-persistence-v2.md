# Nolane Sandbox Persistence v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist authority epochs, terminal revocation, and world lifecycle truth across Trust Plane restart without ever recreating authority from guest state.

**Architecture:** Introduce a host-only `AuthorityControl` interface, an append-only hash-chained durable authority journal per WorldID, and a hash-chained lifecycle catalog. Generalize World Manager over these stores while retaining in-memory defaults.

**Tech Stack:** Go standard library; JSONL; SHA-256; fsync; advisory OS file locks.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-persistence-v2-design.md`

## Global Constraints

- Host restart may reduce availability but must never resurrect authority.
- Raw WorldID is never a filesystem path.
- Unknown/corrupt persistence state fails closed.
- Durable mutation is fsynced before in-memory authority/lifecycle mutation becomes visible.
- One host writer owns each durable authority file and lifecycle catalog.
- Do not modify CubeSandbox security internals.

---

### Task 1: Host AuthorityControl Interface

**Files:**
- Modify: `NolaneWorld/world/identity.go`
- Create: `NolaneWorld/world/control_test.go`

**Deliverable:** Broker-facing authority remains narrow while Manager can perform error-returning host mutations.

- [ ] Add RED compile/behavior tests for `AuthorityControl`.
- [ ] Add `AdvanceAuthority`, `CloseAuthority`, and `Release` adapters to in-memory State.
- [ ] Keep legacy `AdvanceEpoch`/`Close` behavior intact.
- [ ] Run world + authority tests.

### Task 2: Durable Authority Journal

**Files:**
- Create: `NolaneWorld/world/durable.go`
- Create: `NolaneWorld/world/durable_lock_unix.go`
- Create: `NolaneWorld/world/durable_lock_other.go`
- Create: `NolaneWorld/world/durable_test.go`

**Deliverable:** Per-world hash-chained authority truth survives restart and rejects tamper/corruption.

- [ ] Write RED tests for restart epoch, terminal restart, stale epoch, tamper, malformed tail, identity mismatch, and single writer.
- [ ] Implement hashed filename from WorldID.
- [ ] Implement exclusive create + fsynced init record.
- [ ] Implement strict replay with `DisallowUnknownFields`.
- [ ] Implement hash-chain verification and legal transitions.
- [ ] Implement fsynced advance/close before memory update.
- [ ] Implement OS lifetime lock.
- [ ] Run race tests.

### Task 3: Lifecycle Catalog

**Files:**
- Create: `NolaneWorld/control/catalog.go`
- Create: `NolaneWorld/control/catalog_lock_unix.go`
- Create: `NolaneWorld/control/catalog_lock_other.go`
- Create: `NolaneWorld/control/catalog_test.go`

**Deliverable:** `creating -> ready -> terminal -> destroyed` lifecycle truth survives restart with hash-chain verification.

- [ ] Write RED legal-transition/recovery tests.
- [ ] Write RED illegal transition, tamper, malformed tail, duplicate, and single-writer tests.
- [ ] Implement append+fsync transition journal.
- [ ] Bind every record to sequence, previous hash, WorldID, handle and state.
- [ ] Reject all transition ambiguity.

### Task 4: Persistent World Manager

**Files:**
- Modify: `NolaneWorld/control/manager.go`
- Modify: `NolaneWorld/control/manager_test.go`
- Create: `NolaneWorld/control/persistent_manager_test.go`

**Deliverable:** Runtime lifecycle ordering uses durable authority/catalog transitions and recovers fail-closed after restart.

- [ ] Generalize manager entries from `*world.State` to `world.AuthorityControl`.
- [ ] Add state factory + catalog interfaces and in-memory defaults.
- [ ] Add `NewPersistentManager` / recovery constructor.
- [ ] Persist create intent before substrate creation and ready after returned handle.
- [ ] Persist epoch advance before rollback callback.
- [ ] Persist terminal authority and catalog state before destroy callback.
- [ ] Quarantine `creating` entries on recovery and report possible orphan.
- [ ] Fail recovery if cataloged authority storage is missing/corrupt.
- [ ] Run race tests.

### Task 5: Full Verification

**Files:**
- Modify: `NolaneWorld/README.md`

**Deliverable:** Persistence v2 is documented and regression gated.

- [ ] Update README with persistent authority/lifecycle semantics and non-goals.
- [ ] Run `go test ./...`.
- [ ] Run `go test -race ./...`.
- [ ] Run `go vet ./...`.
- [ ] Inspect diff for forbidden Cube security-core changes.
- [ ] Push implementation commit and verify GitHub `Nolane World Check`.
