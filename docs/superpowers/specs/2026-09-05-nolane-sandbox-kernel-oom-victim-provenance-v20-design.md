# Nolane Sandbox Wave 20 — Kernel OOM Victim Provenance

## Purpose and trust statement

Wave 20 proves when Linux `mark_oom_victim()` marks the exact Wave 19 host CubeShim/VMM **process** as an OOM victim during one Wave 17 realization.

Waves 17–19 already prove exact task outcome/generation, realization-scoped cgroup OOM-counter change, and PID-reuse-resistant host-process identity plus trusted cgroup placement. None identifies the process selected by the kernel OOM path.

Wave 20 may assert only:

> Linux emitted the `oom:mark_victim` tracepoint for a victim thread whose TGID, boot identity and process lifetime exactly match the Wave 19 CubeShim/VMM process bound to generation G. When exact cgroup-v2 event identity is available, that victim thread also belonged to the exact sandbox cgroup at event time.

Wave 20 does **not** add `OOMKilled=true`. Linux can call `mark_oom_victim()` for a task already exiting, so the event proves victim marking, not that the call newly killed the task. Guest-process OOM causality remains a separate Wave 21-style trust boundary.

## Kernel semantics and TID/TGID rule

`mark_oom_victim(struct task_struct *tsk)` installs OOM-victim state and then calls `trace_mark_victim(tsk, uid)`.

The ordinary trace payload lacks enough lifetime identity to defeat PID reuse. Wave 20 therefore reads identity from the original `struct task_struct *` inside eBPF at event time.

The marked `task_struct` may be a non-leader thread. Therefore:

- event `PID` is the exact victim **TID**;
- event `TGID` is the process leader ID;
- Wave 19 `HostPID` must equal **TGID**;
- victim TID may differ from HostPID;
- `PID == HostPID` is never required for process-level proof.

## Architecture

Wave 20 uses a **BTF-driven dynamically assembled eBPF raw-tracepoint program**. Cubelet already depends on `github.com/cilium/ebpf v0.17.3`.

No checked-in BPF ELF, clang runtime dependency, `bpf2go` generation, or hard-coded `task_struct` offsets are required.

```text
running kernel BTF
      -> BTF layout resolver
      -> Go asm.Instructions
      -> ebpf.RawTracepoint `mark_victim`
      -> BPF ring buffer
      -> kernelvictim bounded event store
      -> sandbox-controller Wave17/18/19 correlation
      -> HostProcessKernelOOMVictimProof
      -> resource-metrics exact transport
      -> NolaneWorld one-scrape strict fusion
```

The raw tracepoint global name is `mark_victim`, backing the tracefs event `oom/mark_victim`. If the running kernel does not expose/allow this attachment, evidence is unavailable.

## Package boundary

Create:

```text
Cubelet/plugins/cube/internals/kernelvictim
```

It owns BTF resolution, dynamic BPF assembly, collector/ring-buffer lifecycle, event validation, kernel-starttime conversion, cgroup-v2 ID resolution, a positive-only bounded event store, and capability diagnostics.

It owns no sandbox generation, task outcome, Prometheus, or NolaneWorld authority. The sandbox controller remains the only realization correlation authority. Resource metrics consumes accepted proofs through a primitive/std-library structural visitor.

## Versioned event record

```go
type RawVictimEvent struct {
    Version         uint32
    Flags           uint32
    PID             uint32 // victim TID
    TGID            uint32 // process leader
    StartBootTimeNS uint64
    EventBootTimeNS uint64
    CgroupV2ID      uint64 // zero means unavailable
}
```

Wave 20 requires:

```text
Version == 1
PID != 0
TGID != 0
StartBootTimeNS != 0
EventBootTimeNS != 0
EventBootTimeNS >= StartBootTimeNS
```

Required reads must succeed before emission. Optional cgroup failure may emit with `CgroupV2ID=0`; zero is unknown, never root and never known-negative.

## BTF resolver

Load running-kernel BTF with `btf.LoadKernelSpec()`.

Required `task_struct` fields:

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

The resolver unwraps typedef/const/volatile/restrict, verifies pointer/struct transitions, field width, non-bitfield shape and representable offsets. Missing/wrong required layout disables collection. Failure only in the cgroup chain downgrades cgroup correlation to unknown.

No offsets survive a different running kernel. No fuzzy field-name or byte-pattern lookup is permitted.

## Dynamic eBPF program

Load an `ebpf.RawTracepoint` program with GPL-compatible license metadata. Use verifier-safe kernel reads and a runtime-created `ebpf.RingBuf` map.

```text
ctx.args[0] -> victim task
read pid
read tgid
read start_boottime
optionally read default cgroup-v2 id
call bpf_ktime_get_boot_ns
reserve fixed v1 record
write record
submit
return 0
```

The BPF program contains no sandbox ID, generation, path or policy filtering. It reads only kernel pointers derived from the trusted tracepoint task argument.

Production capability effectively requires a kernel new enough for raw tracepoint + `bpf_ktime_get_boot_ns` + BPF ringbuf; failure of any feature, BTF, verifier, permission or attach is observational-only and never fails Cubelet/sandbox execution.

Forbidden fallbacks: dmesg/journal parsing, exit 137, SIGKILL, Wave 18 OOM delta, or post-event PID-only `/proc` reads.

## Exact Wave 19 start-time bridge

Wave 19 stores `/proc/<pid>/stat` field 22 `starttime_ticks`. Linux derives it from:

```text
nsec_to_clock_t(timens_add_boottime_ns(task->start_boottime))
```

The `/proc` renderer applies the **reader/current process time namespace**, so Wave 20 must use the Cubelet process's own boottime namespace offset, not the victim thread's namespace.

For repository production architectures `amd64` and `arm64`, Linux UAPI `USER_HZ` is 100:

```text
visible_start_ns = StartBootTimeNS + cubelet_boottime_namespace_offset
starttime_ticks  = floor(visible_start_ns / 10_000_000)
```

Parse `/proc/self/timens_offsets` exactly when supported. If it is absent, zero is allowed only when time namespaces are demonstrably unavailable (for example `/proc/self/ns/time` is absent). Otherwise correlation is unknown.

Rules: integer arithmetic only; exact signed offset normalization; overflow/underflow rejection; only amd64/arm64; converted ticks must equal Wave 19 `StartTimeTicks` exactly. TGID equality with starttime mismatch is PID reuse/a different process and is rejected.

## Boot identity

Read canonical `/proc/sys/kernel/random/boot_id` at collector start and tag every userspace event with it. Exact equality with Wave 19 `BootID` is mandatory.

Collector restart observes future events only and cannot reconstruct missed history. Host reboot naturally invalidates old identity by changing boot ID.

## Cgroup-v2 event-time identity

When BTF supports it, read the victim task's default cgroup-v2 kernel ID from its `css_set -> dfl_cgrp -> kernfs_node` chain.

Userspace independently resolves Wave 19 `CGroupPath` through the discovered cgroup-v2 mount with `unix.NameToHandleAt`. Linux exposes cgroup-v2 IDs via the file-handle API.

Require: cgroup-v2 filesystem, canonical absolute hierarchy path, no mount escape, exactly 8 handle bytes, non-zero native-endian uint64, exact equality to event ID.

Unexpected handle layout, mount/path failure or zero ID means cgroup correlation unknown. Wave 20 does not synthesize cgroup-v2 IDs for cgroup v1; exact process-victim proof may still exist on v1 with cgroup correlation false.

## Positive-only bounded store

Key events by:

```text
boot_id + tgid + starttime_ticks
```

Victim TID is provenance, not the process-lifetime key.

Fixed Wave 20 limits:

```text
max events = 1024
max age    = 10 minutes
```

Exact duplicates are idempotent. Conflicting records for one process lifetime are not merged into stronger proof. Deterministic oldest-first eviction is required.

Ring-buffer reserve can fail and the collector can be unavailable, so absence is never known false:

```text
proof present -> (true, true)
proof absent  -> (false, false)
```

## Exact realization boottime window

BPF event time uses `bpf_ktime_get_boot_ns()`. It must never be compared with RFC3339 wall time.

The controller uses a package-neutral helper backed by `unix.ClockGettime(CLOCK_BOOTTIME)` and records:

```go
type victimWindow struct {
    Generation            uint64
    StartedBootNS         uint64
    OutcomeObservedBootNS uint64 // zero while generation is open
}
```

`StartedBootNS` is captured after Wave 17 generation authority advances. `OutcomeObservedBootNS` is captured exactly once when authoritative Wait/stopped-State outcome is accepted. It is deliberately named **observed**, not exited: it is a monotonic upper bound on evidence receipt, not the process's actual exit time.

Final proof requires:

```text
StartedBootNS <= EventBootTimeNS <= OutcomeObservedBootNS
```

This safely permits an event consumed after the terminal response when its immutable kernel timestamp falls inside the closed generation window. Clock capture failure means Wave 20 unknown and does not fail execution.

New Start replaces the current generation/window. Create fences all prior sandbox-lifetime Wave 20 state.

## Controller correlation

`kernelvictim` never receives/invents a generation. The sandbox controller correlates positive events with Waves 17–19.

Accepted proof requires:

1. exact Wave 17 outcome exists for generation G;
2. exact Wave 19 host identity exists for G;
3. event boot ID equals Wave 19 boot ID;
4. event TGID equals Wave 19 HostPID;
5. converted starttime equals Wave 19 StartTimeTicks;
6. event boot time is inside G's exact Wave 20 window;
7. Wave 19 cgroup path is canonical;
8. if Wave 18 proof exists, generation/path equals Waves 17/19;
9. cgroup-v2 correlation true only on exact independently resolved ID match;
10. no Create/newer-generation fence superseded the candidate.

Victim TID may differ from HostPID. An event can become a candidate while G is open, but exported proof exists only after exact task outcome closes G. A late-consumed event can be accepted only by its kernel timestamp and exact lifetime identity, never by PID/path heuristics.

## Accepted proof shape

```go
type HostProcessKernelOOMVictimProof struct {
    SandboxID          string
    Generation         uint64
    BootID             string
    HostPID            uint32 // process/TGID from Wave 19
    VictimTID          uint32 // task_struct pid marked by kernel
    StartTimeTicks     uint64
    CGroupPath         string
    EventBootTimeNS    uint64
    CgroupV2ID         uint64
    CgroupV2Correlated bool
    Source             string
}
```

Fixed source:

```text
kernel.oom.mark_victim.raw_tracepoint
```

`CgroupV2Correlated=true` requires non-zero event/path ID equality. Internally, false correlation carries ID zero.

## Public claim vocabulary

```go
func (e TaskTerminationEvidence) HostKernelOOMVictimMarked() (marked bool, known bool)
```

Only `(true,true)` and `(false,false)` exist in Wave 20.

Forbidden names/claims include `OOMKilled`, `TaskOOMKilled`, `GuestOOMKilled`, `ApplicationOOMKilled`, or any wording that converts host victim marking into guest kill causality.

## Prometheus transport

Export exactly one info metric shape:

```text
cubesandbox_host_kernel_oom_victim_info{
  sandbox_id="sandbox-a",
  generation="18446744073709551615",
  boot_id="11111111-2222-3333-4444-555555555555",
  host_pid="12345",
  victim_tid="12347",
  starttime_ticks="987654321",
  cgroup_path="/cube_sandbox_v1/42",
  event_boot_time_ns="18446744073709551600",
  cgroup_v2_id="123456",
  cgroup_v2_correlated="true",
  source="kernel.oom.mark_victim.raw_tracepoint"
} 1
```

All integers are canonical decimal string labels; boolean is exact `true`/`false`; metric value is numeric one. When correlation is false, `cgroup_v2_id` is empty, never `0`. Resource metrics validates and exports only; it cannot synthesize proof.

## NolaneWorld consumer

`TaskTerminationEvidence` gains optional `HostKernelOOMVictim` parsed from the same scrape as Waves 17–19.

Fail closed on: no exact outcome, no Wave 19 identity, sandbox/generation mismatch, host PID/starttime/boot mismatch, zero victim TID, cgroup-path mismatch, unsupported source, malformed cgroup-v2 state, duplicate target sample, detached victim sample, non-unit metric or noncanonical integer.

Victim TID may differ from HostPID. NolaneWorld never opens `/proc`, BPF, tracefs or cgroup files using transported values.

## Failure and security semantics

All of the following mean Wave 20 unknown and never workload failure: BTF missing/incompatible; raw tracepoint unavailable; helper/ringbuf unsupported; insufficient privilege; verifier rejection; malformed record; unsupported architecture; time-namespace bridge unavailable; boot ID/boottime clock failure; cgroup-v2 resolution failure; store eviction; collector restart; stale generation; lifetime mismatch; cgroup-ID mismatch.

BTF offsets are validated kernel metadata. BPF reads only pointers from the tracepoint task argument. Sandbox IDs/paths remain userspace-only. Transported PID/TID/path fields are data, never executable authority.

## TDD contract

Tests must cover at least:

1. required BTF member success and missing/wrong-width/bitfield rejection;
2. optional cgroup-chain downgrade;
3. assembler uses injected BTF offsets rather than hard-coded kernel offsets;
4. decoder rejects wrong size/version/zero fields/impossible event time;
5. TID may differ while TGID matches HostPID;
6. TGID mismatch rejects correlation;
7. exact amd64/arm64 starttime conversion;
8. signed time-namespace offsets and overflow/underflow;
9. boot mismatch and starttime mismatch rejection;
10. exact realization window acceptance and before/after rejection;
11. cgroup-v2 exact 8-byte handle decoding;
12. exact cgroup ID match can set correlation true; mismatch cannot;
13. cgroup-v1/unknown ID still permits process-victim proof with correlation false;
14. 1024-event deterministic eviction and 10-minute expiry;
15. duplicate idempotency and conflict fail-closed;
16. max uint64 Prometheus labels remain exact;
17. NolaneWorld positive proof -> `(true,true)`, absence -> `(false,false)`;
18. no API/evidence shape contains OOMKilled-style classification;
19. exit 137, SIGKILL, Wave 18 delta, or Wave 19 identity alone cannot synthesize proof;
20. collector/BPF absence never fails sandbox execution.

The dedicated Wave 20 contract is deterministic and unprivileged for core semantics. Privileged live attach is additive/capability-gated, never the only proof.

Final readiness also requires Wave 17, Wave 18, Wave 19, Host Resource, NolaneWorld, Unit Test, Build, Format, DCO, Docs and Live Substrate workflows to remain green.

## Explicit non-goals

Wave 20 does not prove a new SIGKILL was sent, prove exit was caused by OOM, prove guest-process victim identity, expose a known-negative victim result, reconstruct collector downtime, support exact non-amd64/non-arm64 correlation, claim cgroup-v1 event-time identity, or parse logs as victim authority.

Wave 21 may add guest-side victim provenance and an explicit host/guest identity bridge.
