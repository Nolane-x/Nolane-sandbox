# Wave 29 — Exact Runtime Cgroup Helper Placement Authority

Date: 2026-09-08
Base: Wave28 exact-final `53ecdfdedd651c87f2b44e18fce352127c9f4242`

## 1. Purpose

Wave29 closes the next trust prerequisite after Wave28. Wave28 proves that one fresh Wave27 runtime process is a member of one exact cgroup-v2 path and that finite CPU/memory limits plus counters were read from that exact cgroup. It deliberately does not prove that Nolane can place a package-owned probe process into that cgroup without a race, identify the exact probe process, release it only after placement, or observe its exact termination.

Wave29 adds that missing capability. It proves a package-owned helper lifecycle with this order:

1. mint a fresh Wave28 readback from the exact Wave27 runtime;
2. start a helper process in a parked internal mode;
3. cryptographically challenge the helper with a fresh nonce and require a READY acknowledgement before trusting the process as the helper protocol;
4. record the helper PID and canonical `/proc/<pid>/stat` starttime;
5. attach that exact PID to the Wave28 cgroup through package-owned `cgroup.procs` write authority;
6. verify the exact PID is present in `cgroup.procs` and the same PID still has the same starttime;
7. release the parked helper only after successful placement;
8. require a DONE acknowledgement and exact exit status 0;
9. mint a second fresh Wave28 readback and require the same runtime identity, cgroup path, CPU quota/period and memory limit as the pre-helper readback;
10. mint an opaque Wave29 authority only if every step succeeds.

Wave29 does not induce CPU or memory pressure. Its helper mode is intentionally `park-exit`: after release it performs no resource claim and exits cleanly. CPU causality is a later wave. Memory OOM causality is separate because an uncontrolled parent-cgroup OOM can kill the long-lived runtime rather than the helper.

## 2. Why this wave is separate

Three approaches were considered.

### A. Directly add a production `HostPressureRunner`

Rejected. The legacy `resourceproof.HostPressureRunner` is an interface and no production implementation exists. Wiring caller-supplied or public-DI implementations into trusted evidence would permit fabricated pressure and outcomes.

### B. Add CPU and memory pressure immediately

Rejected for Wave29. CPU pressure can be made causal once exact helper placement exists, but memory OOM requires an additional victim-selection design. A helper that merely allocates beyond the parent cgroup limit does not prove the kernel killed that helper instead of the runtime.

### C. Package-owned parked helper lifecycle, then pressure in later waves

Selected. This closes the execution/placement authority without laundering it into resource enforcement. It gives later waves a stable, testable primitive with exact PID/starttime and release ordering.

## 3. Trust model

### 3.1 Authoritative inputs

Wave29 accepts only existing opaque capabilities and trusted observers:

- `realm.Controller` plus `realm.RealizationAuthority`;
- Wave27 `RealmResourceRuntimeAuthority`;
- Wave24 `RealizationEpochObserver`;
- exact Cube `Client`;
- Wave27 `RuntimeRealizationObserver`;
- Wave28 `RuntimeCgroupReadbackObserver`;
- a sealed package-owned `RuntimeCgroupHelperExecutor`.

The public validator does not accept:

- executable path;
- shell command;
- argv supplied by the caller;
- environment map supplied by the caller;
- cgroup root/path;
- arbitrary file reader/writer;
- PID supplied by the caller;
- starttime supplied by the caller;
- `ResourceBinding`;
- `HostFileSource`;
- `HostPressureRunner`;
- a caller-generated helper nonce.

### 3.2 Production executable identity

The production helper executor resolves `os.Executable()` once when it is constructed, resolves symlinks to a canonical absolute path, rejects non-regular files, and hashes the executable bytes with SHA-256. The same executable is launched as the helper.

The helper binary digest is recorded as lowercase 64-hex data in Wave29 authority. It is descriptive identity for the exact executable used by the helper lifecycle. It is not a software-supply-chain attestation and is not compared against an externally blessed digest in Wave29.

The production lifecycle is intended for an executable that installs the Wave29 helper entrypoint. In this wave that executable is `NolaneWorld/cmd/nolane-gauntlet-live`. If the current executable does not enter the internal helper protocol, READY never validates and authority minting fails closed.

### 3.3 Internal helper mode

`NolaneWorld/cmd/nolane-gauntlet-live` must call this package function before ordinary flag parsing:

`MaybeRunInternalCgroupHelper()`

The helper mode is enabled only when all of the following package-defined conditions are present:

- `NOLANE_INTERNAL_CGROUP_HELPER=park-exit`;
- `NOLANE_INTERNAL_CGROUP_HELPER_NONCE=<64 lowercase hex>`;
- inherited file descriptor 3 is the release pipe, parent -> child;
- inherited file descriptor 4 is the acknowledgement pipe, child -> parent.

No alternative FD numbers or environment key names are supported in Wave29 production.

The parent generates exactly 32 random bytes with `crypto/rand.Read`, encodes those bytes as 64 lowercase hex, and passes that value as `NOLANE_INTERNAL_CGROUP_HELPER_NONCE`.

Helper protocol:

1. validate internal mode and canonical nonce;
2. open inherited fd 3 as release input and fd 4 as acknowledgement output;
3. write exactly `READY <nonce>\n` to fd 4;
4. block until fd 3 yields exactly `GO <nonce>\n` and EOF follows that one record;
5. write exactly `DONE <nonce>\n` to fd 4;
6. close descriptors and exit 0.

Any malformed, duplicate, oversized or unexpected protocol record exits non-zero without performing pressure. Ordinary gauntlet execution is completely bypassed in helper mode.

The nonce is not itself an authority token. Its role is to correlate the exact child protocol instance with the parent lifecycle and prevent accidental acknowledgement from unrelated inherited I/O.

## 4. Helper process identity

After the child sends READY and before it is released, the parent owns a live OS process handle and obtains:

- PID from the started process handle;
- `starttime_ticks` from canonical parsing of `/proc/<pid>/stat` field 22;
- helper executable SHA-256;
- cgroup path from fresh Wave28 readback;
- placement/release/exit timestamps from package-owned UTC clock.

Wave29 never identifies a helper by PID alone.

`/proc/<pid>/stat` parsing must handle the command-name parenthesized field correctly and reject:

- missing file;
- PID <= 0;
- malformed stat line;
- field count too short;
- non-canonical decimal starttime;
- zero starttime.

The parent reads starttime before placement and again after `cgroup.procs` membership is observed. The values must match exactly. A PID reuse or process replacement therefore fails closed.

## 5. Cgroup placement

The cgroup target is derived only from the fresh Wave28 snapshot. The production root remains `/sys/fs/cgroup`; Wave29 does not introduce another caller-controlled root.

The executor writes the helper PID plus newline to:

`<fixed-root>/<wave28-cgroup-path>/cgroup.procs`

Then it reads `cgroup.procs` and requires exact canonical membership of the helper PID. Existing runtime membership is not required to disappear; Wave28 already proved the runtime belongs to the target cgroup.

Placement is complete only after:

- the write succeeded;
- the helper PID appears in a canonical `cgroup.procs` snapshot;
- helper starttime still matches the pre-placement value;
- the helper has not exited.

Only then may the parent send `GO <nonce>`.

No pressure work may happen before this release point.

## 6. Fresh Wave28 bracketing

Wave29 does not trust a stale Wave28 object by shape. Instead it internally mints a fresh Wave28 authority before the helper lifecycle and another after the helper exits by calling `ValidateRuntimeCgroupReadbackAuthority` with the same Wave27 authority and observers.

The pre/post Wave28 snapshots must agree on immutable binding data:

- `RuntimeDigest`;
- `SandboxID`;
- `Generation`;
- runtime `HostPID`;
- runtime `StartTimeTicks`;
- runtime `BootID`;
- `CGroupPath`;
- `CPUQuotaMicros`;
- `CPUPeriodMicros`;
- `MemoryLimitBytes`.

Counter fields may increase and are not required to be equal:

- `NrThrottled`;
- `ThrottledUsec`;
- `OOMEvents`;
- `OOMKillEvents`.

Wave29 records before/after readback digests but does not interpret any counter delta as caused by the helper, because the helper does no pressure work in this wave.

## 7. Opaque authority

New type:

`RuntimeCgroupHelperPlacementAuthority`

It is sealed with package-private state and cannot be reconstructed by JSON or public field assignment.

Descriptive snapshot fields:

- runtime digest;
- Wave28 before readback digest;
- Wave28 after readback digest;
- sandbox ID;
- generation;
- cgroup path;
- CPU quota/period;
- memory limit;
- helper PID;
- helper starttime ticks;
- helper executable SHA-256;
- helper nonce SHA-256: SHA-256 of the original 32 raw nonce bytes, lowercase 64-hex;
- READY/placed/released/exited UTC timestamps;
- exit code.

The authority digest is:

`runtime-cgroup-helper-placement-v29:<64-lowercase-hex>`

using domain separator:

`nolane.runtime-cgroup-helper-placement.v29\x00`

The digest document includes all immutable Wave28 binding fields, both Wave28 readback digests, helper identity fields, nonce SHA-256, timestamps and exact exit code.

The raw nonce and its directly encoded 64-hex challenge are not exported and are not retained in the public snapshot.

## 8. Fail-closed cleanup

Every path after child start must own cleanup.

If any check fails before release:

- close the release pipe without GO;
- kill the exact child process if still live;
- wait/reap it;
- return no authority.

If the child is released but DONE is missing, malformed, times out, or exit code is non-zero:

- kill if needed;
- wait/reap;
- return no authority.

Context cancellation must abort the lifecycle, kill/reap the helper and return the context error where appropriate.

No leaked child process is acceptable on a failed authority mint.

## 9. Public surface

Production constructor:

`NewRuntimeCgroupHelperExecutor() (*RuntimeCgroupHelperExecutor, error)`

No arguments.

Primary validator:

`ValidateRuntimeCgroupHelperPlacementAuthority(...) (RuntimeCgroupHelperPlacementAuthority, error)`

It accepts the same authority/observer dependencies required to mint fresh Wave28 plus the sealed helper executor. It accepts no path/string/file/process callback evidence.

Package-private test constructors may inject process, filesystem, clock, nonce and executable-hash seams. They must never be exported.

`MaybeRunInternalCgroupHelper() (handled bool, exitCode int)` is the single command-facing helper entrypoint. It exposes no authority and is called before normal `nolane-gauntlet-live` flag parsing.

## 10. Required tests

### Behavioral RED/GREEN tests

1. exact fresh Wave28 + READY + exact PID/starttime + placement + GO + DONE + exit 0 mints authority;
2. authority digest deterministic for identical authority-owned input;
3. zero authority invalid;
4. helper executable digest canonical lower SHA-256;
5. READY nonce mismatch fails closed;
6. child exits before placement fails closed;
7. `cgroup.procs` write failure fails closed;
8. helper PID absent after placement write fails closed;
9. helper PID duplicate/malformed membership fails closed;
10. PID starttime changes after placement fails closed;
11. GO must not be sent before membership and second starttime validation;
12. DONE missing/mismatched fails closed;
13. non-zero helper exit fails closed;
14. context cancellation kills/reaps helper;
15. runtime replacement during helper lifecycle makes post-Wave28 validation fail;
16. cgroup path/finite limits changing across pre/post Wave28 fails closed;
17. counter increases across pre/post Wave28 are allowed and remain descriptive;
18. public constructor accepts no executable/root/files/process hooks;
19. public validator accepts no string locator, `ResourceBinding`, `HostFileSource`, `HostPressureRunner`, PID or nonce;
20. JSON round-trip cannot restore authority.

### Command integration tests

`nolane-gauntlet-live` must invoke `MaybeRunInternalCgroupHelper()` before ordinary flag parsing. Internal helper mode must bypass the normal gauntlet flow.

### Regression gates

- Wave28 focused tests;
- Wave27 focused tests;
- `go vet ./...`;
- `go test ./...`;
- focused race tests;
- static anti-shortcut contract.

## 11. Static anti-shortcut contract

The Wave29 source must contain the production fixed root, `os.Executable`, executable hashing, `/proc/` starttime read, `cgroup.procs` write/read, fresh Wave28 validation on both sides, READY/GO/DONE protocol, exact internal environment keys and FD numbers, and the Wave29 digest domain separator.

The public signatures must not expose:

- custom executable;
- custom root;
- custom file reader/writer;
- `exec.Cmd` factory;
- process launcher;
- `HostPressureRunner`;
- `HostFileSource`;
- caller PID/starttime;
- raw nonce;
- `TrustedReport`;
- `LIVE_PASS`.

## 12. Explicit non-claims

Wave29 does not prove:

- CPU throttling caused by the helper;
- memory OOM caused by the helper;
- OOM victim identity;
- requested CPU/memory policy equivalence;
- task success/failure beyond the helper's own park-exit protocol;
- disk enforcement;
- guest/filesystem correctness;
- binary provenance beyond a descriptive local SHA-256 measurement;
- hardware/TEE attestation;
- `resourceproof.TrustedReport` provenance;
- public `LIVE_PASS`.

## 13. Next safe seams

After Wave29 closes, CPU pressure can be added as a separate authority that reuses the exact parked-helper placement protocol, runs a bounded busy-loop only after GO, requires exact helper exit, and requires monotonic throttle-counter deltas across fresh Wave28 readbacks.

Memory pressure remains separate until the design can prove exact victim selection without risking the long-lived Wave27 runtime. A memory wave must not infer helper victim identity from an OOM counter alone.
