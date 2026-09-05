# Nolane Sandbox Wave 19 — Host Process Identity Implementation Plan

## Goal

Implement the Wave 19 trust boundary defined in `docs/superpowers/specs/2026-09-05-nolane-sandbox-host-process-identity-v19-design.md`: exact PID-reuse-resistant identity for the CubeShim/VMM host process after trusted cgroup placement, controller-generation binding, exact Prometheus transport, and strict NolaneWorld correlation. Wave 19 must not classify an OOM victim.

## Execution Rules

- Use TDD: focused contract tests must fail before production implementation is added.
- Keep the CubeBox → sandbox-controller dependency package-neutral through structural interfaces using only standard-library/primitives.
- `cgroup.AddProc` success is the only authority that may initiate a new placement proof.
- `/proc` evidence failure is observational and must never turn successful sandbox execution into failure.
- Wave 16 task-outcome generation remains the sole realization-generation authority.
- Never infer OOM victim identity from exit 137, SIGKILL, Wave 18 OOM delta, or PID equality.
- Every authored commit/PR description includes `Autonomously-by: ChatGPT:GPT-5.6-Sol` and never a DCO `Signed-off-by` trailer.

## Task 1 — RED host-process inspector contract

**Create:**
- `Cubelet/plugins/cube/internals/sandbox/host_process_identity_v19_test.go`

**Later implementation:**
- `Cubelet/plugins/cube/internals/sandbox/host_process_identity.go`

Contract tests cover:

1. `/proc/<pid>/stat` parser finds the closing parenthesized command field and parses Linux field 22 exactly, including command names containing spaces and parentheses.
2. PID zero, malformed stat lines, signed/non-canonical start times, zero start time, and stat PID mismatch fail closed.
3. stat-A → cgroup → stat-B sandwich rejects PID/starttime changes.
4. canonical boot UUID validation rejects blank, uppercase/non-canonical, malformed values.
5. cgroup v2 `0::/path` and v1 controller entries are parsed structurally.
6. exact expected hierarchy path is required; suffix/substring/basename/path-traversal matches are rejected.
7. capture produces `(boot_id,pid,starttime_ticks,cgroup_path,placed_at,observed_at)` only when every check succeeds.

RED command:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox -run 'HostProcess|ProcessIdentity' -count=1
```

## Task 2 — RED lifecycle and trusted placement contract

**Create:**
- `Cubelet/plugins/cube/internals/sandbox/host_process_identity_lifecycle_v19_test.go`
- `Cubelet/services/cubebox/host_process_identity_v19_test.go`

**Modify later:**
- `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go`
- `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- `Cubelet/services/cubebox/local.go`
- `Cubelet/services/cubebox/cube_container_create.go`

Required behavior:

- Extend the existing controller-local proof store so host placement/binding transitions share the same lock and generation authority as exact task outcome.
- `Clear(sandboxID)` fences and removes placement/binding from the previous sandbox lifetime.
- `BeginRealization` clears prior realization binding and opens the new controller generation.
- A placement candidate captures a generation/lifetime token before slow `/proc` I/O and commits only if the token is still current.
- Successful first placement after Start binds the current open generation.
- Placement before Start may retain lifetime placement but is freshly revalidated before a later Start binds it.
- A newer Start cannot accept stale capture work from the previous token.
- Once exact task outcome is accepted, late placement cannot bind or repair that generation.
- Later Start revalidates the exact stored `(boot_id,pid,starttime,cgroup)` identity before binding.
- `math.MaxUint64` generation survives binding exactly.
- CubeBox resolves the cube sandbox controller through its existing plugin dependency and asserts a local primitive/std-library recorder interface.
- `setCgroup` calls the recorder only after `cgroupp.AddProc(...) == nil` and passes sandbox identity, exact cgroup ID/path, PID, and placement timestamp.
- `AddProc` failure never records evidence; identity-recorder failure is logged/best-effort and does not fail sandbox execution.

Focused RED commands:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox -run 'HostProcess|ProcessIdentity' -count=1
go test ./services/cubebox -run 'HostProcess|ProcessIdentity' -count=1
```

## Task 3 — Exact resource-metrics transport

**Create:**
- `Cubelet/plugins/cube/internals/resourcemetrics/host_process_identity_prometheus.go`
- `Cubelet/plugins/cube/internals/resourcemetrics/host_process_identity_v19_test.go`

**Modify:**
- `Cubelet/plugins/cube/internals/resourcemetrics/realization_oom_prometheus.go`
- `Cubelet/plugins/cube/internals/resourcemetrics/plugin.go`

Transport one atomic sample:

```text
cubesandbox_host_process_identity_info{
  sandbox_id,
  generation,
  host_pid,
  starttime_ticks,
  boot_id,
  cgroup_path,
  runtime_role="cube-shim-vmm",
  source="cubebox.cgroup.add_proc",
  placed_at,
  bound_at
} 1
```

Rules:

- generation/PID/starttime are exact decimal string labels;
- timestamps are UTC RFC3339Nano;
- invalid or incomplete controller evidence emits no sample;
- `math.MaxUint64` generation/starttime transport exactly;
- resource metrics consumes a structural visitor and gains no authority to construct identity proofs;
- existing `NewService` public API remains unchanged.

Focused command:

```bash
cd Cubelet
go test ./plugins/cube/internals/resourcemetrics -run 'HostProcess|ProcessIdentity|TaskOutcome|RealizationOOM' -count=1
```

## Task 4 — Strict NolaneWorld single-scrape correlation

**Create:**
- `NolaneWorld/substrate/cube/host_process_identity.go`
- `NolaneWorld/substrate/cube/host_process_identity_v19_test.go`

**Modify:**
- `NolaneWorld/substrate/cube/task_termination.go`
- `NolaneWorld/substrate/cube/task_termination_v18_test.go` only if existing fixtures need the new optional sample (absence remains valid).

Produce `HostSandboxProcessIdentityProof` as provenance only. Extend single-scrape task termination evidence with an optional host-process identity proof.

Strict parsing/correlation requires:

- exactly ten labels and numeric metric value exactly one;
- exact supported runtime role/source constants;
- canonical uint generation/PID/starttime without sign or leading zeros;
- canonical lowercase boot UUID;
- canonical cgroup path;
- canonical UTC RFC3339Nano timestamps with `placed_at <= bound_at`;
- target sandbox and exact outcome generation match;
- if Wave 18 OOM proof exists, exact cgroup path match;
- duplicate/malformed target samples fail closed;
- other sandbox samples are ignored only after safe sandbox-label parsing;
- identity absence does not invalidate an otherwise exact Wave 17/18 task termination observation.

Negative tests explicitly prove that exit 137, SIGKILL-like outcomes, positive Wave 18 OOM delta, or PID equality never creates an OOM-victim classification/API.

Focused command:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'HostProcess|ProcessIdentity|TaskTermination|TaskOutcome' -count=1
```

## Task 5 — Dedicated Wave 19 CI contract

**Create:**
- `.github/workflows/cube-host-process-identity-contract.yml`

Path filters include:

- sandbox proof/controller files;
- CubeBox create path;
- resource-metrics transport;
- Cubelet module files;
- NolaneWorld cube substrate/module files;
- Wave 19 spec/plan;
- the workflow itself.

Run focused Cubelet sandbox, CubeBox, resource-metrics and NolaneWorld tests. Use the Go versions already declared by each module.

## Task 6 — GREEN implementation and regression verification

After RED is captured from GitHub Actions, implement the smallest trust-complete production path satisfying Tasks 1–4. Fix root causes only; never weaken tests to make them green.

Focused GREEN verification:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics ./services/cubebox -run 'HostProcess|ProcessIdentity|TaskOutcome|RealizationOOM' -count=1

cd ../NolaneWorld
go test ./substrate/cube -run 'HostProcess|ProcessIdentity|TaskTermination|TaskOutcome' -count=1
```

Then require final-head compatibility gates:

- Wave 17 exact task outcome contract;
- Wave 18 realization OOM contract;
- host-resource contract;
- broad Cubelet unit matrix;
- repository build matrix;
- format check;
- DCO policy check;
- NolaneWorld unit/race/vet/evidence gates;
- live-harness semantics where available.

Any infrastructure-only bot failure must be reported separately from code/test failure.

## Task 7 — Trust audit and PR closure

Before marking ready:

- diff audit for any `exit 137 => OOM`, SIGKILL=>OOM, status fallback, fuzzy cgroup match, host-PID-as-guest identity, persistence/reconstruction, or public CubeAPI expansion;
- verify CubeBox calls evidence recorder only after successful `AddProc`;
- verify slow `/proc` I/O is outside proof-store lock and commit rechecks lifecycle token;
- verify resource metrics is transport-only;
- verify NolaneWorld treats PID/path as provenance, never executable authority;
- verify all final-head required Actions are green except clearly external infrastructure failures.

Wave 20 remains separate: authoritative kernel victim-event capture and event-time correlation. Wave 19 alone must never claim an OOM victim.
