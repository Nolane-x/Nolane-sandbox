# Wave30 Exact CPU Throttle Intervention Authority — Closure Record

Date: 2026-09-08 UTC / 2026-09-09 Asia/Bangkok
Repository: `Nolane-x/Nolane-sandbox`
PR: #41 — `Wave 30: exact CPU throttle intervention authority`
Branch: `gpt/wave30-exact-cpu-throttle-intervention-authority`
Base branch: `gpt/wave29-runtime-cgroup-helper-placement-authority`
Exact Wave29 base: `9c9f33ae605b902d702dfd6510785d56a64ea779`

## Code-head closed before this record

Exact Wave30 code-head before this documentation-only closure commit:

`6e7a6ee10215a3ea6d5b44af19c9183e12638300`

That code-head is a provenance-clean squashed integration commit whose tree is byte-for-byte the reviewed Wave30 tree. It has exact parent `9c9f33ae605b902d702dfd6510785d56a64ea779` and accepted trailer:

`Autonomously-by: ChatGPT:GPT-5.6-Sol`

The squash was required because PR DCO run `34289933748` proved that two early design commits (`01d110e2e66ef5c4ab6bfaca493dd6979af3d78a` and `7178604acdeb83082aa47c2f87fac4b4a40c7e9a`) predated the repository contribution-provenance trailer convention. The DCO workflow itself was not weakened or bypassed. The branch was rebuilt from the exact reviewed tree with the Wave29 exact-final commit as parent; all committed RED/GREEN SHAs and workflow evidence remain recorded in the Wave30 RED verification document and Actions history.

## Trust claim closed by Wave30

Wave30 closes exactly one causal seam:

> The exact package-owned helper, after fresh runtime/cgroup provenance and exact cgroup placement/identity verification, performed the bounded package-owned CPU intervention, and the exact fresh cgroup-v2 CPU throttling counters increased under that intervention protocol.

A valid `RuntimeCPUThrottleInterventionAuthority` requires all of the following:

1. valid exact Wave27 runtime provenance;
2. fresh Wave28 cgroup readback provenance;
3. package-owned zero-argument production executor construction;
4. production cgroup root fixed to `/sys/fs/cgroup`;
5. package-owned helper executable measurement and exact running-child image verification;
6. package-generated 32-byte nonce;
7. separate Wave30 mode `cpu-throttle-v30`, leaving Wave29 `park-exit` behavior intact;
8. bounded exact protocol `READY -> START -> BURN_DONE -> EXIT -> DONE`;
9. exact helper PID/starttime/executable/cgroup-membership identity revalidation at phase boundaries;
10. CPU burn locked to an OS thread and required to run on the process-leader task (`gettid()==getpid()`), so `/proc/<pid>/schedstat` measures the exact burn task;
11. a quiet control window with zero movement in both `nr_throttled` and `throttled_usec`;
12. final attribution ordering `quiet-control -> helper identity -> schedstat baseline -> fresh control-after Wave28 readback -> helper identity -> START`;
13. minimum exact-helper schedstat runtime delta tied to the configured CPU quota;
14. strict post-intervention increase of both `nr_throttled` and `throttled_usec`;
15. fresh Wave28 readbacks across basis, control-before, control-after, pressure-after, and final phases;
16. immutable runtime/cgroup identity and configured limits across the full intervention;
17. exact helper completion with zero exit status and cleanup/reap semantics.

Only then is the sealed digest minted:

`runtime-cpu-throttle-intervention-v30:<64-lowercase-hex>`

with domain separator:

`nolane.runtime-cpu-throttle-intervention.v30\x00`

## TDD / review-driven RED lineage

The implementation retained strict RED-before-production and review-driven hardening evidence:

- Protocol RED `1a8566dad1536c5afebfae3e1799d85926e957a2`, run `34244807094`: setup succeeded; focused Wave30 compile failed only because protocol production symbols were absent.
- Protocol GREEN `3ed2efc9ae283234f02f29f4ebd14e8ee5d98676`, run `34245038083`: focused, command, prior-regression, vet, full test, race SUCCESS.
- Behavioral authority RED `5e872f212030ef394f8bc3fe4319e6d4b362b43b`, run `34245546394`: failed only on intentionally absent executor/authority/validator/digest/schedstat production seams.
- Initial authority candidate `c45ea55666a58d3bec4878959c2915f6b2bcfc98`, run `34245992297`: full dedicated SUCCESS.
- Attribution review RED `be6ab64eba74b66bd57802b91efe8589135d7354`, run `34246408715`: proved schedstat baseline was sampled before quiet-control completion and could include parked helper CPU.
- Hardened head `5510cce5ccb6deef5d0327cbec9d5efa90535ea3`, run `34246888689`: full dedicated SUCCESS after moving baseline post-control.
- Second attribution review RED `ea5bcc085445513fa78e7e97a457276b71444952`, run `34289228077`: proved the final control-after cgroup snapshot preceded schedstat baseline, leaving stale control counters relative to START.
- Final attribution order head `576413741bae35a88dba88671c4a110d6be409c6`, run `34289580242`: focused, command, static, prior regressions, vet, full test, race SUCCESS.
- Verification-lineage head `994efc66a7e7026b5b1b43a625a9b99e78ef4f8e`, run `34289749590`: full dedicated SUCCESS before PR creation.

## DCO blocker and resolution

The first PR code-head exposed a real repository integration blocker:

- DCO run `34289933748`: FAILURE.
- Exact cause: only early design commits `01d110e2...` and `7178604a...` lacked accepted contribution-provenance trailers.
- No Wave30 code/test failure was involved.
- The DCO workflow was not edited.
- The branch was squashed to the exact reviewed tree with exact Wave29 parent and an accepted `Autonomously-by` trailer.
- New code-head: `6e7a6ee10215a3ea6d5b44af19c9183e12638300`.
- Replacement DCO run `34290037849`: SUCCESS.

## Exact code-head ancestry / scope audit

Compare exact Wave29 base -> exact code-head `6e7a6ee10215a3ea6d5b44af19c9183e12638300`:

- status: `ahead`;
- merge base: exactly `9c9f33ae605b902d702dfd6510785d56a64ea779`;
- `ahead_by=1`;
- `behind_by=0`;
- exactly 11 changed files before this closure record.

Code-head changed files:

1. `.github/workflows/wave30-exact-cpu-throttle-intervention-contract.yml`
2. `NolaneWorld/substrate/cube/runtime_cgroup_helper_protocol.go`
3. `NolaneWorld/substrate/cube/runtime_cpu_intervention.go`
4. `NolaneWorld/substrate/cube/runtime_cpu_intervention_protocol_v30_test.go`
5. `NolaneWorld/substrate/cube/runtime_cpu_intervention_v30_attribution_review_test.go`
6. `NolaneWorld/substrate/cube/runtime_cpu_intervention_v30_review_test.go`
7. `NolaneWorld/substrate/cube/runtime_cpu_intervention_v30_test.go`
8. `docs/superpowers/plans/2026-09-08-exact-cpu-throttle-intervention-authority-v30.md`
9. `docs/superpowers/specs/2026-09-08-exact-cpu-throttle-intervention-authority-v30-design.md`
10. `docs/superpowers/verification/2026-09-08-wave30-exact-cpu-throttle-intervention-red.md`
11. `tests/wave30_exact_cpu_throttle_intervention_contract.py`

Only one pre-existing production file is modified: `runtime_cgroup_helper_protocol.go`. Its existing Wave29 `runInternalCgroupHelper` park-exit protocol remains unchanged; Wave30 adds a distinct mode and bounded protocol path. No Wave27 or Wave28 production authority implementation file is modified.

## Exact code-head dedicated verification

Push workflow on exact code-head `6e7a6ee10215a3ea6d5b44af19c9183e12638300`:

- `34290035408` — Wave30 Exact CPU Throttle Intervention Contract — **SUCCESS**.

It completed all of:

- focused Wave30 tests;
- command integration;
- static Wave30 anti-shortcut contract;
- Wave29 regression;
- Wave28 regression;
- Wave27 regression;
- `go vet ./...`;
- full `go test ./...`;
- focused Wave30 `-race` tests.

## Exact code-head PR integration verification

All 16/16 applicable PR-triggered workflows on exact code-head `6e7a6ee10215a3ea6d5b44af19c9183e12638300` completed **SUCCESS**:

1. `34290037849` — DCO Check
2. `34290037867` — Wave 26 Provider Endpoint SPKI Contract
3. `34290037837` — Nolane Live Substrate Gauntlet
4. `34290037884` — Deploy VitePress site to GitHub Pages
5. `34290037852` — Cube Task Outcome Contract
6. `34290037977` — Cube Kernel OOM Victim Contract
7. `34290037868` — Docs Build Check
8. `34290037843` — Cube Host Process Identity Contract
9. `34290037876` — Wave29 Runtime Cgroup Helper Placement Contract
10. `34290037906` — Cube Realization OOM Contract
11. `34290037874` — Wave 25 Provider Incarnation Contract
12. `34290037875` — Wave 24 Cubelet Realization Epoch Contract
13. `34290037860` — Wave30 Exact CPU Throttle Intervention Contract
14. `34290037882` — Nolane World Check
15. `34290037915` — Format Check
16. `34290037881` — Cube Guest Kernel OOM Victim Contract

Within Guest Kernel OOM run `34290037881`, the deepest `cubelet-wave21` job also completed **SUCCESS**, including `Verify CubeBox live pre-Start binding in canonical builder`.

Thus the code-head has **17/17 relevant verification gates green**: one dedicated push gate plus all 16 applicable PR integration workflows.

## PR review state at code-head closure

PR #41 at code-head closure:

- open;
- draft;
- unmerged;
- mergeable after GitHub mergeability calculation stabilized;
- no submitted reviews;
- no review threads.

## Explicit non-claims

Wave30 does **not** prove or claim:

- memory enforcement;
- memory OOM causality;
- disk enforcement;
- aggregate resource enforcement;
- generic task success/failure;
- filesystem or guest correctness;
- image/binary measurement beyond the helper image checks already described;
- hardware/TEE attestation;
- `resourceproof.TrustedReport` provenance;
- public `LIVE_PASS`;
- any change in Wave29 helper-placement authority semantics.

Wave30 remains a narrow CPU-causality authority. A later wave must explicitly bridge this sealed authority into any broader resource-proof producer instead of treating Wave30 itself as a public live verdict.

## Closure rule

This file is documentation-only. Its commit becomes the Wave30 exact-final head. No completion claim is valid until the dedicated Wave30 push gate and every applicable PR-triggered workflow are freshly rerun and successful on that exact-final SHA, followed by a fresh ancestry/scope/PR-state audit.

This PR intentionally remains draft and unmerged. Merge requires explicit user authorization.

Autonomously-by: ChatGPT:GPT-5.6-Sol
