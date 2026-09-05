# Nolane Sandbox Wave 19 — Cgroup-Bound Host Sandbox Process Identity

## Purpose

Wave 19 establishes an exact host-process identity proof for the host process that CubeSandbox places into a sandbox cgroup, and binds that identity to the controller-local task realization generation used by Waves 16–18.

Wave 19 exists because Wave 18 can prove that the kernel cgroup `oom_kill` counter increased during one realization window, but it still cannot prove which process the kernel selected as an OOM victim. A victim-level kernel event can safely be correlated in a later wave only after the sandbox has a PID-reuse-resistant identity for the relevant host process.

Wave 19 therefore proves process identity and cgroup membership. It does **not** prove an OOM victim, an OOM cause, a guest-workload process identity, or an `OOMKilled` classification.

## Existing Trust Boundaries

### Waves 16–17 — exact task outcome

The sandbox controller owns a monotonically advancing realization generation for each sandbox identity. Exact task outcome is accepted only from authoritative containerd task `Wait` or stopped `State` responses and is transported into NolaneWorld without converting arbitrary `uint64` values through binary64.

### Wave 18 — realization-scoped kernel OOM evidence

Wave 18 captures a cgroup OOM counter baseline for a controller-local generation and performs one terminal final snapshot attempt after exact task outcome is accepted. It can assert that one or more kernel OOM kills occurred in the cgroup during the measured realization window, but it deliberately cannot identify the victim process.

### Runtime PID semantics

CubeShim reports `SandBox::pid()` as the task PID in its Create, Start and State responses. `SandBox::new` initializes that value from the CubeShim process's `std::process::id()`. The Cube hypervisor/VMM is hosted by that CubeShim runtime process rather than being represented by a distinct containerd task PID in this contract.

CubeBox later places `sandBox.Endpoint.Pid` into `sandBox.CGroupPath` through `cgroup.AddProc`.

The normal CubeBox path performs that placement asynchronously after runtime task startup. Therefore a one-shot `/proc/<pid>/cgroup` check inside sandbox-controller `Start` is not authoritative: it can race a correct later `AddProc` and produce a false negative.

This timing fact determines Wave 19's architecture.

## Wave 19 Trust Statement

Wave 19 may assert only:

> Host process `(boot_id, pid, starttime_ticks)` was observed as the process identified by CubeSandbox runtime endpoint PID, was successfully placed through the trusted CubeBox `AddProc` path into exact sandbox cgroup path C, remained the same Linux process during the placement verification read, and was bound to controller-local realization generation G while G was still current and open.

Wave 19 must never infer:

- that the PID identifies a guest application process;
- that the host process was selected by the kernel OOM killer;
- that the main guest task was OOM-killed;
- that exit code `137` means OOM;
- that SIGKILL means OOM;
- that a positive Wave 18 OOM delta identifies any particular victim;
- that PID equality alone is process identity;
- that a cgroup path substring or suffix match proves membership;
- that a placement event arriving after a realization is closed can be retroactively bound to that realization;
- that state reconstructed after controller restart is equivalent to evidence captured during the original realization.

## Why PID Alone Is Not Sufficient

Linux PIDs can be reused. A stale record containing only `pid=1234` can accidentally refer to an unrelated process after the original task exits.

Wave 19 therefore defines host process identity as the tuple:

```text
boot_id + host_pid + starttime_ticks
```

where:

- `boot_id` is the current kernel boot identifier read from `/proc/sys/kernel/random/boot_id`;
- `host_pid` is the CubeShim/CubeSandbox runtime endpoint PID;
- `starttime_ticks` is Linux `/proc/<pid>/stat` field 22, measured from boot and parsed as an exact unsigned integer.

The tuple prevents ordinary PID reuse within one boot and prevents an identity from silently surviving a host reboot.

## Architecture

Wave 19 separates two different facts that have different lifetimes:

1. **HostProcessPlacementProof** — lifetime/placement scoped. It proves that one exact Linux process was successfully placed into one exact sandbox cgroup.
2. **HostProcessRealizationBinding** — realization scoped. It proves that an exact placement identity was still valid and was associated with controller generation G while that generation was current and open.

The authority graph is:

```text
CubeShim task API
  └─ endpoint PID = CubeShim host PID
           │
           ▼
CubeBox sandbox create path
  └─ cgroup.AddProc(CGroupPath, Endpoint.Pid)
           │
           ├─ failure -> no identity evidence
           │
           └─ success
                │
                ▼
trusted host-process inspector
  ├─ /proc/sys/kernel/random/boot_id
  ├─ /proc/<pid>/stat          (A)
  ├─ /proc/<pid>/cgroup
  └─ /proc/<pid>/stat          (B)
           │
           ├─ A/B identity mismatch -> unknown
           ├─ exact cgroup mismatch -> unknown
           └─ stable identity
                │
                ▼
HostProcessPlacementProof
                │
                ├───────────────┐
                │               │
                ▼               ▼
controller generation G    later Start generation G+1
(current + open)            revalidate same placement identity
                │               │
                └───────┬───────┘
                        ▼
HostProcessRealizationBinding
                        │
                        ▼
resource-metrics atomic info sample
                        │
                        ▼
NolaneWorld strict ResourceBinding-scoped observer
```

The first-realization race is closed by allowing the successful CubeBox placement event to bind the current generation after `AddProc` succeeds. Later realizations may reuse an existing placement proof only after fresh process/cgroup revalidation.

## Trusted Internal Boundary

Wave 19 introduces a small package-neutral internal host-identity component shared by the sandbox controller and CubeBox placement path. It must not import either package back, so the dependency graph remains acyclic.

Conceptually the component owns:

```go
type HostProcessPlacementProof struct {
    SandboxID     string
    CGroupPath    string
    BootID        string
    HostPID       uint32
    StartTimeTicks uint64
    PlacedAt      time.Time
    ObservedAt    time.Time
    Source        HostProcessPlacementSource
}

type HostProcessRealizationBinding struct {
    SandboxID      string
    Generation     uint64
    CGroupPath     string
    BootID         string
    HostPID        uint32
    StartTimeTicks uint64
    PlacedAt       time.Time
    BoundAt        time.Time
    Source         HostProcessPlacementSource
}
```

The exact concrete API may be smaller than these illustrative Go declarations, but the trust fields and lifecycle semantics are mandatory.

The source value is fixed to the trusted placement path, for example:

```text
cubebox.cgroup.add_proc
```

No caller-supplied arbitrary source string is accepted.

## Placement Authority

The only event that can create a new placement proof is the trusted CubeBox path after:

```text
cgroup.AddProc(exact_cgroup_path, endpoint_pid) == nil
```

Rules:

- blank sandbox identity is rejected for evidence purposes;
- blank or non-canonical cgroup path is rejected for evidence purposes;
- PID zero is rejected;
- `AddProc` failure produces no placement proof;
- process-inspection failure after successful `AddProc` produces no placement proof;
- evidence failure must not turn a successful sandbox execution into an execution failure;
- a placement proof is not accepted before `AddProc` returns success;
- no metric-cache, Wave 15 baseline, task `Status` fallback, or guessed cgroup membership may synthesize a placement proof.

The CubeBox call site already has sandbox identity, `CGroupPath`, and `Endpoint.Pid`; Wave 19 must pass all three into the evidence recorder after successful placement rather than weakening identity to only `(cgroupPath, pid)`.

## Host Process Inspector

The inspector runs on the trusted host and treats `/proc` as kernel-backed evidence.

### Capture sequence

After successful `AddProc`, capture:

1. canonical host boot ID;
2. `/proc/<pid>/stat` snapshot A;
3. `/proc/<pid>/cgroup`;
4. `/proc/<pid>/stat` snapshot B;
5. host UTC `observed_at` immediately after all validation succeeds.

The implementation may read boot ID between the two stat reads as long as both stat reads enclose the cgroup-membership observation. The essential invariant is that the cgroup observation is sandwiched between two matching process-identity observations.

### `/proc/<pid>/stat`

The parser must handle the parenthesized `comm` field correctly rather than splitting the whole line on spaces. It must locate the closing delimiter for the parenthesized command field and parse Linux field 22 (`starttime`) as canonical base-10 `uint64`.

Capture succeeds only when:

- both stat reads are syntactically valid;
- both report the requested PID;
- both report the same non-zero `starttime_ticks`;
- the process remains observable for the complete sandwich.

Any mismatch means unknown evidence, not a retry-based repair of the same placement event.

### Boot ID

`/proc/sys/kernel/random/boot_id` must parse as one canonical UUID-form identifier. Leading/trailing whitespace from the proc file may be removed once; the normalized value is stored. Empty, malformed, or non-canonical values reject the proof.

### Cgroup membership

`/proc/<pid>/cgroup` must be parsed structurally.

Wave 19 requires an exact hierarchy-path match to the expected CubeBox `CGroupPath`. It must never use substring, suffix, basename, or fuzzy matching.

The inspector must handle both supported Linux layouts:

- cgroup v2 entries such as `0::/exact/path`;
- cgroup v1 entries with controller lists and hierarchy IDs.

The expected path is canonicalized conservatively to one absolute cgroup hierarchy path. Parent traversal, empty components, ambiguous hierarchy matches, or an inability to identify an exact matching hierarchy result in unknown evidence.

A process appearing in another cgroup is not accepted even when the PID and start time are otherwise valid.

## Placement Proof Replacement

A later successful trusted placement for the same sandbox may replace an older placement proof only after a complete fresh capture succeeds.

Replacement rules:

- same `(boot_id, pid, starttime_ticks, cgroup_path)` may refresh the trusted placement observation;
- a different process identity replaces the old lifetime-scoped placement proof only as a new trusted placement event;
- replacing placement proof immediately invalidates any still-unclosed realization binding that refers to a different identity;
- a failed replacement attempt does not mutate the previously captured historical placement proof, but it cannot be used to bind a new generation without fresh revalidation.

## Controller Generation Binding

Wave 16 remains the sole generation authority. The Wave 19 component never increments or invents a generation.

### Begin realization

When sandbox-controller `Start(sandbox_id)` advances to generation G:

1. clear the previous generation's host-process binding;
2. mark G as the current open generation for Wave 19;
3. if a placement proof already exists, perform a fresh host-process continuity check against that exact identity and cgroup;
4. if the continuity check succeeds, bind it to G;
5. if no placement exists yet, leave G open and unbound so the later trusted `AddProc` placement event can bind it.

Failure to bind must not fail sandbox Start.

### First placement after Start

When a successful trusted placement event produces a placement proof:

- inspect the controller-supplied current generation state;
- bind only if that generation is still current and open;
- never bind to a generation number supplied by CubeBox;
- never bind to an older generation;
- never bind after exact task outcome has closed the current generation;
- if no generation is open, retain at most the lifetime-scoped placement proof and emit no realization binding.

This closes the normal asynchronous placement race without allowing a late placement observation to rewrite historical task evidence.

### Close realization

As soon as an exact Wave 16 task outcome for generation G is accepted, Wave 19 marks G closed before any later asynchronous placement callback can create a new binding.

An already accepted binding remains immutable historical evidence for G. A missing binding remains unknown and cannot be repaired after closure.

### New Start

A newer `Start` generation invalidates the previous current binding before revalidation. A stale placement callback cannot bind to the older generation and cannot poison the newer one.

### Create fence

`Create(sandbox_id)` clears:

- current Wave 19 generation state;
- current realization binding;
- placement proof for the prior sandbox lifetime.

This mirrors the existing task-outcome recovery fence: a recreated sandbox identity does not inherit host process identity from the previous lifetime.

### Controller restart

Wave 19 state is not persisted.

A controller process restart loses current generation binding state. Wave 19 must not reconstruct a historical binding solely from a surviving PID, cgroup path, Wave 18 metric, or stored assignment metadata.

A later authoritative runtime lifecycle may establish new evidence, but absence of original realization-time binding remains unknown.

## Concurrency

Placement events are asynchronous relative to sandbox controller lifecycle. The Wave 19 state machine must therefore be synchronized around:

- current generation;
- open/closed state;
- placement proof identity;
- realization binding.

The transition that checks `generation == current && open == true` and stores a binding must be atomic under the Wave 19 state lock.

Slow `/proc` I/O must not hold the state lock. Capture and revalidation happen outside the lock; the final commit step re-checks the generation/lifetime token before accepting evidence. If state changed while I/O was in progress, the candidate is discarded.

This prevents stale placement or revalidation work from crossing a Create fence or newer Start.

## Exact Binding Proof

An accepted `HostProcessRealizationBinding` requires all of the following:

- non-empty sandbox identity;
- non-zero controller generation;
- non-zero host PID;
- non-zero start time ticks;
- canonical boot ID;
- canonical non-empty cgroup path;
- trusted placement source;
- a successful `AddProc` placement proof for the same sandbox/process/cgroup lifetime;
- fresh or placement-time `/proc` continuity validation as defined above;
- current controller generation still equal to the candidate generation at commit time;
- generation still open at commit time;
- no Create fence or newer Start superseded the candidate;
- `placed_at` and `bound_at` are non-zero UTC timestamps with `placed_at <= bound_at` for first-placement binding.

For a later-generation revalidation, `bound_at` may be later than the original `placed_at`; the original placement timestamp remains provenance for the host-process lifetime.

## Transport

Resource metrics exports one atomic info-style sample per current accepted host-process realization binding:

```text
cubesandbox_host_process_identity_info{
  sandbox_id="sandbox-a",
  generation="18446744073709551615",
  host_pid="12345",
  starttime_ticks="987654321",
  boot_id="11111111-2222-3333-4444-555555555555",
  cgroup_path="/cube_sandbox_v1/42",
  runtime_role="cube-shim-vmm",
  source="cubebox.cgroup.add_proc",
  placed_at="2026-09-05T06:00:00.123456789Z",
  bound_at="2026-09-05T06:00:00.223456789Z"
} 1
```

Rules:

- metric value is exactly numeric one;
- every integer is emitted as a decimal string label, including generation, PID and start time ticks;
- timestamps are canonical UTC RFC3339Nano strings;
- `runtime_role` is a fixed normalized contract value and must not imply guest application identity;
- no sample is emitted for unknown/incomplete evidence;
- duplicate current bindings for one `(sandbox_id, generation)` are forbidden;
- resource-metrics does not gain authority to create, repair or reinterpret identity evidence.

The production package graph must remain acyclic. Resource metrics consumes bindings through a primitive/standard-library structural visitor or an equally package-neutral interface. It must not import the sandbox package merely to access proof structs.

## NolaneWorld Consumer

Wave 19 adds strict parsing for host-process realization identity while preserving the opaque `ResourceBinding` scope used by Waves 17–18.

A public evidence shape is conceptually:

```go
type HostSandboxProcessIdentityProof struct {
    SandboxID      string
    Generation     uint64
    HostPID        uint32
    StartTimeTicks uint64
    BootID         string
    CGroupPath     string
    RuntimeRole    string
    Source         string
    PlacedAt       time.Time
    BoundAt        time.Time
}
```

NolaneWorld treats these fields as provenance only. In particular:

- it never opens `/proc/<pid>` using a transported PID;
- it never treats `CGroupPath` as a filesystem authority;
- it never converts the identity into an OOM cause by itself;
- it never calls the process a guest application process.

## Cross-Proof Correlation

When task outcome, Wave 18 realization OOM proof, and Wave 19 identity proof appear in one evidence scrape for the target sandbox:

- sandbox identity must match;
- generation must match exact task outcome generation;
- when Wave 18 OOM proof is present, Wave 19 cgroup path must exactly match the Wave 18 cgroup path;
- an identity proof for another generation is not silently ignored when it claims the target sandbox as the current target proof; it is a correlation failure;
- source/runtime-role values must be exact supported constants.

The evidence model becomes:

```text
Task outcome              exact / unknown
Realization kernel OOM    exact / unknown
Host process identity     exact / unknown
```

These three facts still do not prove victim causality.

## NolaneWorld Fail-Closed Parsing

For the target `ResourceBinding` sandbox, reject the Wave 19 proof when any of the following holds:

- duplicate target identity samples;
- missing or extra labels;
- metric value is not finite numeric one;
- sandbox identity is blank or non-canonical for the binding;
- generation is zero, non-canonical decimal, or mismatches exact task outcome;
- PID is zero, signed, overflowing `uint32`, or non-canonical decimal;
- start time ticks is zero, signed, overflowing `uint64`, or non-canonical decimal;
- boot ID is malformed or non-canonical;
- cgroup path is blank, whitespace-normalized, contains parent traversal, or otherwise non-canonical;
- runtime role is unsupported;
- source is unsupported;
- timestamps are non-canonical UTC RFC3339Nano or zero;
- `placed_at > bound_at`;
- Wave 18 cgroup path is present and differs from Wave 19 cgroup path.

Metrics belonging to another sandbox are ignored after their sandbox identity can be parsed safely.

An exact task outcome may remain usable when host process identity is absent. Missing Wave 19 proof means unknown identity evidence, not failure of the independent Wave 17 outcome proof.

## Failure Semantics

Wave 19 is observational and must not make sandbox execution unavailable.

- `AddProc` itself keeps its existing execution semantics;
- identity capture failure after successful `AddProc`: sandbox continues; Wave 19 evidence unknown;
- `/proc` process disappears during sandwich: unknown;
- PID/starttime mismatch: unknown;
- cgroup membership mismatch: unknown;
- malformed boot ID: unknown;
- placement arrives after generation closure: no realization binding;
- placement callback unavailable: sandbox continues; no Wave 19 proof;
- revalidation failure on later Start: Start continues; identity unknown for that generation;
- malformed transported target proof: NolaneWorld fails closed for Wave 19 correlation.

No fallback changes unknown into zero, false, or guessed identity.

## Security Properties

### PID reuse resistance

The identity tuple includes host boot ID and Linux start time ticks. A numeric PID alone is never accepted as identity.

### Placement ordering

Evidence begins only after trusted cgroup placement succeeds. A pre-placement observation cannot masquerade as cgroup-bound identity.

### Generation non-forgery

CubeBox supplies no generation. Only the sandbox controller's existing Wave 16 generation state can establish realization binding.

### Late-event resistance

Closing a generation on exact task outcome prevents asynchronous placement completion from retroactively manufacturing evidence for an exited realization.

### Lifecycle fencing

Create/new-Start transitions invalidate stale realization state before any pending candidate can commit.

### Package-boundary containment

The shared internal host-identity component contains only evidence capture/state mechanics. It does not import higher-level sandbox or resource-metrics packages and does not gain execution authority.

## Explicit Non-Goals

Wave 19 does not:

- collect kernel OOM victim events;
- add eBPF, perf, audit, or tracefs event ingestion;
- classify any process as OOM-killed;
- claim guest application process identity;
- infer OOM from exit code 137 or SIGKILL;
- persist identity proof across controller restart;
- expose `/proc`, cgroup filesystem paths, or host PIDs as executable authority in NolaneWorld;
- add a public CubeAPI endpoint;
- replace Wave 18 realization OOM counter evidence;
- use fuzzy cgroup matching;
- repair a closed realization with a late placement or later status read.

## TDD Contract

Wave 19 implementation must begin with failing tests for:

1. robust `/proc/<pid>/stat` parsing including parenthesized command names and exact field-22 start time;
2. PID reuse detection through stat A/B mismatch;
3. exact cgroup v1 and v2 membership parsing and rejection of substring/suffix matches;
4. canonical boot-ID validation;
5. placement proof created only after trusted `AddProc` success;
6. first-realization asynchronous placement binding after controller Start;
7. late placement after exact outcome cannot bind a closed generation;
8. new Start invalidates old binding and revalidates surviving placement identity;
9. Create fence prevents prior lifetime identity leakage;
10. stale concurrent capture cannot cross generation/lifetime token changes;
11. arbitrary `math.MaxUint64` generation and starttime labels survive Prometheus transport exactly;
12. NolaneWorld strict target parsing and cross-proof generation/cgroup correlation;
13. explicit negative tests proving PID equality, exit 137, Wave 18 positive OOM delta, or SIGKILL never creates an OOM-victim classification;
14. execution remains available when identity evidence capture is unavailable.

A dedicated GitHub Actions contract should run the focused Wave 19 Cubelet and NolaneWorld tests. Final readiness additionally requires Wave 17, Wave 18, host-resource, broad unit, build, format, DCO and NolaneWorld gates to remain green.

## Next Trust Closure — Wave 20

Linux exposes the `oom:mark_victim` tracepoint when the kernel marks a task as an OOM victim. Its event payload includes the victim PID and process metadata, but the tracepoint payload by itself does not provide the complete cgroup/starttime identity needed to defeat PID reuse and prove correlation with a Wave 19 realization binding.

A future Wave 20 may add an authoritative host-kernel victim-event collector. That collector must enrich or bind the victim event at event time with sufficient kernel identity — for example cgroup identity plus a PID-reuse-resistant process lifetime identity — before correlating it with Wave 19.

Only after that additional trust boundary is closed may Nolane Sandbox make a causal statement about the **host CubeShim/VMM process** being selected as an OOM victim.

Even then, guest application OOM causality remains a separate problem requiring guest-side victim provenance; host-process victim evidence must not be relabeled as proof that a particular guest application process was killed.

Autonomously-by: ChatGPT:GPT-5.6-Sol
