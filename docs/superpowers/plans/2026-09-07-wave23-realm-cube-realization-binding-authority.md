# Wave 23 Exact Realm/Cube Realization Binding Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind the current host-owned Realm World realization to the exact Cube sandbox/resource binding using opaque, freshness-checked package authority.

**Architecture:** `realm.Controller` mints and revalidates an opaque realization authority only from exact package-owned concrete stores, and the authority retains the exact Store instance that minted it. The Cube adapter revalidates that authority and requires exact equality with `ResourceBinding.SandboxID()` before minting a second opaque cross-boundary authority. No serialized object, caller Store, external embedding wrapper, or byte-identical second Store instance can restore or transfer authority.

**Tech Stack:** Go, existing `realm`, `substrate/cube`, and `resourceproof` packages; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-07-wave23-realm-cube-realization-binding-authority.md`

## Global Constraints

- Realm `WorldRecord.RealizationRevision` is never substituted with Cube task generation.
- Canonical policy identity is `realm.PolicyDigest(current.Spec, current.Revision)`.
- Runtime digest is not fabricated; Wave23 does not create a generic LIVE_PASS runtime/CLI.
- Zero, stale, terminal, pre-ready, handle-mismatched, caller-reconstructed, caller-Store, embedded-wrapper, and cross-Store authority fails closed.
- Only exact non-nil dynamic types `*realm.MemoryStore` and `*realm.DurableStore` are Realm realization authority trust roots; Go method promotion through embedding is not authority.
- Existing Wave17–22 provenance boundaries remain unchanged.

---

### Task 1: Behavioral RED contract

**Files:**
- Create: `NolaneWorld/realm/realization_authority_v23_test.go`
- Create: `NolaneWorld/realm/realization_authority_external_v23_test.go`
- Create: `NolaneWorld/substrate/cube/realm_resource_authority_v23_test.go`

**Interfaces:**
- Consumes: existing `Controller`, `MemoryStore`, `RealmRecord`, `WorldRecord`, `ResourceBinding`.
- Produces expected API: `Controller.CurrentRealizationAuthority`, `Controller.ValidateRealizationAuthority`, `RealizationAuthority.Binding`, `ValidateRealmResourceAuthority`.

- [x] Write tests proving current authoritative phases mint and pre-ready/terminal phases fail.
- [x] Write tests proving Realm revision, realization revision and substrate-handle drift stale old authority.
- [x] Write external-package tests proving caller-owned Stores cannot mint authority.
- [x] Write external-package RED proving a wrapper embedding `*realm.MemoryStore` cannot inherit authority through method promotion.
- [x] Write test proving authority from one package-owned Store instance cannot validate against a byte-identical second Store instance.
- [x] Write tests proving exact Cube sandbox accepts and mismatch rejects.
- [x] Run focused/full Go tests and record behavioral RED before each production trust-boundary repair.
- [x] Commit RED changes with autonomous provenance trailers.

### Task 2: Realm realization authority

**Files:**
- Create/Modify: `NolaneWorld/realm/realization_authority.go`
- Test: `NolaneWorld/realm/realization_authority_v23_test.go`
- Test: `NolaneWorld/realm/realization_authority_external_v23_test.go`

**Interfaces:**
- Produces `type RealizationBinding`, opaque `type RealizationAuthority`, `(*Controller).CurrentRealizationAuthority(ctx, realmID, worldID)`, `(*Controller).ValidateRealizationAuthority(ctx, authority)`.

- [x] Implement sealed zero-invalid authority.
- [x] Compute policy digest from current Realm spec/revision inside the controller.
- [x] Admit only observed-ready, leased, paused World phases with nonempty handle.
- [x] Retain exact package-owned Store instance inside the opaque capability and reject cross-Store validation.
- [x] Restrict mint/validation trust root to exact non-nil dynamic types `*MemoryStore` and `*DurableStore`; reject external embedding wrappers despite promoted marker methods.
- [x] Re-read current store state for validation and exact-compare all sealed dimensions.
- [x] Preserve opaque/non-serializable semantics.
- [x] Run focused Realm tests GREEN after the trust-boundary repairs.
- [x] Commit production boundary with autonomous provenance.

### Task 3: Cube exact sandbox bridge

**Files:**
- Create: `NolaneWorld/substrate/cube/realm_resource_authority.go`
- Test: `NolaneWorld/substrate/cube/realm_resource_authority_v23_test.go`

**Interfaces:**
- Consumes: `realm.RealizationAuthority`, `realm.Controller`, `cube.ResourceBinding`.
- Produces opaque `RealmResourceAuthority` and `ValidateRealmResourceAuthority(ctx, controller, realizationAuthority, resourceBinding)`.

- [x] Revalidate Realm authority immediately before Cube comparison.
- [x] Require nonempty exact equality between sealed substrate handle and `ResourceBinding.SandboxID()`.
- [x] Mint opaque zero-invalid cross-boundary authority only on exact match.
- [x] Run Cube-focused tests GREEN.
- [x] Commit bridge.

### Task 4: Dedicated static/CI contract

**Files:**
- Create/Modify: `tests/wave23_realm_cube_realization_contract.py`
- Create: `.github/workflows/wave23-realm-cube-realization-contract.yml`

**Interfaces:**
- Static contract checks opacity/trust shape; workflow runs Realm and Cube Wave23 tests plus full affected packages and vet.

- [x] Assert no exported field-based `NewRealizationAuthority` constructor exists.
- [x] Assert controller source computes canonical `PolicyDigest` and re-reads Realm/World state.
- [x] Assert authority retains exact Store instance and validation rejects cross-Store provenance.
- [x] Assert trusted Store selection contains explicit `case *MemoryStore`, `case *DurableStore`, and fail-closed default so embedding wrappers cannot become authority roots.
- [x] Assert Cube bridge compares exact ResourceBinding sandbox identity.
- [x] Add workflow for Python static contract, focused Go tests, race tests, package suites and `go vet`.
- [x] Commit CI contract.

### Task 5: Full verification and closure

**Files:**
- Create after GREEN: `docs/superpowers/verification/2026-09-07-wave23-realm-cube-realization-binding-authority-closure.md`

**Interfaces:**
- Consumes exact final-head workflow evidence.

- [ ] Observe dedicated Wave23 workflow, Nolane World Check, live substrate, format, docs and DCO on the exact final implementation/docs HEAD.
- [ ] Fix only evidence-backed failures and repeat verification on every new HEAD.
- [ ] Record caller-Store RED, cross-Store RED, embedding-promotion RED, final GREEN SHA, and explicit non-claims in closure note.
- [ ] Re-run final-head verification after closure-note commit.
- [ ] Update draft stacked PR #34 with exact final SHA and closure evidence; do not merge.
