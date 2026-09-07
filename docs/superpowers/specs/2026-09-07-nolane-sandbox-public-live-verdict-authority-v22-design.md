# Wave 22 — Public Live Verdict Authority

## Status

Normative design for the Wave 22 implementation stacked on Wave 21.

## Context

Waves 11–21 progressively separated resource observations, runtime outcome, cgroup OOM evidence, host process identity, host-kernel victim provenance, and guest-kernel victim provenance from the authority to make a public capability claim. That separation is intentional: a copyable source label, exit code 137, a counter delta, a serialized report, or a positive OOM event is not by itself authority to emit `LIVE_PASS`.

Wave 22 activates the **public verdict projection** over evidence that is already inside a package-owned trusted boundary. It does not create a new observation authority and it does not weaken any earlier trust boundary to make activation easier.

A source audit before this design found one important remaining boundary: the current Cube `ResourceBinding` is opaque and binds host observations to an exact sandbox, while `resourceproof.Binding` binds a report to Realm ID/revision, policy digest, realization revision, and runtime digest. The repository still has no package-owned primitive proving that an arbitrary caller-supplied Realm/policy binding and a concrete Cube sandbox are the same realization. Wave 22 therefore MUST NOT introduce a generic constructor that accepts public Cube metrics plus public Realm metadata and mints trusted resource evidence. Such a constructor would launder correlation into authority.

Wave 22 is deliberately a projection layer: it accepts only `resourceproof.TrustedReport`, whose non-zero authority state is package-owned, and emits a serializable verdict document. A later live producer may feed this projection only after it can mint the existing trusted report without caller-controlled provenance.

## Goals

1. Expose a deterministic public verdict for explicitly requested resource dimensions.
2. Preserve `LIVE_PASS`, `LIVE_FAIL`, and `UNAVAILABLE` semantics.
3. Allow CPU, memory, and CPU+memory to be evaluated independently.
4. Keep disk unavailable until direct disk-enforcement evidence exists.
5. Keep aggregate resource enforcement unavailable when disk is required.
6. Bind every positive verdict to the exact trusted report binding.
7. Make `require-live` fail closed on unavailable evidence and on contradictory evidence.
8. Produce canonical, bounded, secret-safe JSON suitable for artifacts and audit.
9. Ensure serialized verdict bytes never become an authority token.
10. Preserve all Wave 11–21 non-claims.

## Non-goals

Wave 22 does not:

- mint `TrustedReport` from public observations;
- accept `resourceproof.Report` as trusted input;
- infer memory enforcement from exit 137, SIGKILL, `memory_failures_total`, TaskOOM, host victim evidence, or guest victim evidence alone;
- infer a negative OOM claim from absence of victim evidence;
- claim disk enforcement;
- claim aggregate resource enforcement when disk is requested;
- bind a caller-selected Realm/policy tuple to a Cube sandbox;
- turn a management endpoint into cryptographic attestation;
- persist or revive stale realization authority;
- grant capability authority to a serialized `Verdict` document.

## Public model

### Dimension

Wave 22 defines a closed resource dimension set:

- `cpu`
- `memory`
- `disk`

A request is a non-empty set of unique dimensions, with a hard maximum equal to the number of known dimensions. Unknown, empty, duplicate, or non-canonical dimensions are invalid input.

### Verdict

A public verdict contains:

- schema version `22`;
- mode (`probe` or `require-live`);
- status (`LIVE_PASS`, `LIVE_FAIL`, `UNAVAILABLE`);
- approved bit;
- exact `resourceproof.Binding` copied from the trusted report;
- canonical sorted requested dimension list;
- one result per requested dimension;
- evidence digest only for a verified dimension;
- a deterministic verdict digest.

The verdict MUST NOT contain raw cgroup paths, Cubelet URLs, task handles, sandbox locators, API keys, guest command text, host process IDs, realization tokens, or other authority handles.

### Dimension states

Each requested dimension is one of:

- `VERIFIED`: exact trusted evidence exists for that dimension;
- `UNAVAILABLE`: the trusted report does not establish that dimension;
- `CONTRADICTED`: trusted evidence exists but fails the causal verifier.

A serialized result is descriptive. It never carries the opaque trusted state.

## Status reduction

Reduction is deterministic:

1. If input is structurally invalid, return an invalid-input error and no approved verdict.
2. If the package-owned trusted report fails trusted verification, the result is `UNAVAILABLE` and never `LIVE_PASS`.
3. If any requested dimension is contradicted by trusted evidence, status is `LIVE_FAIL`, `approved=false`.
4. Else if any requested dimension is unavailable, status is `UNAVAILABLE`, `approved=false`.
5. Else all requested dimensions are verified: status is `LIVE_PASS`, `approved=true`.

`require-live` returns:

- success only with `LIVE_PASS`;
- `ErrLiveFailed` with `LIVE_FAIL`;
- `ErrLiveUnavailable` with `UNAVAILABLE`.

`probe` returns the same verdict document but treats `UNAVAILABLE` as an ordinary, non-success claim state rather than fabricating failure evidence. `LIVE_FAIL` remains an error because it represents a trusted contradiction.

## Resource semantics

### CPU

CPU can be `VERIFIED` only when the trusted report itself is a valid package-owned report and its CPU claim is `agentruntime.Verified` with a non-empty evidence digest. The underlying Wave 11 verifier still requires exact requested/effective quota+period and observed throttling under pressure.

### Memory

Memory can be `VERIFIED` only when the trusted report itself is a valid package-owned report and its memory claim is `agentruntime.Verified` with a non-empty evidence digest. The underlying verifier still requires exact requested/effective limit, attempted bytes above the limit, an authoritative OOM event delta, and authoritative `OOMKilled/137` task status from the trusted observation path. Exit 137 alone remains insufficient.

Wave 18–21 evidence may strengthen future trusted producers, but Wave 22 does not reinterpret those evidence families as a substitute for the existing trusted memory verifier.

### Disk

Disk is always `UNAVAILABLE` in Wave 22 because the trusted resource report intentionally has no direct disk proof. A request containing disk can never be `LIVE_PASS`.

### Aggregate resource enforcement

There is no implicit `all` shortcut. Callers must name dimensions. A caller requesting `cpu,memory,disk` receives `UNAVAILABLE` until disk proof exists. CPU+memory may pass without creating an aggregate disk-inclusive resource-enforcement claim.

## Binding rules

A verdict copies the exact binding from the `TrustedReport`. A caller may additionally provide an expected binding. If provided, it must match exactly:

- Realm ID;
- Realm revision;
- policy digest;
- realization revision;
- runtime digest.

Any mismatch is `UNAVAILABLE`, not repairable by changing report bytes. The expected binding is a selector/check, not a source of authority.

## Public-report forgery boundary

The public API MUST NOT expose a path of the form:

`Report -> TrustedReport -> LIVE_PASS`

or:

`Verdict bytes -> authority-bearing capability source`.

`resourceproof.VerifyReport` remains a validator for the public untrusted report domain and must continue rejecting caller-forged `LIVE_PASS` documents. Wave 22 consumes only `TrustedReport`.

## Canonicalization and verification

Verdict encoding is canonical JSON generated only after `VerifyVerdict` succeeds. Verification requires:

- exact schema version;
- valid mode/status relationship;
- canonical sorted unique requested dimensions;
- exactly one result per request in the same order;
- no evidence string on non-verified dimensions;
- non-empty evidence on verified dimensions;
- `approved=true` iff status is `LIVE_PASS` and all requested results are verified;
- deterministic digest match;
- no `LIVE_PASS` containing disk in Wave 22.

`VerifyVerdict` validates document integrity only. It does not restore provenance authority.

## CLI boundary

Wave 22 may expose a CLI only if it has a genuine package-owned producer of `TrustedReport`. A CLI that accepts trusted-looking JSON from disk MUST NOT be added. If no live producer is wired in this wave, the library verdict projection is the complete public activation surface and the CLI remains explicitly deferred rather than weakening provenance.

## TDD contracts

The implementation must prove at least:

1. a zero or forged trusted report cannot yield `LIVE_PASS`;
2. copied `SourceLiveHost` metadata and a public `Report` cannot enter the verdict projection;
3. exact valid trusted CPU request can pass;
4. exact valid trusted memory request can pass;
5. CPU+memory can pass when both trusted dimensions are verified;
6. disk request is unavailable;
7. CPU+memory+disk is unavailable and not approved;
8. trusted CPU contradiction yields `LIVE_FAIL` for CPU requests;
9. trusted memory contradiction yields `LIVE_FAIL` for memory requests;
10. expected-binding mismatch yields `UNAVAILABLE`;
11. `require-live` fails on unavailable and contradiction;
12. verdict marshal/verify is deterministic and tamper-evident;
13. serialized verdict cannot be fed back into `NewCapabilityEvidenceSource` or any other authority constructor;
14. previous v11 provenance-forgery tests remain green.

## Closure criterion

Wave 22 closes only when all focused resourceproof tests, NolaneWorld tests/race/vet, live gauntlet negative controls, formatting, documentation, and existing Wave 17–21 contracts remain green on the exact final SHA. Missing live infrastructure remains `UNAVAILABLE`; it is not a failed test of the source and never counts as `LIVE_PASS`.
