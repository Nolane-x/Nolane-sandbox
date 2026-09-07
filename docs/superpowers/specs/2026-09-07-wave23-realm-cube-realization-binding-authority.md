# Wave 23 — Exact Realm/Cube Realization Binding Authority

## Goal

Close the provenance gap between a host-owned Realm/World realization and the concrete Cube sandbox used by resource evidence, without allowing caller metadata, serialized documents, stale World records, or a coincidentally equal numeric generation to create authority.

## Existing truth boundaries

Wave 22 can project a public resource verdict only from `resourceproof.TrustedReport`, but caller-facing Realm identity is not yet cryptographically or structurally tied to the exact Cube sandbox/resource binding that produced the trusted host observations. `realm.PolicyDigest` is the canonical digest of a host-owned Realm spec and revision. `realm.WorldRecord.RealizationRevision` is the Realm World realization revision and MUST NOT be substituted with Cube controller-local task generation.

## Authority model

Add an opaque package-owned `realm.RealizationAuthority`. Its zero value is invalid and no public constructor accepts caller-supplied binding fields.

The authority describes exactly:

- Realm ID;
- Realm revision;
- canonical `realm.PolicyDigest` computed from the current host-owned `RealmRecord`;
- World ID;
- Realm World realization revision;
- substrate handle.

Authority is minted only by a live `realm.Controller` from its own store after re-reading both current Realm and World records. Minting fails closed unless:

1. Realm exists and is not closed;
2. World exists under that exact Realm and World ID;
3. World phase is an authority-bearing live phase: `observed-ready`, `leased`, or `paused`;
4. World realization revision is non-zero;
5. World substrate handle is non-empty;
6. the canonical policy digest for the current Realm revision can be computed.

`requested`, `creating`, and `terminal` Worlds cannot mint realization authority.

## Freshness

`Controller.ValidateRealizationAuthority` MUST re-read current store state. An authority becomes stale if any authoritative dimension changes, including Realm revision/spec/policy digest, World realization revision, World phase becoming non-authoritative/terminal, or substrate handle.

Validation MUST compare the sealed package-owned authority, not a caller-reconstructed descriptive value.

## Cube binding

The Cube package may consume a valid `realm.RealizationAuthority` through a dedicated verifier. It MUST additionally require the authority's exact substrate handle to equal the concrete package-owned `cube.ResourceBinding.SandboxID()`.

Successful verification produces an opaque `cube.RealmResourceAuthority` whose zero value is invalid. This object is only a proof that one current Realm World realization and one concrete Cube resource binding identify the same sandbox. It does not itself claim CPU, memory, disk, OOM, or runtime correctness.

The Cube verifier MUST revalidate the Realm authority through the owning `realm.Controller` before accepting it, so stale authorities cannot survive re-realization.

## Resource-proof projection

Wave 23 may expose a helper that derives the Realm/policy/realization portion of `resourceproof.Binding` only from the opaque current authority. It MUST NOT fabricate `RuntimeDigest`; runtime binding remains unavailable until a package-owned runtime provenance source exists.

Therefore Wave 23 does not enable a generic public `LIVE_PASS` CLI by itself.

## Fail-honest rules

- no authority from serialized JSON;
- no public constructor from IDs/revisions/digests/handles;
- no `ResourceBinding` sandbox mismatch acceptance;
- no stale authority after Realm update or World re-realization;
- no numeric-generation aliasing between Realm realization revision and Cube task generation;
- no exit-code/OOM inference;
- no disk proof;
- absence of exact current authority is `unavailable`, never PASS.

## Verification requirements

Behavioral tests must prove:

- pre-ready phases cannot mint;
- exact current ready/leased/paused realization can mint;
- zero authority is invalid;
- Realm revision update stales old authority;
- World realization revision change stales old authority;
- handle change stales old authority;
- terminal transition stales old authority;
- exact Cube `ResourceBinding` matches;
- wrong Cube sandbox is rejected;
- descriptive binding/JSON cannot restore opacity;
- policy digest equals `realm.PolicyDigest(current.Spec, current.Revision)`.

A dedicated Wave23 CI contract must run focused Realm/Cube tests and static checks for the absence of a public field-based authority constructor.
