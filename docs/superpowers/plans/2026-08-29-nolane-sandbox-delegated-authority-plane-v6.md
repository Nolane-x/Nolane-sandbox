# Delegated Authority Plane v6 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a host-owned delegated external-authority plane where agents can request typed actions without receiving credentials, and where uncertain effects require explicit reconciliation before any further execution.

**Architecture:** Add a new `delegation` package beside the existing `authority` package. Reuse world authority epochs and the existing effect ledger, extend ledgers only with read-only status inspection, persist grants/revocations in an independent host journal, and add a separate deterministic authority gauntlet so Release Gauntlet v4 remains byte-stable.

**Tech Stack:** Go 1.23, standard library only, existing Nolane `world`, `authority`, and `gauntlet` packages, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-delegated-authority-plane-v6-design.md`

## Global Constraints

- Agent-visible intent contains no secret handle and no adapter kind.
- No generic authenticated HTTP adapter.
- Grants bind exact world, epoch, adapter, resource, operation allowlist, host-only secret handle, and expiry.
- Revocation is monotonic and durable.
- Uncertain actions are never executed again automatically.
- Reconciliation observes only; it never calls adapter execution.
- Raw adapter/vault error strings and secret bytes never reach agent-facing evidence.
- V4 deterministic evidence must remain byte-stable.
- Cube security-core implementation must not be modified.

---

### Task 1: Ledger inspection contract

**Files:**
- Modify: `NolaneWorld/authority/types.go`
- Modify: `NolaneWorld/authority/ledger.go`
- Modify: `NolaneWorld/authority/journal_ledger.go`
- Modify: `NolaneWorld/authority/broker_test.go` or create `NolaneWorld/authority/status_test.go`

**Interfaces:**
- Produces `ActionStatus`, `InspectableLedger`, and `Status(world.ID,string,string)` on memory/journal ledgers.

- [ ] Write tests proving memory ledger reports missing/completed and journal ledger reports pending after reopen.
- [ ] Run focused authority tests and prove RED because the inspection API does not exist.
- [ ] Add `ActionMissing`, `ActionPending`, `ActionCompleted` plus the optional inspection interface.
- [ ] Implement collision-safe status reads without changing `Ledger.ExecuteOnce` behavior.
- [ ] Run authority unit/race tests and vet.

### Task 2: Delegation types, digesting, and in-memory control separation

**Files:**
- Create: `NolaneWorld/delegation/types.go`
- Create: `NolaneWorld/delegation/digest.go`
- Create: `NolaneWorld/delegation/store.go`
- Create: `NolaneWorld/delegation/store_test.go`

**Interfaces:**
- Produces `ID`, `SecretHandle`, `AdapterKind`, `Operation`, `Grant`, `GrantState`, `Resolver`, `Controller`, `Intent`, and deterministic grant/request digest helpers.

- [ ] Write RED tests for grant validation, operation canonicalization, immutable delegation IDs, revoke monotonicity, and digest changes on every trust-bearing field.
- [ ] Implement strict validation and sorted/unique operation canonicalization.
- [ ] Implement memory controller/resolver with separate read versus mutation interfaces.
- [ ] Verify duplicate exact issue is idempotent, rebinding collides, and revoked state remains queryable.

### Task 3: Durable grant/revocation journal

**Files:**
- Create: `NolaneWorld/delegation/journal_store.go`
- Create: `NolaneWorld/delegation/journal_lock_unix.go`
- Create: `NolaneWorld/delegation/journal_lock_other.go`
- Create: `NolaneWorld/delegation/journal_store_test.go`

**Interfaces:**
- Produces `OpenJournalStore(path) (*JournalStore,error)`, `Close`, `Issue`, `Revoke`, and `Lookup`.

- [ ] Write RED tests for restart survival, revoked-after-restart, tamper rejection, malformed tail rejection, duplicate writer lock, revoke-before-issue rejection, and duplicate-revoke rejection.
- [ ] Implement mode `0600`, sequence numbers, domain-separated length-prefixed SHA-256 record hashes, fsync-before-memory, and strict replay.
- [ ] Implement lifetime single-writer file locking matching the existing authority journal platform pattern.
- [ ] Run focused delegation tests with race detector and vet.

### Task 4: Vault and adapter registry

**Files:**
- Create: `NolaneWorld/delegation/vault.go`
- Create: `NolaneWorld/delegation/adapter.go`
- Create: `NolaneWorld/delegation/adapter_test.go`

**Interfaces:**
- Produces `Vault.Use`, host-only `Secret`, test-only `MemoryVault`, `Adapter`, `AdapterRequest`, `Effect`, `ReconcileResult`, `Registry`.

- [ ] Write RED tests proving generic HTTP kinds are rejected, duplicate adapter kinds fail, secret work buffers are zeroed after callback, and vault lookup failures are normalized.
- [ ] Implement a memory vault that clones secret bytes into a callback-scoped buffer and wipes it on return.
- [ ] Implement adapter registry with exact kind lookup and explicit generic-transport denylist.
- [ ] Ensure adapter/vault raw errors are never returned by public plane APIs.

### Task 5: Delegated execution plane

**Files:**
- Create: `NolaneWorld/delegation/plane.go`
- Create: `NolaneWorld/delegation/plane_test.go`

**Interfaces:**
- Produces `NewPlane(authorityState, Resolver, Vault, Registry, authority.Ledger, clock)` and `Execute(context.Context, Intent) (Receipt,error)`.

- [ ] Write RED tests for resource rebinding, operation escalation, stale epoch, revoked/expired grants, adapter selection from grant rather than intent, action collision, secret echo, and raw error sanitization.
- [ ] Implement exact scope checks under `AuthorityState.WithEpoch`.
- [ ] Compute request digest from intent + grant digest + secret-handle digest.
- [ ] Execute through existing ledger exactly once and adapter through vault callback only.
- [ ] Treat adapter error/secret echo as uncertain once provider execution was entered.
- [ ] Return only digests/opaque receipt fields; never credential material.

### Task 6: Explicit reconciliation

**Files:**
- Modify: `NolaneWorld/delegation/plane.go`
- Modify: `NolaneWorld/delegation/plane_test.go`

**Interfaces:**
- Produces `Reconcile(context.Context, Intent) (Receipt,error)` and stable states/errors for observed/absent/unknown.

- [ ] Write RED tests proving pending action never auto-reexecutes.
- [ ] Test `observed` reconciliation resolves a journal pending action without calling `Adapter.Execute`.
- [ ] Test `absent` and `unknown` leave action pending and non-replayable.
- [ ] Test reconciliation still works after grant revoke/expiry/epoch advance because it is historical safety observation.
- [ ] Implement inspection + `JournalLedger.Resolve` path; never call `Execute` from reconciliation.

### Task 7: Delegated Authority Gauntlet v6

**Files:**
- Create: `NolaneWorld/gauntlet/delegation/scenarios.go`
- Create: `NolaneWorld/gauntlet/delegation/scenarios_test.go`
- Create: `NolaneWorld/cmd/nolane-authority-gauntlet/main.go`
- Create: `NolaneWorld/cmd/nolane-authority-gauntlet/main_test.go`

**Interfaces:**
- Produces a separate mandatory scenario suite and deterministic JSON report using the existing gauntlet evidence contract without modifying the v4 standard suite.

- [ ] Add mandatory scenarios for resource rebinding, operation escalation, stale epoch, revoke, expiry, adapter confusion, generic HTTP, action collision, uncertain replay, observed reconciliation, absent reconciliation, secret echo, evidence secret absence, journal restart/revoke, and journal tamper.
- [ ] Run each attack against real v6 package paths; scenarios cannot self-declare PASS.
- [ ] Add CLI generating `release-evidence/nolane-authority-v6.json`.
- [ ] Run CLI twice and require byte-for-byte equality.
- [ ] Assert the synthetic secret marker is absent from the JSON bytes.

### Task 8: CI and documentation

**Files:**
- Modify: `.github/workflows/nolane-world-check.yml`
- Modify: `NolaneWorld/README.md`

**Interfaces:**
- CI emits both unchanged v4 evidence and new v6 authority evidence.

- [ ] Add v6 CLI generation twice, `cmp`, secret-marker grep-negative check, and artifact upload.
- [ ] Keep existing v4 generation unchanged.
- [ ] Update README with v6 architecture, non-goals, verification commands, and explicit statement that MemoryVault is not production KMS.
- [ ] Run full `go test ./...`, `go test -race ./...`, `go vet ./...`.
- [ ] Confirm v4 artifact SHA remains `94ef192c57f2587d34a8340a8bfd8d297782e121c88ad4aa96792e42bf40c6f4`.

### Task 9: GitHub release gate

**Files:** none beyond fixes found by CI.

- [ ] Compare branch to `master`; confirm no Cube security-core file changed.
- [ ] Open PR with `Autonomously-by: ChatGPT:GPT-5.6-Sol` and no `Signed-off-by`.
- [ ] Require Nolane World, Docs, Format, and v6 evidence technical gates to succeed on exact head.
- [ ] Treat DCO failure as the human attestation gate; never forge it.
- [ ] Squash-merge only with expected head SHA after technical gates are green.
- [ ] Verify `master` contains v6 and its post-merge Nolane World Check succeeds.
