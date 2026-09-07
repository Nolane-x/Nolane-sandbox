# Wave 22 — Public Live Verdict Authority Closure

## Scope closed

Wave 22 exposes a public, deterministic resource verdict **only** from package-owned `resourceproof.TrustedReport` authority. It does not create a new observer, does not add a public `Report -> TrustedReport` constructor, and does not turn canonical JSON into provenance authority.

The projection is dimension-local:

- CPU may become `VERIFIED` only after the trusted CPU observation passes the existing exact quota/period plus throttling verifier.
- Memory may become `VERIFIED` only after the trusted memory observation passes the existing exact limit, pressure, authoritative OOM-event and `OOMKilled/137` verifier.
- An unrequested contradiction cannot poison an independently valid requested dimension.
- A requested trusted contradiction produces `LIVE_FAIL`.
- Missing trusted evidence produces `UNAVAILABLE`.
- Disk is unconditionally `UNAVAILABLE` in Wave 22.
- `cpu,memory,disk` therefore cannot become an aggregate `LIVE_PASS`.

Every positive verdict carries the exact trusted report binding, supports an optional exact expected-binding check, canonicalizes the requested dimension set, and seals a deterministic tamper-evident digest. `VerifyVerdict` validates only the public document domain and returns no authority-bearing object. `MarshalVerdict` verifies before encoding and can reject caller-specified forbidden material.

## Provenance boundary retained

The following paths remain impossible by public API:

- copyable `SourceLiveHost` label -> trusted authority;
- public `resourceproof.Report` -> `TrustedReport`;
- public `resourceproof.Report` -> `ProjectVerdict`;
- serialized Wave 22 `Verdict` -> `TrustedReport`;
- serialized Wave 22 `Verdict` -> capability evidence authority;
- exit code 137 alone -> memory enforcement proof;
- ambient host/guest OOM signal alone -> memory enforcement proof;
- disk absence -> verified aggregate resource enforcement.

A generic runtime producer/CLI remains deliberately deferred because the repository still lacks a package-owned primitive proving that a caller-facing Realm/policy/runtime binding and a concrete Cube sandbox/resource binding are the exact same realization. Wave 22 does not bypass this with caller-selected metadata.

## TDD evidence

### RED 1 — API surface absent

Wave 22 first introduced an executable reflection contract, without touching production implementation.

- RED head: `e8f4c711b0a71b254e9c8d903e282295f62b61a6`
- Nolane World Check run: `34113020268`
- job: `101713345918`
- exact behavioral failure: `Wave 22 RED: TrustedReport.ProjectVerdict is absent`

The test compiled and ran. This was not a harness/compiler failure.

### RED 2 — semantic reduction absent

After adding only a fail-closed API skeleton, the typed semantic matrix was added before semantic production logic.

- RED head: `d1076739a03e0a347a9d6de48fa291d1e06c0380`
- Nolane World Check run: `34113218599`
- job: `101714044882`

Observed behavioral failures included:

- trusted CPU remained `UNAVAILABLE` instead of `LIVE_PASS`;
- valid CPU was poisoned by an unrequested memory contradiction;
- requested contradiction did not return `ErrLiveFailed`;
- disk result was absent;
- exact expected binding did not pass.

Again, the test package compiled and executed; the RED was behavioral.

## GREEN evidence before closure-note commit

Implementation candidate `44b385bb56bc8377397932e9d900c71c6ad073d4` passed fresh exact-head checks:

- Wave 22 Public Live Verdict Contract run `34113624026`: PASS
  - static trust shape: PASS
  - focused Wave 22 behavior: PASS
  - complete resourceproof suite: PASS
  - resourceproof vet: PASS
- Nolane World Check run `34113624044`: PASS
  - unit: PASS
  - race: PASS
  - vet: PASS
  - historical deterministic/negative-control evidence generation: PASS
- Nolane Live Substrate Gauntlet run `34113624000`: PASS
  - unit/race/vet: PASS
  - hosted missing-live negative control remains non-PASS
  - live Cube/KVM proof skipped because no eligible live runner; no live-KVM claim is made
- Format Check run `34113623974`: PASS on amd64 and arm64
- Docs Build Check run `34113623977`: PASS
- DCO Check run `34113623973`: PASS
- Cube Kernel OOM Victim Contract run `34113624001`: PASS
- Wave 21 Cube Guest Kernel OOM Victim Contract was still finishing its canonical CubeBox builder leg when this note was authored; all completed Wave 21 guest, shim, protocol, and NolaneWorld legs were already PASS. Final-head closure requires that workflow to finish successfully as well.

## Public API delivered

- `TrustedReport.ProjectVerdict(VerdictRequest) (Verdict, error)`
- `VerifyVerdict(Verdict) error`
- `MarshalVerdict(Verdict, forbidden ...string) ([]byte, error)`
- closed resource dimensions: `cpu`, `memory`, `disk`
- closed dimension states: `VERIFIED`, `UNAVAILABLE`, `CONTRADICTED`
- exact expected-binding selector/check
- deterministic schema version `22` verdict digest

## Explicit non-claims

Wave 22 does not claim:

- live Cube/KVM availability on hosted CI;
- cryptographic attestation of the operator-selected Cubelet management endpoint;
- disk enforcement;
- disk-inclusive aggregate resource enforcement;
- universal kernel/process isolation;
- that Wave 18–21 OOM evidence by itself satisfies the existing trusted resource verifier;
- that a generic public resource-proof CLI can safely mint `LIVE_PASS` today.

## Final gate

This closure note itself changes the candidate SHA. Completion must therefore be based only on fresh checks attached to the post-note final SHA. Carried-forward checks above document development provenance, not the final integration gate.
