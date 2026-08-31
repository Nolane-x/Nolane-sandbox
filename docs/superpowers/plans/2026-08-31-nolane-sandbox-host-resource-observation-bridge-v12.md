# Host Resource Observation Bridge v12 — Implementation Plan

## Objective
Wire real Linux cgroup observations into the already-merged v11 typed resource verifier without importing the Cubelet module into NolaneWorld or exposing host authority to agents.

## Task 1 — RED: host observer contract

Files:
- add `NolaneWorld/gauntlet/live/resourceproof/hostobserver/observer_v12_test.go`

Tests:
- cgroup v2 bounded readback + throttle/OOM normalization;
- cgroup v1 bounded readback + throttle/OOM normalization;
- malformed/unlimited values fail closed;
- voluntary exit 137 and counter-only OOM cannot manufacture memory proof;
- pressure with no throttle delta cannot manufacture CPU proof;
- private host locator/sentinel never appears in `resourceproof.MarshalReport` output.

Run on GitHub Actions before production code. Expected RED must be missing observer production API only; historical packages remain GREEN.

## Task 2 — GREEN: standard-library cgroup observer

Files:
- add `NolaneWorld/gauntlet/live/resourceproof/hostobserver/observer.go`

Implement:
- private read-only file source;
- v1/v2 parsers;
- host pressure runner interface;
- authoritative task-status interface;
- fail-honest `Observe(ctx, resourceproof.RunSpec)`;
- no host locator in returned observation.

The observer emits only v11 scalar observations and `SourceLiveHost` when every required live step completed through the host-owned production path.

## Task 3 — Integration boundary

Files:
- add focused adapter/integration tests beside `resourceproof/hostobserver`;
- only if necessary, add a tiny host wiring shim under `NolaneWorld/gauntlet/live/cube`.

Requirements:
- exact realization/fingerprint binding remains host-side;
- no setter/control method is added to `agentruntime.Runtime`;
- no raw cgroup path, task handle, secret, token or provider handle reaches canonical evidence.

## Task 4 — CLI/CI live surface

Reuse the v11 resource proof CLI if available; otherwise add only the minimal host-observer selection needed for explicit live execution.

CI:
- hosted runners exercise deterministic fixture/negative-control only;
- self-hosted live runner is the only place permitted to claim live host proof;
- absence of cgroup/task infrastructure => `UNAVAILABLE`.

## Task 5 — Regression and nondrift

On exact candidate:
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- format checks
- docs build
- provenance gate
- live negative-control
- v4–v11 artifact byte/hash nondrift audit
- v12 deterministic negative-control + secret scans if a new artifact family is introduced.

## Task 6 — Release hygiene

- audit diff and review surface;
- preserve full RED/GREEN history on a backup branch;
- compact final audited tree into one provenance commit direct child of then-current `master`;
- fresh exact-head CI;
- merge;
- verify `master` exact tree and fresh post-merge World Check/artifact before declaring closure.
