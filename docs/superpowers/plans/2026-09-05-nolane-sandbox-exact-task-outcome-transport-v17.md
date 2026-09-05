# Exact Task Outcome Transport Implementation Plan

> **Execution status:** implemented on `gpt/wave17-exact-task-outcome-transport` with RED→GREEN CI evidence. This file records the final implementation shape, including the package-boundary correction discovered by CI.

**Goal:** Transport Wave 16 exact task-outcome proof from Cubelet into NolaneWorld without precision loss, operational-state fallback, or OOM inference.

**Final architecture:** Keep Wave 16 as the only truth producer; enumerate its in-memory proofs deterministically; expose those accepted proofs to resource-metrics through a package-neutral structural visitor; encode each proof as one atomic Prometheus info metric with exact string labels; consume exactly one matching sample through a strict ResourceBinding-scoped NolaneWorld observer.

**Tech Stack:** Go 1.24.8 (Cubelet), Go 1.23 module compatibility (NolaneWorld), containerd task proof types, Prometheus client_golang, net/http, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-05-nolane-sandbox-exact-task-outcome-transport-v17-design.md`

## Global Constraints

- Wave 16 remains the only task-outcome truth producer.
- Do not infer outcome from CubeBox status, NotFound reconciliation, `time.Now()`, or exit code heuristics.
- Preserve arbitrary `uint64` generation values exactly; do not transport generation through binary64 sample values.
- Preserve `ExitCode uint32` and nanosecond `ExitedAt` exactly.
- Exit code `137` is numeric evidence only; do not set or infer OOM.
- No new public CubeAPI endpoint.
- Missing proof means unknown.
- Duplicate, malformed, partial, or unsupported proof transport fails closed.
- Do not introduce a production `resourcemetrics -> sandbox` package dependency.
- AI commits/PR descriptions use `Autonomously-by: ChatGPT:GPT-5.6-Sol`; never add a human DCO sign-off.

---

### Task 1: Deterministic Wave 16 Proof Enumeration

**Files:**
- Create: `Cubelet/plugins/cube/internals/sandbox/task_outcome_transport.go`
- Test: `Cubelet/plugins/cube/internals/sandbox/task_outcome_transport_v17_test.go`

**Final interfaces:**

```go
type TaskOutcomeProofLister interface {
    ListTaskOutcomeProofs() []TaskOutcomeProof
}

func (s *taskOutcomeProofStore) List() []TaskOutcomeProof
func (c *controllerLocal) ListTaskOutcomeProofs() []TaskOutcomeProof
```

- [x] Write RED tests for sorted detached snapshots, clearing, and full-width generation values.
- [x] Confirm RED in GitHub Actions before implementation.
- [x] Copy proof map values under `RLock`; sort by sandbox ID then generation.
- [x] Expose controller-local listing without changing Wave 16 lifecycle semantics.
- [x] Preserve `math.MaxUint64` exactly.

### Task 2: Package-Neutral Exact Cubelet Transport

**Files:**
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/task_outcome_prometheus.go`
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/plugin.go`
- Test: `Cubelet/plugins/cube/internals/resourcemetrics/task_outcome_transport_v17_test.go`

**Initial hypothesis:** resource-metrics could directly consume `sandbox.TaskOutcomeProofLister`.

**CI finding:** that dependency was invalid. Existing sandbox integration tests import resource-metrics, so importing sandbox from resource-metrics created a Go import cycle.

**Root-cause correction:** preserve the proof lister inside the sandbox package and add a package-neutral exact visitor on the concrete controller:

```go
func (c *controllerLocal) VisitTaskOutcomeProofs(
    visit func(
        sandboxID string,
        generation uint64,
        exitCode uint32,
        exitedAt time.Time,
        source string,
    ),
)
```

Resource-metrics consumes the same method structurally through a local interface, avoiding a reverse package dependency.

- [x] Reproduce the import-cycle failure in CI and inspect the package graph.
- [x] Remove the production `resourcemetrics -> sandbox` import rather than weakening tests.
- [x] Add an atomic `cubesandbox_task_outcome_info` descriptor with labels:
  - `sandbox_id`
  - `generation`
  - `source`
  - `exit_code`
  - `exited_at`
- [x] Emit metric value exactly `1` and carry all proof payload fields as exact strings.
- [x] Encode generation with `strconv.FormatUint(..., 10)` and timestamp with UTC `RFC3339Nano`.
- [x] Keep outcome collection independent from `SandboxResourceCache` availability.
- [x] Load the Cube sandbox controller once in `plugin.go` and require the structural exact-proof visitor.
- [x] Preserve the existing `containerdsandbox.Controller` assertion only for guest workload sampling.
- [x] Re-run focused contract after the package-boundary fix and confirm GREEN.

### Task 3: ResourceBinding-Scoped NolaneWorld Observer

**Files:**
- Create: `NolaneWorld/substrate/cube/task_outcome.go`
- Test: `NolaneWorld/substrate/cube/task_outcome_v17_test.go`

**Final API:**

```go
type TaskOutcomeProofSource string

const (
    TaskOutcomeProofSourceWait  TaskOutcomeProofSource = "containerd.task.wait"
    TaskOutcomeProofSourceState TaskOutcomeProofSource = "containerd.task.state"
)

type TaskOutcomeProof struct {
    SandboxID  string
    Generation uint64
    ExitCode   uint32
    ExitedAt   time.Time
    Source     TaskOutcomeProofSource
}

type TaskOutcomeConfig struct {
    BaseURL    string
    HTTPClient *http.Client
}

func NewTaskOutcomeObserver(TaskOutcomeConfig) (*TaskOutcomeObserver, error)
func (o *TaskOutcomeObserver) Observe(context.Context, ResourceBinding) (TaskOutcomeProof, bool, error)
```

- [x] Write RED consumer tests before production implementation.
- [x] Keep tests compatible with NolaneWorld's Go 1.23 module by using `context.Background()`.
- [x] Read at most 1 MiB from the existing Cubelet management metrics endpoint.
- [x] Ignore unrelated metrics and target samples belonging to other sandboxes.
- [x] Require exactly five labels for a matching sample.
- [x] Parse canonical nonzero generation as exact `uint64`.
- [x] Parse canonical exit code as exact `uint32`.
- [x] Restrict source to Wave 16 wait/state authority.
- [x] Require canonical UTC `RFC3339Nano` nonzero exit timestamp.
- [x] Require a finite metric value numerically equal to `1`.
- [x] Reject duplicate matching samples.
- [x] Return `(zero, false, nil)` only when no matching exact proof exists.
- [x] Keep exit code `137` free of OOM classification.
- [x] Pass full NolaneWorld unit, race, vet, and evidence generation gates.

### Task 4: Permanent Cross-Module Contract

**Files:**
- Modify: `.github/workflows/cube-task-outcome-contract.yml`
- Update: Wave 17 spec and plan.

The existing Wave 16 contract is upgraded instead of adding a parallel workflow. This keeps the repository's verification graph concentrated around the trust boundary it protects.

- [x] Trigger the contract when sandbox proof, resource-metrics transport, NolaneWorld Cube consumer, Wave 17 docs, or the workflow itself changes.
- [x] Run producer + transport focused tests:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics \
  -run 'TaskOutcome|OutcomeCandidate' -count=1
```

- [x] Run exact NolaneWorld consumer tests:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'TaskOutcome' -count=1
```

- [x] Keep broader Cubelet/CubeCow unit, build, format, NolaneWorld race/vet/evidence, and trust workflows as independent final gates.

### Task 5: Final Trust Review

- [x] Confirm there is no `OOMKilled` field introduced by Wave 17.
- [x] Confirm there is no `137 => OOM` branch.
- [x] Confirm there is no CubeBox operational `Status` fallback.
- [x] Confirm there is no task-proof persistence or new public CubeAPI endpoint.
- [x] Confirm generation and timestamp never pass through floating-point proof representation.
- [x] Confirm malformed/duplicate/partial consumer evidence fails closed.
- [x] Confirm package dependency remains one-way and cycle-free.
- [ ] Require all final-head path-triggered GitHub Actions to finish before marking the PR ready for review.
- [ ] Update PR #28 with final RED/GREEN evidence and mark it ready only after final-head verification is green.

## Deferred Wave 18 Boundary

Do not correlate Wave 15 OOM evidence with Wave 17 task outcome yet. Wave 15 is scoped to sandbox/cgroup assignment, while task outcome generation is scoped to task realization. The next wave must first establish a realization-scoped OOM baseline or another authoritative realization binding; only then may NolaneWorld expose attributed OOM termination.

Autonomously-by: ChatGPT:GPT-5.6-Sol
