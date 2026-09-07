# Wave 23 Exact Realm/Cube Realization Binding Authority — Closure

## Scope

Wave 23 closes the repository-local provenance gap between the current host-owned Realm/World realization and the concrete Cube `ResourceBinding` used for host resource observation. It does not manufacture runtime provenance or turn descriptive serialized state into authority.

Base / parent wave:

- Wave 22 final head: `6027b91a131bee8371bcb6b392557100902d63c8`

Verified Wave 23 implementation/trust-shape head before this closure-note commit:

- `c7ae08808f0b2d4a44a904ab0169485d54da532e`

## Closed trust boundaries

### Realm realization authority

`realm.Controller.CurrentRealizationAuthority` now mints a sealed, non-serializable authority only when all of the following are current at the same package-owned store boundary:

- exact Realm ID and current Realm revision;
- canonical `PolicyDigest(current.Spec, current.Revision)`;
- exact World ID;
- non-zero World realization revision;
- a live authoritative World phase (`observed-ready`, `leased`, or `paused`);
- non-empty substrate handle;
- an exact trusted concrete Store type (`*realm.MemoryStore` or `*realm.DurableStore`).

`ValidateRealizationAuthority` re-reads current Realm and World state, requires the exact Store instance that minted the capability, and rejects revision, policy, phase, realization-revision, handle, Store-instance, or Store-type drift.

### Handle transition fence

Both MemoryStore and DurableStore reject replacement of an established non-empty substrate handle while `RealizationRevision` is unchanged. Durable journal replay goes through the same validator. This prevents a same-revision `A -> B -> A` descriptive handle cycle from resurrecting an old authority.

### Exact Cube bridge

`cube.ValidateRealmResourceAuthority` revalidates the Realm capability immediately before bridging it and requires exact non-empty equality between the sealed current Realm substrate handle and `ResourceBinding.SandboxID()`. Only then does it mint opaque `RealmResourceAuthority`.

The resulting capability means only: the current Realm/World realization and the supplied package-owned Cube resource binding name the same exact repository-visible Cube sandbox identity at validation time.

## Behavioral RED -> GREEN evidence

### Caller-owned Store could mint package authority

RED commit:

- `4c6ff949eb42583c4de8726a67b0eadd572090bd` — `test(wave23): prove caller stores cannot mint authority`

Fresh RED workflow:

- Wave 23 Realm Cube Realization Contract run `34125633157`
- behavior job `101753532600`
- failing test: `TestV23CallerOwnedStoreCannotMintPackageAuthority`
- observed failure: caller-owned Store minted an authority with `err=<nil>` when the contract required unavailable.

GREEN repair:

- `99d8c94a57043e9f142711d3ea69049227648950` — `fix(wave23): seal realization authority to package-owned stores`

### Authority could cross byte-identical Store instances

RED commit:

- `9fbc206ca93ab684e4accc99503a1135cc923002` — `test(wave23): prove authority cannot cross store instances`

Fresh RED workflow:

- Wave 23 Realm Cube Realization Contract run `34126110465`
- behavior job `101755047730`
- failing test: `TestV23AuthorityCannotCrossIdenticalStoreInstances`
- observed failure: authority minted by Store A validated against byte-identical Store B with `err=<nil>`.

GREEN repair:

- `a7471fdc404a9292a7030a3c36338c0f2da05569` — `fix(wave23): bind authority to exact store instance`

### Embedded concrete Store inherited authority through method promotion

RED commit:

- `576a3c5a722d3eebfb3240720b8a1f528535c6c1` — `test(wave23): reject embedded store authority`

Fresh RED workflow:

- Wave 23 Realm Cube Realization Contract run `34126650369`
- behavior job `101756783040`
- failing test: `TestV23EmbeddedPackageStoreCannotMintPackageAuthority`
- observed failure: an external wrapper embedding the package Store minted authority with `err=<nil>`.

GREEN repair:

- `fdd07c1e12ad7c2487ee5a93407f3c74c5ee2e1f` — `fix(wave23): seal authority to exact store types`

### Same-revision handle alias

The Wave 23 history also contains a dedicated RED proving that a same-revision substrate-handle transition could resurrect descriptive authority:

- `183869655611a9e12fc5a7f05cba684b23d76161` — `test(wave23): prove handle alias can resurrect authority`
- MemoryStore repair: `18dfc6726436a999a5cd40b2724f5ebdb287c906` — `fix(wave23): fence same-revision MemoryStore handle rebinding`

The final contract extends this trust shape to DurableStore and durable journal replay.

## Exact implementation-head verification

On exact implementation/trust-shape head `c7ae08808f0b2d4a44a904ab0169485d54da532e`, the required Wave 23 gates completed successfully:

- Wave 23 Realm Cube Realization Contract — run `34127419367` — `success`
- Nolane World Check — run `34127419422` — `success`
- Nolane Live Substrate Gauntlet — run `34127419361` — `success`
- Format Check — run `34127419344` — `success`
- Docs Build Check — run `34127419369` — `success`
- DCO Check — run `34127419420` — `success`
- Cube Task Outcome Contract — run `34127419357` — `success`
- Cube Kernel OOM Victim Contract — run `34127419389` — `success`
- Cube Realization OOM Contract — run `34127419288` — `success`
- Cube Host Process Identity Contract — run `34127419325` — `success`

This closure-note commit is metadata-only. Its exact head must be freshly re-verified before the PR is described as final.

## Explicit non-claims

Wave 23 deliberately does **not** claim any of the following:

- RuntimeDigest provenance. Wave 23 does not mint or guess a runtime digest and does not enable the deferred generic public LIVE_PASS/resource-proof CLI.
- Disk enforcement.
- CPU, memory, OOM, process, or task-outcome proof merely from `RealmResourceAuthority`; those remain separate evidence families with their own authority requirements.
- OOM causality from exit code 137, memory failure counters, or ambient host/guest signals.
- Equality between Realm `RealizationRevision` and Cube task `Generation`; those are distinct namespaces and are never substituted for one another.
- Cryptographic attestation of the configured Cube/Cubelet management endpoint.
- A provider-level guarantee that a literal Cube `sandboxID` can never be destroyed and later reused for a different remote incarnation. The current Cube create/connect API exposed to this package supplies the sandbox ID and access material but no general immutable incarnation identifier suitable for this bridge. Wave 23 therefore does not fabricate a local nonce and mislabel it as remote provenance. Any future requirement to survive provider-level ID reuse must be closed by an authoritative Cube incarnation/generation token at the generic resource-binding boundary.

## Closure criterion

Wave 23 is code-closed when this documentation-only head receives fresh success from the dedicated Wave 23 contract, Nolane World Check, live-substrate negative-control gate, Format, Docs, and DCO. The PR remains stacked and draft; this note authorizes neither Ready-for-review transition nor merge.
