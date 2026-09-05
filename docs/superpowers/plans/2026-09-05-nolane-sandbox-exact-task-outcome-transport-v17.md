# Exact Task Outcome Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transport Wave 16 exact task-outcome proof from Cubelet into NolaneWorld without precision loss, operational-state fallback, or OOM inference.

**Architecture:** Add deterministic proof enumeration to the Wave 16 in-memory proof plane, then expose each accepted proof as one atomic Prometheus info metric whose payload is entirely string labels. Add a dedicated ResourceBinding-scoped NolaneWorld observer that parses exactly one matching sample and fails closed on malformed or ambiguous evidence.

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
- AI commits/PR descriptions use `Autonomously-by: ChatGPT:GPT-5.6-Sol`; never add a human DCO sign-off.

---

### Task 1: Deterministic Wave 16 Proof Enumeration

**Files:**
- Modify: `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go`
- Modify: `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- Test: `Cubelet/plugins/cube/internals/sandbox/task_outcome_transport_v17_test.go`

**Interfaces:**
- Consumes: existing `TaskOutcomeProof`, `taskOutcomeProofStore`, and `controllerLocal.ensureTaskOutcomeProofStore()`.
- Produces:

```go
type TaskOutcomeProofLister interface {
    ListTaskOutcomeProofs() []TaskOutcomeProof
}

func (s *taskOutcomeProofStore) List() []TaskOutcomeProof
func (c *controllerLocal) ListTaskOutcomeProofs() []TaskOutcomeProof
```

- [ ] **Step 1: Write the failing enumeration tests**

Create tests that record proofs for `sandbox-b` and `sandbox-a`, require returned order `sandbox-a`, `sandbox-b`, mutate the returned slice and verify a second read is unchanged, then `Clear("sandbox-a")` and require the cleared proof to disappear.

- [ ] **Step 2: Run the focused test and verify RED**

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox -run 'TaskOutcome.*Transport|TaskOutcome.*List' -count=1
```

Expected: compile/test failure because `ListTaskOutcomeProofs` / store `List` does not exist.

- [ ] **Step 3: Implement detached deterministic listing**

In `task_outcome_proof.go`, add `sort` and copy the map values under `RLock`, then sort by `SandboxID` and `Generation`. Never return the internal map or references to internal mutable storage.

In `cube_sandbox_manager.go`, expose `ListTaskOutcomeProofs()` through `ensureTaskOutcomeProofStore()` and add a compile-time interface assertion for `TaskOutcomeProofLister`.

- [ ] **Step 4: Run focused tests and verify GREEN**

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox -run 'TaskOutcome' -count=1
```

Expected: PASS.

### Task 2: Exact Atomic Cubelet Transport

**Files:**
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/plugin.go`
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/prometheus.go`
- Test: `Cubelet/plugins/cube/internals/resourcemetrics/task_outcome_transport_v17_test.go`

**Interfaces:**
- Consumes: `sandbox.TaskOutcomeProofLister` from Task 1.
- Produces:

```go
func NewServiceWithTaskOutcomes(cache *SandboxResourceCache, outcomes sandbox.TaskOutcomeProofLister) *Service
func NewPrometheusHandlerWithTaskOutcomes(cache *SandboxResourceCache, outcomes sandbox.TaskOutcomeProofLister) http.Handler
func newPrometheusHandlerWithTaskOutcomes(cache *SandboxResourceCache, outcomes sandbox.TaskOutcomeProofLister, now func() time.Time) http.Handler
```

Keep existing `NewService`, `NewPrometheusHandler`, and `newPrometheusHandler` as compatibility wrappers with a nil outcome lister.

- [ ] **Step 1: Write failing transport tests**

Use a fake lister with one proof containing:

```go
sandbox.TaskOutcomeProof{
    SandboxID:  "sandbox-a",
    Generation: math.MaxUint64,
    ExitCode:   137,
    ExitedAt:   time.Date(2026, 9, 5, 4, 5, 6, 123456789, time.UTC),
    Source:     sandbox.TaskOutcomeProofSourceWait,
}
```

Require the HTTP exposition to contain `cubesandbox_task_outcome_info` and exact string labels for sandbox, full decimal generation, source, exit code, and `2026-09-05T04:05:06.123456789Z`. Run the handler with `cache=nil` to prove outcome transport is independent from resource sampling. Add a no-proof test requiring the metric to be absent.

- [ ] **Step 2: Run focused test and verify RED**

```bash
cd Cubelet
go test ./plugins/cube/internals/resourcemetrics -run 'TaskOutcome' -count=1
```

Expected: compile failure because the outcome-aware handler does not exist.

- [ ] **Step 3: Implement the info metric**

Define:

```go
var taskOutcomeInfo = prometheus.NewDesc(
    "cubesandbox_task_outcome_info",
    "Exact containerd task-outcome proof accepted by the Cube sandbox controller.",
    []string{"sandbox_id", "generation", "source", "exit_code", "exited_at"},
    nil,
)
```

For each listed proof, emit exactly one gauge with value `1`. Encode generation and exit code with `strconv.FormatUint`, and encode time with `proof.ExitedAt.UTC().Format(time.RFC3339Nano)`.

Do not depend on `SandboxResourceCache.ListLatest` to discover proofs.

- [ ] **Step 4: Wire the real Cube controller proof lister**

In `plugin.go`, load the required `SandboxControllerPlugin` once, retain the existing `containerdsandbox.Controller` assertion when guest workload sampling is enabled, and additionally require the Cube controller to implement `sandbox.TaskOutcomeProofLister`. Construct `NewServiceWithTaskOutcomes(cache, taskOutcomes)`.

- [ ] **Step 5: Run focused and package tests**

```bash
cd Cubelet
go test ./plugins/cube/internals/resourcemetrics -run 'TaskOutcome|Prometheus' -count=1
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics -count=1
```

Expected: PASS.

### Task 3: ResourceBinding-Scoped NolaneWorld Observer

**Files:**
- Create: `NolaneWorld/substrate/cube/task_outcome.go`
- Test: `NolaneWorld/substrate/cube/task_outcome_v17_test.go`

**Interfaces:**
- Consumes: existing `ResourceBinding`, `splitMetricToken`, Cubelet management metrics path.
- Produces:

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

type TaskOutcomeObserver struct { /* host-owned endpoint/client */ }

func NewTaskOutcomeObserver(TaskOutcomeConfig) (*TaskOutcomeObserver, error)
func (o *TaskOutcomeObserver) Observe(context.Context, ResourceBinding) (TaskOutcomeProof, bool, error)
```

- [ ] **Step 1: Write failing consumer tests**

Cover an exact sample with `math.MaxUint64` generation, exit code `137`, and nanosecond time. Require exact equality and no OOM classification field. Cover absent proof, wrong sandbox, duplicate matching samples, missing labels, extra labels, zero/invalid generation, out-of-range exit code, unsupported source, invalid timestamp, and metric value other than exactly `1`.

- [ ] **Step 2: Run focused test and verify RED**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'TaskOutcome' -count=1
```

Expected: compile failure because `TaskOutcomeObserver` does not exist.

- [ ] **Step 3: Implement strict parsing**

Read at most 1 MiB from `/v1/metrics/resource`. Ignore unrelated metrics and target samples for other sandbox IDs. For a matching target sample, require exactly these labels:

```text
sandbox_id
generation
source
exit_code
exited_at
```

Parse `generation` with `strconv.ParseUint(..., 10, 64)` and require nonzero. Parse `exit_code` with 32-bit width. Restrict source to the two Wave 16 values. Parse `exited_at` with `time.RFC3339Nano`, normalize to UTC, and reject zero. Require scalar value exactly the lexical number `1` after numeric parsing. Reject a second matching sample.

Return `(TaskOutcomeProof{}, false, nil)` only when no matching target sample exists.

- [ ] **Step 4: Run focused and package tests**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'TaskOutcome|V15' -count=1
go test ./substrate/cube -count=1
```

Expected: PASS.

### Task 4: Permanent Cross-Module Contract and Verification

**Files:**
- Create: `.github/workflows/cube-task-outcome-transport-contract.yml`
- Create: `docs/superpowers/specs/2026-09-05-nolane-sandbox-exact-task-outcome-transport-v17-design.md`
- Create: `docs/superpowers/plans/2026-09-05-nolane-sandbox-exact-task-outcome-transport-v17.md`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces: permanent CI evidence that producer, transport, and consumer remain compatible.

- [ ] **Step 1: Add the workflow**

Trigger on changes to Wave 16 sandbox proof files, resource-metrics files, NolaneWorld Cube substrate files, this workflow, and Wave 17 spec/plan. Use `actions/setup-go@v6` with `go-version-file: Cubelet/go.mod`.

- [ ] **Step 2: Run the exact contract commands in CI**

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics -run 'TaskOutcome' -count=1
cd ../NolaneWorld
go test ./substrate/cube -run 'TaskOutcome' -count=1
```

- [ ] **Step 3: Run broader fresh verification on the final branch**

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics -count=1
go vet ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics

cd ../NolaneWorld
go test ./...
go test -race ./...
go vet ./...
```

Also require all path-triggered GitHub Actions for the final head to complete successfully before creating a merge-ready conclusion.

- [ ] **Step 4: Review the final diff against the spec**

Confirm there is no `OOMKilled` inference, no `137 => OOM` branch, no operational `Status` fallback, no task-proof persistence, no public CubeAPI endpoint, and no floating-point transport of generation/timestamp proof fields.

- [ ] **Step 5: Open a PR from `gpt/wave17-exact-task-outcome-transport` to `master`**

The PR body must include:

```text
Autonomously-by: ChatGPT:GPT-5.6-Sol
```

and summarize RED evidence, GREEN CI evidence, exactness guarantees, and the intentionally deferred realization-scoped OOM correlation boundary.
