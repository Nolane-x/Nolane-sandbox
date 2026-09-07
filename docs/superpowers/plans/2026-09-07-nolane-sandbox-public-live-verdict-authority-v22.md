# Wave 22 — Public Live Verdict Authority Implementation Plan

**Goal:** expose a fail-honest public resource verdict over package-owned `resourceproof.TrustedReport` without creating a new authority source or weakening the v11–v21 provenance boundaries.

**Base:** Wave 21 exact head `65e75143d097c1ffaca6cd3d7450bfab0c825074`.

**Branch:** `gpt/wave22-public-live-verdict-authority`.

## Task 1 — RED: public verdict surface is absent

Add focused tests in `NolaneWorld/gauntlet/live/resourceproof` that detect the missing Wave 22 verdict surface without depending on a compiler failure. The first RED must execute and fail because the required API/behavior is absent, not because of a broken harness.

Contracts:

- `TrustedReport` must expose a Wave 22 verdict projection surface;
- zero/untrusted authority must not pass;
- the verdict domain is dimension-specific.

Run:

```sh
cd NolaneWorld
go test ./gauntlet/live/resourceproof -run 'V22|Wave22' -count=1
```

Record the exact RED workflow/run before production implementation.

## Task 2 — Minimal projection model

Create `NolaneWorld/gauntlet/live/resourceproof/verdict_v22.go` with:

- schema version 22;
- `ResourceDimension` closed set (`cpu`, `memory`, `disk`);
- `DimensionVerdictState` (`VERIFIED`, `UNAVAILABLE`, `CONTRADICTED`);
- `VerdictReason` bounded reason set;
- `VerdictRequest` containing mode, requested dimensions, and optional expected binding;
- immutable-value `Verdict` document;
- deterministic digest and canonical dimension ordering.

Do not add a constructor from public `Report`.

## Task 3 — Trusted reduction semantics

Implement projection from `TrustedReport` only.

Rules:

- verify the opaque trusted wrapper first;
- exact optional expected binding match;
- CPU and memory project only from verified trusted dimension proofs;
- trusted dimension causal mismatch projects `CONTRADICTED` / `LIVE_FAIL`;
- unavailable trusted provenance projects `UNAVAILABLE`;
- disk always unavailable in Wave 22;
- `LIVE_PASS` only when every requested dimension is verified;
- `approved=true` iff `LIVE_PASS`;
- `require-live` returns the live package's existing unavailable/failure sentinel errors.

Keep the report bytes descriptive only.

## Task 4 — RED/GREEN semantic matrix

Add typed tests after the surface exists:

- valid trusted CPU PASS;
- valid trusted memory PASS;
- CPU+memory PASS;
- disk UNAVAILABLE;
- CPU+memory+disk UNAVAILABLE;
- CPU causal mismatch FAIL;
- memory causal mismatch FAIL;
- expected binding mismatch UNAVAILABLE;
- zero `TrustedReport` UNAVAILABLE;
- invalid request fails closed;
- probe vs require-live error semantics.

Test trusted fixtures are created only inside package tests through the package-private trusted builder. External-package tests must continue proving callers cannot construct trusted state.

## Task 5 — Public document verifier and canonical marshal

Implement:

- `VerifyVerdict(Verdict) error`;
- `MarshalVerdict(Verdict, forbidden ...string) ([]byte, error)`.

Verifier checks exact schema, canonical dimensions/results, status reduction, evidence rules, approved bit, binding validity, disk non-PASS rule, and digest.

Add tamper tests for:

- status/approved flips;
- reordered/duplicate dimensions;
- evidence injected into unavailable result;
- missing evidence from verified result;
- binding mutation;
- digest mutation;
- secret/forbidden-string scan.

`VerifyVerdict` MUST NOT produce or return any opaque authority type.

## Task 6 — Provenance regression guard

Extend `provenance_v11_external_test.go` or add `provenance_v22_external_test.go` in `resourceproof_test` to prove:

- public callers cannot construct a passing `TrustedReport`;
- public `Report` plus copied `SourceLiveHost` still cannot be projected to a trusted verdict;
- serialized `Verdict` has no authority constructor back into `TrustedReport` or capability evidence.

No reflection/unsafe escape hatch may be added to production.

## Task 7 — Static trust contract

Add `tests/wave22_public_live_verdict_contract.py` to lock the production trust shape:

- projection accepts `TrustedReport`, not public `Report`;
- no JSON unmarshal-to-trusted path;
- disk cannot become verified in Wave 22;
- exact existing `VerifyTrustedReport` is invoked;
- existing `NewCapabilityEvidenceSource` still requires `TrustedReport`.

This contract is a regression guard, not a substitute for Go behavior tests.

## Task 8 — Dedicated CI contract

Add `.github/workflows/wave22-public-live-verdict-contract.yml` scoped to relevant NolaneWorld/resourceproof paths and docs/tests.

Jobs:

1. Python static trust contract;
2. focused Go Wave 22 tests;
3. external provenance tests;
4. `go test ./gauntlet/live/resourceproof`;
5. `go test ./...` for NolaneWorld when practical in the existing runner image/toolchain.

The hosted workflow does not claim a live Cube/KVM PASS.

## Task 9 — Full verification

On the final candidate SHA run/review fresh results for:

- Wave 22 dedicated contract;
- Nolane World Check;
- Nolane Live Substrate Gauntlet;
- Format Check;
- Docs Build Check;
- DCO/contribution provenance;
- Build Check and Unit Test Check when triggered;
- Wave 17/18/19/20/21 contracts to ensure no trust-boundary regression.

Do not accept carried-forward green runs as final-head evidence.

## Task 10 — Closure audit and PR

Before closure:

- inspect final diff for any public `Report -> TrustedReport` or JSON -> authority path;
- search for accidental disk verification/aggregate overclaim;
- confirm no CLI accepts trusted-looking serialized input;
- verify no source/credential/locator leak in verdict JSON;
- update a Wave 22 closure note with exact SHA and fresh CI run IDs;
- open a stacked draft PR against `gpt/wave21-guest-kernel-oom-victim-provenance`;
- keep it unmerged unless explicitly authorized.

## Explicit follow-up

A genuine runtime CLI that mints positive resource verdicts remains blocked until a package-owned producer can bind the exact Realm/policy/runtime realization to the exact Cube sandbox and its host/guest observations. Wave 22 must expose that absence honestly rather than laundering caller-selected metadata into provenance.
