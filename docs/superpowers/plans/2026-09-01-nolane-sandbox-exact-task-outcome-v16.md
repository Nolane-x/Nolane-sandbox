# Exact Task Outcome Proof Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fail-closed, realization-fenced task-outcome proof plane sourced only from authoritative containerd task responses, without promoting synthetic lifecycle state or exit-code heuristics into evidence.

**Architecture:** The Cube sandbox controller owns a concurrency-safe in-memory proof registry separate from operational status. `Wait` records only exact successful runtime outcomes and fails closed on resolver/NotFound/Wait errors or invalid timestamps; `Status` may reconstruct proof only from exact runtime `STOPPED` evidence. `Create` clears authority and fences recovery; `Start` begins the new realization. A fresh controller after restart may recover a local generation only from fresh authoritative runtime evidence.

**Tech Stack:** Go, containerd v2 task/sandbox APIs, protobuf timestamps, existing CubeSandbox GitHub Actions CI.

**Spec:** `docs/superpowers/specs/2026-09-01-nolane-sandbox-exact-task-outcome-v16-design.md`

## Global Constraints

- Operational synthetic status must never populate task-outcome proof.
- `ExitCode == 137` remains numeric evidence only; no OOM classification is added.
- Missing proof means unknown.
- Proof is not persisted across controller restart in Wave 16.
- `Create` must block stale restart-style recovery until `Start`.
- New sandbox realization invalidates prior proof.
- Conflicting exact outcomes inside one generation fail closed.
- No textual `Reason` is invented because the authoritative task responses at this boundary do not provide one.

---

### Task 1: RED contract for the proof registry and runtime response conversion

**Files:**
- Create: `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof_v16_test.go`

- [x] Write failing registry and conversion tests.
- [x] Correct the containerd state enum dependency before accepting RED evidence.
- [x] Prove RED through the dedicated contract gate: missing Wave 16 symbols caused the expected failure.

### Task 2: Implement the minimal proof registry

**Files:**
- Create: `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go`

- [x] Implement exact immutable proof storage.
- [x] Preserve exact exit code/time and source without semantic reinterpretation.
- [x] Reject conflicting exact outcomes.
- [x] Validate Wait/State runtime timestamps.
- [x] Prove focused GREEN with `Cube Task Outcome Contract #2`.

### Task 3: Realization lifecycle and restart recovery fencing

**Files:**
- Modify: `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/task_outcome_controller_v16_test.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/task_outcome_recovery_v16_test.go`

- [x] Prove controller provider/lifecycle RED before implementation.
- [x] Add nil-safe controller proof-provider access.
- [x] Add a Create fence that clears proof and active generation.
- [x] Add `Start` realization generation reset.
- [x] Add fresh-controller recovery from current authoritative runtime evidence while forbidding recovery across a Create fence.
- [x] Prove lifecycle/recovery GREEN with `Cube Task Outcome Contract #10`.

### Task 4: Harden public Wait and Status producers

**Files:**
- Modify: `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/task_outcome_producer_v16_test.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/task_outcome_resolver_v16_test.go`

- [x] Add a narrow task-runtime/endpoint seam so public methods can be tested without a real shim.
- [x] Prove behavioral RED showing six existing trust violations: fabricated NotFound Wait success, partial Wait leakage, missing-timestamp acceptance, missing Wait proof, missing STOPPED reconstruction, and conflict acceptance.
- [x] Add resolver-error RED and reproduce the pre-fix nil-pointer panic.
- [x] Harden `Wait` to return zero outcome on every resolver/RPC/validation/proof error and record only successful authoritative responses.
- [x] Keep Status NotFound synthetic state operational-only.
- [x] Reconstruct proof only from exact STOPPED state, keep incomplete STOPPED operational/unproven, and fail on exact conflicts.
- [x] Prove focused producer GREEN with `Cube Task Outcome Contract #14` on pre-documentation head `b784ef8e08e0d99c798c2addf022152a7913ba57`.

### Task 5: Make the proof contract permanent

**Files:**
- Create: `.github/workflows/cube-task-outcome-contract.yml`

- [x] Identify that repository `cubelet-pkg-test` covers only `./pkg/...` and misses `plugins/cube/internals/sandbox`.
- [x] Add permanent path-scoped `Cube Task Outcome Contract` running the exact sandbox package contract.
- [x] Keep the workflow in the final tree so future regressions cannot bypass ordinary Unit CI coverage.

### Task 6: Repository verification and closure

**Files:**
- Modify documentation/PR metadata only unless verification discovers a tested defect.

- [x] Update the design with Create fencing, restart recovery, producer error semantics, and permanent CI coverage.
- [x] Record RED/GREEN implementation state in this plan.
- [ ] Require fresh `Cube Task Outcome Contract` success on the final squashed candidate.
- [ ] Require fresh `Format Check` success on both architectures.
- [ ] Require fresh `Unit Test Check` success and inspect all jobs.
- [ ] Require fresh `Build Check` success and inspect all jobs.
- [ ] Require fresh DCO, Nolane World Check, docs and page/build checks.
- [ ] Review final diff for heuristic leakage (`137 => OOM`), synthetic proof, invented reason, and temporary files.
- [ ] Inspect PR reviews, threads and comments.
- [ ] Squash Wave 16 onto exact Wave 15 master with a provenance-correct commit and rerun all fresh PR gates.
- [ ] Merge only with expected-head protection.
- [ ] Verify `master` points exactly to the merge commit and require fresh post-merge path-triggered checks before declaring Wave 16 closed.
