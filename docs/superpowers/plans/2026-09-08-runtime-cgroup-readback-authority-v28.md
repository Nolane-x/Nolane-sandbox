# Wave 28 Implementation Plan — Exact Runtime Cgroup Readback Authority

Date: 2026-09-08
Base: `f42d42dec3809bbbd6b8a6b5862c8a3268f6f1b2`
Branch: `gpt/wave28-runtime-cgroup-readback-authority`
Design: `docs/superpowers/specs/2026-09-08-runtime-cgroup-readback-authority-v28-design.md`

## Goal

Create an opaque cgroup-v2 readback authority that can only be minted from a freshly reconstructed Wave27 runtime authority and package-owned reads of the exact cgroup containing the Wave27 host PID. Do not mint trusted resource enforcement evidence.

## Task 1 — Commit RED contract and observer tests

Files:
- add `NolaneWorld/substrate/cube/runtime_cgroup_readback_v28_test.go`
- add `tests/wave28_runtime_cgroup_readback_contract.py`
- add `.github/workflows/wave28-runtime-cgroup-readback-contract.yml`
- add `docs/superpowers/verification/2026-09-08-wave28-runtime-cgroup-readback-red.md`

Behavioral RED requirements:
- reference `RuntimeCgroupReadbackObserver`, `RuntimeCgroupReadbackAuthority`, and validator APIs that do not yet exist;
- build an in-package fake read root/reader through an unexported test constructor;
- happy-path v2 fixture requires exact host PID membership and finite canonical `cpu.max`, `cpu.stat`, `memory.max`, `memory.events`;
- wrong PID membership fails;
- `cpu.max=max` and `memory.max=max` fail;
- malformed/duplicate stats fail;
- zero Wave28 authority accessors fail closed;
- deterministic digest is asserted.

Static RED requirements:
- require planned production file and domain separator;
- require production observer root `/sys/fs/cgroup`;
- forbid Wave28 production API surface from taking or referring to `Binding`, `CgroupRoot`, `HostFileSource`, `HostPressureRunner`;
- forbid `TrustedReport`, `buildTrustedReport`, `LIVE_PASS`, `CapabilityEvidence` in Wave28 production file;
- require exact Wave27 pre/post validation calls and PID membership check.

Workflow order:
1. setup Go/Python;
2. focused Wave28 Go tests;
3. static Wave28 contract;
4. prior Wave27 focused regression;
5. `go vet ./...` in NolaneWorld;
6. `go test ./...` in NolaneWorld;
7. focused race.

RED acceptance:
- workflow reaches focused Wave28 compile;
- failure is specifically missing Wave28 production API/types, not dependency/harness/test syntax failure.

## Task 2 — Implement cgroup-v2 observer and canonical parsers

Production file:
- add `NolaneWorld/substrate/cube/runtime_cgroup_readback.go`

Implement:
- opaque `RuntimeCgroupReadbackObserver`;
- `NewRuntimeCgroupReadbackObserver()` fixed to `/sys/fs/cgroup` + `os.ReadFile`;
- package-private test constructor for root/read function;
- secure target path derivation from authority-owned absolute cgroup path;
- root `cgroup.controllers` v2 check;
- canonical `cgroup.procs` parser with exact PID membership;
- canonical finite `cpu.max` parser;
- canonical `cpu.stat` unique-key parser with required `nr_throttled`, `throttled_usec`;
- canonical finite `memory.max` parser;
- canonical `memory.events` unique-key parser with required `oom`, `oom_kill`;
- context fail-closed checks around reads.

Run focused tests. If they reveal a test-fixture defect, fix tests before proceeding.

## Task 3 — Implement Wave27 pre/post reconstruction and opaque authority

In `runtime_cgroup_readback.go` implement:
- `RuntimeCgroupReadbackSnapshot` descriptive exported value;
- sealed `RuntimeCgroupReadbackAuthority`;
- structural `Valid()` and fail-closed accessors;
- internal `sameRealmResourceRuntimeAuthority`;
- `ValidateRuntimeCgroupReadbackAuthority(...)` (final name may be `ValidateRealmResourceRuntimeCgroupAuthority` if tests lock that spelling) that receives the Wave27 freshness context, supplied Wave27 authority, and Wave28 observer;
- pre-read `ValidateRealmResourceRuntimeAuthority(...)` reconstruction using nested authority-owned Wave27 inputs;
- exact equality with supplied authority;
- exact cgroup readback using authority-owned runtime process;
- post-read Wave27 reconstruction and exact equality;
- sealed authority mint only after all checks pass.

Important: validator must not accept an external cgroup root/path/filesystem/pressure source/binding.

## Task 4 — Digest and anti-substitution hardening

Implement:
- domain separator `nolane.runtime-cgroup-readback.v28\x00`;
- `runtime-cgroup-readback-v28:<sha256>` over canonical JSON of Wave27 runtime digest + exact process/cgroup identity + readback snapshot;
- strict lowercase digest validation;
- `Valid()` recomputation;
- tests for changed PID/cgroup/runtime digest/counter/limit substitution where reachable through in-package fixture helpers;
- pre/post Wave27 substitution/staleness tests.

## Task 5 — Dedicated GREEN closure on branch

Run/observe dedicated Wave28 workflow on the exact branch head until all steps are SUCCESS:
- focused Wave28 tests;
- static Wave28 contract;
- Wave27 regression;
- NolaneWorld vet;
- NolaneWorld full tests;
- focused race.

Do not claim closure from an earlier SHA.

## Task 6 — Open stacked draft PR

Open a draft PR:
- base branch: `gpt/wave27-runtime-realization-provenance-authority`
- head: `gpt/wave28-runtime-cgroup-readback-authority`
- explicitly state Wave28 does not mint `TrustedReport` or `LIVE_PASS`.

Do not merge.

## Task 7 — Exact-head integration audit

For the PR exact head:
- observe every applicable pull-request workflow to completion;
- inspect failures and fix only with evidence;
- compare base→head: merge base must equal Wave27 final SHA, `behind_by=0`;
- list changed files and ensure scope is Wave28-only;
- check review submissions and inline review threads.

## Task 8 — Closure record and exact-final rerun

After first integration GREEN:
- commit `docs/superpowers/verification/2026-09-08-wave28-runtime-cgroup-readback-closure.md` with RED→GREEN lineage, run IDs, scope and non-claims;
- this commit changes SHA, so invalidate prior exact-head completion evidence;
- rerun/observe all applicable PR workflows on the new final SHA;
- only after all final-SHA workflows succeed, update PR body metadata with exact-final evidence without changing the head.

## Completion criteria

Wave28 is code-closed only when:
- TDD RED is committed and proven to fail for missing Wave28 production behavior;
- behavioral/static tests are GREEN;
- no public arbitrary cgroup locator/filesystem/pressure injection exists;
- pre/post fresh Wave27 reconstruction is enforced;
- exact cgroup-v2 host PID membership is enforced;
- finite CPU/memory readback and counters are sealed into a deterministic digest;
- Wave28 production code cannot mint `TrustedReport`, capability evidence, or `LIVE_PASS`;
- full applicable PR CI is GREEN on one exact final SHA;
- branch remains stacked, draft and unmerged.
