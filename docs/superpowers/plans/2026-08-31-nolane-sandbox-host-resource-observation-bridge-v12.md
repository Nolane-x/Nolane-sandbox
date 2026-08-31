# Host Resource Observation Bridge v12 — Implementation Plan

## Objective
Close the missing mechanical Linux cgroup observation layer behind the already-merged v11 typed resource verifier while preserving its package-owned authority boundary. Do not add a generic public live constructor until Cube exposes an opaque host realization binding.

## Task 1 — Design / authority correction — COMPLETE
The initial child-package design was rejected after inspecting v11 `TrustedReport`: its unexported state is intentionally mintable only inside `resourceproof`.

Final rule:
- authority-bearing observer stays in parent package `resourceproof`;
- helper parsing may be split later;
- no public API may accept arbitrary cgroup roots/callbacks/observations and return trusted proof;
- no agentruntime setter/control surface is added.

## Task 2 — RED host observer contracts — COMPLETE
File:
- `NolaneWorld/gauntlet/live/resourceproof/host_observer_v12_test.go`

Locked behavior:
- cgroup v2 exact CPU/memory readback and throttle/OOM normalization;
- cgroup v1 exact readback and throttle/OOM normalization;
- v1 throttled nanoseconds normalize to v11 microseconds;
- malformed/unlimited bounded values fail closed;
- voluntary exit 137 cannot manufacture OOM proof;
- missing throttle delta cannot manufacture CPU proof;
- readback mismatch is carried to v11 semantic verifier;
- pressure/infrastructure failure is unavailable, never PASS;
- private cgroup sentinel cannot enter trusted serialized evidence.

RED was observed on exact GitHub SHA before production implementation.

## Task 3 — GREEN package-owned mechanical bridge — COMPLETE
File:
- `NolaneWorld/gauntlet/live/resourceproof/host_observer.go`

Implemented:
- v1/v2 parser/readback normalization;
- exact finite-limit handling;
- causal counter snapshots;
- host pressure/task-status interface composition;
- v2 `oom_kill` preference with safe counter fallback;
- v1 effectively-unlimited memory sentinel rejection;
- fail-honest unavailable error surface without leaking raw host errors/paths;
- package-owned `TrustedReport` mint path;
- trusted report serialization that verifies before encoding and contains no host locator.

## Task 4 — Integration boundary review — COMPLETE FOR v12
A generic resource-proof CLI is deliberately **not** added in v12.

Reason: the present Cube live API lacks one opaque object that binds exact Realm/policy/runtime realization to its host cgroup, host pressure execution and authoritative OOM task status. Accepting those values independently from CLI flags or exported callbacks would allow provenance self-assertion and undo v11.

v12 therefore closes the mechanical bridge but keeps its trusted constructor/observe path package-private. Live use remains unavailable until an opaque Cube-host realization binding exists.

Required next wave:
- RED contract for opaque Cube-host realization handle;
- exact binding freshness/staleness semantics;
- host cgroup association owned by Cube host, not caller;
- pressure/task status methods on the opaque authority;
- only then a minimal explicit live CLI/CI path.

## Task 5 — Regression and nondrift — IN PROGRESS
On the exact final candidate require:
- Go unit tests for `NolaneWorld`;
- race tests for live/resourceproof surface;
- `go vet` for live/resourceproof surface;
- repository format checks;
- docs build;
- contribution provenance gate;
- Nolane World Check;
- live negative-control semantics (`UNAVAILABLE != PASS`);
- comparison against v11 base proving no historical v11 source/artifact mutation outside intended v12 docs/tests/new observer.

No new canonical evidence family is introduced by v12; schema remains v11. Therefore v12 must not rewrite historical v4–v11 release evidence.

## Task 6 — Release hygiene — PENDING
- audit exact PR diff and review surface;
- preserve RED/GREEN history on a backup branch;
- compact the audited v12 tree into one provenance commit direct child of then-current `master` if `master` has not moved incompatibly;
- fresh exact-head CI;
- update PR from RED text to final scope/non-claims;
- complete same-session review;
- merge only after required gates are green or a clearly unrelated review-infrastructure failure is documented;
- verify merged `master` exact tree;
- require fresh post-merge Nolane World Check before declaring v12 closed.

## Explicit deferred scope
Cube-host realization authority binding and an executable resource-proof live CLI belong to the next wave, not v12. This is a security boundary, not an implementation omission: until the substrate can prove that binding without caller assertion, the correct result is `UNAVAILABLE`.
