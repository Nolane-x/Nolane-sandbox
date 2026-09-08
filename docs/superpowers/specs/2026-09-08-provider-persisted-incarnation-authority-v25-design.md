# Wave 25 — Provider-Persisted Sandbox Incarnation Authority

## Status

Design contract for Wave 25. Stacked on the exact Wave 24 code-closure head `e29a956310b0e420a7b57662567307376322ed8d`.

## Problem

Wave 24 proves a non-aliasing Cubelet-local realization identity:

```text
{ sandbox_id, Wave17 generation, Cubelet realization epoch token }
```

That closes `Clear -> Start` reuse inside one Cubelet producer lifecycle, but the epoch exists only in Cubelet memory. After Cubelet restart, Wave 24 intentionally refuses to reconstruct prior epoch authority. It also cannot distinguish a remote sandbox that was deleted and later recreated with the same literal `sandbox_id` unless some provider-owned identity survives independently of Cubelet.

The current NolaneWorld Cube client exposes only `sandboxID` from `POST /sandboxes`, even though CubeAPI itself owns the create boundary and CubeMaster persists internal sandbox annotations. CubeAPI already reads those annotations back through sandbox detail on `GET`, `connect`, and `resume`. Public `NewSandbox` has no caller-controlled `annotations` field.

Therefore Wave 25 can close the next trust seam without inventing identity from timestamps, host IDs, request IDs, or local counters: CubeAPI can mint an incarnation nonce at remote creation and persist it as an internal CubeMaster annotation attached to that exact sandbox object.

## Scope

Wave 25 introduces a **provider-persisted sandbox incarnation authority** rooted in the configured CubeAPI/CubeMaster trust boundary.

The authoritative provider identity tuple is:

```text
{ sandbox_id, provider_incarnation_id }
```

and the strongest cross-boundary identity after Wave 25 is:

```text
fresh Realm realization
+ exact Cube ResourceBinding
+ fresh Wave24 Cubelet realization epoch
+ fresh provider-persisted incarnation proof
```

The incarnation ID is not a credential. Secrecy is not its trust property. Its authority comes from being minted by CubeAPI at create time, written into an internal provider annotation that public sandbox-create input cannot set, and freshly re-read from CubeMaster before bridge authority is accepted.

## Provider incarnation representation

Use an exact canonical UUIDv4 string minted with the already-declared Rust `uuid` dependency:

```text
xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
```

Requirements:

1. non-empty canonical lowercase UUID text;
2. UUID version exactly 4;
3. minted independently from CubeMaster request ID, `sandbox_id`, host ID, timestamps, Realm IDs, World IDs, Wave24 token, and client metadata;
4. persisted under one reserved internal annotation key, proposed:

```text
nolane.provider.incarnation.v1
```

5. never reconstructed when the annotation is missing or malformed.

A single UUIDv4 provides sufficient non-aliasing entropy for this identity role while reusing an existing dependency. The design does not equate UUID randomness with cryptographic attestation of the endpoint.

## Required properties

### 1. Producer-owned minting

CubeAPI, not NolaneWorld, Cubelet, Realm, metadata callers, or CubeMaster request IDs, mints the provider incarnation ID immediately before the create request is sent to CubeMaster.

### 2. Provider persistence

The exact ID is inserted into the internal `CreateSandboxRequest.annotations` map and therefore travels with the remote sandbox object. CubeAPI restart and Cubelet restart must not change the ID for the same surviving sandbox.

### 3. Remote reuse non-aliasing

A later create operation that happens to receive the same literal `sandbox_id` must receive a fresh provider incarnation ID. An old proof cannot validate against the recreated object.

### 4. Caller cannot nominate authority

`NewSandbox` must not gain a public `annotations` field or an incarnation input field. Caller metadata with a key that resembles the reserved annotation key is not provider authority because metadata is stored as labels, not the internal annotation.

### 5. No operation-ID promotion

CubeMaster/CubeAPI request IDs and existing `clientID` fields are operational identifiers and must not be promoted to sandbox-incarnation authority. Create, connect, and resume currently populate `clientID` from different backend values, so it is explicitly non-authoritative for Wave 25.

### 6. No timestamp promotion

`startedAt`, `create_at`, local `time.Now`, or any hash derived from time/sandbox ID is forbidden as an incarnation substitute.

### 7. Additive API projection

CubeAPI may add an optional read-only response field to `Sandbox` and `SandboxDetail`:

```json
{"incarnationID":"<canonical-uuid-v4>"}
```

For newly created Wave25 sandboxes it is present. For pre-Wave25 or otherwise unproven sandboxes it is absent rather than synthesized.

### 8. Strict NolaneWorld observation

NolaneWorld obtains provider incarnation only through the configured hardened Cube `Client` and the existing CubeAPI HTTPS/loopback trust rules. It must reject:

- missing incarnation field;
- malformed UUID;
- uppercase/non-canonical UUID text;
- UUID version other than 4;
- empty or mismatched sandbox ID;
- duplicate/ambiguous identity if an endpoint shape ever permits it;
- observations from a different Cube client authority context when validating a sealed proof.

### 9. Opaque in-process proof

`ProviderIncarnationProof` is package-owned and sealed. Descriptive `{sandbox_id, incarnation_id}` fields, JSON, copied API response structs, or caller strings cannot reconstruct authority.

The proof retains the exact `*Client` trust context that observed it, similar to Wave23 Realm authority retaining its exact Store authority context.

### 10. Freshness at bridge time

A previously sealed provider proof is insufficient by itself. Bridge validation must make a fresh CubeAPI observation of the sandbox and require exact equality with the sealed provider incarnation proof before minting Wave25 cross-boundary authority.

This is the remote counterpart to Wave24 re-scraping the current Cubelet realization epoch.

### 11. Wave24 remains mandatory

Provider incarnation does not replace Wave24. Provider identity answers “which remote sandbox object?”; Wave24 answers “which current Cubelet realization of that sandbox?”. Wave25 bridge authority requires both.

### 12. Fail closed for legacy objects

A sandbox created before Wave25 with no provider incarnation annotation is observationally unavailable. It is never backfilled from sandbox ID, host ID, request ID, timestamps, metadata, or the Wave24 token.

## Proposed CubeAPI producer shape

CubeAPI already constructs internal annotations in `CubeAPI/src/services/sandboxes.rs` before creating `CreateSandboxRequest`. Add one reserved key and helpers such as:

```rust
const PROVIDER_INCARNATION_ANNOTATION: &str = "nolane.provider.incarnation.v1";

fn new_provider_incarnation_id() -> String {
    uuid::Uuid::new_v4().hyphenated().to_string()
}

fn provider_incarnation_from_annotations(
    annotations: &std::collections::HashMap<String, String>,
) -> Option<String>;
```

The exact implementation may use a small focused module if that keeps the large sandbox service file manageable, but the authority semantics above are mandatory.

`create_sandbox` must mint once per create request and insert it before the CubeMaster call. `get_sandbox`, `connect_sandbox`, and `resume_sandbox` must project only the exact persisted annotation value; they must not mint or repair it.

## Proposed NolaneWorld proof shape

A dedicated file should keep authority separate from ordinary API DTOs:

```go
type ProviderIncarnationProof struct {
    sandboxID     string
    incarnationID string
    client        *Client
    seal          *providerIncarnationProofSeal
}

func (c *Client) ObserveProviderIncarnation(
    ctx context.Context,
    resource ResourceBinding,
) (ProviderIncarnationProof, error)

func (c *Client) ValidateProviderIncarnation(
    ctx context.Context,
    resource ResourceBinding,
    proof ProviderIncarnationProof,
) error
```

The observer should use `GET /sandboxes/{sandboxID}` rather than trusting a caller-retained create response, because bridge freshness requires a current provider read.

## Wave25 bridge

Wave25 adds an opaque authority above Wave24, conceptually:

```go
type RealmResourceProviderIncarnationAuthority struct {
    local    RealmResourceEpochAuthority
    provider ProviderIncarnationProof
    seal     *realmResourceProviderIncarnationAuthoritySeal
}
```

Minting requires:

1. a fresh Wave24 `RealmResourceEpochAuthority` for the same `ResourceBinding`;
2. a valid sealed provider proof from the exact Cube client trust context;
3. a fresh provider re-observation;
4. exact sandbox equality;
5. exact provider incarnation equality.

If the provider object is deleted/recreated under the same literal sandbox ID, the old provider proof must fail even if later local numeric generations happen to repeat.

## Trust model and explicit non-claims

Wave25 closes identity only inside the selected CubeAPI/CubeMaster authority context. It does **not** claim:

- cryptographic or hardware attestation that a configured API endpoint is the intended provider;
- immunity if the trusted CubeAPI/CubeMaster control plane maliciously rewrites reserved annotations;
- global uniqueness across unrelated providers that do not share this protocol;
- that incarnation identity proves task outcome, OOM causality, resource enforcement, filesystem state, or guest state;
- that the provider incarnation alone can mint a Realm or LIVE_PASS authority;
- that missing legacy incarnation can be repaired safely.

## TDD closure requirements

Wave25 is not code-closed until committed RED -> GREEN evidence proves at minimum:

1. create mints a canonical provider incarnation and persists it in the reserved internal annotation;
2. public input cannot nominate the reserved annotation authority;
3. get/connect/resume return the same persisted incarnation for one surviving sandbox and never remint it;
4. missing/malformed persisted incarnation stays unavailable;
5. two creates can reuse the same literal sandbox ID in a test backend but must have different incarnation IDs;
6. NolaneWorld rejects malformed/noncanonical/missing incarnation transport;
7. descriptive or serialized proof reconstruction is invalid;
8. proof from another `*Client` authority context is invalid;
9. stale provider proof fails after remote delete/recreate under the same sandbox ID;
10. exact fresh Wave24 authority + exact fresh provider proof mints Wave25 bridge authority;
11. stale Realm, stale Wave24 epoch, wrong sandbox, or stale provider incarnation fails closed;
12. dedicated Wave25 static contract forbids timestamp/request-ID/client-ID/Wave24-token reconstruction shortcuts;
13. fresh exact-head CI is green across Wave25, NolaneWorld, CubeAPI tests, Build, Unit, Format, Docs, DCO, and relevant prior trust contracts.

Autonomously-by: ChatGPT:GPT-5.6-Sol
