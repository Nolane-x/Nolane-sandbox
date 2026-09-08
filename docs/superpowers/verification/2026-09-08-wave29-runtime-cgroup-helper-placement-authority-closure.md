# Wave29 Runtime Cgroup Helper Placement Authority — Closure Evidence

Date: 2026-09-08

## Stack boundary

- Repository: `Nolane-x/Nolane-sandbox`
- Wave28 stacked base / PR #39 exact-final head: `53ecdfdedd651c87f2b44e18fce352127c9f4242`
- Wave29 branch: `gpt/wave29-runtime-cgroup-helper-placement-authority`
- Wave29 PR: #40
- Code-head verified before this closure record: `6a5a8c6a2e635b45e2358e07170910c13f57553e`

This record is documentation-only. The commit containing this file becomes the exact-final candidate and must itself pass fresh dedicated and PR-triggered verification before Wave29 is called code-closed.

## Contract closed by Wave29

Wave29 introduces package-owned exact helper placement authority on top of Wave28 readback provenance, without promoting descriptive cgroup counters into causal enforcement evidence.

A valid `RuntimeCgroupHelperPlacementAuthority` requires all of the following:

1. A valid supplied Wave27 runtime authority.
2. A fresh Wave28 readback authority before helper launch.
3. A package-owned production executor with a zero-argument constructor.
4. Production cgroup root fixed to `/sys/fs/cgroup`.
5. Production helper executable derived from `os.Executable()` and SHA-256 measured.
6. A package-generated 32-byte nonce.
7. Fixed bounded internal protocol over package-owned file descriptors: `READY <nonce>`, `GO <nonce>`, `DONE <nonce>`.
8. Helper PID and `/proc/<pid>/stat` starttime captured from the launched child, not supplied by the caller.
9. The running child executable SHA-256 remeasured through `/proc/<helper-pid>/exe` and required to equal the expected executable digest before placement.
10. Exact write of the helper PID to the Wave28-owned target `cgroup.procs`.
11. Exact helper PID membership readback plus starttime revalidation before `GO`.
12. Bounded helper completion with exit code zero.
13. Context-aware final wait; cancellation after `DONE` kills and reaps the exact child rather than allowing a synchronous wait to hang indefinitely.
14. A fresh Wave28 readback authority after helper exit.
15. Immutable runtime/cgroup identity and limits unchanged across the helper lifecycle.
16. Counter values may increase descriptively, but Wave29 does not infer causal pressure from those changes.

The authority digest uses:

- prefix: `runtime-cgroup-helper-placement-v29:`
- domain separator: `nolane.runtime-cgroup-helper-placement.v29\x00`

The public validator accepts no caller-selected executable, cgroup root/path, PID, nonce, filesystem, process launcher, raw process identity, pressure runner, or evidence byte payload.

## TDD lineage

### Initial feature RED

- Head: `64f00e22658619c4a406acf80a8dcd406eb762b7`
- Dedicated run: `34232297457`
- Result: expected RED.
- Evidence: setup completed successfully; focused Wave29 build then failed specifically because production `RuntimeCgroupHelperExecutor`, `RuntimeCgroupHelperPlacementAuthority`, internal helper protocol constants/functions, and validator seams did not yet exist.

### First GREEN progression

The first production candidate surfaced one local compile-only issue: an unused `fmt` import. The fix removed only that import.

A subsequent determinism test failure was traced to a test assumption: two separately-created nested Wave26/27 fixtures were not identical authority-owned input. The test was corrected to derive twice from the same sealed snapshot; production digest binding was not weakened.

- Head: `57a377a5deb5565cfd1bf3f003f48a1d43734309`
- Dedicated run: `34233478738`
- Result: SUCCESS across focused Wave29 tests, command integration, static anti-shortcut contract, Wave28 regression, Wave27 regression, `go vet ./...`, full `go test ./...`, and focused race.

### Review-driven hardening RED

Whole-PR trust review after the first full GREEN found two important gaps that the initial suite had not covered:

1. Post-`DONE` `Wait()` was synchronous, so context cancellation could not kill/reap a child that closed the ack stream and then remained alive.
2. The executable digest was measured before launch from the parent-selected path, but the actually-running helper image had not yet been remeasured from the child process.

- Review RED head: `2be1a07a9c66ca79676f27df5c0874df5d390f29`
- Dedicated run: `34233867121`
- Result: expected RED at the missing running-child executable verification seam.

The production hardening then:

- measures `/proc/<helper-pid>/exe` and requires its SHA-256 to match the expected helper digest before cgroup placement; and
- performs final child wait context-aware, killing and reaping the child on cancellation.

- Hardened head: `43dd0cdaa2f605c20f2c78a732c2d1edef66f8b0`
- Dedicated run: `34234202137`
- Result: full SUCCESS including focused/static/regression/vet/full/race gates.

The two review safeguards were then added to the static contract and PR path trigger.

## Code-head verification before closure commit

Code head: `6a5a8c6a2e635b45e2358e07170910c13f57553e`

### Dedicated push gate

- Run `34234426161` — `Wave29 Runtime Cgroup Helper Placement Contract` — SUCCESS.
- Covered focused Wave29 tests, command integration, static contract, Wave28 regression, Wave27 regression, `go vet ./...`, full `go test ./...`, and focused race.

### PR #40 integration gates

All 15 applicable PR-triggered workflow runs on the same exact code head completed SUCCESS:

1. `34238782545` — DCO Check — SUCCESS
2. `34238782487` — Cube Kernel OOM Victim Contract — SUCCESS
3. `34238782225` — Wave 26 Provider Endpoint SPKI Contract — SUCCESS
4. `34238782675` — Nolane Live Substrate Gauntlet — SUCCESS
5. `34238782758` — Deploy VitePress site to GitHub Pages — SUCCESS
6. `34238782584` — Wave29 Runtime Cgroup Helper Placement Contract — SUCCESS
7. `34238782639` — Docs Build Check — SUCCESS
8. `34238782960` — Wave 24 Cubelet Realization Epoch Contract — SUCCESS
9. `34238782481` — Cube Task Outcome Contract — SUCCESS
10. `34238782644` — Nolane World Check — SUCCESS
11. `34238782780` — Cube Realization OOM Contract — SUCCESS
12. `34238782679` — Cube Host Process Identity Contract — SUCCESS
13. `34238782662` — Format Check — SUCCESS
14. `34238782801` — Wave 25 Provider Incarnation Contract — SUCCESS
15. `34238782677` — Cube Guest Kernel OOM Victim Contract — SUCCESS

The Guest Kernel OOM workflow's canonical-builder live pre-Start binding job also completed SUCCESS.

Therefore the code-head candidate had 16/16 verification surfaces green: one dedicated push workflow plus 15 applicable PR integration workflows.

## Scope and ancestry audit before closure commit

Compared against exact Wave28 base `53ecdfdedd651c87f2b44e18fce352127c9f4242` at code head `6a5a8c6a2e635b45e2358e07170910c13f57553e`:

- merge base exactly equals the Wave28 exact-final head;
- `ahead_by=17`;
- `behind_by=0`;
- exactly 11 changed files;
- no Wave28 or Wave27 production authority implementation file is modified;
- changed surfaces are Wave29 implementation/protocol/tests, minimal `nolane-gauntlet-live` helper dispatch wiring/tests, Wave29 spec/plan/verification, static contract, and dedicated workflow.

PR #40 was also observed as `mergeable=true` and `rebaseable=true`; its temporary `mergeable_state=unstable` reflected in-flight checks rather than a content conflict.

## Explicit non-claims

Wave29 does not prove:

- induced CPU throttling causality;
- induced memory OOM causality;
- pressure magnitude;
- exact resource-limit enforcement causality;
- task success or task failure;
- guest/filesystem correctness;
- disk enforcement;
- hardware/TEE attestation;
- `resourceproof.TrustedReport` provenance; or
- public `LIVE_PASS`.

A later wave may build causal pressure evidence on top of this placement authority, but that is deliberately outside Wave29.

## Exact-final requirement

Do not treat the code-head evidence above as final closure evidence for the commit containing this file. After this closure record is committed, the new exact head must independently satisfy:

- the dedicated Wave29 push workflow; and
- every applicable PR-triggered integration workflow for PR #40.

Only after those fresh checks are all successful, ancestry/scope remain clean, and PR discussion/review surfaces remain clear may Wave29 be called code-closed. The PR remains draft and unmerged; merge requires explicit user authorization.

Autonomously-by: ChatGPT:GPT-5.6-Sol
