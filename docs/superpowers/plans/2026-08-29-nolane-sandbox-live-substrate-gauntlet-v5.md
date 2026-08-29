# Nolane Sandbox Live Substrate Gauntlet v5 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans and test-driven-development task-by-task.

**Goal:** Add a fail-closed live CubeSandbox/KVM evidence path without allowing absent infrastructure to masquerade as a passing live proof.

**Architecture:** Keep V4 unchanged. Extend the narrow Cube client with sanitized live-session operations, then add `gauntlet/live` for capability attestation, core live scenarios, semantic evidence verification, and probe/require-live policy. Add a separate CLI/workflow so hosted CI tests harness semantics while self-hosted KVM runners can produce actual live artifacts.

**Tech Stack:** Go 1.23 standard library, existing NolaneWorld packages, CubeAPI/envd Connect protocol.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-live-substrate-gauntlet-v5-design.md`

## Global Constraints

- Do not change V4 evidence semantics or digest contract.
- Do not modify Cube security internals.
- Never serialize API, envd, traffic, or injected credentials.
- `UNAVAILABLE` is never live approval.
- `require-live` exits non-zero for `UNAVAILABLE` and `LIVE_FAIL`.
- Cleanup must be observed, not merely requested.
- Target-dependent egress proof requires host preflight.

---

### Task 1: Cube live-session wire contract
**Files:** `substrate/cube/live.go`, `substrate/cube/live_test.go`, modify `substrate/cube/client.go`.

- [ ] Write RED tests for health, connect, guest canary framing/headers/stream parsing, typed network update, and observed destroy.
- [ ] Implement a separate data-plane HTTP client with redirects disabled.
- [ ] Parse connect response into a session whose tokens are private fields.
- [ ] Implement fixed-command execution over envd Connect framing with bounded response size.
- [ ] Poll GET until 404 for observed destroy.
- [ ] Run focused tests, race detector, and vet.

### Task 2: Live evidence model and verifier
**Files:** `gauntlet/live/types.go`, `gauntlet/live/evidence.go`, `gauntlet/live/evidence_test.go`.

- [ ] RED tests: fake LIVE_PASS without guest execution, fake cleanup, scenario mutation, digest mutation, secret-looking serialized fields.
- [ ] Implement status/profile/mode/capability/scenario evidence types.
- [ ] Implement length-prefixed domain-separated hashes over sanitized semantic fields.
- [ ] Implement strict `VerifyReport` and `MarshalReport`.
- [ ] Verify `UNAVAILABLE` report is valid diagnostic evidence but never approved.

### Task 3: Driver abstraction and capability attestation
**Files:** `gauntlet/live/driver.go`, `gauntlet/live/attest.go`, `gauntlet/live/attest_test.go`, `gauntlet/live/cube/driver.go`.

- [ ] RED fake-driver tests for missing control plane, create failure, guest mismatch, cleanup failure, and successful live attestation.
- [ ] Implement minimal `Driver`/`Sandbox` interfaces.
- [ ] Implement canary scenario and cleanup lease.
- [ ] Implement Cube driver over `substrate/cube.Client`.

### Task 4: Real snapshot + authority monotonicity scenario
**Files:** `gauntlet/live/snapshot.go`, `gauntlet/live/snapshot_test.go`.

- [ ] RED test requires observed A→B→rollback→A guest state transition.
- [ ] Advance host authority epoch before rollback and prove old epoch rejection afterwards.
- [ ] Fail if rollback succeeds at API level but guest state remains B/unknown.
- [ ] Require observed cleanup.

### Task 5: Controlled egress target framework
**Files:** `gauntlet/live/targets.go`, `gauntlet/live/targets_test.go`, `gauntlet/live/egress.go`, `gauntlet/live/egress_test.go`.

- [ ] RED tests prove missing target/preflight failure => UNAVAILABLE, never PASS.
- [ ] Add HTTP/TCP/UDP/DNS target descriptors and host-preflight interface.
- [ ] Implement guest probes as fixed templates; never arbitrary shell from CLI.
- [ ] Bind target digests, not credentials, into evidence.
- [ ] Full-egress profile requires all declared target scenarios.

### Task 6: Runner policy and live CLI
**Files:** `gauntlet/live/runner.go`, `gauntlet/live/runner_test.go`, `cmd/nolane-gauntlet-live/main.go`, `cmd/nolane-gauntlet-live/main_test.go`.

- [ ] RED tests for probe vs require-live exit semantics.
- [ ] Read Cube config only from explicit CLI flags/env; no secret output.
- [ ] `probe` with missing config emits verified UNAVAILABLE JSON and exits 0.
- [ ] `require-live` with missing config exits non-zero.
- [ ] Successful live execution emits only verified LIVE_PASS.

### Task 7: CI and documentation
**Files:** `.github/workflows/nolane-live-gauntlet.yml`, modify `NolaneWorld/README.md`.

- [ ] Add hosted harness job running all V5 unit/race/vet tests and CLI probe-mode negative control.
- [ ] Add gated self-hosted `[self-hosted, linux, nolane-kvm]` live job with environment `nolane-live-gauntlet`.
- [ ] Live job runs `--mode require-live` and uploads evidence artifact.
- [ ] Document that skipped/unavailable is not live verification.

### Task 8: Verification and integration
- [ ] Fresh `go test ./...`.
- [ ] Fresh `go test -race ./...`.
- [ ] Fresh `go vet ./...`.
- [ ] Verify V4 evidence SHA remains unchanged.
- [ ] Run V5 probe with no live config; verify `UNAVAILABLE`, not PASS.
- [ ] Compare branch to master; no Cube security-core files modified.
- [ ] Open PR, require Nolane World, Docs, Format green; preserve DCO human gate.
- [ ] Merge only exact verified head.
