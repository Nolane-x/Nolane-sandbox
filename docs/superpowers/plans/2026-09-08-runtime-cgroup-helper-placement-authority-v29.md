# Wave 29 Exact Runtime Cgroup Helper Placement Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove a package-owned parked helper process can be identified by PID+starttime, attached to the exact fresh Wave28 cgroup before release, complete a fixed READY/GO/DONE protocol, exit 0, and leave the Wave28 runtime binding unchanged.

**Architecture:** Split helper protocol from placement authority. `runtime_cgroup_helper_protocol.go` owns the fixed internal env/FD protocol and command-facing entrypoint. `runtime_cgroup_helper_placement.go` owns executable identity, process lifecycle, `/proc` identity, cgroup placement, fresh Wave28 bracketing, cleanup and opaque authority minting. Public production constructors expose no path, process, filesystem, PID, nonce or pressure injection; package-private seams support deterministic tests only.

**Tech Stack:** Go 1.23 standard library (`os`, `os/exec`, `crypto/rand`, `crypto/sha256`, `io`, `bufio`, `syscall`/`os.ProcessState` where needed), existing `realm`, Wave27 and Wave28 Cube authority APIs, pytest static contract, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-08-runtime-cgroup-helper-placement-authority-v29-design.md`

## Global Constraints

- Base is exact Wave28 final `53ecdfdedd651c87f2b44e18fce352127c9f4242`.
- Production cgroup root remains exactly `/sys/fs/cgroup`.
- Internal mode is exactly `NOLANE_INTERNAL_CGROUP_HELPER=park-exit`.
- Nonce environment key is exactly `NOLANE_INTERNAL_CGROUP_HELPER_NONCE` and contains 64 lowercase hex encoding exactly 32 random bytes.
- Release FD is exactly 3; acknowledgement FD is exactly 4.
- Protocol records are exactly `READY <nonce>\n`, `GO <nonce>\n`, `DONE <nonce>\n`.
- Public APIs accept no executable path, cgroup root/path, arbitrary reader/writer, process launcher, PID, starttime, raw nonce, `ResourceBinding`, `HostFileSource` or `HostPressureRunner`.
- Wave29 does not mint `resourceproof.TrustedReport`, does not claim CPU/memory causality and does not emit `LIVE_PASS`.
- Every child-started failure path must kill when necessary and wait/reap the exact child.

---

### Task 1: Commit behavioral and command RED contracts

**Files:**
- Create: `NolaneWorld/substrate/cube/runtime_cgroup_helper_placement_v29_test.go`
- Modify: `NolaneWorld/cmd/nolane-gauntlet-live/main_test.go`
- Create: `tests/wave29_runtime_cgroup_helper_placement_contract.py`
- Create: `.github/workflows/wave29-runtime-cgroup-helper-placement-contract.yml`
- Create: `docs/superpowers/verification/2026-09-08-wave29-runtime-cgroup-helper-placement-red.md` after observing a valid RED run.

**Interfaces expected by RED tests:**
- `func MaybeRunInternalCgroupHelper() (handled bool, exitCode int)`
- `func NewRuntimeCgroupHelperExecutor() (*RuntimeCgroupHelperExecutor, error)`
- `type RuntimeCgroupHelperPlacementAuthority struct`
- `func ValidateRuntimeCgroupHelperPlacementAuthority(context.Context, *realm.Controller, realm.RealizationAuthority, RealmResourceRuntimeAuthority, *RealizationEpochObserver, *Client, *RuntimeRealizationObserver, *RuntimeCgroupReadbackObserver, *RuntimeCgroupHelperExecutor) (RuntimeCgroupHelperPlacementAuthority, error)`
- `var ErrInvalidRuntimeCgroupHelperPlacementAuthority error`

- [ ] **Step 1: Write the failing Cube tests**

Create a deterministic v29 fixture on top of `newV28Fixture`. Package-private executor seams simulate a parked child with ordered events and mutable cgroup/proc fixtures. Tests must cover: successful lifecycle; deterministic digest; zero/JSON opacity; READY mismatch; child exit before placement; cgroup write failure; missing/duplicate/malformed PID membership; starttime replacement; GO ordering; DONE mismatch; non-zero exit; cancellation cleanup; post-Wave28 runtime replacement; immutable limit/path changes; counter increases allowed.

Representative assertions:

```go
auth, err := validateV29(t, f)
if err != nil { t.Fatal(err) }
if !auth.Valid() { t.Fatal("Wave29 authority invalid") }
if got, ok := auth.Digest(); !ok || !strings.HasPrefix(got, "runtime-cgroup-helper-placement-v29:") {
    t.Fatalf("Digest=(%q,%v)", got, ok)
}
```

and ordering:

```go
if index(events, "GO") < index(events, "MEMBERSHIP_OK") || index(events, "GO") < index(events, "STARTTIME_RECHECK_OK") {
    t.Fatal("helper released before exact placement identity was established")
}
```

- [ ] **Step 2: Add command integration RED**

Modify `main_test.go` so the command contract requires helper dispatch before normal flag parsing. Add a source-level ordering assertion or a package test seam that proves helper mode bypasses ordinary gauntlet execution.

- [ ] **Step 3: Add static anti-shortcut RED**

The Python contract must require fixed env keys/FDs, `os.Executable`, SHA-256 executable measurement, `/proc/`, `cgroup.procs`, two fresh `ValidateRuntimeCgroupReadbackAuthority(` calls, `READY ` / `GO ` / `DONE `, domain separator `nolane.runtime-cgroup-helper-placement.v29\\x00`, and forbid trusted evidence shortcuts/public injection surfaces.

- [ ] **Step 4: Add dedicated CI and verify genuine RED**

Workflow order:

```yaml
- go test ./substrate/cube -run 'V29|RuntimeCgroupHelper' -count=1
- go test ./cmd/nolane-gauntlet-live -count=1
- python -m pytest tests/wave29_runtime_cgroup_helper_placement_contract.py -q
- go test ./substrate/cube -run 'V28|RuntimeCgroupReadback' -count=1
- go test ./substrate/cube -run 'V27|RuntimeRealization' -count=1
- go vet ./...
- go test ./...
- go test -race ./substrate/cube -run 'V29|RuntimeCgroupHelper' -count=1
```

Expected initial RED: compile failure only for the missing Wave29 production symbols. If harness/test syntax fails first, fix tests before production and repeat until the RED cause is feature absence.

- [ ] **Step 5: Commit RED lineage record**

Record exact RED head, run ID and missing symbols. Do not call it valid RED unless setup succeeded and the failure reached the intended focused test.

---

### Task 2: Implement the fixed internal helper protocol

**Files:**
- Create: `NolaneWorld/substrate/cube/runtime_cgroup_helper_protocol.go`
- Modify: `NolaneWorld/cmd/nolane-gauntlet-live/main.go`
- Test: focused protocol and command tests from Task 1.

**Produces:**
- `MaybeRunInternalCgroupHelper() (bool, int)`
- package-private `runInternalCgroupHelper(getenv func(string) string, release io.Reader, ack io.Writer) int` for deterministic tests only.

- [ ] **Step 1: Implement canonical helper-mode detection**

`MaybeRunInternalCgroupHelper` returns `(false, 0)` unless mode equals `park-exit`. If mode is selected, validate the nonce as exactly 64 lowercase hex and open only inherited FDs 3/4.

- [ ] **Step 2: Implement one-record bounded protocol**

Use bounded line reads. Emit exactly READY, accept exactly one GO plus EOF, emit exactly DONE, otherwise return non-zero. No pressure loop and no gauntlet execution occurs inside helper mode.

- [ ] **Step 3: Wire command dispatch before normal parsing**

At the first line of `main()` behavior, call helper dispatch and `os.Exit(exitCode)` when handled; otherwise continue existing `run(...)` flow unchanged.

- [ ] **Step 4: Run focused command/protocol tests and commit**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V29.*Protocol|InternalCgroupHelper' -count=1
go test ./cmd/nolane-gauntlet-live -count=1
```

Expected: protocol/command portion GREEN; placement tests may remain RED until Task 3.

---

### Task 3: Implement package-owned executor, exact placement and process identity

**Files:**
- Create: `NolaneWorld/substrate/cube/runtime_cgroup_helper_placement.go`
- Test: `NolaneWorld/substrate/cube/runtime_cgroup_helper_placement_v29_test.go`

**Produces:**
- `RuntimeCgroupHelperExecutor`
- `NewRuntimeCgroupHelperExecutor() (*RuntimeCgroupHelperExecutor, error)`
- internal parked child handle abstraction and package-private deterministic test constructor.

- [ ] **Step 1: Implement production executable measurement**

Resolve `os.Executable()`, `filepath.EvalSymlinks`, require canonical absolute regular file, hash bytes with SHA-256, retain only canonical path internally and lowercase digest in authority data.

- [ ] **Step 2: Implement parked child launch**

Create parent/child release and ack pipes. Launch the same executable with inherited ExtraFiles mapped so child sees release FD 3 and ack FD 4. Environment is the current environment plus only package-defined helper mode and nonce. Generate nonce with exactly 32 bytes from `crypto/rand.Read`.

- [ ] **Step 3: Implement exact `/proc/<pid>/stat` identity**

Read field 22 robustly by locating the final `)` of comm before splitting remaining fields. Reject PID <= 0, missing/malformed file, short fields, plus/minus/noncanonical decimal, zero starttime.

- [ ] **Step 4: Implement exact cgroup placement**

Derive target from fresh Wave28 snapshot and fixed root, write `"<pid>\n"` to target `cgroup.procs`, read it back with canonical membership parser, reread starttime, verify child is still live, and only then write GO.

- [ ] **Step 5: Implement DONE/exit/cleanup**

Require exact DONE nonce and exit code 0. Every failure after Start must close pipes, kill if live and Wait exactly once. Context cancellation follows the same cleanup path and returns the context error where applicable.

- [ ] **Step 6: Run placement-focused tests and commit**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V29|RuntimeCgroupHelper' -count=1
```

Expected: executor/ordering/fail-closed tests GREEN except authority-bracketing tests that depend on Task 4 if separated by test names.

---

### Task 4: Mint the opaque Wave29 authority with fresh Wave28 bracketing

**Files:**
- Modify: `NolaneWorld/substrate/cube/runtime_cgroup_helper_placement.go`
- Test: `NolaneWorld/substrate/cube/runtime_cgroup_helper_placement_v29_test.go`

**Produces:**
- `RuntimeCgroupHelperPlacementSnapshot`
- `RuntimeCgroupHelperPlacementAuthority`
- `ValidateRuntimeCgroupHelperPlacementAuthority(...)`
- accessors `Valid()`, `Digest()`, `Snapshot()`.

- [ ] **Step 1: Mint fresh Wave28 before launch**

Call `ValidateRuntimeCgroupReadbackAuthority` using the sealed Wave27 authority and observers; do not accept a Wave28 authority from the caller.

- [ ] **Step 2: Execute parked placement lifecycle**

Use only the sealed executor. Record helper PID/starttime, executable digest, SHA-256 of raw nonce, READY/placed/released/exited UTC timestamps and exact exit code.

- [ ] **Step 3: Mint fresh Wave28 after exit and compare immutable binding**

Require equality for runtime digest, sandbox/generation/runtime PID+starttime+boot ID, cgroup path, CPU quota/period and memory limit. Permit counter increases; reject counter decreases as malformed readback continuity only if existing Wave28 parser/semantics require it, otherwise leave counters descriptive and unconstrained.

- [ ] **Step 4: Derive sealed digest**

Canonical JSON/document digest under domain separator:

```text
nolane.runtime-cgroup-helper-placement.v29\x00
```

Public digest format:

```text
runtime-cgroup-helper-placement-v29:<64-lowercase-hex>
```

`Valid()` recomputes the digest from sealed authority-owned state. JSON round-trip must not restore seal.

- [ ] **Step 5: Run full dedicated gate**

Run all workflow commands from Task 1. Expected: all GREEN.

---

### Task 5: Integration closure and stacked PR

**Files:**
- Create: `docs/superpowers/verification/2026-09-08-wave29-runtime-cgroup-helper-placement-authority-closure.md`
- Update PR body metadata only after exact-final CI; metadata edits must not create code commits.

- [ ] **Step 1: Audit diff against Wave28 exact final**

Require merge base exactly `53ecdfdedd651c87f2b44e18fce352127c9f4242`, `behind_by=0`, and changed files only within Wave29 implementation/tests/spec/plan/verification/workflow plus the one command entrypoint/test.

- [ ] **Step 2: Open draft PR stacked on Wave28 branch**

Head: `gpt/wave29-runtime-cgroup-helper-placement-authority`

Base: `gpt/wave28-runtime-cgroup-readback-authority`

Keep draft/unmerged. Do not merge without explicit user authorization.

- [ ] **Step 3: Run/observe all applicable PR workflows on exact code head**

Require dedicated Wave29, NolaneWorld, Format, DCO, Docs and all inherited trust regression gates to complete SUCCESS.

- [ ] **Step 4: Perform whole-PR trust review**

Check for caller-controlled path/process/nonce authority, helper release-before-placement, PID-only identity, missing reap paths, stale Wave28 trust, counter-to-causality laundering, `TrustedReport`/`LIVE_PASS` shortcuts and public test seams.

- [ ] **Step 5: Commit closure record then re-verify the new exact final SHA**

Closure commit changes SHA; therefore re-run/observe applicable CI on the closure head. Only after the final SHA itself is fully GREEN, ancestry/scope remain clean and review threads are empty may Wave29 be called code-closed.
