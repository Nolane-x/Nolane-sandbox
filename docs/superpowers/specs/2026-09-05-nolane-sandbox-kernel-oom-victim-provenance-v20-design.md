# Nolane Sandbox Wave 20 — Kernel OOM Victim Provenance

## Purpose

Wave 20 proves when Linux `mark_oom_victim()` marks the exact Wave 19 host CubeShim/VMM **process** as an OOM victim during a controller realization.

Existing evidence is insufficient on its own:

- Wave 17 proves the exact terminal task outcome and realization generation.
- Wave 18 proves that the sandbox cgroup `oom_kill` counter increased during one realization window.
- Wave 19 proves PID-reuse-resistant host CubeShim/VMM identity and trusted cgroup placement.

None of those identifies the process selected by the kernel OOM path.

Wave 20 may assert only:

> Linux emitted `oom:mark_victim` for a victim thread whose TGID, boot identity and kernel process lifetime exactly match the Wave 19 host CubeShim/VMM process bound to generation G. When cgroup-v2 event-time identity is also available, the victim thread belonged to the exact sandbox cgroup at the event.

Wave 20 must **not** introduce `OOMKilled=true`. Linux can mark a task that is already exiting, so `mark_oom_victim` proves victim marking, not that this call newly killed the task. Guest-process OOM causality remains outside Wave 20.

## Kernel Semantics

`mark_oom_victim(struct task_struct *tsk)` installs kernel OOM-victim state and then invokes `trace_mark_victim(tsk, uid)`.

The ordinary trace-event payload contains PID and memory statistics but not enough lifetime identity to defeat PID reuse. A userspace sequence of "receive PID, then read `/proc/<pid>`" is therefore not authoritative.

Wave 20 captures identity from the original `struct task_struct *` argument at event time in eBPF.

### Thread versus process identity

The marked `task_struct` may represent a non-leader thread with an address space. Therefore:

- `event.PID` is the exact victim **TID**;
- `event.TGID` is the victim process leader ID;
- Wave 19 `HostPID` is process identity and must match **TGID**, not necessarily TID;
- a differing TID is valid when TGID and process lifetime match;
- `PID == HostPID` is never required for process-level correlation.

This distinction is mandatory throughout storage, transport and NolaneWorld parsing.

## Architecture

Wave 20 uses a **BTF-driven dynamically assembled eBPF raw-tracepoint program**.

No checked-in BPF ELF, clang runtime dependency, `bpf2go` generation step or hard-coded `task_struct` offset is required.

Runtime flow:

```text
running kernel BTF
      |
      v
BTF layout resolver
      |
      v
Go-generated cilium/ebpf asm.Instructions
      |
      v
ebpf.RawTracepoint -> oom mark_victim
      |
      v
BPF ring buffer: versioned RawVictimEvent
      |
      v
kernelvictim collector + bounded event store
      |
      v
sandbox controller correlation authority
      |
      +-- Wave 17 exact outcome/generation
      +-- Wave 18 realization OOM window
      +-- Wave 19 exact host process identity
      |
      v
HostProcessKernelOOMVictimProof
      |
      v
resource-metrics exact transport
      |
      v
NolaneWorld one-scrape strict fusion
```

Cubelet already depends on `github.com/cilium/ebpf v0.17.3`; Wave 20 does not introduce another BPF framework.

## Package Boundaries

Create:

```text
Cubelet/plugins/cube/internals/kernelvictim
```

This package owns only:

- BTF layout resolution;
- dynamic eBPF instruction construction;
- raw-tracepoint/ring-buffer collector lifecycle;
- event decoding/validation;
- exact kernel-starttime conversion;
- cgroup-v2 ID resolution;
- bounded positive-event storage;
- capability diagnostics.

It owns **no** sandbox generation, task-outcome, Prometheus or NolaneWorld authority.

The sandbox controller remains the only realization authority. Resource metrics consumes accepted proofs through a primitive/std-library structural visitor, preserving the dependency direction used by Waves 17–19.

## Versioned Kernel Event

Wave 20 uses a fixed v1 record:

```go
type RawVictimEvent struct {
    Version         uint32
    Flags           uint32
    PID             uint32 // victim TID
    TGID            uint32 // process leader ID
    StartBootTimeNS uint64
    EventBootTimeNS uint64
    CgroupV2ID      uint64 // optional; zero = unavailable
}
```

Required validity:

```text
Version == 1
PID != 0
TGID != 0
StartBootTimeNS != 0
EventBootTimeNS != 0
EventBootTimeNS >= StartBootTimeNS
```

`CgroupV2ID == 0` means unknown. It is never interpreted as root cgroup or a known-negative fact.

Required task reads must all succeed before the program emits a record. Failure to read optional cgroup-v2 identity may still emit the exact process-victim record with ID zero.

## BTF Layout Resolver

Load the running kernel BTF using `btf.LoadKernelSpec()` and resolve exact member offsets structurally.

Required `task_struct` members:

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

The resolver must:

- unwrap typedef/const/volatile/restrict wrappers;
- verify pointer-to-struct transitions;
- verify scalar width and non-bitfield layout;
- reject missing or ambiguous members;
- reject offsets not representable by the instruction sequence;
- never cache layout across a different running kernel.

Failure of a required field disables Wave 20 collection. Failure only in the optional cgroup chain disables cgroup-v2 event identity but preserves process-victim collection.

If a running kernel represents `kernfs_node.id` with a different structural shape than the supported exact 64-bit cgroup-ID contract, cgroup correlation is unavailable rather than guessed.

## Dynamic eBPF Program

Construct `asm.Instructions` and load an `ebpf.RawTracepoint` program with GPL-compatible license metadata.

Program semantics:

```text
ctx.args[0] = victim task pointer

read task.pid
read task.tgid
read task.start_boottime
optionally read task default cgroup-v2 ID
call bpf_ktime_get_boot_ns
reserve fixed v1 ringbuf record
write record
submit record
return 0
```

All pointer dereferences use verifier-safe kernel reads. No sandbox ID, generation, path or user-controlled policy is present in BPF.

The ring buffer is runtime-created by Go and referenced by map FD in the generated instructions.

## Attachment and Capability Failure

Attach only to the raw tracepoint corresponding to `oom:mark_victim`.

Unavailable BTF, raw tracepoint support, ring-buffer support, kernel helper support, verifier acceptance, tracing capability or permission means **victim evidence unavailable**.

There is no fallback to:

- dmesg/journal text;
- exit code 137;
- SIGKILL;
- Wave 18 OOM counter delta;
- post-event PID-only `/proc` inspection.

Collector startup failure must not fail Cubelet or sandbox execution.

## Exact Start-Time Bridge

Wave 19 stores `/proc/<pid>/stat` field 22 `starttime_ticks`. Linux derives it from `task->start_boottime` with:

```text
nsec_to_clock_t(timens_add_boottime_ns(task->start_boottime))
```

Important: `/proc` formatting applies the **reader/current process time namespace**, not the target victim task's namespace. Wave 19's inspector and Wave 20's correlator live in the same Cubelet process, so Wave 20 must use the Cubelet process's own `boottime` namespace offset.

For repository production architectures `amd64` and `arm64`, Linux UAPI `USER_HZ` is 100. Exact conversion is:

```text
visible_start_ns = StartBootTimeNS + cubelet_boottime_namespace_offset
starttime_ticks  = floor(visible_start_ns / 10_000_000)
```

Read `/proc/self/timens_offsets` when supported and parse the `boottime` signed seconds/nanoseconds exactly.

If `/proc/self/timens_offsets` is absent, zero offset is allowed only when time namespaces are demonstrably unavailable for the running kernel (for example `/proc/self/ns/time` is absent). Otherwise conversion is unavailable.

Rules:

- only `amd64` and `arm64` are supported in Wave 20;
- no generic `HZ=100` assumption for future architectures;
- integer arithmetic only;
- signed offset normalization is exact;
- overflow/underflow rejects correlation;
- converted ticks must equal Wave 19 `StartTimeTicks` exactly.

`TGID == HostPID` with different starttime is a different process and must fail closed.

## Boot Identity

Read canonical `/proc/sys/kernel/random/boot_id` at collector startup. Each accepted userspace event carries that boot ID.

Victim correlation requires exact equality with Wave 19 `BootID`.

Collector restart can observe only future events. It never reconstructs missed historical events. Reboot changes boot ID and automatically prevents old identity correlation.

## Cgroup-v2 Event-Time Identity

When BTF supports the optional chain, eBPF captures the victim task's default cgroup-v2 kernel ID at event time.

Userspace independently resolves the expected Wave 19 cgroup path to the same ID using `unix.NameToHandleAt` on the discovered cgroup-v2 filesystem. Linux exposes the cgroup-v2 ID through the file-handle API.

Validation requires:

- filesystem is cgroup v2;
- Wave 19 path is canonical absolute hierarchy path;
- joined filesystem path cannot escape the cgroup-v2 mount;
- returned file handle payload is exactly 8 bytes;
- decoded native-endian ID is non-zero;
- event ID equals path-resolved ID exactly.

Unexpected handle layout, mount mismatch, unsupported filesystem, missing path or zero ID means cgroup correlation unavailable.

Wave 20 does not synthesize a cgroup-v2 ID for cgroup v1. On v1, exact process-victim proof may still exist while cgroup-v2 correlation remains unknown.

## Positive-Only Bounded Event Store

The `kernelvictim` store is keyed by process lifetime:

```text
boot_id + tgid + starttime_ticks
```

Victim TID is retained as event provenance but is not the process-lifetime key.

The store uses fixed internal limits:

```text
max events: 1024
max age:    10 minutes by event/collector boottime
```

Tests lock these values and deterministic eviction order.

Exact duplicate events are idempotent. Conflicting records for the same process lifetime are not merged into stronger evidence.

Ring-buffer reservation can fail and the collector can be down, so event absence is never a known-negative fact.

Wave 20 has only:

```text
positive proof -> known true
no proof       -> unknown
```

It never exposes known false.

## Realization Boot-Time Window

Kernel event timestamps use `bpf_ktime_get_boot_ns()`. They must never be compared directly with RFC3339 wall time.

The sandbox controller captures `CLOCK_BOOTTIME` nanoseconds at realization boundaries through a package-neutral clock helper based on `unix.ClockGettime(CLOCK_BOOTTIME)`.

Each Wave 20 generation records:

```go
type victimWindow struct {
    Generation    uint64
    StartedBootNS uint64
    ExitedBootNS  uint64 // zero while open
}
```

`BeginRealization` records `StartedBootNS` after generation authority advances. Exact task outcome closes the window exactly once with `ExitedBootNS`.

Clock capture failure makes Wave 20 correlation unknown but never fails Start/Wait/State.

An accepted event requires:

```text
StartedBootNS <= EventBootTimeNS <= ExitedBootNS
```

for terminal exported proof.

This permits a ring-buffer event consumed after task outcome to be correlated safely by kernel event time without retroactively binding by PID/path alone.

New Start clears prior current candidate state. Create fences all prior sandbox-lifetime Wave 20 state.

## Controller Correlation

The sandbox controller correlates event-store facts with Wave 17/18/19 authority. `kernelvictim` never sees or invents a generation.

Final proof requires all of:

1. exact Wave 17 task outcome exists for generation G;
2. exact Wave 19 host identity exists for G;
3. event boot ID equals Wave 19 boot ID;
4. event TGID equals Wave 19 HostPID;
5. event starttime converts exactly to Wave 19 StartTimeTicks;
6. event boot time lies inside the exact Wave 20 realization boot-time window;
7. Wave 19 cgroup path remains canonical;
8. if Wave 18 OOM proof exists, its generation and cgroup path equal Wave 17/19;
9. if cgroup-v2 correlation is claimed, event ID equals independently resolved Wave 19 path ID;
10. no Create/newer-generation fence superseded the candidate.

Event PID/TID is transported as provenance but does not have to equal HostPID/TGID.

A victim event may arrive while generation G is open and become a candidate. Final transport proof is emitted only after exact task outcome closes G. A late-consumed event may still be accepted only from its immutable kernel event time inside G's closed window.

## Accepted Proof

```go
type HostProcessKernelOOMVictimProof struct {
    SandboxID          string
    Generation         uint64
    BootID             string
    HostPID            uint32 // Wave 19 TGID/process leader
    VictimTID          uint32 // exact task_struct pid marked by kernel
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

`CgroupV2Correlated=true` requires non-zero `CgroupV2ID` and exact independent path-ID match. If unavailable, `CgroupV2Correlated=false` and `CgroupV2ID=0` internally.

## Claim Vocabulary

Public helper:

```go
func (e TaskTerminationEvidence) HostKernelOOMVictimMarked() (marked bool, known bool)
```

Semantics:

```text
accepted Wave 20 proof -> (true, true)
no Wave 20 proof       -> (false, false)
```

There is no `(false, true)` in Wave 20.

Forbidden causal names/claims include:

```text
OOMKilled
TaskOOMKilled
GuestOOMKilled
ApplicationOOMKilled
```

The proof means only that Linux marked the exact Wave 19 **host process** as an OOM victim.

## Prometheus Transport

Export one atomic info-style sample:

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

Rules:

- metric value exactly numeric one;
- all integer values are canonical decimal-string labels;
- boolean exactly `true`/`false`;
- when correlation is false, `cgroup_v2_id` label is empty, never decimal zero;
- resource metrics validates but cannot create/repair proof;
- no OOM-killed metric or label.

## NolaneWorld Consumer

`TaskTerminationEvidence` gains optional `HostKernelOOMVictim` provenance parsed from the same management scrape as Waves 17–19.

Strict requirements:

- exact task outcome must exist;
- Wave 19 host identity must exist;
- sandbox and generation equal exact outcome;
- host PID equals Wave 19 HostPID;
- starttime/boot ID equal Wave 19 exactly;
- victim TID is non-zero but may differ from HostPID;
- cgroup path equals Wave 19 and, when present, Wave 18;
- event source fixed exactly;
- cgroup-v2 correlated proof has canonical non-empty ID;
- non-correlated proof has empty ID;
- duplicate target samples fail closed;
- detached/mismatched victim samples fail closed.

NolaneWorld never opens `/proc`, BPF, tracefs or cgroup files using transported values.

## Failure Semantics

All evidence failures are observational and never workload failures:

- BTF missing/incompatible;
- tracepoint unavailable;
- BPF/ringbuf helper unsupported;
- insufficient privilege;
- verifier rejection;
- malformed or unknown event version;
- unsupported architecture;
- time-namespace conversion unavailable;
- boot ID failure;
- boottime clock failure;
- cgroup-v2 ID resolution failure;
- bounded-store eviction;
- collector restart;
- stale generation;
- process-lifetime mismatch;
- cgroup-ID mismatch.

No evidence failure may cause sandbox Start, Wait or State to fail.

## Security Boundaries

- BTF offsets are validated kernel metadata, never user input.
- BPF reads only pointers originating from the trusted raw tracepoint argument.
- Sandbox IDs and paths remain userspace-only.
- Cgroup path resolution is constrained under the discovered cgroup-v2 mount.
- Transported PID/TID/path fields are data, not execution authority.
- NolaneWorld cannot signal or inspect host processes from transported identity.

## Portability

Exact production victim provenance requires Linux with:

- raw tracepoint BPF support;
- kernel BTF with required task fields;
- ring-buffer support;
- `bpf_ktime_get_boot_ns` support;
- sufficient tracing capability;
- `amd64` or `arm64` exact USER_HZ bridge.

Other hosts continue to execute sandboxes with Wave 20 evidence unknown.

## TDD Contract

Tests must cover at least:

1. required BTF member success and missing/wrong-width/bitfield rejection;
2. optional cgroup BTF chain capability downgrade;
3. dynamic assembler uses injected BTF offsets instead of hard-coded task offsets;
4. ringbuf record decoder rejects wrong size/version/zero fields/impossible event times;
5. victim TID may differ while TGID matches HostPID;
6. TGID mismatch rejects correlation;
7. exact amd64/arm64 starttime conversion;
8. signed time-namespace offsets and overflow/underflow;
9. boot ID mismatch rejects correlation;
10. process starttime mismatch rejects PID reuse;
11. exact realization boottime window acceptance and before/after rejection;
12. cgroup-v2 handle exact 8-byte ID decode;
13. exact cgroup ID match can set `CgroupV2Correlated=true`;
14. mismatch never sets correlated true;
15. cgroup v1/unknown ID still allows process-victim proof with correlation false;
16. bounded 1024-event deterministic eviction and 10-minute age expiry;
17. duplicates are idempotent and conflicts fail closed;
18. max `uint64` Prometheus labels survive exactly;
19. NolaneWorld positive proof returns `(true,true)`;
20. missing proof returns `(false,false)`;
21. no evidence/API shape contains forbidden OOMKilled-style classification;
22. exit 137, SIGKILL, Wave 18 positive delta or Wave 19 identity alone cannot synthesize Wave 20 proof;
23. collector/BPF absence does not fail sandbox execution.

The dedicated Wave 20 contract must be deterministic and runnable without privileged BPF. Privileged live-attach tests are additive/capability-gated, never the sole proof of core semantics.

Final readiness also requires Wave 17, Wave 18, Wave 19, Host Resource, NolaneWorld, Unit Test, Build, Format, DCO, Docs and Live Substrate workflows to remain green.

## Explicit Non-Goals

Wave 20 does not:

- claim `OOMKilled`;
- prove a new SIGKILL was sent by this OOM event;
- prove process exit was caused by OOM;
- prove guest application victim identity;
- expose a known-negative victim result;
- reconstruct events missed during collector downtime;
- support exact non-amd64/non-arm64 starttime correlation;
- claim cgroup-v1 event-time cgroup identity;
- parse logs as victim authority.

## Next Trust Closure

Wave 21 may establish guest-side kernel/process victim provenance and bridge it to the host realization. Only that separate identity domain can justify claims about a particular guest application process.
