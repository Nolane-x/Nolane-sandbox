# Nolane Sandbox Release Gauntlet v4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a deterministic adversarial release gate that exercises Nolane trust invariants and emits self-verifying release evidence.

**Architecture:** Add an isolated `gauntlet` package with scenario contracts, runner-owned probes, deterministic SHA-256 evidence, and built-in attacks against existing authority/artifact/capability boundaries. Keep the core independent of Cube internals and add Go fuzz/property tests around evidence integrity and determinism.

**Tech Stack:** Go 1.23 standard library; SHA-256; JSON; existing `NolaneWorld` packages; Go fuzzing.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-release-gauntlet-v4-design.md`

## Global Constraints

- Do not modify KVM/RustVMM/CubeEgress/CubeNet/CubeCoW security internals.
- Every registered scenario is release-required in v4.
- A scenario cannot directly declare itself passed.
- A passing scenario requires attack, boundary, denial, and every required marker.
- Report hashing excludes wall-clock timestamps and uses length-prefixed fields.
- Verification failure always rejects release.
- Gauntlet output may veto release but may never grant runtime authority.

---

### Task 1: Scenario Contract and Probe

**Files:**
- Create: `NolaneWorld/gauntlet/types.go`
- Create: `NolaneWorld/gauntlet/probe.go`
- Create: `NolaneWorld/gauntlet/probe_test.go`

**Interfaces:**
- Produces: `ScenarioSpec`, `Scenario`, `ScenarioFunc`, `Probe`, `Event`, `EventKind`, `Severity`, `Outcome`.

- [ ] Write failing tests proving empty IDs/invariants/attacks/defenses are invalid and a scenario cannot obtain a mutable event slice from a probe.
- [ ] Run `go test ./gauntlet -run 'TestProbe|TestScenarioSpec'` and confirm RED.
- [ ] Implement validated scenario specs and append-only probe recording.
- [ ] Require non-empty marker/detail values and copy returned event slices.
- [ ] Run focused tests and confirm GREEN.

### Task 2: Deterministic Runner and Proof-of-Exercise Gate

**Files:**
- Create: `NolaneWorld/gauntlet/runner.go`
- Create: `NolaneWorld/gauntlet/runner_test.go`

**Interfaces:**
- Consumes: `Scenario`, `Probe`.
- Produces: `Policy`, `Runner`, `Report`, `ScenarioEvidence`, `Run(context.Context, []Scenario) (Report, error)`.

- [ ] Write failing tests for vacuous success, missing marker, panic, timeout, duplicate scenario ID, one-failure-rejects-release, and registration-order determinism.
- [ ] Run focused tests and confirm RED.
- [ ] Implement spec validation, stable ID sorting, bounded per-scenario context, panic recovery, event-class checks, required-marker checks, and fail-closed approval.
- [ ] Make failure codes stable machine-readable constants rather than depending on arbitrary error strings.
- [ ] Run focused tests and `go test -race ./gauntlet`.

### Task 3: Evidence Hashing and Strict Verification

**Files:**
- Create: `NolaneWorld/gauntlet/evidence.go`
- Create: `NolaneWorld/gauntlet/evidence_test.go`
- Create: `NolaneWorld/gauntlet/evidence_fuzz_test.go`

**Interfaces:**
- Produces: `VerifyReport(Report) error`, `MarshalReport(Report) ([]byte, error)`, stable scenario/report digests.

- [ ] Write failing tests for scenario-field mutation, event mutation, approved-bit mutation, digest mutation, reordered scenario evidence, and valid report verification.
- [ ] Add a RED property test proving registration order must produce the same report digest.
- [ ] Implement domain-separated length-prefixed SHA-256 hashing.
- [ ] `VerifyReport` must recompute all hashes, order, outcome/proof semantics, and policy digest.
- [ ] Implement JSON marshaling only after successful verification.
- [ ] Add fuzz target that mutates serialized trust-bearing data and confirms corrupted reconstructed reports are rejected.
- [ ] Run focused tests + fuzz seed corpus + race test.

### Task 4: Authority Adversarial Scenarios

**Files:**
- Create: `NolaneWorld/gauntlet/scenarios/authority.go`
- Create: `NolaneWorld/gauntlet/scenarios/authority_test.go`

**Interfaces:**
- Produces: `StaleEpochScenario()`, `TerminalAuthorityScenario()`, `ActionCollisionScenario()` returning `gauntlet.Scenario`.

- [ ] Write RED tests that execute each built-in through `gauntlet.Runner` and require approved evidence.
- [ ] Add executor counters so proof includes “executor not called” or “called exactly once” observations.
- [ ] Implement stale epoch, terminal authority, and action-ID rebinding attacks using real `world.State`, `authority.Broker`, and `authority.MemoryLedger`.
- [ ] Require exact expected errors (`world.ErrStaleEpoch`, `world.ErrClosedWorld`, `authority.ErrActionCollision`).
- [ ] Run focused and race tests.

### Task 5: Artifact and Capability Adversarial Scenarios

**Files:**
- Create: `NolaneWorld/gauntlet/scenarios/artifact.go`
- Create: `NolaneWorld/gauntlet/scenarios/capability.go`
- Create: `NolaneWorld/gauntlet/scenarios/storage_test.go`

**Interfaces:**
- Produces: `ArtifactTraversalScenario()`, `CapabilityBlobTamperScenario()`, `CapabilityJournalTamperScenario()`.

- [ ] Write RED tests requiring all three scenarios to pass the runner.
- [ ] Implement traversal attack corpus covering `../`, absolute paths, backslashes, dot components, empty segments, and NUL.
- [ ] Implement exact durable capability promotion with content, manifest, and evidence bytes.
- [ ] Tamper exact CAS blob and require registry reopen to return `capability.ErrRegistryCorrupt`.
- [ ] Tamper a trust-bearing journal byte and require strict reopen rejection.
- [ ] Ensure temp directories are isolated per scenario and cleaned after execution.
- [ ] Run focused + race tests.

### Task 6: Standard Release Suite and Negative Controls

**Files:**
- Create: `NolaneWorld/gauntlet/suite.go`
- Create: `NolaneWorld/gauntlet/suite_test.go`

**Interfaces:**
- Produces: `StandardSuite() []Scenario`, `RunStandard(context.Context, Policy) (Report, error)`.

- [ ] Write RED test requiring exact stable IDs and deterministic report digest across repeated runs.
- [ ] Write negative-control scenarios that omit denial/boundary events and prove the runner rejects them.
- [ ] Implement standard suite composition without Cube imports.
- [ ] Verify all standard scenarios are required and no percentage/score bypass exists.
- [ ] Run full module tests and race detector.

### Task 7: CI Release Evidence Artifact

**Files:**
- Create: `NolaneWorld/cmd/nolane-gauntlet/main.go`
- Create: `NolaneWorld/cmd/nolane-gauntlet/main_test.go`
- Modify: `.github/workflows/nolane-world-check.yml`
- Modify: `NolaneWorld/README.md`

**Interfaces:**
- Produces: CLI writing verified JSON to stdout or `--out <path>`; CI uploads deterministic evidence artifact.

- [ ] Write RED CLI tests for successful JSON output and non-zero exit on verification failure.
- [ ] Implement CLI with `--out` only; no network/credential options.
- [ ] Add CI step `go run ./cmd/nolane-gauntlet --out release-evidence/nolane-gauntlet-v4.json` after unit/race/vet.
- [ ] Add CI artifact upload for the report.
- [ ] Document v4 security model and exact local verification commands.
- [ ] Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and CLI twice; compare report bytes for determinism.

### Task 8: Repository Verification and Integration

**Files:**
- No new production files unless verification reveals a defect.

- [ ] Compare branch against `master`; confirm only `NolaneWorld/`, v4 docs, and Nolane workflow changes.
- [ ] Run fresh `go test ./...`.
- [ ] Run fresh `go test -race ./...`.
- [ ] Run fresh `go vet ./...`.
- [ ] Run the standard gauntlet and call `VerifyReport` on its output.
- [ ] Push exact verified tree and open PR against `master`.
- [ ] Require GitHub `Nolane World Check`, docs, and format gates to be green before merge; preserve human DCO policy rather than fabricating sign-off.
