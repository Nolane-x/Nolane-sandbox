# Wave 30 Exact CPU Throttle Intervention Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a sealed CPU-only authority proving a package-owned single-worker intervention is associated with exact cgroup-v2 CFS throttling on the fresh Wave28 runtime cgroup, without minting `TrustedReport` or `LIVE_PASS`.

**Architecture:** Reuse Wave29's package-owned launch/placement identity model but keep its `park-exit` behavior unchanged. Wave30 adds a separate internal helper protocol (`READY -> START -> BURN_DONE -> EXIT -> DONE`), a zero-argument `RuntimeCPUInterventionExecutor`, a control/intervention sequence with fresh Wave28 readbacks, per-helper `/proc/<pid>/schedstat` evidence, and a sealed `RuntimeCPUThrottleInterventionAuthority`. Wave30 v1 supports only the provable single-worker range `0 < CPUQuotaMicros < CPUPeriodMicros`; other finite CPU tuples fail honestly with `ErrRuntimeCPUInterventionUnavailable`.

**Tech Stack:** Go, Linux cgroup v2, `/proc/<pid>/stat`, `/proc/<pid>/schedstat`, SHA-256, GitHub Actions, Python static-contract tests.

**Spec:** `docs/superpowers/specs/2026-09-08-exact-cpu-throttle-intervention-authority-v30-design.md`

## Global Constraints

- Base exact SHA is Wave29 final `9c9f33ae605b902d702dfd6510785d56a64ea779`.
- Keep Wave29 public APIs and `park-exit` protocol semantics unchanged.
- Wave30 is CPU-only; no memory/OOM proof, `TrustedReport`, `CapabilityEvidenceSource`, or public `LIVE_PASS` activation.
- Production constructor is exactly `NewRuntimeCPUInterventionExecutor() (*RuntimeCPUInterventionExecutor, error)` with zero arguments.
- Production cgroup root remains fixed to `/sys/fs/cgroup`.
- Public validator accepts no executable/root/path/PID/starttime/schedstat/cpu.stat/worker/duration/pressure callback/`HostPressureRunner`/`ResourceBinding`/raw evidence.
- Wave30 v1 accepts only finite `0 < quota < period`; `quota >= period` is unavailable, not VERIFIED.
- Control window is exactly two CPU periods, bounded by package constants; intervention is exactly four CPU periods, bounded by package constants.
- Both `NrThrottled` and `ThrottledUsec` must be unchanged during control and strictly increase during intervention.
- Helper schedstat runtime must strictly increase by at least one quota quantum expressed in nanoseconds.
- Every post-child-start failure/cancellation must kill and reap the exact child.
- Production code must follow committed RED evidence.

---

### Task 1: Wave30 Helper Protocol and Command Dispatch

**Files:**
- Create: `NolaneWorld/substrate/cube/runtime_cpu_intervention_protocol_v30_test.go`
- Modify: `NolaneWorld/substrate/cube/runtime_cgroup_helper_protocol.go`
- Modify: `NolaneWorld/cmd/nolane-gauntlet-live/main.go`
- Modify: `NolaneWorld/cmd/nolane-gauntlet-live/main_test.go`

**Interfaces:**
- Produces package-private helper protocol support for mode `cpu-throttle-v30`.
- Produces child protocol records `READY`, `START`, `BURN_DONE`, `EXIT`, `DONE` using the existing nonce format and FDs 3/4.
- Preserves Wave29 `park-exit` behavior exactly.

- [ ] **Step 1: Write failing helper-protocol tests**

Add tests that require:

```go
func TestV30HelperDoesNoWorkBeforeStart(t *testing.T)
func TestV30HelperStartBurnExitProtocol(t *testing.T)
func TestV30HelperRejectsWrongNonceOrOrder(t *testing.T)
func TestV30HelperKeepsWave29ParkExitSemantics(t *testing.T)
```

Use package-private readers/writers and a deterministic burn seam in tests so the test can assert zero burn calls before `START`, exactly one bounded burn phase after `START`, `BURN_DONE` before `EXIT`, and `DONE` before exit 0.

- [ ] **Step 2: Run focused test and verify RED**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'TestV30Helper|TestInternalCgroupHelper' -count=1
```

Expected: compile/test failure because the Wave30 mode/protocol production symbols do not exist. Any syntax/fixture failure must be fixed before accepting RED.

- [ ] **Step 3: Add minimal Wave30 helper protocol production code**

Extend the command-facing dispatcher without changing Wave29 mode semantics. Introduce package-private constants/functionality equivalent to:

```go
const runtimeCPUInterventionMode = "cpu-throttle-v30"

func runInternalCPUInterventionHelper(
    getenv func(string) string,
    release io.Reader,
    ack io.Writer,
    burn func(time.Duration) error,
) int
```

The production burn callback must be package-owned and single-worker; no public duration/worker parameter exists.

- [ ] **Step 4: Verify helper protocol GREEN and Wave29 regression**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'TestV30Helper|TestInternalCgroupHelper|TestRuntimeCgroupHelper' -count=1
go test ./cmd/nolane-gauntlet-live -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add NolaneWorld/substrate/cube/runtime_cgroup_helper_protocol.go \
        NolaneWorld/substrate/cube/runtime_cpu_intervention_protocol_v30_test.go \
        NolaneWorld/cmd/nolane-gauntlet-live/main.go \
        NolaneWorld/cmd/nolane-gauntlet-live/main_test.go
git commit -m "feat(wave30): add CPU intervention helper protocol"
```

---

### Task 2: RED Behavioral Contract for Exact CPU Intervention Authority

**Files:**
- Create: `NolaneWorld/substrate/cube/runtime_cpu_intervention_v30_test.go`
- Create: `NolaneWorld/substrate/cube/runtime_cpu_intervention_v30_review_test.go`

**Interfaces:**
- Requires future `RuntimeCPUInterventionExecutor`.
- Requires future `RuntimeCPUThrottleInterventionAuthority`.
- Requires future `ValidateRuntimeCPUThrottleInterventionAuthority(...)`.
- Requires `ErrInvalidRuntimeCPUThrottleInterventionAuthority` and `ErrRuntimeCPUInterventionUnavailable`.

- [ ] **Step 1: Write GREEN-path wished-for API tests**

Tests must require a package-private test executor that can deterministically simulate child lifecycle, Wave28 readbacks, schedstat, clock, and wait phases while preserving the zero-arg public constructor.

Minimum tests:

```go
func TestRuntimeCPUThrottleInterventionAuthorityValidPath(t *testing.T)
func TestRuntimeCPUThrottleInterventionAuthorityDeterministicDigest(t *testing.T)
func TestRuntimeCPUThrottleInterventionAuthorityZeroInvalid(t *testing.T)
func TestRuntimeCPUThrottleInterventionAuthorityJSONRoundTripCannotRestore(t *testing.T)
```

The valid fixture must have:

```text
quota=25000us
period=100000us
control-before == control-after throttle counters
pressure-after nr_throttled > control-after
pressure-after throttled_usec > control-after
schedstat_after - schedstat_before >= quota*1000ns
exit=0
```

- [ ] **Step 2: Write failure and unavailable tests before production**

Require fail-closed coverage for:

- `quota >= period` -> `ErrRuntimeCPUInterventionUnavailable`;
- throttle delta during control;
- no `nr_throttled` increase during pressure;
- no `throttled_usec` increase during pressure;
- schedstat missing/malformed/rollback/unchanged/insufficient delta;
- PID/starttime/executable drift;
- helper leaves cgroup;
- runtime/cgroup/limit drift;
- counter rollback;
- protocol wrong order/nonce/missing `BURN_DONE`/`DONE`;
- non-zero exit;
- cancellation in each phase kills and reaps.

- [ ] **Step 3: Verify behavioral RED**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'RuntimeCPUThrottleIntervention|V30CPUIntervention' -count=1
```

Expected: fail specifically because Wave30 executor/authority/validator/error production symbols are absent. Do not count fixture/compile mistakes as RED lineage.

- [ ] **Step 4: Commit RED tests**

```bash
git add NolaneWorld/substrate/cube/runtime_cpu_intervention_v30_test.go \
        NolaneWorld/substrate/cube/runtime_cpu_intervention_v30_review_test.go
git commit -m "test(wave30): define exact CPU intervention authority contract"
```

---

### Task 3: Minimal Wave30 Executor and Authority GREEN

**Files:**
- Create: `NolaneWorld/substrate/cube/runtime_cpu_intervention.go`
- Modify if required only for private reuse: `NolaneWorld/substrate/cube/runtime_cgroup_helper_placement.go`

**Interfaces:**
- Produces:

```go
func NewRuntimeCPUInterventionExecutor() (*RuntimeCPUInterventionExecutor, error)

func ValidateRuntimeCPUThrottleInterventionAuthority(
    ctx context.Context,
    controller *realm.Controller,
    realization realm.RealizationAuthority,
    runtimeAuthority RealmResourceRuntimeAuthority,
    epochObserver *RealizationEpochObserver,
    client *Client,
    runtimeObserver *RuntimeRealizationObserver,
    cgroupObserver *RuntimeCgroupReadbackObserver,
    executor *RuntimeCPUInterventionExecutor,
) (RuntimeCPUThrottleInterventionAuthority, error)
```

- Produces opaque `RuntimeCPUThrottleInterventionAuthority`, `Snapshot()`, `Digest()`, `Valid()`.
- Produces canonical prefix `runtime-cpu-throttle-intervention-v30:` and domain `nolane.runtime-cpu-throttle-intervention.v30\x00`.

- [ ] **Step 1: Implement exact schedstat parser and single-worker availability gate**

Package-private parser contract:

```go
func parseRuntimeCPUSchedstat(raw []byte) (uint64, error)
```

Require exactly three canonical unsigned decimal fields and return field 1 CPU runtime ns. Reject zero/malformed/overflow/extra/missing fields.

Availability rule:

```go
if quota <= 0 || period == 0 || uint64(quota) >= period {
    return ErrRuntimeCPUInterventionUnavailable
}
```

- [ ] **Step 2: Implement production executor with package-owned timings**

Derive:

```text
control = 2 * CPUPeriodMicros
burn    = 4 * CPUPeriodMicros
minimum helper CPU runtime delta = CPUQuotaMicros * 1000 ns
```

Clamp only to fixed package maxima defined in the spec; if the tuple exceeds the provable bounded range, return unavailable rather than shortening below required periods.

Reuse the Wave29 executable identity/PID/starttime/cgroup placement principles through private helpers or equivalent code; do not widen Wave29 public API.

- [ ] **Step 3: Implement control/intervention/final readback sequence**

Order is strict:

```text
fresh placement basis
READY
exact placement + identity
control-before Wave28
schedstat-before
wait 2 periods
control-after Wave28 (throttle delta == 0)
START
BURN_DONE
identity + membership recheck
schedstat-after (delta >= quota quantum)
pressure-after Wave28 (both throttle counters strictly increase)
EXIT
DONE
wait/reap exit 0
final Wave28
mint sealed authority
```

All readbacks must have exact immutable binding equality. OOM counters may stay equal or increase but may never roll back; no memory proof is minted.

- [ ] **Step 4: Verify focused GREEN**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'RuntimeCPUThrottleIntervention|V30CPUIntervention|V30Helper' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit production GREEN**

```bash
git add NolaneWorld/substrate/cube/runtime_cpu_intervention.go \
        NolaneWorld/substrate/cube/runtime_cgroup_helper_placement.go
git commit -m "feat(wave30): add exact CPU throttle intervention authority"
```

---

### Task 4: Static Anti-Shortcut Contract and Dedicated CI

**Files:**
- Create: `tests/wave30_exact_cpu_throttle_intervention_contract.py`
- Create: `.github/workflows/wave30-exact-cpu-throttle-intervention-contract.yml`
- Create: `docs/superpowers/verification/2026-09-08-wave30-exact-cpu-throttle-intervention-red.md`

**Interfaces:**
- Static contract locks public signatures and required production seams.
- Dedicated workflow provides committed RED/GREEN lineage and exact-head verification.

- [ ] **Step 1: Write static contract first**

Require source contains:

```text
cpu-throttle-v30
START
BURN_DONE
EXIT
/proc/
schedstat
/sys/fs/cgroup
cgroup.procs
ValidateRuntimeCgroupReadbackAuthority
runtime-cpu-throttle-intervention-v30:
nolane.runtime-cpu-throttle-intervention.v30\x00
```

Reject public signatures exposing strings/paths/PIDs/raw stat/callbacks/worker count/duration/`HostPressureRunner`/`ResourceBinding`/`TrustedReport`/`LIVE_PASS`.

- [ ] **Step 2: Add dedicated workflow**

Workflow order:

```text
setup -> focused Wave30 -> command integration -> static contract -> Wave29 regression -> Wave28 regression -> Wave27 regression -> go vet ./... -> go test ./... -> focused race
```

Path triggers must cover every Wave30 implementation/test/spec/plan/verification/static/workflow/command-dispatch file.

- [ ] **Step 3: Record valid RED lineage**

The RED record must include exact pre-production test SHA/run ID and exact missing production seams. Do not record failed harness/setup runs as RED.

- [ ] **Step 4: Run exact code-head dedicated gate**

Expected all steps SUCCESS on one unchanged SHA.

- [ ] **Step 5: Commit static/workflow/verification files**

```bash
git add tests/wave30_exact_cpu_throttle_intervention_contract.py \
        .github/workflows/wave30-exact-cpu-throttle-intervention-contract.yml \
        docs/superpowers/verification/2026-09-08-wave30-exact-cpu-throttle-intervention-red.md
git commit -m "ci(wave30): lock exact CPU intervention contract"
```

---

### Task 5: Whole-PR Trust Review, Stacked PR, and Exact-Final Closure

**Files:**
- Create: `docs/superpowers/verification/2026-09-08-wave30-exact-cpu-throttle-intervention-closure.md`
- Modify tests/production only if review finds a real gap, always with review-driven RED first.

**Interfaces:**
- Produces draft PR stacked on `gpt/wave29-runtime-cgroup-helper-placement-authority`.
- Produces exact-final SHA evidence; no merge.

- [ ] **Step 1: Whole-diff trust review before PR**

Audit specifically:

- control-window attribution assumptions;
- schedstat parser/task-vs-thread semantics;
- process replacement and executable TOCTOU;
- cgroup membership before/after intervention;
- cancellation after `BURN_DONE` and after `DONE`;
- child reap on every error path;
- background throttle ambiguity;
- unavailable vs invalid distinction;
- no `TrustedReport`/`LIVE_PASS` laundering.

If a gap is found, commit a review-driven RED test before fixing production.

- [ ] **Step 2: Verify code head and scope**

Require merge base exactly Wave29 final, `behind_by=0`, and only Wave30 scoped files changed.

- [ ] **Step 3: Open draft stacked PR**

Base branch:

```text
gpt/wave29-runtime-cgroup-helper-placement-authority
```

Keep PR draft/open/unmerged. Do not merge without explicit user authorization.

- [ ] **Step 4: Wait for all applicable exact code-head PR workflows**

Require all code/integration gates SUCCESS. Repository-level review automation failures caused solely by missing credentials must be separately classified from code gates and documented accurately.

- [ ] **Step 5: Commit closure record**

Closure must record:

- exact RED heads/runs;
- review-driven hardening, if any;
- exact code head and dedicated gate;
- all applicable PR integration run IDs;
- ancestry/scope audit;
- explicit non-claims.

- [ ] **Step 6: Re-run exact-final verification after closure commit**

The closure commit creates a new final candidate SHA. Freshly re-run dedicated and PR-triggered gates on that exact SHA; do not reuse code-head GREEN evidence.

- [ ] **Step 7: Update PR body metadata only**

Update exact-final evidence without changing head SHA. Re-fetch PR metadata and verify it remains draft/open/unmerged and mergeable absent unrelated repository automation status.
