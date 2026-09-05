# Nolane Sandbox Wave 21 — Guest Kernel OOM Victim Provenance

## Status

This document is the normative Wave 21 design.

Wave 21 is stacked on Wave 20 and extends the evidence chain from host-side kernel victim marking into the guest workload. It preserves all Wave 17–20 trust boundaries and does not reinterpret any existing OOM signal.

## Purpose and trust statement

Today the guest agent already exposes `GetOOMEvent`, whose `OOMEvent` contains only `container_id`. The existing guest cgroup notifier can observe a container cgroup OOM condition and CubeShim can publish containerd `TaskOOM`, but that path does not identify the exact guest process selected by Linux.

Wave 21 adds a separate evidence contract capable of asserting only:

> During one exact host-authoritative sandbox realization, the guest Linux kernel emitted `oom:mark_victim` for an exact guest process lifetime belonging to the exact container realization. The proof states whether that lifetime is the exact container main process or an exact member process. It does not claim that victim marking caused process exit.

Wave 21 does **not** add `OOMKilled=true`, does not change the meaning of containerd `TaskOOM`, and does not infer victim identity from exit code 137, SIGKILL, cgroup counters, logs, or host-side Wave 20 evidence.

## Existing OOM path remains compatibility-only

The existing API remains unchanged:

```proto
rpc GetOOMEvent(GetOOMEventRequest) returns (OOMEvent);

message OOMEvent {
    string container_id = 1;
}
```

Its current behavior remains available for containerd compatibility. Wave 21 must not add fields to `OOMEvent` and then treat old producers or consumers as exact victim evidence.

The two paths have different semantics:

```text
GetOOMEvent / TaskOOM
    -> container-level OOM notification
    -> no process-lifetime authority

Wave 21 victim evidence
    -> kernel mark_victim event
    -> exact guest process lifetime
    -> exact realization token
    -> MAIN or MEMBER scope
```

No code path may promote the first into the second.

## Authority chain

Wave 17 generation remains the sole realization authority. The guest agent and CubeShim are forbidden from inventing, incrementing, recovering, or interpreting a Wave 17 generation number.

Wave 21 introduces an opaque realization token as the cross-boundary correlation bridge:

```text
Cubelet Wave 17 generation G
    -> host CSPRNG token T
    -> CubeShim one-shot pending binding
    -> guest StartContainer(..., T)
    -> guest-local realization window for T
    -> guest kernel mark_victim evidence tagged to T
    -> finalized guest evidence returned through CubeShim
    -> Cubelet accepts only if T == current token for G
    -> Prometheus lossless transport
    -> NolaneWorld same-scrape strict fusion
```

The token binds clocks and asynchronous delivery without pretending that host and guest boottime clocks are comparable.

## Realization token

The token is exactly 32 random bytes generated with a host operating-system CSPRNG after Wave 17 advances the sandbox generation.

Rules:

- zero-length, non-32-byte, or all-zero tokens are invalid;
- the guest never generates a token;
- CubeShim never substitutes a token;
- a new Wave 17 Start creates a new token;
- Create fences and deletes every prior token for the sandbox;
- a token is an identity nonce, not an authentication secret;
- protobuf carries raw 32 bytes;
- Prometheus and textual diagnostics carry exactly 64 lowercase hexadecimal characters;
- no deterministic token derived from sandbox ID, PID, generation, timestamp, or cgroup path is permitted.

Token generation failure is observational only: the workload still starts and Wave 21 is unknown.

## Host-to-CubeShim binding without a new public service

CubeShim currently exposes the containerd Task service and already supports extension actions through `UpdateTaskRequest.annotations`. Wave 21 reuses that existing extension surface instead of introducing a second host-local RPC server.

The reserved annotations are:

```text
cube.shimapi.update.action = BindOOMVictimRealization
cube.shimapi.update.oom_victim_realization_token = <64 lowercase hex chars>
```

`BindOOMVictimRealization` is an internal one-shot control action. It must not alter workload resources, pause/restore state, or guest policy.

Binding semantics:

1. Cubelet creates token T for exact generation G.
2. Before the corresponding main Task Start can reach the guest, Cubelet sends the reserved Task Update action to that sandbox's CubeShim.
3. CubeShim validates T and stores `(container_id, T)` as the pending binding for the next **main** Start only.
4. The next main `Task.Start` atomically consumes the pending binding from CubeShim state before building the guest start request.
5. Exec starts never consume or inherit the pending main token.
6. A successful main start cannot reuse a consumed token on a later start.
7. A failed main start discards the consumed token; it is never recycled.
8. Delete/Create/shim teardown clears pending and active Wave 21 bridge state.

A second conflicting bind before consumption replaces the pending value but cannot create proof for the controller unless the echoed token equals the controller's exact token for G. Duplicate exact binds are idempotent.

If the extension action is unsupported, malformed, races after Task Start, or otherwise cannot be installed before the guest start, CubeShim proceeds with the ordinary workload start and Wave 21 remains unknown. Evidence plumbing must never make Task Start fail.

The implementation must place the bind call at a lifecycle point that is proven to precede the actual main Task Start. `controllerLocal.Start` currently owns Wave 17 generation advancement but does not itself invoke the runtime Task Start; therefore implementation must not assume method-name order. The dedicated Wave 21 contract must prove the real call ordering with a fake task service before any live integration claim is accepted.

## Guest start protocol extension

The canonical agent protocol adds an optional token to `StartContainerRequest`:

```proto
message StartContainerRequest {
    string container_id = 1;
    bytes oom_victim_realization_token = 2;
}
```

This is wire-compatible with existing clients. An empty field means "no Wave 21 evidence for this realization" and must not fail container start.

Both protocol copies used by the repository — the guest agent protocol and CubeShim's generated protocol input — must remain field-for-field identical for Wave 21 messages. Generated Rust bindings are regenerated from those synchronized inputs; hand-edited generated code is forbidden.

CubeShim passes T only on the exact main `StartContainer` corresponding to the consumed pending binding. It never sends T in CreateContainer, ExecProcess, GetOOMEvent, StatsContainer, or an unrelated container start.

## Guest collector architecture

The guest agent adds an observational kernel-victim collector with the same evidence philosophy as Wave 20:

```text
running guest kernel BTF
    -> validated task_struct layout
    -> dynamically loaded mark_victim probe
    -> bounded kernel-event buffer
    -> active guest realization registry
    -> exact process/cgroup/lifetime correlation
    -> finalized positive evidence set
```

The collector is capability-gated. Missing BTF, missing tracepoint, verifier rejection, insufficient BPF privilege, unsupported helper/map facility, ring-buffer loss, malformed records, or collector startup failure disables Wave 21 evidence only.

No collector failure may fail agent startup, CreateContainer, StartContainer, WaitProcess, RemoveContainer, CubeShim Task Start, Cubelet Start, Wait, or State.

## Kernel event semantics

The evidence source is fixed:

```text
guest.kernel.oom.mark_victim.raw_tracepoint
```

The guest collector observes the Linux `oom:mark_victim` event and reads process identity from the original kernel task at event time. A raw event has the exact v1 shape:

```text
version             uint32
flags               uint32
victim_tid          uint32
victim_tgid         uint32
start_boottime_ns   uint64
event_boottime_ns   uint64
cgroup_v2_id        uint64
```

Validation requires:

```text
version == 1
victim_tid != 0
victim_tgid != 0
start_boottime_ns != 0
event_boottime_ns != 0
event_boottime_ns >= start_boottime_ns
```

`victim_tid` is the exact marked thread ID. `victim_tgid` is the process leader ID. They may differ.

Required BTF `task_struct` members are `pid`, `tgid`, and `start_boottime`. The optional cgroup-v2 identity chain is the same semantic chain as Wave 20:

```text
task_struct.cgroups
  -> css_set.dfl_cgrp
  -> cgroup.kn
  -> kernfs_node.id
```

Required-layout failure disables the collector. Failure of only the optional cgroup chain sets `cgroup_v2_id=0`, which means unknown. Zero never means root cgroup and never means known-negative membership.

Forbidden fallbacks include dmesg/journal parsing, `/dev/kmsg` parsing, OOM text matching, post-event PID-only `/proc` lookup, exit 137, SIGKILL, `memory.events` deltas, the existing `GetOOMEvent`, or host Wave 20 evidence.

## Guest main-process identity

Every accepted Wave 21 proof is anchored to an exact main-process identity for the same guest realization, even for MEMBER scope.

After a successful runtime main-process start, the agent obtains the main guest PID from the runtime authority and captures:

```text
container_id
realization_token
main_pid
main_starttime_ticks
expected_cgroup_v2_id
GuestBootID
```

The `/proc/<pid>/stat` parser must locate field 22 safely even when `comm` contains spaces or `)` characters. Main identity capture uses a reuse-resistant sandwich:

```text
read /proc/<pid>/stat starttime
read /proc/<pid>/cgroup and resolve expected cgroup-v2 identity
read /proc/<pid>/stat starttime again
require first == second and pid still exists
```

`main_pid` and `main_starttime_ticks` must be non-zero. Canonical guest boot ID is read from `/proc/sys/kernel/random/boot_id`.

If exact main identity cannot be captured, the workload remains valid but the entire Wave 21 realization is unknown; MEMBER proof is not allowed to bypass this anchor.

## Exact guest start-time bridge

Raw BPF evidence uses `task_struct.start_boottime`, while `/proc/<pid>/stat` field 22 uses clock ticks visible through the reader's current time namespace. Wave 21 reproduces that conversion exactly rather than comparing PID alone.

For repository production architectures `amd64` and `arm64`, `USER_HZ` is 100:

```text
visible_start_ns = start_boottime_ns + agent_current_time_namespace_boottime_offset
starttime_ticks  = floor(visible_start_ns / 10_000_000)
```

Use integer arithmetic only, with signed-offset normalization and overflow/underflow rejection.

The namespace authority rule is identical in principle to the normative Wave 20 amendment:

- when `/proc/self/timens_offsets` exists, both `/proc/self/ns/time` and `/proc/self/ns/time_for_children` must be readable canonical `time:[inode]` handles and must identify the same namespace before the `boottime` offset is authoritative;
- if the handles differ, conversion is unavailable;
- if `timens_offsets` is absent, zero offset is permitted only when time-namespace exposure is demonstrably absent rather than partially unavailable;
- unsupported architectures yield unknown.

A victim TGID match with a starttime mismatch is a different process lifetime and is rejected.

## Guest cgroup-v2 event-time identity

The agent independently resolves the exact active container cgroup-v2 object to a non-zero kernel cgroup ID. Resolution must use the cgroup-v2 mount and kernel file-handle identity rather than a path hash or inode guess.

The accepted representation follows the Wave 20 rule: exactly the kernel identity represented by the cgroup-v2 handle, non-zero, with unexpected handle layout or mount/path failure treated as unavailable.

The expected cgroup ID is captured while the exact main identity is live and is immutable for that Wave 21 realization.

For a MEMBER proof, exact equality is mandatory:

```text
event.cgroup_v2_id != 0
expected_cgroup_v2_id != 0
event.cgroup_v2_id == expected_cgroup_v2_id
```

Wave 21 does not claim exact MEMBER scope on cgroup v1 or when event-time cgroup identity is unavailable.

## MAIN versus MEMBER classification

Wave 21 has exactly two positive scopes:

```text
MAIN
MEMBER
```

`MAIN` requires:

```text
victim_tgid == main_pid
converted(victim start_boottime) == main_starttime_ticks
```

A MAIN proof may remain valid when cgroup-v2 event identity is unavailable because the exact runtime-owned main process lifetime is independently proven.

`MEMBER` requires:

```text
victim process lifetime is valid
exact event-time cgroup-v2 ID == exact expected container cgroup-v2 ID
```

A MEMBER proof does not require `victim_tgid != main_pid`; however if exact main lifetime equality is established the evidence is classified MAIN, never downgraded to MEMBER.

No code may treat MEMBER as MAIN, `container init`, `application main`, or equivalent wording.

## Guest-local realization window

Host and guest boottime clocks are never numerically compared.

The guest agent maintains one local realization window keyed by `(container_id, token)`:

```text
started_boot_ns
outcome_observed_boot_ns
```

Rules:

1. `started_boot_ns` is captured with guest `CLOCK_BOOTTIME` before invoking the runtime main start, after a valid token has been accepted as pending for that container.
2. If runtime start fails, the window and token are discarded.
3. Raw kernel events may be buffered globally before exact main identity capture, allowing an OOM event immediately after process creation to be correlated later without an early-observation gap.
4. After successful start and main identity capture, buffered candidates are eligible only when exact lifetime/cgroup rules pass.
5. `outcome_observed_boot_ns` is captured exactly once after authoritative runtime main-process WaitProcess observes the terminal outcome.
6. Final evidence requires:

```text
started_boot_ns <= event_boot_time_ns <= outcome_observed_boot_ns
```

7. `outcome_observed_boot_ns` is an observation upper bound, not the process's kernel exit timestamp.
8. A new token for the same container fences prior open state. Old finalized evidence remains addressable only by its old exact token until bounded eviction.

Guest clock-read failure makes Wave 21 unknown and never changes workload result.

## Positive-only bounded evidence store

Wave 21 stores positive evidence only. Absence is always unknown, never known false.

Per guest agent:

```text
max finalized realizations = 256
max victim records per realization = 64
max finalized age = 10 minutes
```

Raw collector buffering remains independently bounded to 1024 records and 10 minutes, matching Wave 20's bounded-evidence philosophy.

Rules:

- exact duplicate records are idempotent;
- conflicting records are never merged into a stronger scope;
- deterministic order is `(event_boot_time_ns, victim_tgid, victim_tid, victim_starttime_ticks)`;
- deterministic oldest-first eviction is required;
- overflow beyond 64 victims makes that realization's Wave 21 evidence unavailable rather than silently truncating and presenting a complete-looking set;
- collector/ring-buffer drops mark affected evidence availability unknown;
- agent restart cannot reconstruct missed kernel history and does not fabricate historical proof.

## New finalized guest evidence RPC

Wave 21 adds a new RPC; it does not reuse `GetOOMEvent`:

```proto
rpc GetOOMVictimEvidence(GetOOMVictimEvidenceRequest)
    returns (GetOOMVictimEvidenceResponse);

message GetOOMVictimEvidenceRequest {
    string container_id = 1;
    bytes realization_token = 2;
}

enum OOMVictimScope {
    OOM_VICTIM_SCOPE_UNSPECIFIED = 0;
    OOM_VICTIM_SCOPE_MAIN = 1;
    OOM_VICTIM_SCOPE_MEMBER = 2;
}

message OOMVictimEvidence {
    uint32 version = 1;
    string container_id = 2;
    bytes realization_token = 3;
    string guest_boot_id = 4;
    uint32 victim_tid = 5;
    uint32 victim_tgid = 6;
    uint64 victim_starttime_ticks = 7;
    uint64 event_boot_time_ns = 8;
    uint64 cgroup_v2_id = 9;
    uint32 main_pid = 10;
    uint64 main_starttime_ticks = 11;
    OOMVictimScope scope = 12;
    uint64 realization_started_boot_ns = 13;
    uint64 outcome_observed_boot_ns = 14;
    string source = 15;
}

message GetOOMVictimEvidenceResponse {
    repeated OOMVictimEvidence evidence = 1;
}
```

The request is valid only with non-empty container ID and exact 32-byte token.

The agent returns records only for a finalized guest-local realization. A realization that is unsupported, still open, evicted, malformed, collector-loss-affected, or has no positive victim proof returns no authoritative positive evidence. CubeShim and Cubelet must interpret all such cases as unknown.

Every returned record requires:

```text
version == 1
container_id == request.container_id
realization_token == request.realization_token
guest_boot_id canonical UUID
victim_tid != 0
victim_tgid != 0
victim_starttime_ticks != 0
event_boot_time_ns != 0
main_pid != 0
main_starttime_ticks != 0
realization_started_boot_ns != 0
outcome_observed_boot_ns >= realization_started_boot_ns
event within guest-local window
source == guest.kernel.oom.mark_victim.raw_tracepoint
scope == MAIN or MEMBER
```

For MAIN, victim TGID/starttime must equal main PID/starttime. For MEMBER, cgroup_v2_id must be non-zero and already exactly correlated to the active container cgroup.

## CubeShim evidence handling

CubeShim continues the existing asynchronous `GetOOMEvent -> TaskOOM` path unchanged.

Wave 21 adds a separate finalized-evidence retrieval path tied to the token consumed by main Start.

After the guest main WaitProcess returns, CubeShim may request finalized Wave 21 evidence for the consumed `(container_id, token)` and cache the immutable response. Query failure, unsupported method, guest disconnect, timeout, malformed response, or empty result is observational-only.

CubeShim validates protobuf invariants and token equality but does not stamp a Wave 17 generation.

The cache is positive-only and bounded using the same realization/token identity. Delete/shim teardown may discard it. CubeShim restart does not reconstruct prior Wave 21 evidence.

## Returning evidence to Cubelet

Wave 21 reuses the standard Task service rather than adding a second host-local service. The exact transport is a versioned protobuf `Any` carried by an internal reserved Task Stats query for the sandbox main task.

The reserved Stats request convention is:

```text
ID = <sandbox/container id>
```

and the response may carry a Wave 21 evidence `Any` only through a new dedicated type URL:

```text
io.cubesandbox.v1.GuestOOMVictimEvidenceSet
```

This evidence type is distinct from the existing cgroup metrics `Any`. Ordinary workload metrics requests retain their existing metrics type and behavior.

To avoid overloading one Stats call ambiguously, CubeShim selects the Wave 21 evidence response only when the request metadata contains the internal ttrpc metadata key:

```text
cube-wave21-guest-oom-evidence = <64 lowercase token hex>
```

The key is host-internal. A missing or malformed key follows the ordinary Stats path. The value must equal the exact cached token before CubeShim returns evidence.

The Wave 21 evidence-set payload contains the raw 32-byte token and the complete ordered guest evidence records. Cubelet validates both the metadata token and payload token against generation G.

If the ttrpc library cannot preserve request metadata for this internal call on a supported repository target, implementation must use an equivalent reserved Task extension that preserves the same one-shot token equality and does not change ordinary Stats semantics. Such a fallback is acceptable only when the deterministic contract demonstrates that ordinary Stats cannot return Wave 21 data without the exact token selector. A Metrics-type collision or tokenless evidence query is forbidden.

## Cubelet controller correlation

The controller extends the Wave 17 generation-owned store with Wave 21 token and accepted guest evidence state.

For generation G:

1. begin realization;
2. generate token T;
3. attempt one-shot CubeShim bind before actual main Task Start;
4. if binding cannot be proven, mark Wave 21 unavailable for G without failing the workload;
5. accept Wave 17 authoritative Wait or stopped-State outcome exactly as before;
6. after outcome acceptance, perform at most one finalized Wave 21 retrieval for T;
7. validate every returned record;
8. store accepted proofs under `(sandbox_id, generation, T)`;
9. never retry a failed/missing finalization later, preventing post-outcome ambient state from repairing evidence;
10. Create or new Start fences prior generation state.

A stopped-State recovery after controller restart may recover Wave 17 outcome under existing rules, but cannot recover Wave 21 unless the exact in-memory token authority for that generation survived. No token reconstruction from sandbox ID, generation, PID, cgroup path, TaskOOM, or guest data is permitted.

Wave 18, 19, and 20 evidence may be cross-checked for sandbox/generation consistency when present, but none is required to manufacture Wave 21 proof and none can substitute for T.

## Accepted host proof shape

Each accepted positive record becomes:

```go
type GuestProcessKernelOOMVictimProof struct {
    SandboxID                string
    Generation               uint64
    RealizationTokenHex      string
    GuestBootID              string
    VictimTID                uint32
    VictimTGID               uint32
    VictimStartTimeTicks     uint64
    MainPID                  uint32
    MainStartTimeTicks       uint64
    Scope                    string // "main" or "member"
    EventBootTimeNS          uint64
    CgroupV2ID               uint64
    RealizationStartedBootNS uint64
    OutcomeObservedBootNS    uint64
    Source                   string
}
```

Canonical host scope strings are exactly `main` and `member`.

For MAIN, `CgroupV2ID` may be zero when exact event-time cgroup identity is unavailable. For MEMBER it must be non-zero. Zero is transported as empty where a Prometheus label represents optional cgroup identity; it is never presented as a known cgroup ID.

The accepted proof does not carry exit code, signal, TaskOOM state, Wave 18 counter delta, or host Wave 20 victim state because those are independent evidence dimensions.

## Public claim vocabulary

NolaneWorld adds positive/unknown helpers:

```go
func (e TaskTerminationEvidence) GuestKernelOOMVictimMarked() (marked bool, known bool)
func (e TaskTerminationEvidence) GuestMainKernelOOMVictimMarked() (marked bool, known bool)
```

Semantics:

```text
at least one accepted MAIN or MEMBER proof -> GuestKernelOOMVictimMarked() == (true, true)
no accepted proof                         -> (false, false)
at least one accepted MAIN proof          -> GuestMainKernelOOMVictimMarked() == (true, true)
no accepted MAIN proof                    -> (false, false)
```

The absence of MAIN when MEMBER exists does **not** mean the main process was not marked; collection can be incomplete. Therefore no `(false,true)` result is introduced.

Forbidden public names/claims include `OOMKilled`, `GuestOOMKilled`, `ApplicationOOMKilled`, `MainOOMKilled`, `TaskOOMKilled`, `OOMCausedExit`, or equivalent causal wording.

## Prometheus transport

Resource metrics exports one sample per accepted guest victim record:

```text
cubesandbox_guest_kernel_oom_victim_info{
  sandbox_id="sandbox-a",
  generation="18446744073709551615",
  realization_token="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  guest_boot_id="11111111-2222-3333-4444-555555555555",
  victim_tid="231",
  victim_tgid="229",
  victim_starttime_ticks="987654321",
  main_pid="229",
  main_starttime_ticks="987654321",
  scope="main",
  event_boot_time_ns="18446744073709551600",
  cgroup_v2_id="123456",
  realization_started_boot_ns="18446744073709550000",
  outcome_observed_boot_ns="18446744073709551614",
  source="guest.kernel.oom.mark_victim.raw_tracepoint"
} 1
```

Rules:

- metric value is exactly numeric one;
- all integers use canonical decimal labels;
- token is exact lowercase 64-character hex;
- guest boot ID is canonical lowercase UUID form;
- scope is exactly `main` or `member`;
- source is fixed;
- optional MAIN cgroup ID uses an empty label when unknown, never `0`;
- MEMBER requires a non-empty non-zero cgroup ID;
- resource metrics validates and transports; it cannot synthesize guest proof.

Multiple records for one realization are legal and ordered semantically by their immutable fields; Prometheus ordering itself is not authoritative.

## NolaneWorld one-scrape fusion

`TaskTerminationEvidence` gains a bounded slice of `GuestProcessKernelOOMVictimProof` parsed from the same scrape as Waves 17–20.

NolaneWorld requires an exact Wave 17 task outcome for the target generation before accepting Wave 21 samples. Every Wave 21 sample for the target must agree on:

```text
sandbox_id
generation
realization_token
guest_boot_id
main_pid
main_starttime_ticks
realization_started_boot_ns
outcome_observed_boot_ns
source
```

Per-record victim fields may differ.

Fail closed on malformed token, malformed UUID, noncanonical number, zero required identity, invalid scope, invalid window, event outside window, MAIN lifetime mismatch, MEMBER missing cgroup ID, duplicate target sample, conflicting record, unsupported source, detached generation, mixed tokens, mixed guest boots, mixed main identity, non-unit metric, or more than 64 records.

NolaneWorld never opens guest `/proc`, cgroup files, BPF maps, tracefs, vsock, CubeShim sockets, or host `/proc` using transported Wave 21 values.

Wave 20 host victim proof and Wave 21 guest victim proof are independent. Their simultaneous presence means both exact kernel victim-marking facts were observed in their respective kernels; it does not establish that one caused the other.

## Failure and security semantics

Every Wave 21 evidence failure is fail-closed and workload-noninterfering.

Unknown includes, without limitation:

- host CSPRNG failure;
- bind Update unsupported or late;
- pending-token loss;
- CubeShim restart;
- guest agent restart;
- malformed/missing token;
- guest BTF missing/incompatible;
- mark_victim attach unavailable;
- BPF verifier or privilege failure;
- ring-buffer loss;
- raw store overflow/expiry;
- guest boot ID failure;
- guest boottime failure;
- exact main identity unavailable;
- PID reuse detected;
- time-namespace authority unavailable;
- unsupported architecture;
- cgroup-v2 identity unavailable for MEMBER;
- event outside the guest-local realization window;
- finalized realization overflow;
- evidence RPC unsupported/disconnected;
- CubeShim cache loss;
- controller restart without exact token authority;
- token/generation mismatch;
- malformed transport;
- NolaneWorld correlation mismatch.

The 32-byte token is unpredictable to prevent accidental or ambient cross-generation attachment, but security does not rely on secrecy. A workload learning a token cannot create an accepted proof without the trusted guest kernel collector producing a matching positive victim event and the controller matching the exact host-authoritative token/generation.

Transported IDs and labels are data, never executable authority. No path from Wave 21 evidence may invoke a signal, kill, restart, resource change, rollback, or policy action in this wave.

## Negative-space matrix

The following inputs alone must never synthesize a Wave 21 proof:

| Input | Allowed Wave 21 conclusion |
| --- | --- |
| containerd `TaskOOM` | unknown |
| existing guest `OOMEvent{container_id}` | unknown |
| exit code 137 | unknown |
| SIGKILL | unknown |
| Wave 18 cgroup `oom_kill` delta | unknown |
| Wave 19 host process identity | unknown |
| Wave 20 host `mark_victim` proof | unknown |
| guest `memory.events: oom_kill > 0` | unknown |
| guest PID without starttime | unknown |
| guest cgroup path without kernel cgroup ID | unknown for MEMBER |
| old realization token | unknown for current generation |
| valid token without guest kernel event | unknown |
| guest kernel event without valid token | unknown |
| guest kernel event outside token window | unknown |
| MEMBER proof | never MAIN |

## TDD contract

Wave 21 must begin with failing deterministic contract tests before production implementation. The dedicated contract must cover at least:

1. exact 32-byte host token generation and malformed/all-zero rejection;
2. Create/new Start fencing of prior token state;
3. reserved Update bind validation, duplicate idempotency, conflict replacement, and one-shot consumption;
4. exec Start cannot consume the main token;
5. failed main Start discards consumed token;
6. bind/collector/evidence failures never fail ordinary workload Start or Wait;
7. synchronized guest/CubeShim proto definitions and generated-code regeneration checks;
8. optional StartContainer token wire compatibility;
9. raw victim decoder version/size/zero/time validation;
10. BTF required-member validation and optional cgroup-chain downgrade;
11. TID may differ from TGID;
12. exact guest boot-ID parsing;
13. `/proc/stat` parser with spaces and `)` inside comm;
14. double-read PID-reuse rejection;
15. amd64/arm64 starttime conversion with signed time-namespace offsets;
16. equal-vs-different current/for-children time-namespace authority;
17. main identity exact lifetime match -> MAIN;
18. TGID match + starttime mismatch rejects MAIN;
19. exact event-time cgroup-v2 match -> MEMBER;
20. missing/mismatched cgroup ID cannot create MEMBER;
21. exact guest-local window boundary acceptance and before/after rejection;
22. early raw event buffering after window start but before main identity completion;
23. 1024 raw-event eviction and 10-minute expiry;
24. 256 finalized-realization eviction and 10-minute expiry;
25. 64-victim overflow makes realization unavailable rather than truncating;
26. exact duplicate idempotency and conflicting-record fail-close;
27. finalized evidence request requires exact container ID + token;
28. existing GetOOMEvent/TaskOOM behavior remains unchanged;
29. Wave 21 evidence path never publishes containerd TaskOOM itself;
30. CubeShim token selector cannot return evidence for a different token;
31. ordinary Stats without Wave 21 selector remains ordinary metrics;
32. max uint64 fields survive protobuf -> CubeShim -> Prometheus -> NolaneWorld exactly;
33. NolaneWorld accepts multiple consistent victim samples and rejects mixed token/boot/main/window/source;
34. positive MEMBER yields GuestKernelOOMVictimMarked `(true,true)` but GuestMainKernelOOMVictimMarked `(false,false)`;
35. positive MAIN yields both helpers `(true,true)`;
36. absence remains `(false,false)`, never `(false,true)`;
37. no production API/evidence type contains OOMKilled-style causal classification;
38. TaskOOM, exit137, SIGKILL, Wave18 delta, Wave19 identity, Wave20 host proof, or guest cgroup OOM signal cannot synthesize Wave21 proof;
39. controller restart without exact token cannot reconstruct Wave21;
40. supported repository architectures build and format cleanly.

The deterministic core test suite must remain unprivileged. A privileged live guest BPF attach test is additive and capability-gated, never the only proof of semantics.

Final readiness also requires all prior Wave 17–20 contracts, Host Resource, NolaneWorld, Unit Test, Build, Format, DCO, Docs, and Live Substrate workflows to remain green.

## Explicit non-goals

Wave 21 does not:

- claim that `mark_victim` newly sent SIGKILL;
- claim that OOM caused the final process exit;
- change containerd `TaskOOM` semantics;
- prove a negative OOM result;
- infer MAIN from MEMBER;
- compare host boottime numerically with guest boottime;
- reconstruct evidence across collector/agent/shim/controller downtime;
- use guest logs as victim authority;
- add exact cgroup-v1 MEMBER proof;
- add non-amd64/non-arm64 exact starttime correlation;
- turn evidence into automatic remediation or policy;
- merge host Wave 20 and guest Wave 21 into one causal claim.

A later wave may build an explicit causal termination classifier, but only if it adds an additional kernel-level bridge proving the relationship between exact victim marking and exact terminal outcome. Wave 21 intentionally stops before that claim.