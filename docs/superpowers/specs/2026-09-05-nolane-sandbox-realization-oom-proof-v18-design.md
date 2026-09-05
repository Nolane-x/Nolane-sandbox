# Nolane Sandbox Wave 18 — Realization-Scoped Kernel OOM Evidence

## Purpose

Wave 18 closes the trust gap left intentionally open by Waves 15–17: bind kernel cgroup OOM-kill evidence to the same controller-local task realization that owns an accepted exact `TaskOutcomeProof`.

Wave 18 does **not** claim that the main task was killed by the OOM killer. A cgroup `oom_kill` counter proves that the kernel OOM-killed at least one process inside that cgroup during the measured window. It does not identify the victim process. Therefore Wave 18 exposes realization-scoped kernel OOM evidence, not an `OOMKilled` task-cause classification.

## Existing Trust Boundaries

### Wave 15

Wave 15 reads the kernel cgroup OOM-kill counter and normalizes it against a baseline captured when a host cgroup is assigned to a sandbox. That evidence is assignment-scoped. It is suitable for host resource telemetry but cannot be attached to one task realization when multiple `Start` realizations reuse the same cgroup assignment.

### Wave 16

Wave 16 accepts exact task outcome only from authoritative containerd task `Wait` or stopped `State` responses. Accepted proofs are controller-local and carry:

- sandbox identity;
- controller-local realization generation;
- exact unsigned exit code;
- exact runtime exit timestamp;
- authoritative source (`containerd.task.wait` or `containerd.task.state`).

### Wave 17

Wave 17 transports the currently accepted Wave 16 proof into NolaneWorld as one atomic Prometheus info sample and consumes it fail-closed through an opaque `ResourceBinding`.

## Wave 18 Trust Statement

Wave 18 may assert only:

> The kernel cgroup OOM-kill counter increased by N while the cgroup assigned to sandbox S was bound to controller-local task generation G, with a baseline captured during `Start` before that `Start` returned and a final snapshot captured once after the exact task outcome for generation G was accepted.

Wave 18 must never infer:

- that the main task was the OOM victim;
- that exit code `137` means OOM;
- that SIGKILL means OOM;
- that an assignment-scoped Wave 15 counter delta belongs to one task realization;
- that a missing or unreadable OOM counter means zero;
- that a later post-exit cgroup sample can repair a missing finalization sample.

## Architecture

The authority graph is:

```text
CubeBox store ── sandbox_id -> CGroupPath ─┐
                                           ├─ realization OOM snapshot callback
cgroup plugin ── UsageSnapshot(cgroup) ────┘
                     │
                     ▼
sandbox controller Start
  ├─ BeginRealization -> generation G
  └─ capture OOM baseline for (sandbox_id, G) before Start returns

containerd Wait / stopped State
  └─ accepted TaskOutcomeProof(sandbox_id, G, exit_code, exited_at, source)
        │
        └─ one final OOM snapshot attempt
              │
              └─ RealizationOOMProof if continuity checks pass
                     │
                     ▼
resource-metrics Prometheus info sample
                     │
                     ▼
NolaneWorld TaskTerminationObserver
  ├─ exact TaskOutcomeProof
  └─ optional correlated RealizationOOMProof
```

The sandbox package remains independent of `resourcemetrics`. Resource metrics binds a callback into the controller through a package-neutral structural interface whose method signature contains only `context.Context`, primitive values, `time.Time`, and `error`.

## Producer: Snapshot Provider

Resource metrics owns the snapshot provider because it already has access to both required trusted inputs:

1. the CubeBox store, which maps exact `sandbox_id` to persisted `CGroupPath`;
2. the cgroup plugin, whose `UsageSnapshot(ctx, CGroupPath)` reads the kernel-backed `MemoryOOMKillsKnown` and `MemoryOOMKillsTotal` signal.

The provider contract is conceptually:

```go
func(context.Context, string) (
    cgroupPath string,
    oomKillsKnown bool,
    oomKillsTotal uint64,
    capturedAt time.Time,
    err error,
)
```

Rules:

- blank sandbox identity is rejected;
- CubeBox lookup failure is unavailable evidence;
- blank `CGroupPath` is unavailable evidence;
- cgroup read failure is unavailable evidence;
- `MemoryOOMKillsKnown == false` is unknown, never zero;
- `capturedAt` is host UTC time recorded immediately after a successful cgroup snapshot;
- there is no fallback to Wave 15 persisted assignment baseline;
- there is no fallback to host-sampler cached telemetry.

## Producer: Realization Baseline

`controllerLocal.Start` keeps Wave 16 generation authority.

For each non-empty sandbox ID:

1. `BeginRealization` increments the existing task-outcome generation and clears prior task/OOM evidence for that sandbox.
2. The controller invokes the bound OOM snapshot provider synchronously before `Start` returns.
3. A baseline is accepted only when the provider returns a non-blank cgroup path, `oomKillsKnown == true`, a non-zero capture timestamp, and no error.
4. The baseline is stored against exactly `(sandbox_id, generation)`.
5. Failure to capture the baseline does **not** fail sandbox start. OOM evidence for that realization remains unknown.

The baseline record contains:

```go
type realizationOOMBaseline struct {
    SandboxID  string
    Generation uint64
    CGroupPath string
    OOMKills   uint64
    CapturedAt time.Time
}
```

A controller restart may recover a Wave 16 task realization from a fresh authoritative runtime observation, but it must not synthesize a Wave 18 baseline. Therefore recovered realizations have unknown OOM evidence unless the original in-memory baseline is still present.

## Producer: Finalization

A final OOM snapshot is attempted only after the corresponding exact Wave 16 task outcome has been accepted.

The first finalization attempt for a generation is terminal. The store records that finalization was attempted before validating the snapshot. If the provider is absent, returns an error, reports unknown OOM state, reports another cgroup path, reports a regressed counter, or violates timestamp continuity, no OOM proof is emitted and later `Status`/`Wait` calls cannot retry that generation.

This one-shot rule prevents a post-exit scrape from accidentally including OOM kills from cleanup processes or a subsequent workload that reused the cgroup.

A `RealizationOOMProof` is accepted only when all conditions hold:

- an exact `TaskOutcomeProof` for the same sandbox and generation is currently accepted;
- an exact Wave 18 baseline exists for the same sandbox and generation;
- baseline and final snapshots have the same non-blank cgroup path;
- final OOM counter is greater than or equal to the baseline counter;
- `baseline_at <= task_outcome.exited_at <= observed_at`;
- task outcome source is one of the Wave 16 authoritative sources;
- no explicit Create fence or newer Start generation has superseded the realization.

The proof is:

```go
type RealizationOOMProof struct {
    SandboxID        string
    Generation       uint64
    CGroupPath       string
    BaselineOOMKills uint64
    FinalOOMKills    uint64
    OOMKills         uint64
    BaselineAt       time.Time
    ObservedAt       time.Time
    ExitedAt         time.Time
    OutcomeSource    TaskOutcomeProofSource
}
```

where `OOMKills == FinalOOMKills - BaselineOOMKills`.

A valid proof with `OOMKills == 0` is exact evidence of zero kernel OOM kills in the measured realization window. Absence of a proof means unknown.

## Lifecycle and Concurrency

The Wave 18 state lives under the same task-outcome proof store lock and the same generation authority as Wave 16.

`Create(sandbox_id)`:

- clears task outcome proof;
- clears realization OOM baseline/proof/finalization marker;
- preserves the existing recovery fence behavior.

`Start(sandbox_id)`:

- advances generation;
- clears old task/OOM proof state;
- captures one baseline for the new generation.

`Wait` / stopped `State`:

- first accept exact task outcome through Wave 16 rules;
- then perform exactly one OOM finalization attempt for that generation.

A stale finalizer for an older generation cannot finalize or poison the current generation. A newer `Start` invalidates all prior Wave 18 state before its baseline is captured.

The snapshot callback is installed once by resource-metrics initialization and accessed through a controller-side mutex so plugin initialization and runtime calls are race-safe.

## Transport

Wave 18 exports one atomic info-style sample per accepted realization OOM proof:

```text
cubesandbox_task_realization_oom_info{
  sandbox_id="sandbox-a",
  generation="18446744073709551615",
  cgroup_path="/cube_sandbox_v2/sandbox/numa/...",
  signal="kernel.cgroup.memory.oom_kill",
  baseline_oom_kills="7",
  final_oom_kills="9",
  oom_kills="2",
  baseline_at="2026-09-05T05:00:00.123456789Z",
  observed_at="2026-09-05T05:01:00.987654321Z",
  exited_at="2026-09-05T05:00:59.555555555Z",
  outcome_source="containerd.task.wait"
} 1
```

All integers are decimal string labels, not Prometheus scalar values, so arbitrary `uint64` values remain exact above binary64's 2^53 precision boundary. All timestamps are canonical UTC RFC3339Nano strings.

The `signal` label is the normalized kernel-cgroup signal name `kernel.cgroup.memory.oom_kill`. It deliberately does not claim a victim PID and does not distinguish cgroup v1/v2 in the public proof contract; both are already normalized by `handle.UsageSnapshot` into the same kernel OOM-kill counter semantics.

The `cgroup_path` label is provenance only. NolaneWorld never uses it as a filesystem path or authority input.

No metric is emitted for unknown/incomplete evidence.

## Package Boundary

To preserve the Wave 17 acyclic package graph, `resourcemetrics` must not import `sandbox` in production.

The controller exposes two structural methods using only primitive and standard-library types:

```go
SetRealizationOOMSnapshotReader(func(context.Context, string) (string, bool, uint64, time.Time, error))

VisitRealizationOOMProofs(func(
    sandboxID string,
    generation uint64,
    cgroupPath string,
    baselineOOMKills uint64,
    finalOOMKills uint64,
    oomKills uint64,
    baselineAt time.Time,
    observedAt time.Time,
    exitedAt time.Time,
    outcomeSource string,
))
```

Resource metrics declares matching local interfaces and type-asserts the sandbox-controller plugin structurally.

## NolaneWorld Consumer

Wave 18 adds a `TaskTerminationObserver` that performs one management-metrics HTTP GET and parses both Wave 17 task outcome evidence and Wave 18 realization OOM evidence from the same scrape.

Public evidence types are:

```go
type RealizationOOMProof struct {
    SandboxID        string
    Generation       uint64
    CGroupPath       string
    Signal           string
    BaselineOOMKills uint64
    FinalOOMKills    uint64
    OOMKills         uint64
    BaselineAt       time.Time
    ObservedAt       time.Time
    ExitedAt         time.Time
    OutcomeSource    TaskOutcomeProofSource
}

type TaskTerminationEvidence struct {
    Outcome        TaskOutcomeProof
    RealizationOOM *RealizationOOMProof
}
```

`RealizationOOM == nil` means realization-scoped OOM evidence is unknown. A non-nil proof with `OOMKills == 0` means exact zero. A non-nil proof with `OOMKills > 0` means at least one kernel OOM kill was observed in the cgroup during the measured realization window.

A helper may expose that tri-state explicitly, but its name and documentation must say **kernel OOM observed during realization**, not `OOMKilled`.

## NolaneWorld Fail-Closed Parsing

For the target `ResourceBinding` sandbox:

- missing exact task outcome -> `(zero, false, nil)` when no Wave 18 proof is present;
- exact task outcome with missing Wave 18 proof -> known termination outcome with `RealizationOOM == nil`;
- Wave 18 proof without matching Wave 17 outcome -> `ErrTaskOutcomeUnavailable`;
- duplicate target task outcome -> error;
- duplicate target realization OOM proof -> error;
- malformed target metric -> error;
- extra or missing labels -> error;
- metric value other than finite numeric one -> error;
- non-canonical unsigned decimal labels -> error;
- unsupported outcome source -> error;
- blank/whitespace-normalized cgroup path -> error;
- unsupported `signal` -> error;
- non-canonical UTC RFC3339Nano timestamps -> error;
- counter regression -> error;
- `oom_kills != final_oom_kills - baseline_oom_kills` -> error;
- `baseline_at > exited_at` -> error;
- `observed_at < exited_at` -> error;
- generation mismatch with Wave 17 outcome -> error;
- exit timestamp mismatch with Wave 17 outcome -> error;
- outcome-source mismatch with Wave 17 outcome -> error.

Metrics for other sandbox identities are ignored after their identity can be parsed safely.

## Explicit Non-Goals

Wave 18 does not:

- classify the main task as OOM-killed;
- infer anything from exit code `137`;
- infer anything from SIGKILL;
- correlate Wave 15 assignment-scoped OOM totals directly with Wave 17 outcome;
- add eBPF, audit, PSI, tracepoint, or victim-PID collection;
- add a public CubeAPI endpoint;
- persist realization OOM proof across controller restart;
- retry final OOM sampling after the first exact-outcome finalization attempt;
- change existing host resource telemetry semantics;
- use cgroup path labels as an authority or filesystem input in NolaneWorld.

## Failure Semantics

OOM evidence is observational and must not make sandbox execution unavailable.

- baseline read failure: sandbox starts; Wave 18 evidence remains unknown;
- final read failure: task outcome remains valid; Wave 18 evidence remains unknown;
- missing resource-metrics binder: sandbox starts; Wave 18 evidence remains unknown;
- invalid continuity: no Wave 18 proof is stored or exported;
- malformed exported proof at NolaneWorld: consumer fails closed.

The exact task outcome remains independently usable when OOM evidence is unknown.

## TDD Contract

Wave 18 implementation begins with failing tests for:

1. realization baseline/proof lifecycle and one-shot finalization;
2. controller Start/Wait integration and generation binding;
3. snapshot provider CubeBox/cgroup binding;
4. exact Prometheus transport including `math.MaxUint64` labels;
5. NolaneWorld single-scrape correlation and strict malformed-input rejection;
6. explicit negative tests proving exit code `137` alone never creates OOM evidence;
7. Create/new-Start/restart paths cannot leak or reconstruct prior realization evidence.

A dedicated GitHub Actions contract runs the focused Cubelet and NolaneWorld Wave 18 tests. Final readiness additionally requires the repository's broad unit, build, format, DCO, Wave 15 host-resource, Wave 17 task-outcome, and NolaneWorld gates to remain green.

## Next Trust Closure

A future wave may add victim-level kernel provenance, such as an authoritative kernel event carrying the killed PID/cgroup identity and a trusted binding from that PID to the main task realization. Only such evidence can justify a causal statement that the **main task** was OOM-killed. Wave 18 intentionally stops one trust boundary earlier.

Autonomously-by: ChatGPT:GPT-5.6-Sol
