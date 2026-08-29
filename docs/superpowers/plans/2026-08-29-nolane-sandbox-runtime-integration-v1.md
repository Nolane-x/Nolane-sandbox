# Nolane Sandbox Runtime Integration v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect Nolane Trust Kernel semantics to CubeSandbox lifecycle, fresh-world capability validation, and crash-recoverable effect persistence without modifying Cube security internals.

**Architecture:** Extend the standalone `NolaneWorld/` Go module. Host-owned lifecycle and trust state remain outside Cube guests; Cube is accessed only through a narrow HTTP substrate adapter. External-effect durability uses a conservative append-only journal that refuses replay when an outcome is uncertain.

**Tech Stack:** Go standard library; CubeAPI HTTP contract; SHA-256; OS advisory file locking on supported Unix systems; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-runtime-integration-v1-design.md`

## Global Constraints

- Do not modify KVM, RustVMM, CubeEgress, CubeNet, CubeCoW, hypervisor, or guest-kernel security code.
- Unknown or malformed security state fails closed.
- A destroyed world is terminally revoked.
- Rollback advances authority epoch before execution-state rollback.
- A validation world is created fresh, never cloned from the origin world.
- Promotion occurs only after validator teardown succeeds.
- Remote CubeAPI requires HTTPS; redirects are not followed.
- Durable effect replay uncertainty denies re-execution.
- Commit messages include `Autonomously-by: ChatGPT:GPT-5.6-Sol` and do not add a human DCO signature.

---

### Task 1: Terminal Authority State

**Files:**
- Modify: `NolaneWorld/world/identity.go`
- Create: `NolaneWorld/world/close_test.go`
- Modify: `NolaneWorld/authority/broker.go`

**Deliverable:** Closed worlds reject all future authority, and close linearizes against in-flight authority callbacks.

- [ ] Write close and linearization tests.
- [ ] Verify RED against Trust Kernel v0.
- [ ] Add `ErrClosedWorld`, `AuthorityState`, `Close`, and `Closed`.
- [ ] Make `AdvanceEpoch` stable after terminal close.
- [ ] Generalize Broker to consume `world.AuthorityState`.
- [ ] Run world and authority tests.

### Task 2: Host World Manager

**Files:**
- Create: `NolaneWorld/control/manager.go`
- Create: `NolaneWorld/control/manager_test.go`

**Deliverable:** Create, pause, resume, snapshot, rollback, destroy, and clone obey authority/lifecycle ordering.

- [ ] Write tests for rollback-before-restore epoch advance.
- [ ] Write tests for terminal destroy on substrate failure.
- [ ] Write tests for independent clone authority.
- [ ] Write test hiding authority state during incomplete create.
- [ ] Implement manager with per-world lifecycle serialization.
- [ ] Run race tests.

### Task 3: CubeAPI Substrate Adapter

**Files:**
- Create: `NolaneWorld/substrate/cube/client.go`
- Create: `NolaneWorld/substrate/cube/client_test.go`

**Deliverable:** A fail-closed CubeAPI adapter implements `SandboxSubstrate`.

- [ ] Write HTTP contract tests using `httptest`.
- [ ] Verify remote HTTP is rejected.
- [ ] Verify redirects are never followed.
- [ ] Verify decorated/credentialed base URLs are rejected.
- [ ] Verify bounded response handling.
- [ ] Implement create/destroy/pause/resume/snapshot/rollback/clone.
- [ ] Assert compile-time `SandboxSubstrate` conformance.

### Task 4: Fresh-World Capability Forge

**Files:**
- Create: `NolaneWorld/forge/forge.go`
- Create: `NolaneWorld/forge/forge_test.go`

**Deliverable:** Agent-created bytes can become trusted only after host artifact admission, fresh-world validation, evidence binding, and successful teardown.

- [ ] Test fresh Create and zero Clone calls.
- [ ] Test invalid artifacts never start validator world.
- [ ] Test validation failure cleanup.
- [ ] Test empty evidence rejection.
- [ ] Test validator panic cleanup.
- [ ] Test teardown failure blocks promotion.
- [ ] Implement host-generated candidate and validator identities.
- [ ] Hash exact evidence bytes on host.
- [ ] Promote only after successful teardown.

### Task 5: Crash-Recoverable Effect Journal

**Files:**
- Modify: `NolaneWorld/authority/types.go`
- Create: `NolaneWorld/authority/journal_ledger.go`
- Create: `NolaneWorld/authority/journal_lock_unix.go`
- Create: `NolaneWorld/authority/journal_lock_other.go`
- Create: `NolaneWorld/authority/journal_ledger_test.go`

**Deliverable:** Restart cannot silently re-execute an action whose external outcome may already exist.

- [ ] Test completed receipt recovery after restart.
- [ ] Test executor failure recovers as uncertain.
- [ ] Test policy denial can safely retry.
- [ ] Test host reconciliation of uncertain action.
- [ ] Test request-digest collision.
- [ ] Test journal corruption failure.
- [ ] Test single-writer lock.
- [ ] Implement pending/completed/aborted transitions with `fsync`.
- [ ] Keep uncertain actions pending until explicit reconciliation.

### Task 6: Documentation and Full Verification

**Files:**
- Modify: `NolaneWorld/README.md`
- Create: `docs/superpowers/specs/2026-08-29-nolane-sandbox-runtime-integration-v1-design.md`
- Create: `docs/superpowers/plans/2026-08-29-nolane-sandbox-runtime-integration-v1.md`

**Deliverable:** Repository documents actual trust semantics and remaining production gates.

- [ ] Update module status and verification commands.
- [ ] Run `go test ./...`.
- [ ] Run `go test -race ./...`.
- [ ] Run `go vet ./...`.
- [ ] Inspect diff and verify no Cube security-core path changed.
- [ ] Push to `nolane/world-foundation-v0`.
- [ ] Verify GitHub `Nolane World Check`.
