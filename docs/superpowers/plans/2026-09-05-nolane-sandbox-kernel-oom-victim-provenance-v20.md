# Kernel OOM Victim Provenance V20 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture Linux `oom:mark_victim` at event time, correlate it to the exact Wave 19 host CubeShim/VMM process and Wave 17 realization, and export a positive-only `KernelOOMVictimMarked` proof without introducing `OOMKilled` causality.

**Architecture:** A package-neutral `kernelvictim` subsystem dynamically assembles a BTF-resolved raw-tracepoint eBPF program and writes versioned events into a bounded positive-only store. The sandbox controller remains generation authority, records a `CLOCK_BOOTTIME` realization window, correlates exact Wave 19 identity plus optional cgroup-v2 ID, and exposes immutable accepted proof through a structural resource-metrics visitor. NolaneWorld parses Wave 17–20 evidence from one scrape and fails closed on detached or mismatched victim samples.

**Tech Stack:** Go 1.24.8; `github.com/cilium/ebpf v0.17.3` (`asm`, `btf`, `link`, `ringbuf`); `golang.org/x/sys/unix`; Prometheus client; existing containerd plugin architecture; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-05-nolane-sandbox-kernel-oom-victim-provenance-v20-design.md`

## Global Constraints

- The only Wave 20 positive claim is that Linux marked the exact Wave 19 host process as an OOM victim.
- Never add `OOMKilled`, `TaskOOMKilled`, `GuestOOMKilled`, `ApplicationOOMKilled`, or infer victim proof from exit 137, SIGKILL, Wave 18 counter delta, or PID equality alone.
- Wave 19 `HostPID` matches event `TGID`; event `PID` is victim TID and may differ.
- BPF contains no sandbox ID, generation, cgroup path, or policy filtering.
- Required BTF mismatch disables collection; optional cgroup-chain mismatch disables only cgroup-v2 correlation.
- Evidence failures never fail Cubelet initialization or sandbox Start/Wait/State.
- No checked-in BPF ELF, hard-coded `task_struct` offsets, dmesg parsing, or post-event PID-only `/proc` fallback.
- All transported integers are canonical decimal string labels. No arbitrary uint64 travels through binary64.
- Event absence is unknown, never known false.
- Production exact start-time bridge is limited to amd64 and arm64.
- Every authored commit includes `Autonomously-by: ChatGPT:GPT-5.6-Sol` and never adds DCO `Signed-off-by`.

---

### Task 1: Define deterministic kernel-victim primitives and RED contract

**Files:**
- Create: `Cubelet/plugins/cube/internals/kernelvictim/event.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/event_v20_test.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/store.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/store_v20_test.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/timebridge.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/timebridge_v20_test.go`

**Interfaces:**

```go
package kernelvictim

const (
    EventVersionV1 uint32 = 1
    ProofSource = "kernel.oom.mark_victim.raw_tracepoint"
)

type RawVictimEvent struct {
    Version         uint32
    Flags           uint32
    PID             uint32
    TGID            uint32
    StartBootTimeNS uint64
    EventBootTimeNS uint64
    CgroupV2ID      uint64
}

type Event struct {
    BootID          string
    VictimTID       uint32
    TGID            uint32
    StartTimeTicks  uint64
    EventBootTimeNS uint64
    CgroupV2ID      uint64
}

func DecodeRawVictimEvent(raw []byte) (RawVictimEvent, error)
func StartTimeTicks(startBootNS uint64, offsetSeconds int64, offsetNanoseconds int64, goarch string) (uint64, error)
func ParseTimeNamespaceBootOffset(raw []byte) (seconds int64, nanoseconds int64, err error)

type Store struct { /* private fields only */ }
func NewStore() *Store
func (s *Store) Add(Event) error
func (s *Store) Find(bootID string, tgid uint32, startTimeTicks, minBootNS, maxBootNS uint64) (Event, bool)
```

The store constants are exactly:

```go
const maxStoredEvents = 1024
const maxEventAgeBootNS uint64 = 10 * 60 * 1_000_000_000
```

- [ ] **Step 1: Write event decoder RED tests**

Use `encoding/binary.LittleEndian` to create an exact 40-byte v1 record and assert decode fields. Add independent cases for wrong byte length, version != 1, zero PID, zero TGID, zero start time, zero event time, and event time before process start. The production change that makes these tests pass is only `DecodeRawVictimEvent` validation.

- [ ] **Step 2: Write start-time/time-namespace RED tests**

Exact cases:

```go
StartTimeTicks(12_345_678_999, 0, 0, "amd64") == 1234
StartTimeTicks(12_345_678_999, 0, 0, "arm64") == 1234
StartTimeTicks(12_345_678_999, 1, 500_000_000, "amd64") == 1384
```

Reject `386`, negative visible time, nanoseconds outside `[-999999999,999999999]`, and signed/unsigned overflow. Parse lines in `/proc/self/timens_offsets` form and require exactly one canonical `boottime` row.

- [ ] **Step 3: Write bounded-store RED tests**

Insert 1025 monotonically increasing events and assert the oldest is evicted. Assert events older than exactly 10 minutes relative to the newest event are expired, exact duplicate Add is idempotent, and same lifetime key with conflicting victim TID/event timestamp returns an error without strengthening the stored fact.

- [ ] **Step 4: Commit RED primitives tests only**

Expected contract compile failure: `DecodeRawVictimEvent`, `StartTimeTicks`, `ParseTimeNamespaceBootOffset`, `NewStore` or their behaviors do not exist yet. Do not add production code before observing this failure in the dedicated Wave 20 workflow from Task 6.

- [ ] **Step 5: Implement minimal primitives after RED is verified**

`DecodeRawVictimEvent` uses exactly 40 bytes and explicit little-endian field offsets. `StartTimeTicks` uses integer arithmetic and 10,000,000 ns/tick only for amd64/arm64. `Store` keeps deterministic insertion order and rebuilds/compacts on eviction rather than using map iteration order.

- [ ] **Step 6: Run focused tests and commit GREEN primitives**

Run:

```bash
cd Cubelet
go test ./plugins/cube/internals/kernelvictim -run 'V20|Victim|StartTime|Store' -count=1
```

Expected: PASS.

---

### Task 2: BTF layout resolver and dynamically assembled raw-tracepoint program

**Files:**
- Create: `Cubelet/plugins/cube/internals/kernelvictim/layout.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/layout_v20_test.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/program.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/program_v20_test.go`

**Interfaces:**

```go
type Layout struct {
    PIDOffset           int16
    TGIDOffset          int16
    StartBootTimeOffset int16
    Cgroup              *CgroupV2Layout
}

type CgroupV2Layout struct {
    TaskCgroupsOffset int16
    DefaultCgroupOffset int16
    KernfsNodeOffset int16
    KernfsIDOffset int16
}

func ResolveLayout(spec *btf.Spec) (Layout, error)
func BuildProgramSpec(layout Layout, events *ebpf.Map) (*ebpf.ProgramSpec, error)
```

- [ ] **Step 1: Write BTF layout RED tests using synthetic BTF types**

Construct `btf.Struct`/`btf.Pointer` graphs with non-default offsets. Assert exact resolved offsets. Separate tests remove `pid`, change `start_boottime` to a 32-bit integer, set a bitfield, and break an optional cgroup pointer. Required errors must reject; optional chain break must return `Layout{Cgroup:nil}`.

- [ ] **Step 2: Write assembler RED tests**

Create an `ebpf.MapSpec{Type: ebpf.RingBuf, MaxEntries: 4096}` map only when the test kernel allows it; for deterministic instruction-shape tests use a small internal `buildInstructions(layout, mapFD int)` helper and arbitrary non-default offsets such as 12, 28, 104. Assert those immediates appear in kernel-read address calculations and that the instruction list includes `asm.FnKtimeGetBootNs`, `asm.FnRingbufReserve`, and `asm.FnRingbufSubmit`.

- [ ] **Step 3: Implement exact BTF unwrapping and field validation**

Use `btf.Type` inspection; recursively unwrap `*btf.Typedef`, `*btf.Const`, `*btf.Volatile`, `*btf.Restrict`. Require 32-bit signed integer shape for pid/tgid and 64-bit scalar for start_boottime. `Member.BitfieldSize != 0` rejects. Convert `member.Offset.Bytes()` to int16 only after bounds checking.

- [ ] **Step 4: Implement generated eBPF program**

The raw context load obtains `ctx.args[0]`; each task/cgroup dereference uses `asm.FnProbeReadKernel`. Reserve a fixed 40-byte ringbuf record, write version/flags/PID/TGID/start/event/cgroup fields, submit, and return zero. Optional cgroup-chain read failure branches to record emission with ID zero rather than dropping required process evidence.

- [ ] **Step 5: Verify focused tests and commit**

Run:

```bash
cd Cubelet
go test ./plugins/cube/internals/kernelvictim -run 'Layout|Program|V20' -count=1
```

Expected: PASS without privileged BPF attach.

---

### Task 3: Live collector, boot identity and cgroup-v2 resolver

**Files:**
- Create: `Cubelet/plugins/cube/internals/kernelvictim/collector_linux.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/collector_other.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/collector_v20_test.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/cgroupv2.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/cgroupv2_v20_test.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/boottime_linux.go`
- Create: `Cubelet/plugins/cube/internals/kernelvictim/boottime_other.go`

**Interfaces:**

```go
type Source interface {
    Find(bootID string, tgid uint32, startTimeTicks, minBootNS, maxBootNS uint64) (Event, bool)
}

func StartBestEffortCollector(ctx context.Context) (Source, error)
func BootTimeNS() (uint64, error)
func ResolveCgroupV2ID(cgroupPath string) (uint64, bool)
```

- [ ] **Step 1: Write decoder-to-store collector tests without attaching BPF**

Inject `io.Reader`/record-processing helpers and prove valid raw records become `Event` only after canonical boot ID and exact starttime conversion. Invalid records, invalid boot ID, unsupported architecture or time-offset parse failure are dropped/reported as capability loss rather than creating events.

- [ ] **Step 2: Write cgroup-v2 pure resolver tests**

Split path validation from syscall execution. Test canonical `/cube_sandbox_v1/42`, reject `/`, `../`, duplicate-clean variants and mount escape. Add a helper `decodeCgroupV2Handle([]byte) (uint64,error)` that accepts exactly 8 bytes, uses native endianness, rejects zero and any other length.

- [ ] **Step 3: Implement `CLOCK_BOOTTIME` and non-Linux stubs**

Linux calls `unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts)` and checks nonnegative seconds/nanoseconds plus uint64 overflow. Non-Linux returns unsupported.

- [ ] **Step 4: Implement best-effort live collector**

`StartBestEffortCollector` loads kernel BTF, resolves Layout, creates `ebpf.RingBuf`, builds/loads program, attaches `link.AttachRawTracepoint(link.RawTracepointOptions{Name:"mark_victim", Program:prog})`, opens `ringbuf.NewReader`, then consumes records until context cancellation. Any initialization error is returned to the caller, which must treat it as observational-only.

- [ ] **Step 5: Implement cgroup-v2 path ID resolution**

Parse `/proc/self/mountinfo` for one cgroup2 mount visible to Cubelet, validate joined path remains beneath that mount, call `unix.NameToHandleAt(unix.AT_FDCWD, fullPath, 0)`, require exactly 8 handle bytes, and decode native-endian nonzero ID. Return `(0,false)` on any capability/path/syscall mismatch.

- [ ] **Step 6: Verify package tests and commit**

Run all unprivileged tests. Live attach test skips unless BTF, privileges and helpers are genuinely available; skip is not used to prove core semantics.

---

### Task 4: Bind positive victim facts to Wave 17/19 realization authority

**Files:**
- Modify: `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go`
- Modify: `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/kernel_oom_victim.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/kernel_oom_victim_v20_test.go`

**Interfaces:**

Add to `taskOutcomeProofStore`:

```go
victimWindows map[string]victimWindow
kernelVictimProofs map[string]HostProcessKernelOOMVictimProof
```

Types:

```go
type victimWindow struct {
    Generation            uint64
    StartedBootNS         uint64
    OutcomeObservedBootNS uint64
}

type HostProcessKernelOOMVictimProof struct {
    SandboxID          string
    Generation         uint64
    BootID             string
    HostPID            uint32
    VictimTID          uint32
    StartTimeTicks     uint64
    CGroupPath         string
    EventBootTimeNS    uint64
    CgroupV2ID         uint64
    CgroupV2Correlated bool
    Source             string
}
```

Controller fields:

```go
kernelVictimSource kernelvictim.Source
bootTimeNS func() (uint64,error)
cgroupV2IDResolver func(string) (uint64,bool)
```

Visitor:

```go
func (c *controllerLocal) VisitHostKernelOOMVictimProofs(
    visit func(string, uint64, string, uint32, uint32, uint64, string, uint64, uint64, bool, string),
)
```

- [ ] **Step 1: Write RED lifecycle/correlation tests**

Cover TID != HostPID while TGID matches; TGID mismatch; starttime mismatch; boot mismatch; event before Start; event after `OutcomeObservedBootNS`; Create fence; new Start generation; cgroup ID exact match/mismatch; cgroup ID unavailable with process proof still accepted; and absence of source producing no proof without Start/Wait error.

- [ ] **Step 2: Add realization boottime window lifecycle**

`Start` calls `BeginRealization`, then best-effort `BootTimeNS`; store only when generation is still current. `Clear` deletes windows/proofs. `BeginRealization` deletes previous proof/window. When exact Wait/stopped-State proof is accepted, capture `OutcomeObservedBootNS` exactly once and close the matching generation window.

- [ ] **Step 3: Correlate only after exact terminal outcome**

At outcome closure, query `kernelVictimSource.Find(binding.BootID, binding.HostPID, binding.StartTimeTicks, StartedBootNS, OutcomeObservedBootNS)`. Build accepted proof only when exact Wave 19 binding generation matches the task outcome. Resolve cgroup-v2 ID: if both expected/event IDs are nonzero and equal, set correlated true; if event ID is zero or resolver unavailable, preserve process proof with false/zero; if both are available and differ, do not strengthen to correlated true.

- [ ] **Step 4: Wire best-effort collector during controller plugin init**

Call `kernelvictim.StartBestEffortCollector(ic.Context)`. On error leave source nil and continue initialization. Assign `bootTimeNS=kernelvictim.BootTimeNS` and resolver. No plugin-init error may be returned solely from Wave 20 evidence initialization.

- [ ] **Step 5: Verify sandbox tests and prior Wave 17–19 tests**

Run focused sandbox V20 tests plus all existing `task_outcome`, `realization_oom`, and `host_process_identity` contract tests.

---

### Task 5: Exact Prometheus transport and NolaneWorld one-scrape fusion

**Files:**
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/kernel_oom_victim_prometheus.go`
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/kernel_oom_victim_v20_test.go`
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/realization_oom_prometheus.go`
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/plugin.go`
- Create: `NolaneWorld/substrate/cube/kernel_oom_victim.go`
- Create: `NolaneWorld/substrate/cube/kernel_oom_victim_v20_test.go`
- Modify: `NolaneWorld/substrate/cube/task_termination.go`

**Interfaces:**

Resource metrics visitor:

```go
type hostKernelOOMVictimProofVisitor interface {
    VisitHostKernelOOMVictimProofs(func(
        sandboxID string, generation uint64, bootID string,
        hostPID, victimTID uint32, startTimeTicks uint64,
        cgroupPath string, eventBootTimeNS, cgroupV2ID uint64,
        cgroupV2Correlated bool, source string,
    ))
}
```

NolaneWorld:

```go
type HostKernelOOMVictimProof struct {
    SandboxID string
    Generation uint64
    BootID string
    HostPID uint32
    VictimTID uint32
    StartTimeTicks uint64
    CGroupPath string
    EventBootTimeNS uint64
    CgroupV2ID uint64
    CgroupV2Correlated bool
    Source string
}

func (e TaskTerminationEvidence) HostKernelOOMVictimMarked() (bool, bool)
```

- [ ] **Step 1: Write RED Prometheus exactness tests**

Use `math.MaxUint64` for generation/starttime/event/cgroup ID and assert labels contain exact decimal text. For `CgroupV2Correlated=false`, assert `cgroup_v2_id=""`. Reject zero PID/TID/starttime/event, bad boot UUID/path/source, false+nonzero ID, true+zero ID.

- [ ] **Step 2: Add collector to the existing all-evidence registry**

Extend `newServiceWithAllTaskEvidence` / `newPrometheusHandlerWithAllTaskEvidence` with one extra optional visitor parameter and register `hostKernelOOMVictimPrometheusCollector` only when non-nil. Preserve old helper functions by passing nil.

- [ ] **Step 3: Write RED NolaneWorld fusion tests**

One scrape includes outcome + Wave 19 identity + victim sample. Assert positive helper `(true,true)` and TID may differ. Add independent fail-closed cases for detached victim sample, missing Wave19 identity, duplicate sample, generation/boot/hostPID/starttime/cgroup mismatch, malformed uint64, invalid boolean/source, correlated-with-empty-ID, uncorrelated-with-nonempty-ID. Outcome/identity without victim must return `(false,false)`.

- [ ] **Step 4: Implement strict parser/correlation**

Metric name exactly `cubesandbox_host_kernel_oom_victim_info`, exactly eleven labels from the spec, unit value exactly 1, canonical integers, lowercase canonical UUID, canonical cgroup path, fixed source. Correlate only after outcome and Wave 19 identity are available from the same scrape.

- [ ] **Step 5: Add structural guard against kill-causality fields**

Reflection test over `TaskTerminationEvidence` and `HostKernelOOMVictimProof` rejects exported field names containing `OOMKilled`, `TaskOOMKilled`, `GuestOOMKilled`, or `ApplicationOOMKilled`. Tests separately prove exit 137 + Wave18 positive delta + Wave19 identity with no Wave20 metric remains `(false,false)`.

- [ ] **Step 6: Verify focused Cubelet/NolaneWorld tests and commit**

Run exact V20 packages and all existing Wave17–19 consumer tests.

---

### Task 6: Dedicated RED/GREEN workflow, stacked PR and final verification

**Files:**
- Create: `.github/workflows/cube-kernel-oom-victim-contract.yml`

**Workflow:**

```yaml
name: Cube Kernel OOM Victim Contract
on:
  pull_request:
  push:
    branches:
      - gpt/wave20-kernel-oom-victim-provenance
jobs:
  contract:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: Cubelet/go.mod
      - name: Verify kernel-victim pure contract
        working-directory: Cubelet
        run: go test ./plugins/cube/internals/kernelvictim ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics -run 'V20|KernelOOMVictim|Victim' -count=1
      - name: Verify NolaneWorld victim consumer
        working-directory: NolaneWorld
        run: go test ./substrate/cube -run 'V20|KernelOOMVictim|Victim' -count=1
```

- [ ] **Step 1: Commit RED tests + workflow before production code**

Open Draft PR with base `gpt/wave19-host-task-identity-proof`, head `gpt/wave20-kernel-oom-victim-provenance`, title `Wave 20: kernel OOM victim provenance`. PR body states RED scope, exact trust boundary, and autonomous trailer.

- [ ] **Step 2: Verify Actions RED**

The dedicated workflow must fail only because Wave 20 symbols/behavior are absent. If it fails from setup/native dependencies/test syntax, fix the harness and re-run RED before implementing production code.

- [ ] **Step 3: Execute Tasks 1–5 GREEN in small commits**

Do not weaken a RED assertion to make GREEN pass. Treat unexpected failures with systematic-debugging before changing implementation.

- [ ] **Step 4: Run final-head focused and broad verification**

Require success on final Wave 20 head for:

```text
Cube Kernel OOM Victim Contract
Cube Host Process Identity Contract
Cube Realization OOM Contract
Cube Task Outcome Contract
Cube Host Resource Contract
Nolane World Check
Nolane Live Substrate Gauntlet
Unit Test Check
Build Check
Format Check
DCO Check
Docs Build Check
Pages
```

Inspect the actual final SHA and workflow conclusions; do not reuse success from an older head.

- [ ] **Step 5: Trust audit final diff**

Search production diff for `OOMKilled`, `exit 137`, `SIGKILL`, log parsing, hard-coded `task_struct` offsets, and any metrics-side proof creation. Permitted occurrences are negative tests/docs only. Confirm no BPF code receives sandbox ID/generation/path.

- [ ] **Step 6: Update PR body and mark ready only after all final-head gates are green**

Record RED run, focused GREEN run, final SHA, all green workflow IDs, capability limitations, TID/TGID rule, and the exact autonomous trailer. Do not merge the stacked PR or master automatically.
