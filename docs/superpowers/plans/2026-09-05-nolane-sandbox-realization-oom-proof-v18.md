# Realization-Scoped Kernel OOM Evidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce, transport, and consume exact kernel cgroup OOM-kill evidence bound to the same controller-local task realization generation as Wave 16/17 task outcome proof, without claiming the main task was the OOM victim.

**Architecture:** Resource metrics supplies the sandbox controller with a package-neutral cgroup snapshot callback backed by CubeBox `CGroupPath` plus the cgroup plugin's `UsageSnapshot`. The existing task-outcome store owns the generation and stores an in-memory Start baseline plus a one-shot final proof. Resource metrics exports the proof as an exact string-label info metric, and NolaneWorld parses task outcome plus optional realization OOM proof from one scrape and requires exact generation/timestamp/source correlation.

**Tech Stack:** Go 1.24.8 (Cubelet), Go 1.23 module compatibility (NolaneWorld), containerd task runtime API, CubeBox store, Cube cgroup `UsageSnapshot`, Prometheus client_golang, net/http, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-05-nolane-sandbox-realization-oom-proof-v18-design.md`

## Global Constraints

- Wave 16 remains the only exact task-outcome truth producer.
- Wave 18 may assert only realization-scoped kernel cgroup OOM-kill counter evidence.
- Never infer OOM from exit code `137`, SIGKILL, CubeBox status, NotFound reconciliation, or Wave 15 assignment-scoped totals.
- Missing baseline/final signal is unknown, never zero.
- Final OOM sampling is one-shot after exact task outcome acceptance; no post-exit retry.
- A valid proof requires one generation, one cgroup path, non-regressing counter continuity, and `baseline_at <= exited_at <= observed_at`.
- Wave 18 proof is controller-local and non-persistent.
- `resourcemetrics` production code must not import `sandbox`.
- Integers in the public proof transport use canonical decimal string labels; timestamps use canonical UTC RFC3339Nano.
- Existing Wave 15 host telemetry and Wave 17 task-outcome APIs remain backward compatible.
- Every autonomous commit/PR description includes `Autonomously-by: ChatGPT:GPT-5.6-Sol`; never add `Signed-off-by`.

---

### Task 1: RED Contract and Dedicated CI

**Files:**
- Create: `Cubelet/plugins/cube/internals/sandbox/realization_oom_v18_test.go`
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/realization_oom_v18_test.go`
- Create: `NolaneWorld/substrate/cube/task_termination_v18_test.go`
- Create: `.github/workflows/cube-realization-oom-contract.yml`

**Interfaces:**
- Consumes: Wave 16 `TaskOutcomeProof`, Wave 17 `TaskOutcomeObserver`, existing `handle.UsageSnapshot`.
- Produces test contracts for `RealizationOOMProof`, `SetRealizationOOMSnapshotReader`, `VisitRealizationOOMProofs`, resource-metrics exact transport, and `TaskTerminationObserver`.

- [ ] **Step 1: Write failing sandbox lifecycle tests**

Create tests that call the intended store/controller interfaces. Core fixture:

```go
baselineAt := time.Date(2026, 9, 5, 5, 0, 0, 123456789, time.UTC)
exitedAt := baselineAt.Add(30 * time.Second)
observedAt := exitedAt.Add(time.Nanosecond)

store := newTaskOutcomeProofStore()
generation := store.BeginRealization("sandbox-a")
if ok := store.RecordRealizationOOMBaseline("sandbox-a", generation, realizationOOMSnapshot{
    CGroupPath: "/cube_sandbox_v2/sandbox/numa/7/sandbox-a",
    OOMKillsKnown: true,
    OOMKillsTotal: 7,
    CapturedAt: baselineAt,
}); !ok {
    t.Fatal("baseline not accepted")
}
outcome, err := store.Record(taskOutcomeCandidate{
    SandboxID: "sandbox-a",
    ExitCode: 137,
    ExitedAt: exitedAt,
    Source: TaskOutcomeProofSourceWait,
})
if err != nil { t.Fatal(err) }
proof, ok := store.FinalizeRealizationOOM(outcome, realizationOOMSnapshot{
    CGroupPath: "/cube_sandbox_v2/sandbox/numa/7/sandbox-a",
    OOMKillsKnown: true,
    OOMKillsTotal: 9,
    CapturedAt: observedAt,
})
if !ok || proof.OOMKills != 2 || proof.Generation != generation {
    t.Fatalf("proof = %#v, ok=%v", proof, ok)
}
```

Add negative tests for no baseline, unknown final signal, path mismatch, counter regression, exit before baseline, final observation before exit, one-shot freeze after a failed finalization, Create clear, newer Start clear, and exact zero delta.

- [ ] **Step 2: Write failing resource-metrics transport tests**

Use a fake CubeBox store and fake cgroup reader to require a provider that returns exact path/counter/capture time. Use a fake structural proof visitor to require this exact exposition:

```text
cubesandbox_task_realization_oom_info{sandbox_id="sandbox-a",generation="18446744073709551615",cgroup_path="/cube/path",signal="kernel.cgroup.memory.oom_kill",baseline_oom_kills="18446744073709551614",final_oom_kills="18446744073709551615",oom_kills="1",baseline_at="2026-09-05T05:00:00.123456789Z",observed_at="2026-09-05T05:01:00.987654321Z",exited_at="2026-09-05T05:00:59.555555555Z",outcome_source="containerd.task.wait"} 1
```

Require unknown/incomplete proof to emit no Wave 18 metric and require snapshot-provider lookup/read failures to return unavailable evidence rather than a false zero.

- [ ] **Step 3: Write failing NolaneWorld correlation tests**

Use one HTTP fixture containing both Wave 17 and Wave 18 samples. Require:

```go
evidence, known, err := observer.Observe(context.Background(), binding)
if err != nil || !known { t.Fatalf("Observe = %#v, %v, %v", evidence, known, err) }
if evidence.Outcome.Generation != math.MaxUint64 { t.Fatal("generation rounded") }
if evidence.RealizationOOM == nil || evidence.RealizationOOM.OOMKills != 2 {
    t.Fatalf("OOM evidence = %#v", evidence.RealizationOOM)
}
observed, knownOOM := evidence.KernelOOMObservedDuringRealization()
if !knownOOM || !observed { t.Fatal("expected exact positive kernel OOM observation") }
```

Add malformed tables for extra/missing labels, duplicate target OOM sample, OOM without task outcome, generation/source/exit timestamp mismatch, non-canonical uints, unsupported signal, counter regression, incorrect delta, invalid timestamp ordering, non-unit metric value, and exit `137` with no Wave 18 proof returning `RealizationOOM == nil`.

- [ ] **Step 4: Add focused workflow**

Create `cube-realization-oom-contract.yml` with PR/push path filters covering sandbox, resourcemetrics, NolaneWorld cube substrate, this spec/plan, and the workflow. Run:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics -run 'RealizationOOM|TaskTermination' -count=1

cd NolaneWorld
go test ./substrate/cube -run 'RealizationOOM|TaskTermination' -count=1
```

- [ ] **Step 5: Commit RED only and open a draft stacked PR**

Commit tests/workflow without production implementation:

```text
test(v18): define realization OOM proof contract

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

Open the PR with base `gpt/wave17-exact-task-outcome-transport` and verify the dedicated contract fails because Wave 18 APIs/implementation are absent.

---

### Task 2: Generation-Owned OOM Proof Store

**Files:**
- Modify: `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/realization_oom_proof.go`
- Test: `Cubelet/plugins/cube/internals/sandbox/realization_oom_v18_test.go`

**Interfaces:**
- Consumes: `TaskOutcomeProof`, the existing store generation/fence lock.
- Produces: `RealizationOOMProof`, `RecordRealizationOOMBaseline`, `FinalizeRealizationOOM`, `ListRealizationOOMProofs`.

- [ ] **Step 1: Extend store-owned state under the existing mutex**

Add maps for baseline, proof, and terminal finalization generation:

```go
oomBaselines map[string]realizationOOMBaseline
oomProofs map[string]RealizationOOMProof
oomFinalized map[string]uint64
```

Initialize them in `newTaskOutcomeProofStore`; delete their entries in `Clear` and `BeginRealization`.

- [ ] **Step 2: Define exact Wave 18 records**

```go
const realizationOOMSignal = "kernel.cgroup.memory.oom_kill"

type realizationOOMSnapshot struct {
    CGroupPath string
    OOMKillsKnown bool
    OOMKillsTotal uint64
    CapturedAt time.Time
}

type realizationOOMBaseline struct {
    SandboxID string
    Generation uint64
    CGroupPath string
    OOMKills uint64
    CapturedAt time.Time
}

type RealizationOOMProof struct {
    SandboxID string
    Generation uint64
    CGroupPath string
    BaselineOOMKills uint64
    FinalOOMKills uint64
    OOMKills uint64
    BaselineAt time.Time
    ObservedAt time.Time
    ExitedAt time.Time
    OutcomeSource TaskOutcomeProofSource
}
```

- [ ] **Step 3: Implement baseline acceptance**

`RecordRealizationOOMBaseline` returns false unless current generation matches, no Create fence supersedes it, path is canonical non-blank text, signal is known, and timestamp is non-zero. Store the timestamp in UTC.

- [ ] **Step 4: Implement terminal finalization before validation**

`FinalizeRealizationOOM` first verifies the passed outcome is the currently accepted exact task proof for the current generation. It then marks `oomFinalized[sandboxID] = generation` before validating baseline/final snapshot. A second call returns the already stored proof if one exists or remains unknown if the first attempt failed.

Accept proof only when path matches, counter does not regress, and `baselineAt <= outcome.ExitedAt <= observedAt`. Compute delta with unsigned subtraction only after `final >= baseline`.

- [ ] **Step 5: Implement detached deterministic listing**

Copy proof values under `RLock`, unlock, sort by `SandboxID` then `Generation`, and return the detached slice.

- [ ] **Step 6: Run focused sandbox tests and commit GREEN store layer**

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox -run 'RealizationOOM' -count=1
```

Commit:

```text
feat(v18): bind OOM evidence to task generation

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 3: Controller Baseline Capture and One-Shot Finalization

**Files:**
- Modify: `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- Modify/Create: `Cubelet/plugins/cube/internals/sandbox/realization_oom_proof.go`
- Test: `Cubelet/plugins/cube/internals/sandbox/realization_oom_v18_test.go`

**Interfaces:**
- Consumes: store methods from Task 2.
- Produces structural methods consumed by resource metrics:

```go
SetRealizationOOMSnapshotReader(func(context.Context, string) (string, bool, uint64, time.Time, error))
VisitRealizationOOMProofs(func(string, uint64, string, uint64, uint64, uint64, time.Time, time.Time, time.Time, string))
```

- [ ] **Step 1: Add race-safe callback binding**

Add a controller mutex plus callback field. Setter copies the callback under lock; runtime readers copy it under lock and invoke it after unlocking.

- [ ] **Step 2: Capture baseline inside `Start`**

Keep `BeginRealization` as generation authority:

```go
generation := c.beginTaskOutcomeRealization(sandboxID)
c.captureRealizationOOMBaseline(ctx, sandboxID, generation)
return sandbox.ControllerInstance{}, nil
```

A provider failure or unknown signal must not change `Start`'s return value.

- [ ] **Step 3: Finalize only after exact outcome acceptance**

In `Wait`, call Wave 18 finalization only after `recordAuthoritativeTaskOutcomeCandidate` returns its accepted proof. In stopped `Status`, retain the accepted proof returned by `store.Record` and then finalize it. Provider failure must not invalidate the exact task outcome or status response.

- [ ] **Step 4: Add package-neutral visitor**

Visit detached store proofs and pass exact fields through the primitive/std-library callback. Do not expose store internals and do not import resource metrics.

- [ ] **Step 5: Run sandbox race-focused tests and commit**

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox -run 'RealizationOOM|TaskOutcome' -count=1
go test -race ./plugins/cube/internals/sandbox -run 'RealizationOOM' -count=1
```

Commit:

```text
feat(v18): capture realization OOM window

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 4: Trusted Snapshot Provider and Exact Prometheus Transport

**Files:**
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/realization_oom_snapshot.go`
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/realization_oom_prometheus.go`
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/task_outcome_prometheus.go`
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/plugin.go`
- Test: `Cubelet/plugins/cube/internals/resourcemetrics/realization_oom_v18_test.go`

**Interfaces:**
- Consumes: `cubes.CubeboxAPI.Get`, `hostSandboxUsageReader.UsageSnapshot`, controller structural binder/visitor.
- Produces: provider callback and `cubesandbox_task_realization_oom_info`.

- [ ] **Step 1: Implement CubeBox/cgroup snapshot provider**

```go
func newRealizationOOMSnapshotReader(
    store cubes.CubeboxAPI,
    reader hostSandboxUsageReader,
    now func() time.Time,
) func(context.Context, string) (string, bool, uint64, time.Time, error)
```

The closure trims only for validation; it must reject a path whose exact value contains leading/trailing whitespace. It performs `store.Get`, copies `CGroupPath`, invokes `UsageSnapshot`, and returns `now().UTC()` immediately after a successful snapshot. `MemoryOOMKillsKnown == false` returns `known=false` with no synthesized zero claim.

- [ ] **Step 2: Bind provider unconditionally during plugin initialization**

Load the cgroup plugin independent of host telemetry export scope, assert `hostSandboxUsageReader`, assert controller `realizationOOMSnapshotBinder` and `realizationOOMProofVisitor`, then install the callback. Reuse the same cgroup reader for `HostSandboxSampler` when that export scope is enabled.

- [ ] **Step 3: Add exact info collector**

Define 11 labels in this exact order:

```go
[]string{
    "sandbox_id", "generation", "cgroup_path", "signal",
    "baseline_oom_kills", "final_oom_kills", "oom_kills",
    "baseline_at", "observed_at", "exited_at", "outcome_source",
}
```

Validate path/time/source/arithmetic before `prometheus.MustNewConstMetric`, then format all uints with `strconv.FormatUint` and times with UTC RFC3339Nano.

- [ ] **Step 4: Preserve Wave 17 helpers**

Add a new internal evidence-aware handler/service constructor and make existing `newServiceWithTaskOutcomes` / `newPrometheusHandlerWithTaskOutcomes` delegate with a nil Wave 18 visitor, so Wave 17 tests and callers remain source-compatible.

- [ ] **Step 5: Run resource-metrics tests and commit**

```bash
cd Cubelet
go test ./plugins/cube/internals/resourcemetrics -run 'RealizationOOM|TaskOutcome' -count=1
go test -race ./plugins/cube/internals/resourcemetrics -run 'RealizationOOM' -count=1
```

Commit:

```text
feat(v18): export exact realization OOM proof

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 5: NolaneWorld Single-Scrape Correlation

**Files:**
- Create: `NolaneWorld/substrate/cube/task_termination.go`
- Test: `NolaneWorld/substrate/cube/task_termination_v18_test.go`
- Reuse: `NolaneWorld/substrate/cube/task_outcome.go`
- Reuse: `NolaneWorld/substrate/cube/host_resource.go` label parser

**Interfaces:**
- Consumes: Wave 17 exact sample parser helpers and opaque `ResourceBinding`.
- Produces:

```go
type RealizationOOMProof struct { /* exact fields from spec */ }
type TaskTerminationEvidence struct {
    Outcome TaskOutcomeProof
    RealizationOOM *RealizationOOMProof
}
type TaskTerminationObserver struct { endpoint string; http *http.Client }
func NewTaskTerminationObserver(TaskOutcomeConfig) (*TaskTerminationObserver, error)
func (*TaskTerminationObserver) Observe(context.Context, ResourceBinding) (TaskTerminationEvidence, bool, error)
func (TaskTerminationEvidence) KernelOOMObservedDuringRealization() (observed bool, known bool)
```

- [ ] **Step 1: Implement one-GET observer**

Use the same endpoint validation/default timeout contract as `NewTaskOutcomeObserver`. GET `/v1/metrics/resource` once and parse task outcome plus Wave 18 proof from one bounded body stream.

- [ ] **Step 2: Parse exact Wave 18 labels**

Require exactly 11 labels, value exactly one, canonical decimal uint64 strings, exact signal constant, exact non-whitespace path, and canonical UTC RFC3339Nano timestamps. Reject counter regression, bad delta, and invalid ordering.

- [ ] **Step 3: Correlate with Wave 17 proof**

If Wave 18 proof is present, require exact equality for sandbox ID, generation, `ExitedAt`, and outcome source. A detached Wave 18 proof without a target Wave 17 proof fails closed. Missing Wave 18 proof remains unknown and does not invalidate exact task outcome.

- [ ] **Step 4: Add non-causal helper**

Return `(false, false)` when `RealizationOOM == nil`; otherwise return `(OOMKills > 0, true)`. Documentation must explicitly say the helper does not identify the main-task victim.

- [ ] **Step 5: Run NolaneWorld unit/race/vet and commit**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'RealizationOOM|TaskTermination|TaskOutcome' -count=1
go test -race ./substrate/cube -run 'RealizationOOM|TaskTermination' -count=1
go vet ./substrate/cube
```

Commit:

```text
feat(v18): correlate realization OOM evidence

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 6: Final Trust Review and Repository Verification

**Files:**
- Modify only if required by verified failures: Wave 18 implementation/test/workflow files above.
- Update PR body; do not add source comments containing PR metadata.

**Interfaces:**
- Consumes: all Wave 18 components.
- Produces: ready-for-review stacked PR with final-head executable evidence.

- [ ] **Step 1: Run the dedicated contract until GREEN**

Expected focused commands:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics -run 'RealizationOOM|TaskTermination' -count=1

cd NolaneWorld
go test ./substrate/cube -run 'RealizationOOM|TaskTermination' -count=1
```

- [ ] **Step 2: Verify compatibility gates**

Require final-head success for repository unit/build/format/DCO/NolaneWorld checks plus Wave 15 host-resource, Wave 17 task-outcome, and live substrate gates triggered by the changed paths.

- [ ] **Step 3: Audit the final diff for forbidden causal inference**

Search changed production files and ensure there is no logic equivalent to:

```go
if exitCode == 137 && oomKills > 0 { OOMKilled = true }
```

There must be no public `OOMKilled` field, no Status-derived outcome fallback, no Wave 15 assignment-total correlation, and no final OOM retry loop.

- [ ] **Step 4: Review PR comments/threads and fix only technically valid findings**

Apply systematic debugging to any code/test failure. Distinguish infrastructure failures (for example unavailable review-bot credentials) from code failures.

- [ ] **Step 5: Mark stacked PR ready only after final-head verification**

Update the PR body with RED/GREEN evidence, exact final SHA, trust invariants, and deferred victim-level provenance. Keep base branch `gpt/wave17-exact-task-outcome-transport`; do not merge `master` autonomously.

Autonomously-by: ChatGPT:GPT-5.6-Sol
