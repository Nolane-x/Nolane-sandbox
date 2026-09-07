# Wave 23 Exact Realm/Cube Realization Binding Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind the current host-owned Realm World realization to the exact Cube sandbox/resource binding using opaque, freshness-checked package authority.

**Architecture:** `realm.Controller` mints and revalidates an opaque realization authority from its private Store. The Cube adapter revalidates that authority and requires exact equality with `ResourceBinding.SandboxID()` before minting a second opaque cross-boundary authority. No serialized or caller-built object can restore authority.

**Tech Stack:** Go, existing `realm`, `substrate/cube`, and `resourceproof` packages; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-07-wave23-realm-cube-realization-binding-authority.md`

## Global Constraints

- Realm `WorldRecord.RealizationRevision` is never substituted with Cube task generation.
- Canonical policy identity is `realm.PolicyDigest(current.Spec, current.Revision)`.
- Runtime digest is not fabricated; Wave23 does not create a generic LIVE_PASS runtime/CLI.
- Zero, stale, terminal, pre-ready, handle-mismatched, and caller-reconstructed authority fails closed.
- Existing Wave17–22 provenance boundaries remain unchanged.

---

### Task 1: Behavioral RED contract

**Files:**
- Create: `NolaneWorld/realm/realization_authority_v23_test.go`
- Create: `NolaneWorld/substrate/cube/realm_resource_authority_v23_test.go`

**Interfaces:**
- Consumes: existing `Controller`, `MemoryStore`, `RealmRecord`, `WorldRecord`, `ResourceBinding`.
- Produces expected API: `Controller.CurrentRealizationAuthority`, `Controller.ValidateRealizationAuthority`, `RealizationAuthority.Binding`, `ValidateRealmResourceAuthority`.

- [ ] Write tests proving current authoritative phases mint and pre-ready/terminal phases fail.
- [ ] Write tests proving Realm revision, realization revision and substrate-handle drift stale old authority.
- [ ] Write tests proving exact Cube sandbox accepts and mismatch rejects.
- [ ] Run focused Go tests and record RED due missing Wave23 API.
- [ ] Commit RED with autonomous provenance trailer.

### Task 2: Realm realization authority

**Files:**
- Create: `NolaneWorld/realm/realization_authority.go`
- Test: `NolaneWorld/realm/realization_authority_v23_test.go`

**Interfaces:**
- Produces `type RealizationBinding`, opaque `type RealizationAuthority`, `(*Controller).CurrentRealizationAuthority(ctx, realmID, worldID)`, `(*Controller).ValidateRealizationAuthority(ctx, authority)`.

- [ ] Implement sealed zero-invalid authority.
- [ ] Compute policy digest from current Realm spec/revision inside the controller.
- [ ] Admit only observed-ready, leased, paused World phases with nonempty handle.
- [ ] Re-read current store state for validation and exact-compare all sealed dimensions.
- [ ] Run focused Realm tests GREEN.
- [ ] Commit production boundary.

### Task 3: Cube exact sandbox bridge

**Files:**
- Create: `NolaneWorld/substrate/cube/realm_resource_authority.go`
- Test: `NolaneWorld/substrate/cube/realm_resource_authority_v23_test.go`

**Interfaces:**
- Consumes: `realm.RealizationAuthority`, `realm.Controller`, `cube.ResourceBinding`.
- Produces opaque `RealmResourceAuthority` and `ValidateRealmResourceAuthority(ctx, controller, realizationAuthority, resourceBinding)`.

- [ ] Revalidate Realm authority immediately before Cube comparison.
- [ ] Require nonempty exact equality between sealed substrate handle and `ResourceBinding.SandboxID()`.
- [ ] Mint opaque zero-invalid cross-boundary authority only on exact match.
- [ ] Run Cube-focused tests GREEN.
- [ ] Commit bridge.

### Task 4: Dedicated static/CI contract

**Files:**
- Create: `tests/wave23_realm_cube_realization_contract.py`
- Create: `.github/workflows/wave23-realm-cube-realization-contract.yml`

**Interfaces:**
- Static contract checks opacity/trust shape; workflow runs Realm and Cube Wave23 tests plus full affected packages and vet.

- [ ] Assert no exported field-based `NewRealizationAuthority` constructor exists.
- [ ] Assert controller source computes canonical `PolicyDigest` and re-reads Realm/World state.
- [ ] Assert Cube bridge compares exact ResourceBinding sandbox identity.
- [ ] Add workflow for Python static contract, focused Go tests, package suites and `go vet`.
- [ ] Commit CI contract.

### Task 5: Full verification and closure

**Files:**
- Create after GREEN: `docs/superpowers/verification/2026-09-07-wave23-realm-cube-realization-binding-authority-closure.md`

**Interfaces:**
- Consumes exact final-head workflow evidence.

- [ ] Run/observe dedicated Wave23 workflow, Nolane World Check, live substrate, format, docs and DCO on exact implementation HEAD.
- [ ] Fix only evidence-backed failures and repeat verification on every new HEAD.
- [ ] Record RED/GREEN SHAs and explicit non-claims in closure note.
- [ ] Re-run final-head verification after closure-note commit.
- [ ] Open a draft stacked PR targeting `gpt/wave22-public-live-verdict-authority`; do not merge.
