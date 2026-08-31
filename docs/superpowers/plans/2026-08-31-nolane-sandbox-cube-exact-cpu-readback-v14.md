# Nolane Sandbox — Cube Exact CPU Readback Bridge v14 Implementation Plan

Date: 2026-08-31
Base: `43bbbcf48d7539e0ca55139bef911da77f5e3a30`
Design: `docs/superpowers/specs/2026-08-31-nolane-sandbox-cube-exact-cpu-readback-v14-design.md`

## Goal

Carry Cubelet's already-existing exact host cgroup CPU quota/period readback across `/v1/metrics/resource` into the sandbox-bound Nolane host observation without reconstructing exact evidence from the derived cores ratio.

## Task 1 — Prove RED at the real producer boundary

Create `Cubelet/plugins/cube/internals/resourcemetrics/prometheus_v14_test.go`.

Contracts:

- finite host CPU limit exports exact quota microseconds;
- finite host CPU limit exports exact period microseconds;
- existing cores metric remains available;
- unlimited CPU limit exports none of the finite-limit metrics.

Expected RED: exact quota/period metric assertions fail because `prometheus.go` currently exports only the derived cores ratio.

Verification gate: `Unit Test Check` must select Cubelet (and existing CubeCow companion target) for both amd64 and arm64 because the changed path is under `Cubelet/**`.

## Task 2 — Prove RED at the Nolane consumer boundary

Create `NolaneWorld/substrate/cube/host_resource_v14_test.go`.

Contracts:

- observer preserves exact quota and period values;
- a cores-only v13 response fails closed;
- fractional exact quota fails closed;
- overflow exact quota fails closed;
- derived cores must agree with exact quota/period.

Tests use reflection for the new snapshot fields in the RED phase so failures are semantic rather than compile-only.

Expected RED: current observer ignores the new exact metrics and still accepts cores-only / malformed-new-field fixtures.

## Task 3 — Implement producer GREEN

Modify `Cubelet/plugins/cube/internals/resourcemetrics/prometheus.go`:

- add descriptors:
  - `cubesandbox_host_sandbox_cpu_limit_quota_microseconds`
  - `cubesandbox_host_sandbox_cpu_limit_period_microseconds`;
- include them in `Describe`;
- emit them in `collectHostPrometheus` only for finite limits with positive quota/period;
- retain existing `cpu_limit_cores` for compatibility;
- do not export `CGroupPath` or any private host locator.

Run focused producer tests and full Cubelet CI.

## Task 4 — Implement consumer GREEN

Modify `NolaneWorld/substrate/cube/host_resource.go`:

- add `CPULimitQuotaUS uint64` and `CPULimitPeriodUS uint64` to `HostResourceSnapshot`;
- require both exact metric names in `hostResourceMetricNames`;
- parse with `positiveUint`;
- check derived `CPULimitCores` against `quota / period` with a narrow floating tolerance suitable for Prometheus text serialization;
- retain all existing bounded-body, exact-sandbox, duplicate, finite-number, and overflow protections;
- return only scalar observations; never serialize a cgroup path.

Run focused consumer tests and full Nolane World unit/race/vet gates.

## Task 5 — Cross-boundary nondrift verification

Verify that:

- producer metric names exactly equal consumer metric names;
- units are microseconds on both sides;
- finite/unlimited semantics agree;
- two exact tuples with the same cores ratio remain distinguishable in the observer;
- no v13 compatibility field was silently renamed;
- no OOM/task provenance claim was introduced.

## Task 6 — Exact-SHA release closure

1. Confirm every applicable fresh PR workflow for the final SHA.
2. Inspect PR review comments/threads and resolve payload findings.
3. Ensure all commits carry accepted contribution provenance; never fabricate human DCO.
4. Merge with `expected_head_sha` locked to the reviewed candidate.
5. Verify `master` points to the merge result and the merged tree contains the reviewed v14 changes.
6. Verify fresh push-to-master Cubelet Unit Test Check and Nolane World Check.
7. Only then mark Wave 14 closed.

## Deferred next wave

Trace CubeBox/containerd lifecycle writes for `ExitCode`, `Reason`, and termination state and determine whether there is a host-authoritative OOM signal that can be bound to the same sandbox realization. Until that provenance is demonstrated, `MemoryFailures` remains observational and must never become `OOMKilled` proof.
