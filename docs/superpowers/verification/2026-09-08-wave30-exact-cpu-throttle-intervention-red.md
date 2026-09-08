# Wave30 Exact CPU Throttle Intervention — RED Verification

Date: 2026-09-08
Base: Wave29 exact-final `9c9f33ae605b902d702dfd6510785d56a64ea779`

## Protocol RED

Exact RED head:

`1a8566dad1536c5afebfae3e1799d85926e957a2`

Dedicated workflow:

- Run `34244807094`
- Workflow: `Wave30 Exact CPU Throttle Intervention Contract`
- Event: `push`
- Setup Python/pytest/Go: SUCCESS
- Focused Wave30 tests: expected FAILURE

The failure was attributable specifically to missing Wave30 production protocol semantics:

- `undefined: runtimeCPUInterventionMode`
- `undefined: runtimeCPUInterventionBurnMicrosEnv`
- `undefined: runInternalCPUInterventionHelper`

No test syntax, fixture, YAML, checkout, Python, pytest, or Go setup error preceded the failure.

This RED establishes valid TDD lineage before any Wave30 production helper protocol implementation.

## Protocol GREEN checkpoint

Exact protocol GREEN head:

`3ed2efc9ae283234f02f29f4ebd14e8ee5d98676`

Dedicated run `34245038083` completed SUCCESS for:

- focused Wave30 helper protocol tests;
- command integration;
- Wave29 regression;
- Wave28 regression;
- Wave27 regression;
- `go vet ./...`;
- full `go test ./...`;
- focused Wave30 race tests.

The static Wave30 contract was intentionally absent/skipped at this checkpoint.

## Behavioral authority RED

Final pre-production behavioral RED head:

`5e872f212030ef394f8bc3fe4319e6d4b362b43b`

Dedicated workflow:

- Run `34245546394`
- Setup Python/pytest/Go: SUCCESS
- Focused Wave30 tests: expected FAILURE

The failure was attributable specifically to missing Wave30 authority production seams, including:

- `RuntimeCPUInterventionExecutor`;
- `RuntimeCPUThrottleInterventionAuthority`;
- `newRuntimeCPUInterventionExecutorForTest`;
- `runtimeCPUInterventionExecutorTestConfig`;
- `runtimeCPUInterventionChild`;
- `ValidateRuntimeCPUThrottleInterventionAuthority`;
- `deriveV30AuthorityDigest`;
- `parseRuntimeCPUSchedstat`;
- `ErrInvalidRuntimeCPUThrottleInterventionAuthority`.

The review/failure-contract test file also compiled far enough to fail only on these intentionally absent Wave30 production seams. No syntax, fixture, workflow, checkout, Python, pytest, or Go setup error preceded the failure.

This RED therefore establishes valid TDD lineage before the Wave30 executor/authority production implementation.

## Attribution review RED — exclude parked-control CPU

Whole-diff review found that the first implementation sampled helper `schedstat` before the quiet control wait. That could count CPU consumed while the helper was merely parked toward the intervention runtime delta.

Exact review RED head:

`be6ab64eba74b66bd57802b91efe8589135d7354`

Dedicated workflow:

- Run `34246408715`
- Setup Python/pytest/Go: SUCCESS
- Focused Wave30 tests: expected FAILURE
- Failing test: `TestV30SchedstatBaselineIsTakenAfterQuietControl`

Observed ordering evidence ended before the baseline with:

`LAUNCH READY STARTTIME_READ ATTACH MEMBERSHIP_READ STARTTIME_READ`

The test failed specifically because the first `schedstat` baseline occurred before quiet-control completion. Production was then changed to sample the baseline only after the quiet control wait.

The hardened implementation was verified FULL GREEN at head `5510cce5ccb6deef5d0327cbec9d5efa90535ea3` by run `34246888689`, including focused tests, command integration, Wave29/28/27 regressions, `go vet ./...`, full `go test ./...`, and focused race tests.

## Attribution review RED — freshest cgroup baseline before START

A second review refinement tightened the attribution window further. After quiet control, the helper `schedstat` baseline must be sampled before the final control-after Wave28 cgroup readback so that:

1. parked helper CPU is excluded from the intervention runtime delta; and
2. the control-after throttle counters remain the freshest cgroup baseline immediately before `START`.

Exact review RED head:

`ea5bcc085445513fa78e7e97a457276b71444952`

Dedicated workflow:

- Run `34289228077`
- Setup Python/pytest/Go: SUCCESS
- Focused Wave30 tests: expected FAILURE
- Failing test: `TestV30SchedstatBaselinePrecedesFinalControlAfterReadback`

The failure recorded the old sequence explicitly:

`... CONTROL_SLEEP ... POST_CONTROL_CGROUP_READ ... SCHEDSTAT_READ START ...`

No syntax, fixture, checkout, setup, or unrelated regression failure preceded it.

Production was then changed to the exact ordering:

`quiet-control wait -> exact helper identity -> schedstat baseline -> fresh control-after Wave28 readback -> exact helper identity -> START`

The final static contract was updated to encode the same invariant. Exact post-fix head `576413741bae35a88dba88671c4a110d6be409c6` completed dedicated run `34289580242` SUCCESS for focused Wave30 tests, command integration, static trust contract, Wave29/28/27 regressions, `go vet ./...`, full `go test ./...`, and focused race tests.
