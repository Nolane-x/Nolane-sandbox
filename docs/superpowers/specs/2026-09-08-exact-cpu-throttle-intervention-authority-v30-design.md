# Wave 30 — Exact CPU Throttle Intervention Authority

Date: 2026-09-08
Base: Wave29 exact-final `9c9f33ae605b902d702dfd6510785d56a64ea779`
Status: approved in chat; written-spec review pending; implementation not started

## 1. Purpose

Wave29 proves that Nolane can launch one package-owned helper, identify the exact running child, place it into the exact fresh Wave28 cgroup, release it only after placement, and observe its exact clean termination. Wave29 intentionally does no pressure work and treats cgroup counters as descriptive only.

Wave30 closes the next narrow trust seam for CPU only: it performs one package-owned, bounded CPU intervention inside the exact Wave29 cgroup and proves that the intervention is associated with fresh cgroup-v2 CFS throttling evidence while the exact helper itself demonstrably consumed CPU.

Wave30 must not convert a raw `cpu.stat` delta into causal proof. Linux `cpu.stat` is cgroup-scoped and accounts for all fair-scheduler processes in the cgroup. Therefore Wave30 uses a control/intervention design:

1. exact Wave29-compatible helper identity and placement;
2. a quiet control window while the helper is parked;
3. package-owned single-worker CPU work begins only after the parent issues START;
4. exact helper CPU-runtime evidence is read from `/proc/<pid>/schedstat` during the intervention;
5. fresh cgroup throttle counters must remain quiet before START and increase only in the intervention bracket;
6. helper identity, executable identity, cgroup binding, runtime provenance, and finite CPU limit remain stable across the lifecycle;
7. the helper is stopped and reaped cleanly before authority minting.

The resulting claim is intentionally narrow: **controlled intervention-associated CPU throttling on the exact runtime cgroup**. Wave30 does not claim sole-process attribution, memory enforcement, requested-policy equality, `TrustedReport`, or public `LIVE_PASS`.

## 2. Why CPU is separated from memory

Memory OOM requires victim identity and failure-causality semantics that differ from CPU throttling. An over-limit memory allocator in the parent cgroup can cause the long-lived runtime, helper, or another process to be killed. Existing Wave18–21 OOM evidence must be combined with a deliberate victim-selection design before memory pressure can enter the trusted producer path.

Wave30 therefore handles CPU only. Memory remains a later wave.

## 3. Chosen architecture

Three approaches were considered.

### A. Busy loop plus one before/after `cpu.stat` delta

Rejected. A cgroup-level delta may have been produced by unrelated runtime activity. This would launder correlation into causality.

### B. Quiet control window plus exact helper CPU-runtime evidence plus intervention throttle delta

Selected. It reuses Wave29 placement authority, makes one package-owned intervention the only deliberate phase change, proves the exact helper ran CPU work, and requires throttle counters to remain quiet before the intervention and increase during it.

### C. Create an isolated nested probe cgroup

Deferred. It gives stronger isolation but adds cgroup creation/deletion, delegation, controller inheritance, `cpu.max` replication, cleanup, and a different policy surface. It would prove a child cgroup rather than the already-proven exact runtime cgroup. That is too broad for the next wave.

## 4. Kernel evidence semantics

Wave30 relies on two separate kernel evidence families and keeps their meanings distinct.

### 4.1 Exact cgroup throttle counters

Fresh Wave28 readbacks already expose:

- `NrThrottled`;
- `ThrottledUsec`;
- exact finite `CPUQuotaMicros`;
- exact finite `CPUPeriodMicros`;
- exact runtime/cgroup identity.

Wave30 requires both `NrThrottled` and `ThrottledUsec` to remain unchanged across the quiet control window and both to increase across the CPU intervention window.

A rollback in either counter fails closed.

### 4.2 Exact helper CPU runtime

Wave30 v1 deliberately uses one CPU-burn worker in the exact helper process. This keeps `/proc/<helper-pid>/schedstat` semantically aligned with the intervention instead of pretending that one task-level schedstat line proves aggregate work by an arbitrary thread pool.

The first schedstat field is treated as cumulative CPU runtime in nanoseconds for the exact helper task. The line must contain exactly three canonical unsigned decimal fields.

The helper schedstat runtime must:

- be readable after exact placement and before START;
- be readable after the CPU intervention while the helper is still alive and parked;
- never decrease;
- increase by at least one exact CPU quota quantum: `CPUQuotaMicros * 1000` nanoseconds.

Wave30 does not infer cgroup throttling from schedstat. Schedstat proves the exact helper consumed material CPU; fresh Wave28 throttle counters prove the exact cgroup experienced throttling.

If schedstat is unavailable, malformed, hidden by host configuration, or does not increase enough, Wave30 returns honest unavailability rather than weakening the claim.

## 5. Trust boundary

### 5.1 Authoritative inputs

The Wave30 validator accepts only existing opaque authority/observer dependencies and a sealed package-owned Wave30 executor:

- `realm.Controller`;
- `realm.RealizationAuthority`;
- Wave27 `RealmResourceRuntimeAuthority`;
- Wave24 `RealizationEpochObserver`;
- exact Cube `Client`;
- Wave27 `RuntimeRealizationObserver`;
- Wave28 `RuntimeCgroupReadbackObserver`;
- sealed package-owned `RuntimeCPUInterventionExecutor`.

Wave30 revalidates the current runtime/cgroup chain rather than trusting caller-carried descriptive snapshots.

### 5.2 Forbidden public inputs

No public constructor or validator may accept:

- executable path;
- shell command or argv;
- environment map;
- cgroup root/path;
- PID or starttime;
- raw schedstat bytes;
- raw `cpu.stat` bytes;
- arbitrary file reader/writer;
- sleeper/clock callback;
- worker count;
- pressure duration;
- pressure callback/interface;
- `HostPressureRunner`;
- `ResourceBinding`;
- caller-created nonce;
- caller-created Wave30 snapshot/digest;
- `TrustedReport` or `LIVE_PASS` target.

Production uses fixed package-owned behavior. Test seams remain package-private.

## 6. Production helper mode

Wave30 adds a second package-owned internal helper mode without changing Wave29 `park-exit` semantics.

Mode identifier:

`NOLANE_INTERNAL_CGROUP_HELPER=cpu-throttle-v30`

The existing Wave29 nonce environment key remains package-owned:

`NOLANE_INTERNAL_CGROUP_HELPER_NONCE=<64 lowercase hex>`

The parent also supplies one internal package-derived burn duration to the child. That value is derived only from the fresh Wave28 CPU period and is not exposed through public API.

The command-facing `MaybeRunInternalCgroupHelper()` remains the single pre-flag dispatcher and must preserve exact Wave29 behavior while routing the new Wave30 mode to a separate protocol implementation.

## 7. Intervention protocol

Wave30 uses a bounded parent-controlled protocol over package-owned inherited FDs. The exact records are:

1. child -> parent: `READY <nonce>\n`
2. parent -> child: `START <nonce>\n`
3. child performs the package-owned single-worker CPU burn;
4. child -> parent: `BURN_DONE <nonce>\n`
5. child remains alive and parked;
6. parent -> child: `EXIT <nonce>\n`
7. child -> parent: `DONE <nonce>\n`
8. child exits 0.

The child performs no CPU intervention before START. After `BURN_DONE` it performs no further intentional pressure while the parent reads final helper/cgroup evidence.

Malformed, duplicated, oversized, reordered, wrong-nonce, premature EOF, unexpected record, or non-zero exit fails closed.

All protocol waits are context-aware. Cancellation after child start must kill and reap the exact helper.

## 8. Provable CPU range and package-owned timing

Wave30 v1 is deliberately conservative. It only attempts CPU intervention when:

- `CPUQuotaMicros > 0`;
- `CPUPeriodMicros > 0`;
- `CPUQuotaMicros < CPUPeriodMicros`.

Therefore the exact finite limit is below one CPU-equivalent and one continuously runnable worker can saturate it. A quota equal to or above one full CPU-equivalent is outside Wave30 v1's provable range and returns `ErrRuntimeCPUInterventionUnavailable`.

Package-owned durations are exact:

- control interval = `2 * CPUPeriodMicros` converted to wall time;
- burn interval = `4 * CPUPeriodMicros` converted to wall time.

To keep the proof bounded, Wave30 only proceeds when:

- control interval is between 20 ms and 2 s inclusive;
- burn interval is between 40 ms and 4 s inclusive.

A period outside that range returns `ErrRuntimeCPUInterventionUnavailable` rather than changing the proof threshold.

The parent timeout around protocol/pressure work must exceed the derived burn interval by a fixed package-owned safety margin, but the caller cannot configure it.

## 9. Control window

After the helper is READY, exact-identified, executable-verified, and placed in the fresh Wave28 cgroup, but before START:

1. mint a fresh control-before Wave28 authority;
2. require exact immutable binding equality with the placement basis;
3. read helper schedstat-before;
4. wait exactly the package-owned control interval;
5. mint a fresh control-after Wave28 authority;
6. require exact immutable binding equality again;
7. require `NrThrottled` unchanged;
8. require `ThrottledUsec` unchanged;
9. require the exact helper PID/starttime/executable identity unchanged and alive;
10. require exact helper membership in the expected cgroup immediately before START.

Any background throttling during this quiet window makes attribution ambiguous and returns `ErrRuntimeCPUInterventionUnavailable`.

The control window does not require the long-lived runtime to be completely idle. It only requires that no CFS throttle event/time is observed before the package-owned intervention begins.

## 10. Intervention window

Only after the control window passes may the parent send START.

The helper performs the exact package-owned burn interval and sends `BURN_DONE` when that work completes. Before sending EXIT, the parent must obtain:

- exact helper PID unchanged;
- exact helper starttime unchanged;
- exact running-child executable SHA-256 unchanged;
- exact helper still in the expected cgroup;
- helper schedstat after `BURN_DONE`;
- helper schedstat delta at least `CPUQuotaMicros * 1000` nanoseconds;
- a fresh pressure-after Wave28 authority;
- exact immutable Wave28 binding equality with control-before/control-after;
- `NrThrottledAfter > NrThrottledControlAfter`;
- `ThrottledUsecAfter > ThrottledUsecControlAfter`;
- no OOM/OOM-kill counter rollback.

Failure to observe sufficient helper CPU runtime or both throttle deltas returns `ErrRuntimeCPUInterventionUnavailable`; identity drift, malformed evidence, rollback, or protocol corruption returns the invalid-authority error.

Wave30 does not require an OOM counter increase; CPU pressure must not be laundered into memory proof.

After those checks, the parent sends EXIT, requires DONE, waits/reaps the helper, requires exit 0, and mints one final fresh Wave28 authority. The final readback must preserve the same runtime/cgroup identity and finite limits and must not roll counters backward.

## 11. Error classes

Wave30 defines two package errors so later trusted producers can distinguish an environment that cannot currently prove CPU enforcement from evidence corruption.

`ErrRuntimeCPUInterventionUnavailable`

Used for honest inability to prove the claim, including:

- quota outside the Wave30 v1 sub-one-CPU range;
- period outside bounded timing range;
- schedstat unavailable;
- quiet control window already throttled;
- helper consumed insufficient measured CPU;
- intervention completed without both required throttle deltas.

`ErrInvalidRuntimeCPUThrottleInterventionAuthority`

Used for malformed or contradictory authority/evidence, including:

- invalid nested authority;
- identity drift;
- PID reuse/starttime drift;
- executable mismatch;
- helper cgroup membership mismatch;
- malformed schedstat;
- counter rollback;
- malformed protocol;
- non-zero exit.

Context cancellation/deadline returns the context error after exact helper cleanup.

## 12. Opaque authority

New public capability:

`RuntimeCPUThrottleInterventionAuthority`

It is sealed with package-private state and cannot be reconstructed from JSON, public field assignment, or a caller-provided digest.

Descriptive snapshot fields include:

- runtime digest;
- sandbox ID;
- generation;
- runtime host PID/starttime/boot ID;
- exact cgroup path;
- exact CPU quota/period;
- helper PID/starttime;
- helper executable SHA-256;
- helper nonce SHA-256;
- control-before readback digest;
- control-after readback digest;
- pressure-after readback digest;
- final readback digest;
- control `NrThrottled` and `ThrottledUsec` values;
- pressure-after `NrThrottled` and `ThrottledUsec` values;
- helper schedstat runtime before/after;
- READY, START, BURN_DONE, EXIT, DONE/exit timestamps;
- helper exit code.

Authority digest format:

`runtime-cpu-throttle-intervention-v30:<64-lowercase-hex>`

Domain separator:

`nolane.runtime-cpu-throttle-intervention.v30\x00`

The digest binds every immutable identity field, all four Wave28 readback digests, the helper identity, the exact intervention counters, helper runtime counters, derived control/burn durations, and lifecycle timestamps.

## 13. Validation and freshness semantics

`Valid()` validates only internal seal/digest/nested authority consistency. It is not a promise that the world has not changed since minting.

The primary production validator must freshly re-observe the authority chain while minting and fail on:

- Realm revision/policy drift;
- runtime realization replacement;
- provider endpoint/provider incarnation drift inherited from Wave27;
- Cubelet epoch drift;
- runtime PID/starttime/boot drift;
- cgroup path drift;
- CPU quota/period drift;
- memory-limit drift inherited from Wave28 immutable binding;
- helper PID reuse;
- helper executable replacement;
- helper leaving the expected cgroup;
- control-window throttle activity;
- helper CPU runtime not increasing enough;
- pressure-window throttle counters not increasing;
- counter rollback;
- malformed protocol;
- cancellation/timeouts;
- non-zero helper exit.

## 14. Public API

Production constructor:

`NewRuntimeCPUInterventionExecutor() (*RuntimeCPUInterventionExecutor, error)`

No arguments.

Primary validator:

`ValidateRuntimeCPUThrottleInterventionAuthority(...) (RuntimeCPUThrottleInterventionAuthority, error)`

The validator receives the same authority/observer dependencies needed to mint fresh Wave28 plus the sealed Wave30 executor. It accepts no public path/process/pressure/stat evidence.

Wave29 public APIs remain unchanged.

## 15. Required TDD coverage

### Core GREEN path

1. fresh authority chain + quiet control + exact helper CPU runtime quantum + exact cgroup throttle increase + clean exit mints authority;
2. authority digest is deterministic for identical authority-owned inputs;
3. zero authority is invalid;
4. JSON round-trip cannot restore authority.

### Honest unavailability

5. quota equal to period is unavailable;
6. quota greater than period is unavailable;
7. period outside bounded timing range is unavailable;
8. schedstat unavailable is unavailable;
9. background `NrThrottled` increase during control is unavailable;
10. background `ThrottledUsec` increase during control is unavailable;
11. helper CPU runtime delta below one quota quantum is unavailable;
12. intervention with no `NrThrottled` increase is unavailable;
13. intervention with no `ThrottledUsec` increase is unavailable.

### Invalid/contradictory evidence

14. malformed schedstat fails closed;
15. schedstat runtime rollback fails closed;
16. cgroup counter rollback fails closed;
17. runtime/cgroup/limit drift during control fails closed;
18. runtime/cgroup/limit drift during pressure fails closed;
19. helper leaves exact cgroup fails closed;
20. helper PID/starttime/executable drift fails closed.

### Protocol/process failures

21. wrong READY nonce fails closed;
22. START cannot occur before exact placement/control completion;
23. missing/malformed/duplicate BURN_DONE fails closed;
24. EXIT cannot occur before pressure evidence is read;
25. missing/malformed DONE fails closed;
26. child premature exit fails closed;
27. non-zero exit fails closed;
28. cancellation in READY/control/pressure/post-BURN phases kills and reaps helper;
29. executable replacement of running child fails closed;
30. PID reuse/starttime change fails closed.

### Public anti-shortcut tests

31. public constructor has no args;
32. public validator exposes no root/path/PID/schedstat/cpu.stat/worker/duration/pressure callback;
33. caller cannot supply `HostPressureRunner`, `ResourceBinding`, raw evidence, `TrustedReport`, or `LIVE_PASS` target.

## 16. Static contract and CI

Wave30 adds a dedicated static anti-shortcut contract and workflow. Exact-head verification must include:

- focused Wave30 behavioral tests;
- helper protocol/command integration tests;
- static Wave30 contract;
- Wave29 regression;
- Wave28 regression;
- Wave27 regression;
- `go vet ./...`;
- full `go test ./...`;
- focused `-race` Wave30 tests;
- every applicable PR integration workflow on the exact candidate SHA.

RED evidence must be committed and attributable to missing Wave30 production semantics, not fixture, import, runner, or formatting failures.

## 17. Explicit non-claims

Wave30 does not prove:

- CPU quotas equal to or above one CPU-equivalent;
- sole-process attribution of every throttled microsecond;
- that no unrelated process ran during the intervention;
- memory limit enforcement;
- memory OOM causality or victim identity;
- requested Realm CPU policy equals effective cgroup tuple;
- disk enforcement;
- task success/failure outside the helper lifecycle;
- guest/filesystem correctness;
- hardware/TEE attestation;
- software-supply-chain attestation beyond local executable SHA-256 identity;
- `resourceproof.TrustedReport` provenance;
- `CapabilityEvidenceSource` activation;
- public `LIVE_PASS`.

## 18. Completion criterion

Wave30 is complete only when an exact package-owned single-worker CPU helper can be placed into the exact fresh runtime cgroup, a quiet control window proves no pre-intervention throttling, the exact helper consumes at least one measured quota quantum only after START, both exact cgroup CFS throttle counters then increase, the complete runtime/cgroup/helper identity remains stable through clean helper exit, honest unprovability is distinguishable from malformed evidence, and a sealed Wave30 authority is minted on an exact-SHA green test/CI candidate.

The next safe seam after Wave30 is to decide whether this CPU intervention authority is sufficient for the CPU half of a package-owned trusted resource producer, while memory remains blocked on an exact helper-victim/OOM intervention design.