# Wave 28 Design — Exact Runtime Cgroup Readback Authority

Date: 2026-09-08
Base: Wave27 exact-final `f42d42dec3809bbbd6b8a6b5862c8a3268f6f1b2`

## Purpose

Wave27 proves the exact current host runtime process and seals its cgroup path into `RealmResourceRuntimeAuthority`. It deliberately does not prove that CPU or memory limits were read from that exact cgroup, and it does not mint `resourceproof.TrustedReport`.

Wave28 closes the next prerequisite seam: bind a fresh Wave27 runtime authority to package-owned, cgroup-v2 limit/counter readback from the exact cgroup that contains the Wave27 host PID. This removes the legacy `host_observer` weakness where a caller-supplied `CgroupRoot` can point at cgroup A while a binding describes runtime B.

Wave28 is intentionally not the causal pressure/enforcement wave. The repository has a real Cubelet internal `AddProc` startup primitive, but it has no production pressure-helper lifecycle/API that NolaneWorld can safely use to create an independent same-cgroup CPU/memory victim while preserving Wave27 runtime freshness. Wave28 therefore must not pretend that readback alone is enforcement evidence.

## Trust statement

A valid Wave28 capability means:

1. the supplied Wave27 authority was freshly reconstructed before the read;
2. the exact Wave27 runtime process identifies an absolute canonical cgroup path;
3. the observer used the package-owned cgroup-v2 mount root `/sys/fs/cgroup` in production;
4. the target `cgroup.procs` contains the exact Wave27 host PID;
5. finite canonical CPU quota/period and memory limit plus required CPU/memory counters were read from that exact target;
6. the supplied Wave27 authority was freshly reconstructed again after the read; and
7. the before/after Wave27 authorities are exactly equivalent to the supplied authority.

The capability is a bounded readback snapshot, not a statement that a workload was causally throttled or OOM-killed.

## Non-goals

Wave28 does not prove:

- CPU throttling under induced load;
- memory OOM causality or exact victim identity;
- disk enforcement;
- task success/failure;
- guest correctness;
- image/binary measurement;
- hardware/TEE attestation;
- `resourceproof.TrustedReport` provenance;
- public `LIVE_PASS`.

## Why not reuse `resourceproof.hostResourceObserver` directly

`hostResourceObserver` accepts `HostObserverConfig.CgroupRoot`, `HostFileSource`, and `HostPressureRunner`. Those are useful legacy/test seams, but an exported trusted producer built directly on them would let callers choose another cgroup or fake file/pressure evidence.

Wave28 instead lives in `NolaneWorld/substrate/cube`, beside Wave27 authority, so it can consume authority-owned runtime identity without exposing an arbitrary cgroup locator.

## Why cgroup v2 only

The Wave28 production observer is fail-closed unless a unified cgroup-v2 hierarchy is present. Supporting cgroup v1 safely requires controller-specific mount resolution and membership reconciliation; accepting a guessed v1 root would reopen the locator seam Wave28 exists to close.

V1 support can be a later explicit wave with its own mount authority.

## API shape

### Observer

```go
type RuntimeCgroupReadbackObserver struct { /* opaque production root/read seam */ }

func NewRuntimeCgroupReadbackObserver() *RuntimeCgroupReadbackObserver
```

The production constructor is fixed to `/sys/fs/cgroup` and `os.ReadFile`. Tests may use a package-private constructor with a temporary/fake root and reader. There is no exported constructor accepting a root or filesystem interface.

### Opaque authority

```go
type RuntimeCgroupReadbackAuthority struct { /* sealed */ }

func (a RuntimeCgroupReadbackAuthority) Valid() bool
func (a RuntimeCgroupReadbackAuthority) RuntimeDigest() (string, bool)
func (a RuntimeCgroupReadbackAuthority) ReadbackDigest() (string, bool)
func (a RuntimeCgroupReadbackAuthority) Snapshot() (RuntimeCgroupReadbackSnapshot, bool)
```

`RuntimeCgroupReadbackSnapshot` is descriptive immutable data containing only the authority-owned exact runtime/cgroup identifiers and canonical readback values. It cannot reconstruct authority.

### Validator

Wave28 receives the same freshness inputs required to reconstruct Wave27 plus the supplied Wave27 authority and the Wave28 observer. It must:

1. reject structurally invalid supplied Wave27 authority;
2. extract the nested Wave26 authority/runtime proof from the supplied capability internally;
3. freshly call `ValidateRealmResourceRuntimeAuthority(...)` before the read;
4. require exact equivalence with the supplied Wave27 authority;
5. derive the cgroup path from the authority-owned runtime process, never from a caller string;
6. read and validate the exact cgroup-v2 snapshot;
7. freshly call `ValidateRealmResourceRuntimeAuthority(...)` after the read;
8. require exact equivalence with both the supplied and pre-read authorities;
9. mint the sealed Wave28 authority only after all checks pass.

No validator parameter accepts a `Binding`, cgroup root, cgroup path, file source, or pressure runner.

## Exact Wave27 equality

Wave28 adds an internal `sameRealmResourceRuntimeAuthority(a, b)` helper. Equality requires both capabilities to be structurally valid, the same seal lineage, exact nested endpoint authority, exact runtime proof identity, and exact runtime digest.

This prevents a fresh but different process/epoch/endpoint realization from being substituted between pre-read and post-read validation.

## Cgroup path construction

The authority-owned `HostSandboxProcessIdentityProof.CGroupPath` must already satisfy Wave27 canonical absolute-path validation. Wave28 additionally:

- rejects `/`;
- cleans the relative portion after the leading slash;
- joins only beneath the observer's fixed root;
- verifies the resulting target cannot escape the root;
- never follows a caller-selected root.

Production root is exactly `/sys/fs/cgroup`.

## Cgroup-v2 proof of membership

Wave28 first requires root `cgroup.controllers` to be readable and non-empty, establishing a v2 hierarchy from the observer root.

At the exact target it reads `cgroup.procs`, parses canonical decimal PIDs, rejects malformed/duplicate values, and requires exact membership of the Wave27 host PID.

A cgroup path string without matching PID membership cannot mint Wave28 authority.

## Canonical readback fields

From the exact target:

- `cpu.max`: exactly two fields; finite positive quota and positive period; `max` fails closed;
- `cpu.stat`: unique two-field lines and required `nr_throttled` plus `throttled_usec`;
- `memory.max`: one finite positive decimal value; `max` fails closed;
- `memory.events`: unique two-field lines and required `oom` and `oom_kill` counters;
- `cgroup.procs`: exact host PID membership as above.

The snapshot includes:

- runtime digest;
- sandbox ID/generation;
- host PID/starttime/boot ID;
- exact cgroup path;
- CPU quota/period;
- `nr_throttled`/`throttled_usec`;
- memory limit;
- `oom`/`oom_kill` counters.

Mutable counters are snapshot values only, not causal claims.

## Digest

Wave28 derives:

`runtime-cgroup-readback-v28:<64-lowercase-hex>`

with domain separator:

`nolane.runtime-cgroup-readback.v28\x00`

The canonical JSON preimage binds all snapshot fields listed above. `Valid()` recomputes this digest from the sealed Wave27 authority and stored snapshot, so copying a digest string cannot reconstruct authority.

## Fail-closed conditions

Wave28 fails closed on:

- zero/forged/stale Wave27 authority;
- wrong controller/Realm/resource/epoch/client/runtime observer context;
- Wave27 changes before vs after read;
- non-v2 root;
- empty or malformed `cgroup.controllers`;
- non-canonical/escaping/root cgroup path;
- missing exact PID membership;
- malformed or duplicate PID list;
- unlimited CPU or memory;
- missing/malformed/duplicate required CPU or memory counters;
- read errors/context cancellation;
- zero/corrupt Wave28 seal/digest.

## Testing strategy

Behavioral RED tests must cover:

- happy-path exact v2 membership/readback;
- zero Wave27 authority;
- wrong PID membership;
- unlimited CPU/memory;
- malformed/duplicate stats;
- cgroup path/root escape rejection;
- pre/post Wave27 staleness or substitution;
- zero Wave28 authority accessors fail closed;
- deterministic readback digest.

A static contract must reject public trusted shortcuts, especially any Wave28 public API accepting `Binding`, `CgroupRoot`, `HostFileSource`, or `HostPressureRunner`, and must assert there is no `TrustedReport`, `buildTrustedReport`, `LIVE_PASS`, or capability-evidence minting in Wave28 production files.

## Next seam

After Wave28, the safe next step is a separate package-owned cgroup-v2 pressure-helper lifecycle: spawn a known helper process, attach its PID to this exact Wave28 cgroup, generate CPU pressure and a memory victim while keeping the Wave27 runtime alive, correlate counter deltas and exact helper termination, revalidate Wave28/Wave27 afterward, and only then consider minting `resourceproof.TrustedReport`.
