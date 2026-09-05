# Nolane Sandbox Wave 20 — Kernel OOM Victim Provenance

## Purpose

Wave 20 closes the next trust boundary after Waves 17–19: it proves when the Linux kernel marks the exact Wave 19 host CubeShim/VMM process as an OOM victim.

Wave 17 proves exact task outcome. Wave 18 proves that the sandbox cgroup `oom_kill` counter increased during one exact task realization. Wave 19 proves PID-reuse-resistant identity for the host CubeShim/VMM process placed in the sandbox cgroup. None of those facts proves that the kernel selected that exact host process as an OOM victim.

Wave 20 adds event-time kernel evidence for that missing fact.

The strongest statement Wave 20 may make is:

> During controller realization generation G, Linux emitted `oom:mark_victim` for the exact host process identity already bound by Wave 19; when event-time cgroup-v2 identity is available, that victim process was also observed in the exact sandbox cgroup used by the Wave 18 realization proof.

Wave 20 does **not** introduce `OOMKilled=true`. Linux may call `mark_oom_victim()` for a task that is already exiting and expected to free memory, so the event proves the kernel marked the task as an OOM victim; it does not by itself prove that this call newly killed the task.

Wave 20 also does not prove that a guest application process was selected or killed. CubeShim/VMM host-process victim provenance and guest-process victim provenance remain separate identity domains.

## Existing Trust Boundaries

### Wave 17 — exact task outcome

The sandbox controller owns the realization generation. Exact terminal outcome is accepted only from authoritative containerd `Wait` or stopped `State` responses and is transported without binary64 precision loss.

### Wave 18 — realization-scoped cgroup OOM observation

Wave 18 snapshots the exact sandbox cgroup OOM counter before and after one realization. A positive delta means at least one process in the cgroup was OOM-killed during the observation window. It does not identify which process was selected.

### Wave 19 — host process identity

Wave 19 binds a trusted CubeBox `AddProc` placement to exact identity:

```text
boot_id + host_pid + /proc/<pid>/stat starttime_ticks
```

It validates cgroup membership with a stat/cgroup/stat sandwich and binds the placement to the Wave 17 generation while that generation is still current and open.

Wave 20 consumes this proof. It never replaces Wave 19 as lifecycle or process-identity authority.

## Kernel Semantics

Linux `mark_oom_victim(struct task_struct *tsk)` sets the OOM-victim state and emits `trace_mark_victim(tsk, uid)` after the victim state has been installed. The `oom:mark_victim` trace event therefore has direct kernel provenance to the victim task selected by the OOM path.

The normal trace-event payload exposes PID, comm and memory statistics but does not expose enough lifetime or cgroup identity to defeat PID reuse. Waiting until userspace receives PID and then reading `/proc/<pid>` is not sufficient: the victim can exit and the PID can be reused before that read.

Wave 20 therefore captures lifetime identity from the tracepoint's original `struct task_struct *` argument inside eBPF at event time.

## Architecture Decision

Wave 20 uses a **BTF-driven dynamically assembled eBPF raw-tracepoint program**.

It deliberately does not require a checked-in BPF ELF object, `bpf2go`, clang, or hard-coded `task_struct` offsets.

At runtime:

1. Go loads the running kernel BTF using `btf.LoadKernelSpec()`.
2. A layout resolver finds the exact byte offsets needed from BTF.
3. Go constructs `asm.Instructions` for an `ebpf.RawTracepoint` program using those resolved offsets.
4. The program attaches to the kernel OOM victim raw tracepoint and emits fixed-version records through `BPF_MAP_TYPE_RINGBUF`.
5. A userspace collector validates records and feeds them to a bounded evidence correlator.

This architecture fits the repository because Cubelet already depends on `github.com/cilium/ebpf v0.17.3`.

### Why not tracefs + post-event `/proc`

Rejected because PID reuse and fast exit create an evidence race between the kernel event and userspace inspection.

### Why not a kernel module

Rejected because it enlarges the trusted computing base and kernel compatibility surface when BTF/eBPF already provides the required event-time task pointer.

### Why not hard-coded BTF offsets

Rejected because kernel struct layout varies. Missing or incompatible BTF must produce unavailable evidence rather than a potentially wrong read.

## Package Boundaries

Create a package-neutral internal subsystem:

```text
Cubelet/plugins/cube/internals/kernelvictim
```

Responsibilities:

- kernel BTF layout resolution;
- dynamic raw-tracepoint program construction;
- ring-buffer record decoding;
- kernel-native process-start conversion;
- cgroup-v2 ID resolution and validation;
- bounded event buffering;
- no sandbox generation authority;
- no task-outcome interpretation;
- no Prometheus transport.

The sandbox controller remains the realization authority and owns correlation with Wave 17/18/19 evidence.

Resource metrics consumes accepted proof through a structural visitor, preserving the package-direction rule from Waves 17–19.

NolaneWorld remains a strict consumer and gains no host-kernel execution authority.

## Kernel Event Record

Use an explicitly versioned fixed binary record. Conceptually:

```go
type RawVictimEvent struct {
    Version          uint32
    Flags            uint32
    PID              uint32
    TGID             uint32
    StartBootTimeNS  uint64
    EventBootTimeNS  uint64
    CgroupV2ID       uint64
}
```

`Version` is fixed to `1` in Wave 20.

Required event facts:

- `PID != 0`;
- `TGID != 0`;
- `StartBootTimeNS != 0`;
- `EventBootTimeNS != 0`;
- `EventBootTimeNS >= StartBootTimeNS`.

`CgroupV2ID` is optional evidence. Zero means unknown/unavailable, never root cgroup and never a negative statement.

The BPF program must emit a record only after all required task reads succeed. Failure to read optional cgroup identity may still emit the process-victim event with `CgroupV2ID=0`.

## BTF Layout Resolver

The layout resolver uses running-kernel BTF and resolves member offsets by exact member name. It must fail closed if a required type or member is absent, is a bitfield where a normal scalar/pointer is expected, has an unexpected scalar width, or would require an offset outside the eBPF instruction encoding supported by the assembler.

Required direct fields from `task_struct`:

```text
pid
tgid
start_boottime
```

Optional cgroup-v2 chain:

```text
task_struct.cgroups
  -> css_set.dfl_cgrp
  -> cgroup.kn
  -> kernfs_node.id
```

The cgroup chain is independently capability-gated. Failure to resolve it disables event-time cgroup ID only; it does not disable exact victim process identity.

The resolver must unwrap typedef/const/volatile/restrict wrappers and verify pointer/struct transitions structurally rather than assuming type names alone.

No BTF offset is persisted across host reboots or reused for a different running kernel.

## eBPF Program Construction

The program is generated with `github.com/cilium/ebpf/asm` and loaded as `ebpf.RawTracepoint` with GPL-compatible license metadata because kernel tracing/helper availability may require GPL semantics.

The program receives the raw tracepoint context. Its first argument is the original victim `struct task_struct *` passed to the OOM tracepoint.

Program flow:

```text
ctx.args[0] -> victim task pointer
      |
      +-- read pid
      +-- read tgid
      +-- read start_boottime
      +-- read optional dfl cgroup id
      +-- bpf_ktime_get_boot_ns()
      |
      +-- ringbuf_reserve(record-v1)
      +-- write exact fixed record
      +-- ringbuf_submit()
```

All kernel pointer dereferences use verifier-safe kernel reads. The program must not dereference arbitrary userspace addresses.

The ring-buffer map is created by Go at runtime and referenced by FD in the generated instruction stream.

The program contains no mutable policy maps and no sandbox ID, generation, cgroup path or user-controlled filtering. This keeps the kernel observer generic and avoids moving lifecycle authority into BPF.

## Tracepoint Attachment

The collector targets the raw tracepoint corresponding to `oom:mark_victim`.

Attachment is capability-gated. If the running kernel does not expose the raw tracepoint, raw tracepoint programs are unsupported, BTF is unavailable, the verifier rejects the dynamically generated program, or permissions/capabilities are insufficient, Wave 20 victim evidence is unavailable.

Collector startup failure must not fail Cubelet startup or sandbox execution unless a future explicit strict-telemetry configuration says otherwise. Wave 20 does not introduce such a strict mode.

No fallback to log parsing, exit code 137, SIGKILL, dmesg text, Wave 18 OOM delta, or post-event PID-only `/proc` reads is permitted.

## Exact Start-Time Bridge

Wave 19 uses `/proc/<pid>/stat` field 22 `starttime_ticks`. The kernel renders that field from `task->start_boottime` using `nsec_to_clock_t(timens_add_boottime_ns(...))`.

For repository-supported Linux production architectures `amd64` and `arm64`, Linux UAPI `USER_HZ` is 100. Therefore when the Cubelet process is in its own current time namespace with offset `O`, the exact comparison value is:

```text
visible_start_ns = event.start_boottime_ns + O
starttime_ticks  = floor(visible_start_ns / 10_000_000)
```

Wave 20 does **not** assume that time-namespace offset is always zero.

At collector initialization, parse `/proc/self/timens_offsets` when present and obtain the current process's `boottime` offset exactly as signed seconds + nanoseconds. For kernels without time namespaces or with no proc offset file, treat the offset as zero only when the platform's proc behavior confirms that no configurable time namespace is active; otherwise victim correlation is unavailable.

The bridge must:

- support only `runtime.GOARCH == "amd64"` or `"arm64"` in Wave 20;
- use `USER_HZ=100` only on those two explicitly supported architectures;
- reject arithmetic overflow/underflow;
- normalize signed seconds/nanoseconds exactly;
- calculate with integer arithmetic only;
- require the result to equal the Wave 19 `StartTimeTicks` exactly.

A PID match with a start-time mismatch is a different process and must be rejected.

## Boot Identity

The collector reads canonical `/proc/sys/kernel/random/boot_id` at startup. Every accepted victim event is tagged in userspace with that boot ID before correlation.

Wave 20 requires exact equality with Wave 19 `BootID`.

Collector restart in the same boot may observe new events. It cannot reconstruct victim events that occurred while the collector was unavailable.

A host reboot changes boot ID and therefore invalidates correlation with old Wave 19 identity automatically.

## Cgroup-v2 Event-Time Identity

For cgroup v2, Wave 20 captures the victim task's default cgroup ID from the task's `css_set -> dfl_cgrp -> kernfs_node` chain at event time.

Userspace resolves the expected sandbox cgroup-v2 path into the same kernel cgroup ID using `name_to_handle_at(2)` through `golang.org/x/sys/unix.NameToHandleAt`.

The cgroup-v2 file handle must contain exactly one 64-bit cgroup ID. Unexpected handle type/length, unsupported filesystem semantics, mount mismatch, missing path, or zero ID makes event-time cgroup correlation unavailable.

Path handling rules:

- only canonical absolute Wave 19 cgroup paths are accepted;
- the mount root is discovered/validated as cgroup v2, not assumed from arbitrary user input;
- path joining must not allow traversal outside the cgroup-v2 mount;
- the resolved cgroup ID must equal the event's non-zero `CgroupV2ID` exactly.

For cgroup v1, Wave 20 does not claim event-time cgroup identity in the first implementation. Exact process-victim correlation remains possible, but the stronger cgroup-v2 correlation field remains unknown.

No cgroup-v1 hierarchy ID is converted into a fake cgroup-v2 ID.

## Event Store and Loss Semantics

Kernel victim events are short-lived facts and must not create unbounded memory growth.

The `kernelvictim` package owns a bounded in-memory store keyed by:

```text
boot_id + tgid/pid + starttime_ticks
```

The store retains only a small fixed maximum number of newest events and applies a maximum age. Both values are internal constants in Wave 20 and covered by tests.

### Ring-buffer loss

If userspace cannot keep up, the kernel program may fail to reserve ring-buffer space. Absence of a victim event is therefore never proof that no OOM victim mark occurred.

Wave 20 exposes positive evidence only. It does not expose `KernelOOMVictimMarked=false` as a known-negative fact.

### Duplicate events

Linux may attempt to mark an already-marked task; the kernel suppresses repeated victim-state installation. Nevertheless the userspace store treats exact duplicate records idempotently.

Conflicting records for the same process identity are rejected from authoritative correlation rather than merged heuristically.

## Realization Correlation Authority

The sandbox controller correlates victim events with the Wave 19 realization binding. `kernelvictim` itself never receives or invents a generation.

An accepted realization victim proof requires:

1. exact Wave 19 host-process binding exists;
2. event boot ID equals Wave 19 boot ID;
3. event TGID/PID equals Wave 19 host PID according to the runtime's process-leader contract;
4. converted event start time equals Wave 19 `StartTimeTicks` exactly;
5. event occurred no earlier than Wave 19 placement/binding provenance permits;
6. event is correlated while the same realization generation is authoritative;
7. exact Wave 17 task outcome exists before final export of terminal evidence;
8. event ordering is compatible with the task realization window;
9. if cgroup-v2 correlation is claimed, event cgroup ID equals the exact ID resolved from the Wave 19 cgroup path;
10. if Wave 18 OOM proof exists for the same generation, its cgroup path must equal the Wave 19 binding path.

### Event before task outcome

Victim event collection is asynchronous. The controller may observe a valid victim event before terminal task outcome. It may retain a candidate tied to the current generation, but the final transport proof is emitted only after the exact task outcome closes that generation.

### Late event

An event consumed after task outcome may still be accepted only if its kernel boot-time timestamp can be proven to fall within the already-closed realization window and its exact process identity matches the immutable Wave 19 binding. Wall-clock receipt time is not used as event time.

No event may be bound retroactively to an older generation merely because PID or cgroup path matches.

### New Start/Create

A newer Start generation cannot inherit an old victim event unless that event's kernel timestamp and exact process identity fall within the newer generation's own authoritative window. Create fences all prior sandbox-lifetime victim candidates.

## Realization Window

Wave 20 needs a monotonic event-time window compatible with eBPF's boot-time clock.

At Wave 19 placement/binding and Wave 17 terminal outcome boundaries, the controller records a host monotonic/boottime timestamp from one package-neutral clock source capable of matching `bpf_ktime_get_boot_ns()` semantics.

Wave 20 must not compare kernel boottime nanoseconds directly with wall-clock RFC3339 timestamps.

If a trustworthy boot-time boundary cannot be captured for a realization, late-event correlation becomes unavailable. Synchronous event correlation while the generation is still open may still produce a candidate, but final proof must preserve enough ordering evidence to avoid cross-generation ambiguity.

The preferred implementation uses `unix.ClockGettime(CLOCK_BOOTTIME)` and stores exact unsigned nanoseconds beside controller-local provenance. Conversion errors or overflow produce unknown evidence.

## Accepted Proof Shape

Conceptually:

```go
type HostProcessKernelOOMVictimProof struct {
    SandboxID          string
    Generation         uint64
    BootID             string
    HostPID            uint32
    StartTimeTicks     uint64
    CGroupPath         string
    EventBootTimeNS    uint64
    CgroupV2ID         uint64
    CgroupV2Correlated bool
    Source              string
}
```

Fixed source:

```text
kernel.oom.mark_victim.raw_tracepoint
```

`CgroupV2Correlated` may be true only when a non-zero event ID and an independently resolved path ID match exactly.

If cgroup ID is unavailable, the process-victim proof may still exist, but the field is false and `CgroupV2ID` must remain zero in transport rather than pretending root/unknown are equivalent.

## Claim Vocabulary

Wave 20 introduces the term:

```text
KernelOOMVictimMarked
```

It means only:

> Linux `mark_oom_victim` emitted an event for the exact host CubeShim/VMM process identity bound to this realization.

It must never be named or documented as:

```text
OOMKilled
TaskOOMKilled
GuestOOMKilled
ApplicationOOMKilled
```

A helper may expose:

```go
func (e TaskTerminationEvidence) HostKernelOOMVictimMarked() (marked bool, known bool)
```

Semantics:

- no victim proof -> `(false, false)`;
- accepted victim proof -> `(true, true)`;
- there is no `(false, true)` state in Wave 20 because event loss/collector downtime prevents authoritative known-negative evidence.

## Transport

Resource metrics exports one atomic info-style sample per accepted realization victim proof:

```text
cubesandbox_host_kernel_oom_victim_info{
  sandbox_id="sandbox-a",
  generation="18446744073709551615",
  boot_id="11111111-2222-3333-4444-555555555555",
  host_pid="12345",
  starttime_ticks="987654321",
  cgroup_path="/cube_sandbox_v1/42",
  event_boot_time_ns="18446744073709551600",
  cgroup_v2_id="123456",
  cgroup_v2_correlated="true",
  source="kernel.oom.mark_victim.raw_tracepoint"
} 1
```

Rules:

- metric value is exactly numeric one;
- all integers are exact canonical decimal-string labels;
- boolean is canonical `true` or `false`;
- if cgroup-v2 ID is unavailable, label value is empty string, not `0`;
- no sample for malformed or incomplete proof;
- no `OOMKilled` metric or label;
- resource metrics cannot create or repair victim evidence.

## NolaneWorld Consumer

`TaskTerminationEvidence` gains optional host-kernel victim provenance and parses it from the **same management scrape** as Waves 17–19.

Strict correlation requires:

- sandbox ID equals exact task outcome sandbox;
- generation equals task outcome generation;
- victim boot/PID/starttime equals Wave 19 host identity exactly;
- cgroup path equals Wave 19 path;
- if Wave 18 proof exists, cgroup path also equals Wave 18 path;
- event source is exactly the Wave 20 source constant;
- cgroup-v2 correlated proof has canonical non-empty ID;
- non-correlated proof has empty cgroup-v2 ID;
- duplicate target victim samples fail closed;
- detached victim proof without Wave 19 identity fails closed;
- detached victim proof without exact task outcome fails closed.

NolaneWorld never opens `/proc`, tracefs, BPF maps or cgroup files using transported values.

## Failure Semantics

Wave 20 is observational and fail-closed.

The following all mean victim evidence unavailable, never sandbox execution failure:

- missing kernel BTF;
- raw tracepoint unavailable;
- BPF disabled or insufficient capability;
- verifier rejection;
- ring buffer unavailable;
- malformed event record;
- unsupported architecture;
- unsupported time-namespace conversion;
- boot ID failure;
- cgroup-v2 ID resolver unavailable;
- event store overflow;
- collector restart;
- stale generation;
- process lifetime mismatch;
- cgroup ID mismatch.

Evidence failure must not turn Start, Wait or State into workload failure.

## Security Boundaries

- BTF-derived offsets are treated as kernel-layout metadata and validated before assembly.
- BPF reads only kernel pointers originating from the OOM raw tracepoint task argument.
- No user-provided pointer is passed into BPF.
- Sandbox IDs and paths stay in userspace; the BPF program is generic.
- Cgroup path resolution is constrained under the discovered cgroup-v2 mount.
- Transported PID/cgroup values are data, not executable authority.
- NolaneWorld cannot use host PID to signal, inspect, or control a process.

## Portability

Wave 20 production victim collection requires Linux with:

- raw tracepoint BPF support;
- kernel BTF exposing required fields;
- BPF ring-buffer support;
- Cubelet process privilege/capability sufficient for tracing;
- `amd64` or `arm64` for exact Wave 19 starttime conversion.

Hosts outside this capability set continue to run sandboxes with Wave 20 evidence unknown.

The package should expose capability diagnostics so operators can understand why victim provenance is unavailable, but those diagnostics are not proof samples and are not consumed as task outcome evidence.

## Testing Contract

TDD must cover at least:

1. BTF member resolution rejects missing/wrong-width/bitfield fields;
2. nested cgroup field resolution can be capability-disabled without losing required process identity;
3. assembled program shape contains required event-time reads and ringbuf output without hard-coded kernel offsets;
4. raw event decoder rejects wrong version/size/zero identity/impossible timestamps;
5. exact amd64/arm64 `start_boottime_ns -> starttime_ticks` conversion at boundary values;
6. signed time-namespace offset normalization and overflow rejection;
7. PID equality with starttime mismatch never correlates;
8. boot ID mismatch never correlates;
9. stale generation never correlates;
10. event before/after realization boottime window never correlates;
11. cgroup-v2 exact ID match may set correlated=true;
12. cgroup-v2 ID mismatch never produces correlated=true;
13. cgroup-v1/unknown cgroup ID may still produce exact process-victim proof but not cgroup correlation;
14. bounded store eviction/age semantics;
15. duplicate exact events are idempotent;
16. conflicting event identity fails closed;
17. resource-metrics transport preserves max `uint64` values as strings;
18. NolaneWorld parses positive proof and returns `(true,true)`;
19. missing proof returns `(false,false)`;
20. no API or evidence struct contains `OOMKilled`, `TaskOOMKilled`, `GuestOOMKilled` or equivalent victim-to-kill inference;
21. exit 137, SIGKILL and Wave 18 positive OOM delta alone never create Wave 20 proof;
22. collector/BPF absence does not fail sandbox execution.

A dedicated Wave 20 GitHub Actions contract runs deterministic pure-Go layout/conversion/store/correlation/transport tests. Privileged live eBPF attachment tests must be capability-gated and cannot be the only verification of core semantics.

Final readiness additionally requires Wave 17, Wave 18, Wave 19, host-resource, broad Unit Test, Build, Format, DCO, Docs, NolaneWorld and live-substrate gates to remain green.

## Explicit Non-Goals

Wave 20 does not:

- prove the host process received a newly generated SIGKILL from this OOM event;
- prove process exit was caused by OOM;
- prove a guest application process was marked or killed;
- expose a known-negative victim state;
- persist historical kernel victim events across Cubelet restart;
- support non-amd64/non-arm64 exact starttime correlation;
- claim cgroup-v1 event-time cgroup identity;
- parse dmesg/journal text as victim authority;
- synthesize victim proof from exit 137 or Wave 18 counters.

## Next Trust Closure

A later Wave 21 may establish **guest-side victim provenance** inside the sandbox/VM and bridge it to host realization identity. Only that separate trust boundary can justify statements about a particular guest application process.

Even after Wave 21, product-level `OOMKilled` classification must specify exactly which identity domain was killed and which kernel/guest evidence proves causality rather than collapsing host VMM victim evidence into guest workload semantics.
