# Guest Kernel OOM Victim Provenance V21 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or subagent-driven-development task-by-task. Follow strict TDD: observe RED before production code, then make the smallest GREEN change that preserves every Wave 17–20 trust boundary.

**Goal:** Prove, without exit-code/cgroup heuristics, that the guest Linux kernel emitted `oom:mark_victim` for an exact guest workload process lifetime belonging to one exact Wave 17 realization, and expose only positive MAIN/MEMBER victim-marking facts through CubeShim, Cubelet metrics, and NolaneWorld.

**Architecture:** Wave 17 remains the sole generation authority. `controllerLocal.Start` opens generation G and creates an unpredictable 32-byte token T in a generation-owned Wave 21 state. A package-neutral pre-start bridge exposes T to the real CubeBox main-task start path, which performs the one-shot CubeShim `BindOOMVictimRealization` Update before `task.Start`; lack of an exact current token/bind leaves Wave 21 unavailable and never blocks the workload. CubeShim atomically consumes a non-poisoned pending token on the next main Start, sends it only in guest `StartContainerRequest`, and after guest Wait fetches finalized evidence with the exact token. The guest agent owns a bounded best-effort `oom:mark_victim` collector, guest-local realization/loss epochs, exact main lifetime/cgroup correlation, and positive finalized evidence. CubeShim caches immutable exact-token evidence and exposes it only through Task Stats request metadata `cube-wave21-guest-oom-evidence=<64 lowercase hex token>`. Cubelet makes one terminal post-outcome retrieval attempt and exports accepted proof. NolaneWorld performs strict same-scrape correlation with Wave 17.

**Normative specs:**
- `docs/superpowers/specs/2026-09-05-nolane-sandbox-guest-kernel-oom-victim-provenance-v21-design.md`
- `docs/superpowers/specs/2026-09-05-nolane-sandbox-guest-kernel-oom-victim-provenance-v21-trust-closure-amendment.md`

## Global trust constraints

- The amendment is normative where it conflicts with the base design.
- No `OOMKilled`, `GuestOOMKilled`, `TaskOOMKilled`, `ApplicationOOMKilled`, or causal exit claim is introduced.
- Exit 137, SIGKILL, existing `GetOOMEvent`/TaskOOM, Wave 18 cgroup `oom_kill`, Wave 19 host identity, Wave 20 host victim proof, and guest `memory.events` are never substitutes for Wave 21.
- Exact token length is 32 bytes; textual form is canonical 64-character lowercase hex; all-zero is invalid.
- Conflicting CubeShim pre-start binds terminally POISON that pending slot. They never replace the first token.
- Task Stats metadata is the only host selector for cached Wave 21 evidence. Missing/wrong/stale/malformed metadata never leaks evidence and preserves ordinary Stats metrics behavior.
- Guest collector loss increments `loss_epoch`; any realization whose start/final loss epochs differ is evidence-unavailable even if positive records survived.
- Guest finalization is owned only by exact main `WaitProcess`; host stopped-State cannot manufacture guest finalization.
- One host generation gets at most one terminal Wave 21 finalization attempt. Later reconnect/Stats/Wait cannot repair terminal unknown.
- Evidence failures are observational-only at every layer and never fail agent startup, Create, Start, Wait, State, Delete, or workload metrics.
- Every transported uint64 is encoded/parses losslessly as canonical decimal text; no float64 intermediary.
- Every authored commit/PR includes exactly `Autonomously-by: ChatGPT:GPT-5.6-Sol`; never add DCO `Signed-off-by`.

---

### Task 1: RED the cross-language token lifecycle before any collector implementation

**Files:**
- Create: `Cubelet/plugins/cube/internals/sandbox/guest_oom_victim_v21_test.go`
- Create: `Cubelet/services/cubebox/guest_oom_victim_start_v21_test.go`
- Create: `CubeShim/shim/src/service/guest_oom_victim_v21_tests.rs`
- Modify test module wiring in: `CubeShim/shim/src/service/mod.rs` only if needed for Rust test discovery
- Create: `.github/workflows/cube-guest-kernel-oom-victim-contract.yml`

**RED interfaces expected to be missing:**

```go
// sandbox package
type GuestProcessKernelOOMVictimProof struct { /* spec fields */ }
func (s *taskOutcomeProofStore) BeginGuestOOMVictimRealization(sandboxID string, generation uint64, token [32]byte) error
func (s *taskOutcomeProofStore) GuestOOMVictimToken(sandboxID string, generation uint64) ([32]byte, bool)
func (s *taskOutcomeProofStore) FinalizeGuestOOMVictimUnknown(sandboxID string, generation uint64)

// package-neutral pre-start bridge
type StartBinding struct { SandboxID string; Generation uint64; Token [32]byte }
func CurrentStartBinding(sandboxID string) (StartBinding, bool)
```

**Steps:**
- [ ] Add sandbox RED tests proving new Start invalidates old token/proofs, Create fences state, controller restart cannot reconstruct token, CSPRNG failure leaves workload Start successful/unknown, and stale generation callbacks cannot attach.
- [ ] Add CubeBox RED seam with a fake task lifecycle that records `Update` and `Start` calls. Exact current token must produce `Update(BindOOMVictimRealization,T)` strictly before main `Start`; absent/stale token must call Start without bind; bind error must still call Start and mark evidence unavailable.
- [ ] Add CubeShim RED tests for pending slot state machine: empty+T→pending, duplicate T→pending, T then T2→POISONED, POISONED remains poisoned, main Start consumes/clears, exec Start never consumes, failed main Start never recycles, delete clears.
- [ ] Add strict token parser tests: exactly 64 lowercase hex and 32 nonzero bytes; uppercase, odd length, nonhex, all-zero reject.
- [ ] Add dedicated workflow running Go sandbox/CubeBox tests plus CubeShim Rust tests and later agent/NolaneWorld V21 tests.
- [ ] Commit tests/workflow only and verify the dedicated Actions run fails for missing Wave 21 symbols/behavior rather than syntax/setup.

**Focused RED workflow name:** `Cube Guest Kernel OOM Victim Contract`.

---

### Task 2: Implement the host generation token and real pre-Task.Start bind seam

**Files:**
- Create: `Cubelet/plugins/cube/internals/guestvictimbridge/bridge.go`
- Create: `Cubelet/plugins/cube/internals/guestvictimbridge/bridge_v21_test.go`
- Modify: `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go`
- Create: `Cubelet/plugins/cube/internals/sandbox/guest_oom_victim.go`
- Modify: `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- Modify: `Cubelet/services/cubebox/cube_container_create.go`

**Design:**
- `taskOutcomeProofStore.BeginRealization` remains the only generation increment.
- Immediately after generation G is opened, `controllerLocal.Start` calls `crypto/rand.Read` for T and stores T only under current G, then publishes `(sandbox,G,T)` to `guestvictimbridge`.
- `guestvictimbridge` never increments or guesses generations. It is a bounded in-memory handoff keyed by sandbox and exact generation, with explicit Clear/Unavailable/Bound transitions.
- The real CubeBox `runContainer` path for `ci.IsPod` checks the bridge after `NewTask` and before `task.Start`. It performs a direct Task Update extension bind against the exact just-created task/shim, then calls `task.Start` regardless of evidence bind success.
- Non-pod/exec starts never bind/consume Wave 21 state.
- Create/new Start clears stale bridge state; bind success/failure is reported back only if generation/token still match.

**Steps:**
- [ ] Implement immutable token generation and exact current-generation storage.
- [ ] Implement bridge state machine with stale-token rejection and terminal unavailable state; all slow Task RPC work stays outside bridge/store locks.
- [ ] Add a narrow CubeBox helper `bindGuestOOMVictimBeforeStart(ctx, task, sandboxID)` whose test seam can inject Update/Start behavior without a live shim.
- [ ] Encode Update annotations exactly:
  - `cube.shimapi.update.action=BindOOMVictimRealization`
  - `cube.shimapi.update.oom_victim_realization_token=<token hex>`
- [ ] Prove fake-call order `NewTask -> Wait registration -> Bind Update -> Start`; Wave 21 bind failure does not change existing Start result unless Start itself fails.
- [ ] Re-run Wave 16–20 sandbox/CubeBox contracts to prove generation, cgroup, host identity, and host victim behavior unchanged.

---

### Task 3: Synchronize guest protocol and implement CubeShim one-shot/POISONED bridge

**Files:**
- Modify: `agent/libs/protocols/protos/agent.proto`
- Modify: `CubeShim/protoc/protos/agent.proto`
- Regenerate, do not hand edit: protocol generated Rust files under `agent/libs/protocols/src/` and `CubeShim/protoc/src/`
- Modify: `CubeShim/shim/src/sandbox/sb.rs`
- Modify: `CubeShim/shim/src/container/mod.rs`
- Modify: `CubeShim/shim/src/service/update_ext.rs`
- Modify: `CubeShim/shim/src/service/task_srv.rs`
- Create: `CubeShim/shim/src/service/guest_oom_victim.rs`

**Protocol changes:**

```proto
message StartContainerRequest {
  string container_id = 1;
  bytes oom_victim_realization_token = 2;
}

rpc GetOOMVictimEvidence(GetOOMVictimEvidenceRequest) returns (GetOOMVictimEvidenceResponse);
message GetOOMVictimEvidenceRequest { string container_id = 1; bytes realization_token = 2; }
enum OOMVictimScope { OOM_VICTIM_SCOPE_UNSPECIFIED = 0; OOM_VICTIM_SCOPE_MAIN = 1; OOM_VICTIM_SCOPE_MEMBER = 2; }
message OOMVictimEvidence { /* exact fields 1..15 from spec */ }
message GetOOMVictimEvidenceResponse { repeated OOMVictimEvidence evidence = 1; }
```

**Steps:**
- [ ] Add a `PendingOOMVictimBinding` enum (`Empty`, `Pending([u8;32])`, `Poisoned`) owned under sandbox/task synchronization, plus bounded immutable finalized cache keyed `(container_id, token)`.
- [ ] Route `BindOOMVictimRealization` in `update_ext.rs`; exact duplicate is idempotent, conflict poisons, malformed bind is rejected as extension error but never mutates workload state.
- [ ] On main `TaskService.start`, atomically consume pending state before guest request; poisoned means send no token. Exec start never touches it.
- [ ] Extend `Container::start_container` to accept optional exact token and populate only `StartContainerRequest.oom_victim_realization_token`.
- [ ] After exact guest main Wait completes, call `GetOOMVictimEvidence(container_id, consumed token)` at most once and cache only a fully validated immutable set. Empty/unsupported/malformed/error => unknown; never alter Wait result.
- [ ] Add proof-set validator enforcing version, token/container equality, canonical UUID/source/scope, nonzero identities, window, MAIN lifetime equality, MEMBER cgroup requirement, deterministic max 64 records, duplicate/conflict rejection.
- [ ] Add a dedicated `Any` encoder with type URL exactly `io.cubesandbox.v1.GuestOOMVictimEvidenceSet`.
- [ ] In Task `Stats`, inspect request ttrpc metadata key exactly `cube-wave21-guest-oom-evidence`; canonical exact cached token returns evidence `Any`; otherwise execute existing CPU/memory Stats path unchanged.
- [ ] Add Rust metadata tests demonstrating missing/wrong/stale/uppercase selector cannot return Wave 21 data while exact selector can.

---

### Task 4: Implement guest collector, exact lifetime anchor, loss epoch, and finalized evidence

**Files:**
- Create: `agent/src/oom_victim.rs`
- Create: `agent/src/oom_victim_bpf.rs`
- Create: `agent/src/oom_victim_proc.rs`
- Modify: `agent/src/main.rs` (module/init wiring)
- Modify: `agent/src/sandbox.rs`
- Modify: `agent/src/rpc.rs`
- Modify: `agent/Cargo.toml`
- Modify: `agent/Cargo.lock`
- Modify as needed for exact process/cgroup authority: `agent/rustjail/src/container.rs`, `agent/rustjail/src/process.rs`, `agent/rustjail/src/cgroups/` only through narrow read-only helpers

**Core types:**

```rust
const SOURCE: &str = "guest.kernel.oom.mark_victim.raw_tracepoint";
const MAX_FINALIZED_REALIZATIONS: usize = 256;
const MAX_VICTIMS_PER_REALIZATION: usize = 64;
const MAX_FINALIZED_AGE_NS: u64 = 10 * 60 * 1_000_000_000;

enum FinalizedSlot { Valid(FinalizedEvidence), Poisoned(ConflictMeta), Unavailable }
struct GuestRealization { token:[u8;32], main_pid:u32, main_starttime_ticks:u64, expected_cgroup_v2_id:Option<u64>, guest_boot_id:String, started_boot_ns:u64, outcome_observed_boot_ns:Option<u64>, start_loss_epoch:u64, /* ... */ }
```

**Collector requirements:**
- Observe `oom:mark_victim` with event fields version/flags/TID/TGID/start_boottime/event_boottime/cgroup-v2 ID.
- Resolve required `task_struct.pid`, `tgid`, `start_boottime`; optional cgroup chain. Missing required BTF capability disables Wave 21 only.
- Raw buffer max 1024 records/10 minutes.
- Every detected reserve/lost/framing/overflow event increments monotonic `loss_epoch`.
- Production architectures amd64/arm64 only for exact starttime bridge; USER_HZ=100 with integer math.

**Steps:**
- [ ] RED pure parsers first: raw 40-byte event decoding, proc stat field 22 with tricky `comm`, canonical boot UUID, `timens_offsets`/time namespace equality, cgroup-v2 exact identity, start-boottime→ticks overflow/underflow.
- [ ] RED realization store: valid token opens window before runtime start; runtime start failure clears; successful start captures exact main PID/starttime/cgroup/boot sandwich; old token fenced by new start; exact duplicates idempotent; conflicting accepted record poisons slot; >64 victims unavailable; deterministic oldest-first eviction.
- [ ] RED loss epoch: unchanged epoch permits positive finalization; any increment between start/final makes the whole realization unavailable.
- [ ] Integrate token in `do_start_container`: validate pending token, capture `CLOCK_BOOTTIME` before `ctr.exec()`, run runtime start, then capture exact init process identity from runtime-owned `init_process_pid`; evidence failure never fails successful runtime start.
- [ ] Correlate buffered/live kernel events to MAIN by exact TGID + converted starttime; MEMBER only by exact nonzero expected/event cgroup-v2 ID while main anchor is valid.
- [ ] Hook exact main `do_wait_process` terminal observation to capture `outcome_observed_boot_ns` once and finalize exact token. Do not finalize from other RPCs.
- [ ] Implement `GetOOMVictimEvidence`: nonempty container ID + exact token; only finalized Valid slot returns ordered positive records; Poisoned/Unavailable/open/evicted/no-positive returns empty authoritative set, never guesses another token.
- [ ] Start collector best-effort at agent startup. Initialization/attach/verifier/BTF failure leaves collector unavailable without failing agent startup.
- [ ] Keep existing `run_oom_event_monitor`/`GetOOMEvent` compatibility behavior unchanged and completely separate.

**Dependency rule:** prefer a pure-Rust BPF loader already compatible with repository targets. If adding a crate (for example `aya`) is necessary, pin an exact reviewed version and lock it; no build-time network code generation or checked-in opaque BPF ELF. If the pinned toolchain cannot load the collector on a supported target, keep the live collector capability disabled while pure semantics remain fail-closed—do not introduce dmesg/kmsg/exit-code fallbacks.

---

### Task 5: One terminal Cubelet retrieval and lossless Prometheus transport

**Files:**
- Modify: `Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go`
- Modify: `Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go`
- Modify/Create: `Cubelet/plugins/cube/internals/sandbox/guest_oom_victim.go`
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/guest_oom_victim_prometheus.go`
- Create: `Cubelet/plugins/cube/internals/resourcemetrics/guest_oom_victim_v21_test.go`
- Modify: `Cubelet/plugins/cube/internals/resourcemetrics/plugin.go`

**Accepted proof:** exactly the spec `GuestProcessKernelOOMVictimProof` fields with `Scope` only `main` or `member`.

**Steps:**
- [ ] Extend the exact generation store with token/bind status, terminal finalization state, and bounded accepted positive records. BeginRealization/Delete/Create clear prior current-generation state.
- [ ] On authoritative Wave17 Wait outcome, if exact token + successful pre-start bind survived, make exactly one internal Stats evidence query with ttrpc request metadata `cube-wave21-guest-oom-evidence=<token>`.
- [ ] On stopped-State outcome, accept only evidence already immutably available for the exact token at the time of the one terminal attempt; never wait/retry/reconstruct.
- [ ] Add a narrow context-metadata helper using repository-pinned Go ttrpc metadata API; contract test must round-trip exact key/value to fake service. If metadata cannot be proven, finalization becomes terminal unknown on that target.
- [ ] Strictly decode only `io.cubesandbox.v1.GuestOOMVictimEvidenceSet`; validate payload token, max 64, identity/window/source/scope/duplicates, and exact current generation before commit.
- [ ] Slow RPC/decode work outside store lock; final commit rechecks sandbox/generation/token/terminal state atomically.
- [ ] Export metric exactly `cubesandbox_guest_kernel_oom_victim_info`, one unit-valued sample per accepted record, exact label set from spec, canonical decimal integers, optional MAIN `cgroup_v2_id=""`, MEMBER nonzero ID.
- [ ] Resource metrics may only visit/serialize accepted proofs; it cannot query Stats, generate tokens, repair malformed data, or infer proof from Waves 18–20.
- [ ] Test `math.MaxUint64` generation/starttime/event/window/cgroup values for exact decimal labels and zero/one/many cardinality.

---

### Task 6: Strict NolaneWorld same-scrape fusion and negative-space guards

**Files:**
- Create: `NolaneWorld/substrate/cube/guest_oom_victim.go`
- Create: `NolaneWorld/substrate/cube/guest_oom_victim_v21_test.go`
- Modify: `NolaneWorld/substrate/cube/task_termination.go`

**Public helpers:**

```go
func (e TaskTerminationEvidence) GuestKernelOOMVictimMarked() (marked bool, known bool)
func (e TaskTerminationEvidence) GuestMainKernelOOMVictimMarked() (marked bool, known bool)
```

**Steps:**
- [ ] Parse only exact metric name/label set/unit value. Require canonical lowercase token/UUID/source/scope and canonical decimal integers.
- [ ] Require matching Wave17 exact task outcome for target sandbox/generation. Wave18/19/20 may coexist but cannot substitute for missing guest proof.
- [ ] Require all target Wave21 records agree on sandbox/generation/token/guest boot/main identity/window/source; max 64; reject duplicate/conflicting victim rows.
- [ ] MAIN requires victim TGID/main PID and victim/main starttime equality. MEMBER requires nonempty nonzero cgroup ID. Event must be inside window.
- [ ] Positive semantics only: any MAIN/MEMBER => guest victim `(true,true)`; any MAIN => guest main `(true,true)`; absence => `(false,false)`; never `(false,true)`.
- [ ] Reflection/string guard forbids exported production fields/helpers containing `OOMKilled`, `GuestOOMKilled`, `TaskOOMKilled`, `ApplicationOOMKilled`, or equivalent causal wording.
- [ ] Negative tests prove exit 137, SIGKILL-shaped outcome, TaskOOM, Wave18 delta, Wave19 identity, Wave20 host victim, or any combinations without valid Wave21 metric remain guest victim unknown.

---

### Task 7: Stacked PR, RED→GREEN evidence, broad final-head verification

**Files:**
- `.github/workflows/cube-guest-kernel-oom-victim-contract.yml`
- documentation/PR metadata only after implementation unless a test exposes a real defect.

**Steps:**
- [ ] After plan commit, commit RED tests/workflow before any production code.
- [ ] Open Draft PR with base `gpt/wave20-kernel-oom-victim-provenance`, head `gpt/wave21-guest-kernel-oom-victim-provenance`, title `Wave 21: guest kernel OOM victim provenance`, body containing exact autonomous trailer.
- [ ] Verify dedicated RED run fails only on intended missing Wave21 contract. Harness/toolchain failures are fixed before GREEN.
- [ ] Execute Tasks 2–6 in small GREEN commits; never weaken RED assertions.
- [ ] On unexpected build/test behavior, use systematic-debugging and inspect exact failing logs before changing code.
- [ ] Require final-head success for, at minimum:
  - `Cube Guest Kernel OOM Victim Contract`
  - `Cube Kernel OOM Victim Contract`
  - `Cube Host Process Identity Contract`
  - `Cube Realization OOM Contract`
  - `Cube Task Outcome Contract`
  - `Cube Host Resource Contract`
  - CubeShim/agent Rust build+tests touched by this wave
  - `Nolane World Check`
  - `Nolane Live Substrate Gauntlet`
  - `Unit Test Check`
  - `Build Check`
  - `Format Check`
  - `DCO Check`
  - docs/Pages checks triggered by final tree.
- [ ] Trust-audit final diff for forbidden fallbacks (`137`, `SIGKILL`, dmesg/kmsg, `memory.events`, TaskOOM promotion), tokenless Stats access, conflict replacement instead of POISONED, retries after terminal unknown, generated-code hand editing, float conversion of uint64 labels, and public `OOMKilled` wording.
- [ ] Inspect final PR diff/reviews/comments, run requesting-code-review + verification-before-completion + finishing-a-development-branch, and mark PR ready only after final-head evidence is green.
- [ ] Do not merge this stacked PR or master automatically.
