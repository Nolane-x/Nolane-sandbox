# Wave 27 — Exact Runtime Realization Provenance Authority

## Status

Design contract for Wave 27. Stacked on the exact Wave 26 code-closure head `2aff6aaa846fce77c91a36038ca61b15d96b1a67`.

## Problem

Wave 22 can emit public `LIVE_PASS`/`LIVE_FAIL`/`UNAVAILABLE` verdicts only from the package-owned `resourceproof.TrustedReport` boundary. Wave 23 explicitly left `resourceproof.Binding.RuntimeDigest` unavailable until a package-owned runtime provenance source exists. Waves 24–26 then strengthened cross-boundary identity without changing that non-claim:

```text
fresh Realm realization
+ exact Cube ResourceBinding
+ fresh Cubelet realization epoch
+ fresh provider-persisted sandbox incarnation
+ fresh pinned TLS endpoint SPKI
```

That chain answers which Realm realization, which Cube sandbox, which Cubelet-local realization, which provider object and which configured control-plane endpoint participated. It still does not bind an exact current runtime process realization strongly enough to mint `RuntimeDigest` or to make any CPU/memory enforcement claim.

Wave 27 closes only that missing runtime provenance seam.

## Scope

Wave 27 introduces an opaque, in-process **runtime realization provenance authority** above Wave 26. Its purpose is to prove that one exact current host runtime process identity belongs to the same fresh realization already bound by Waves 23–26, then derive one deterministic runtime provenance digest from package-owned authority.

Wave 27 does **not** mint `resourceproof.TrustedReport`, does not classify CPU/memory enforcement, and does not emit `LIVE_PASS`.

The strongest identity after Wave 27 is:

```text
fresh Realm realization
+ exact Cube ResourceBinding
+ fresh Cubelet realization epoch
+ fresh provider incarnation
+ fresh endpoint SPKI
+ exact current host runtime process identity
```

## Runtime identity source

The existing Cubelet management metrics path already exports package-produced host process identity with the descriptive tuple:

```text
sandbox_id
generation
host_pid
starttime_ticks
boot_id
cgroup_path
runtime_role
source
placed_at
bound_at
```

For Wave 27, this tuple remains descriptive until consumed through a package-owned observer that binds it to the exact current Wave24 realization epoch in the **same bounded management scrape**. A caller-provided `HostSandboxProcessIdentityProof`, copied metric text or serialized document cannot mint runtime authority.

The required runtime role is exactly `cube-shim-vmm`. The required producer source is exactly `cubebox.cgroup.add_proc`.

## Same-scrape anti-TOCTOU rule

Wave 27 must not validate a Wave24 epoch in one HTTP scrape and then accept a host process identity from another scrape as if the pair were atomic. The realization may transition between requests.

A dedicated `RuntimeRealizationObserver` therefore performs one GET against the existing Cubelet resource-metrics endpoint and parses both:

- exactly one current `cubesandbox_realization_epoch_info` sample for the requested sandbox; and
- exactly one current `cubesandbox_host_process_identity_info` sample for the same sandbox.

The observer accepts the pair only when:

1. both samples are canonical and non-duplicate;
2. sandbox IDs are exact and equal to `ResourceBinding`;
3. Wave17 generation is exact and equal across both samples;
4. realization epoch token is canonical/non-zero;
5. runtime process identity is canonical under the existing Wave19 parser;
6. runtime role/source are the existing exact constants.

This same-scrape pair is the only Wave27 runtime observation authority.

## Opaque proof

Add a sealed package-owned proof conceptually shaped as:

```go
type RuntimeRealizationProof struct {
    epoch    RealizationEpochProof
    process  HostSandboxProcessIdentityProof
    observer *RuntimeRealizationObserver
    seal     *runtimeRealizationProofSeal
}
```

The exact implementation may retain equivalent private fields, but validity requires:

- package-owned seal;
- exact observer authority context;
- valid sealed epoch proof minted by that observer from the same scrape;
- canonical process identity;
- exact sandbox and generation equality.

The proof is not serializable authority. Descriptive accessors may return copies for diagnostics.

## Observer authority context

A runtime proof is bound to the exact `*RuntimeRealizationObserver` that minted it. A second observer with the same URL/client settings is a distinct authority context and cannot validate the first observer's proof by descriptive equality alone.

This mirrors the exact-context rules used by Realm Store authority and Cube provider/client authority.

## Freshness

`RuntimeRealizationObserver.ValidateCurrent` performs a new same-scrape observation and requires exact equality of all authoritative identity fields:

```text
sandbox_id
generation
realization_epoch_token
host_pid
starttime_ticks
boot_id
cgroup_path
runtime_role
source
placed_at
bound_at
```

Any drift makes the old proof stale. In particular:

- `Clear -> Start` with numeric generation reuse fails because Wave24 epoch changes;
- process restart/replacement fails because PID/starttime and/or process identity changes;
- host reboot fails because boot ID changes;
- cgroup/process rebinding fails;
- stale proof from another observer fails even when fields happen to match.

## Wave27 bridge

Add an opaque authority above Wave26, conceptually:

```go
type RealmResourceRuntimeAuthority struct {
    endpoint RealmResourceProviderEndpointAuthority
    runtime  RuntimeRealizationProof
    digest   string
    seal     *realmResourceRuntimeAuthoritySeal
}
```

Minting requires:

1. a structurally valid Wave26 authority;
2. fresh reconstruction of the full Wave26 chain through existing validators, not shape-only acceptance;
3. exact `ResourceBinding` sandbox equality;
4. a valid runtime proof from the exact supplied runtime observer;
5. fresh same-scrape runtime revalidation;
6. exact Wave24 epoch identity equality between the embedded Wave26 chain and the runtime proof;
7. deterministic runtime digest derivation from the freshly validated authoritative fields.

The bridge must not accept caller-provided Realm IDs, revisions, policy digests, runtime digests, sandbox IDs, PIDs, cgroup paths, timestamps or hashes as authority inputs.

## Runtime digest semantics

Wave 27 defines a deterministic descriptive digest namespace:

```text
runtime-realization-v27:<64-lowercase-hex>
```

The digest is computed only after successful Wave27 authority minting over canonical authority-owned fields. The preimage includes, at minimum:

```text
Realm ID
Realm revision
Policy digest
World ID
Realm realization revision
sandbox ID
Wave17 generation
Wave24 realization epoch token
Wave25 provider incarnation ID
Wave26 endpoint SPKI SHA-256
host PID
host process starttime ticks
host boot ID
host cgroup path
runtime role
runtime source
placed_at
bound_at
```

The digest is a runtime **provenance identifier**, not a binary/image measurement, hardware attestation, secret, credential, resource-enforcement proof or outcome proof.

Public code may inspect the digest only through a valid Wave27 authority accessor. No public constructor accepts a digest string.

## Binding projection

Wave 27 may expose a helper that derives the complete `resourceproof.Binding` descriptive tuple from fresh authority:

```text
RealmID
RealmRevision
PolicyDigest
RealizationRevision
RuntimeDigest
```

The helper must derive Realm/policy/realization fields from the sealed Realm authority embedded in the Wave26 chain and the runtime digest from Wave27. It must not accept caller-supplied binding fields.

This projection is descriptive and is not itself a `TrustedReport` constructor.

## Fail-closed rules

Wave 27 rejects or makes stale:

- zero/forged/deserialized runtime proof;
- duplicate/malformed/missing epoch or process identity samples;
- wrong sandbox;
- generation mismatch between epoch and process identity;
- cross-observer proof reuse;
- stale Realm realization;
- stale Wave24 epoch;
- stale Wave25 provider incarnation;
- rotated Wave26 endpoint SPKI proof;
- process replacement with the same sandbox ID;
- host reboot;
- cgroup/process rebinding;
- any caller-supplied runtime digest used as authority.

## Explicit non-claims

Wave 27 does not prove:

- CPU enforcement;
- memory enforcement;
- disk enforcement;
- aggregate resource enforcement;
- OOM causality or victim identity;
- task success/failure;
- filesystem isolation;
- guest-state correctness;
- image/binary measurement;
- hardware/TEE attestation;
- `resourceproof.TrustedReport` provenance by itself;
- public `LIVE_PASS` by itself.

The runtime process identity may later be one input to a package-owned trusted causal resource producer, but Wave 27 alone cannot elevate observations to resource enforcement.

## TDD closure requirements

Wave 27 is not code-closed until committed RED -> GREEN evidence proves at minimum:

1. runtime observer requires one canonical epoch and one canonical host process identity in the same scrape;
2. missing/duplicate/malformed epoch fails closed;
3. missing/duplicate/malformed process identity fails closed;
4. sandbox or generation mismatch fails closed;
5. zero/forged/deserialized runtime proof is invalid;
6. proof is exact-observer-bound;
7. old proof becomes stale after epoch rotation even if numeric generation repeats;
8. old proof becomes stale after process PID/starttime replacement;
9. old proof becomes stale after host boot-ID change;
10. exact fresh Wave26 authority + exact fresh runtime proof mints Wave27 authority;
11. stale Realm, stale Wave24, stale Wave25, stale/rotated Wave26 endpoint, wrong ResourceBinding or cross-observer proof fails closed;
12. runtime digest is deterministic, namespaced and changes when any authoritative runtime identity dimension changes;
13. binding projection uses sealed Realm provenance and Wave27 runtime digest only;
14. no public field-based runtime-authority or runtime-digest constructor exists;
15. static Wave27 contract forbids PID-only, timestamp-only, sandbox-hash, endpoint-SPKI-only and caller-supplied-runtime-digest shortcuts;
16. focused tests, `go vet`, NolaneWorld tests/race, prior Wave23–26 trust contracts and exact-final-head CI are green.

## Next seam

After Wave27, the next safe seam is a package-owned causal resource producer that combines fresh Wave27 runtime provenance with direct CPU/memory observations and authoritative task/OOM evidence before minting `resourceproof.TrustedReport`. That later wave is where `LIVE_PASS` activation becomes meaningful; it is intentionally not part of Wave27.

Autonomously-by: ChatGPT:GPT-5.6-Sol
