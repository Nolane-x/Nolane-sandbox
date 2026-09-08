# Wave 28 — Runtime Cgroup Readback Authority Closure

Date: 2026-09-08

## Scope

Wave28 is stacked directly on Wave27 exact-final head:

`f42d42dec3809bbbd6b8a6b5862c8a3268f6f1b2`

Wave28 closes the exact-runtime-to-exact-cgroup-readback prerequisite seam. It does not mint `resourceproof.TrustedReport`, does not emit `LIVE_PASS`, and does not claim causal CPU or memory enforcement.

## Final authority shape

Production adds `RuntimeCgroupReadbackObserver` and sealed `RuntimeCgroupReadbackAuthority` in `NolaneWorld/substrate/cube`.

The production observer:

- is fixed to `/sys/fs/cgroup`;
- uses package-owned `os.ReadFile`;
- receives no caller-selected root, cgroup path, file source, pressure runner, or binding;
- derives the sandbox and cgroup target only from sealed Wave27 runtime provenance;
- requires a cgroup-v2 root with CPU and memory controllers;
- requires finite canonical `cpu.max` and `memory.max`;
- reads canonical `cpu.stat` and `memory.events` counters;
- requires exact Wave27 host-PID membership in `cgroup.procs` before readback;
- re-reads `cgroup.procs` after the CPU/memory readback and fails closed if the exact PID no longer belongs to the target;
- freshly reconstructs the supplied Wave27 authority before and after the bounded cgroup read;
- requires exact before/supplied/after Wave27 capability equivalence.

Only after those checks does Wave28 mint:

`runtime-cgroup-readback-v28:<64-lowercase-hex>`

using the domain separator:

`nolane.runtime-cgroup-readback.v28\x00`

The digest binds the sealed Wave27 runtime digest and exact runtime/cgroup identity plus CPU quota/period, throttling counters, memory limit, and OOM/OOM-kill counters. The counters remain descriptive snapshot values from one bounded readback interval, not causal enforcement evidence.

## TDD lineage

### Initial Wave28 RED

Committed test/contract head:

`4d95c7d62bdb4ac8e75e3b02b4c9d1568ee2b199`

Dedicated run:

`34225422787`

The focused Go gate failed specifically because Wave28 production seams did not exist yet:

- `RuntimeCgroupReadbackObserver`;
- package-private test observer constructor;
- `RuntimeCgroupReadbackAuthority`;
- `ValidateRuntimeCgroupReadbackAuthority`;
- `ErrInvalidRuntimeCgroupReadbackAuthority`.

This is the valid feature RED lineage.

### API-hardening correction

Wave28 was narrowed so the public validator no longer accepts `ResourceBinding`. The validator derives the package-private `ResourceBinding` from the exact sealed Wave27 runtime process instead.

The existing behavioral helper still passed the removed argument on `2544de0477b194b30a592c81cdf6108751f9b2db`; run `34225944844` failed with the exact compile signature mismatch. Test-only commit `f6ce40b47828697bd508b778b310242159ec7825` aligned the caller without changing production semantics.

Run `34227333129` on `f6ce40b47828697bd508b778b310242159ec7825` then completed all dedicated gates successfully.

### Review-driven membership TOCTOU RED

A final trust review found that the first production implementation checked `cgroup.procs` only before CPU/memory readback. Post-Wave27 freshness alone does not prove that the exact host PID remained in the same cgroup at the far side of the bounded filesystem snapshot.

Test-only head:

`5974e8ffe3d43a493c829b52e58ee54f5086be18`

Dedicated run:

`34227588155`

The focused test failed exactly as intended:

`TestV28HostPIDMembershipMustBracketReadback`

reported a nil error after the fake exact PID moved out of the cgroup after the first membership read.

Production fix:

`cf9f0d9b010bcee7ecbf2352b5a94811535ad2dd`

The observer now re-reads exact `cgroup.procs` after `memory.events` and before creating the snapshot. A changed/missing PID fails closed. This brackets the readback with two exact membership observations; it does not claim uninterrupted membership at every unobserved instant between them.

## Code-head verification

Exact code head:

`cf9f0d9b010bcee7ecbf2352b5a94811535ad2dd`

Dedicated workflow run:

`34227793276`

All gates completed `SUCCESS`:

1. Focused Wave28 behavioral tests
2. Static Wave28 anti-shortcut contract
3. Prior Wave27 regression
4. `go vet ./...` in `NolaneWorld`
5. `go test ./...` in `NolaneWorld`
6. Focused Wave28 race test

## Exact-final candidate verification

The closure-record candidate `de470bd501b59e078b660b963e1a6b713b4f8be7` was independently verified by dedicated push run `34227974835`, which completed `SUCCESS` on the unchanged production code plus the first closure record.

A final workflow-scope review then found the permanent `pull_request.paths` entry for this closure document omitted `-authority-` from the filename. The finalization lineage corrected that trigger path so future PRs cannot silently skip Wave28 verification when this exact closure evidence surface changes. That correction does not change Wave28 production semantics, tests, digest schema, or trust boundary.

### Requirement-to-test coverage hardening

A final design-to-test audit found that existing production already rejected path/root escape, non-v2 controller roots, and runtime substitution during readback, but behavioral tests did not directly lock those requirements.

Coverage-only head:

`75947ff161ff9a5c52c8aa917a119597f7f11936`

Dedicated workflow run:

`34228811419`

The added tests cover:

- root/non-canonical/escaping cgroup target paths;
- an observer rooted at `/`;
- missing CPU/memory cgroup-v2 controllers;
- runtime replacement after the cgroup snapshot has begun, proving post-read Wave27 reconstruction rejects the changed runtime.

No production code changed in this coverage commit. The focused Wave28 tests, static contract, Wave27 regression, vet, full NolaneWorld suite, and focused race test all completed `SUCCESS`.

## Static trust-boundary audit

The Wave28 static contract requires production to contain the fixed `/sys/fs/cgroup` observer, exact cgroup-v2 files, double Wave27 reconstruction, sealed digest derivation, and runtime identity binding.

It rejects Wave28 production shortcuts containing or accepting:

- `TrustedReport`;
- `buildTrustedReport`;
- `LIVE_PASS`;
- `CapabilityEvidence`;
- `CgroupRoot`;
- `HostFileSource`;
- `HostPressureRunner`;
- public caller-controlled string/binding locator inputs.

## Explicit non-claims

Wave28 does not prove:

- uninterrupted PID membership at every unobserved instant between the two membership reads;
- induced CPU throttling causality;
- induced memory OOM causality;
- exact helper/victim termination;
- disk enforcement;
- task success/failure;
- guest/filesystem correctness;
- image or binary measurement;
- hardware/TEE attestation;
- `resourceproof.TrustedReport` provenance;
- public `LIVE_PASS`.

## Next safe seam

The next wave should introduce a package-owned cgroup-v2 pressure-helper lifecycle. It must spawn a known helper, attach that exact helper PID to the Wave28 cgroup, induce CPU and memory pressure without killing the Wave27 runtime, correlate before/after counters with exact helper outcome, and revalidate Wave28/Wave27 afterward. Only after that causal chain exists should the project consider minting `resourceproof.TrustedReport`.

The exact-final head is the commit containing this final closure evidence with the corrected workflow trigger and coverage-hardening commit already in its ancestry. It must complete dedicated Wave28 CI and stacked PR integration on that exact SHA before Wave28 is called code-closed.
