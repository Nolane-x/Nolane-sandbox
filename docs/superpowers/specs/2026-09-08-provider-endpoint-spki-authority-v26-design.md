# Wave 26 — Provider Endpoint SPKI Identity Authority

## Status

Design contract for Wave 26. Stacked on the exact Wave 25 code-closure head `cbf47fa8b0c652052aa12b7a092ff411c1fc24cb`.

## Problem

Wave 25 closes remote sandbox incarnation aliasing inside the configured CubeAPI/CubeMaster authority context:

```text
{ sandbox_id, provider_incarnation_id }
```

NolaneWorld freshly re-reads that provider-persisted identity before minting Wave25 authority. However, Wave25 intentionally does not prove that the HTTPS endpoint configured as CubeAPI is cryptographically the intended provider endpoint. The current Cube client accepts caller-supplied `HTTPClient` objects, and the mere presence of an `https://` URL does not by itself create a narrow provider identity authority.

The remaining seam is therefore transport identity: a correct sandbox incarnation obtained from the wrong endpoint, a TLS-intercepted endpoint, or a custom transport that bypasses the intended TLS verification boundary must not mint the strongest cross-boundary authority.

Wave26 closes that seam with an explicit, canonical allow-set of SHA-256 SubjectPublicKeyInfo (SPKI) pins verified during the TLS handshake while retaining ordinary certificate-chain and hostname verification.

## Scope

Wave26 introduces **provider endpoint cryptographic identity authority** for the CubeAPI control-plane client.

The endpoint identity is:

```text
SHA-256(leaf_certificate.RawSubjectPublicKeyInfo)
```

represented as exactly 64 lowercase hexadecimal characters.

The strongest cross-boundary identity after Wave26 is:

```text
fresh Realm realization
+ exact Cube ResourceBinding
+ fresh Wave24 Cubelet realization epoch
+ fresh Wave25 provider-persisted sandbox incarnation
+ fresh Wave26 TLS endpoint SPKI proof
```

Wave26 is deliberately client-side and additive. It does not invent a new HTTP self-attestation header, endpoint UUID, timestamp, request ID, or provider-declared identity field.

## Trust root

The trust root is an explicit configured set of accepted SPKI SHA-256 digests.

Requirements:

1. every configured pin is exactly 64 lowercase hexadecimal characters;
2. each pin decodes to exactly 32 bytes;
3. the all-zero digest is invalid;
4. duplicate pins are invalid rather than silently collapsed;
5. an empty pin-set means Wave26 endpoint authority is unavailable, not implicitly trusted;
6. the configured pin-set is immutable inside a constructed `Client`;
7. key rotation is supported only by an explicitly configured overlapping pin-set containing old and new pins during the rotation window;
8. the client never learns, persists, or auto-adopts an unconfigured observed key.

## Required transport properties

### 1. Handshake-time enforcement

SPKI validation must occur inside the TLS handshake, before any HTTP request bytes, API key header, or request body are transmitted.

Post-response certificate inspection alone is insufficient because credentials may already have crossed the boundary.

### 2. Preserve WebPKI/hostname verification

Pinning is an additional constraint, not a substitute for TLS verification. The transport must continue to require normal certificate-chain and hostname validation using Go's standard TLS machinery and any explicitly supplied root CA pool.

`InsecureSkipVerify=true` is forbidden for Wave26-pinned API clients.

### 3. No bypass-capable custom transport

When endpoint pins are configured, the control-plane `HTTPClient.Transport` must be either:

- `nil`, in which case a clone of `http.DefaultTransport` is used; or
- a `*http.Transport` that can be safely cloned and hardened.

A non-`*http.Transport` RoundTripper is rejected for pinned authority because it can fabricate responses or bypass TLS entirely.

A custom `DialTLS`, `DialTLSContext`, or any equivalent hook that bypasses `tls.Config.VerifyConnection` is rejected.

A custom ordinary `DialContext` may remain allowed because TLS certificate verification still occurs after connection establishment.

### 4. Remote HTTPS requirement

Wave26 endpoint authority is available only for Cube API URLs using `https://`.

Existing loopback `http://` support remains available for ordinary development/client operations, but it cannot mint an endpoint cryptographic proof or the Wave26 bridge authority.

### 5. Existing custom trust roots remain usable

A caller may provide an `*http.Transport` with explicit `TLSClientConfig.RootCAs` for private PKI or tests. Wave26 clones the transport and TLS config, forces secure verification semantics, and layers the SPKI allow-set check into `VerifyConnection` without weakening pre-existing verification callbacks.

## Canonical configuration

Extend Cube `Config` with a dedicated field:

```go
EndpointSPKIPins []string
```

`New` validates and decodes this list exactly once. The constructed `Client` retains an immutable internal set/map of `[32]byte` digests plus the canonical configured strings if descriptive access is required by tests.

Pin material is configuration, not a credential, but malformed pin configuration is a hard configuration error.

## TLS verification algorithm

For a pinned HTTPS control-plane client:

1. clone the selected `http.Transport`;
2. clone or create its `tls.Config`;
3. reject `InsecureSkipVerify=true`;
4. reject transport TLS-dial hooks that can bypass the standard handshake;
5. retain the original `VerifyConnection` callback, if any;
6. install a composed `VerifyConnection` callback that:
   - first runs the original callback, if present;
   - requires at least one peer certificate;
   - computes `sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)`;
   - requires exact membership in the immutable configured pin-set;
   - otherwise returns a typed endpoint identity mismatch error;
7. use this hardened transport for every CubeAPI control-plane request.

Standard Go verification runs before `VerifyConnection` when `InsecureSkipVerify` is false, so certificate chain and hostname failures remain fatal before the pin callback can authorize anything.

## Endpoint observation proof

Wave26 adds a sealed in-process proof conceptually shaped as:

```go
type ProviderEndpointSPKIProof struct {
    spkiSHA256 [32]byte
    client     *Client
    seal       *providerEndpointSPKIProofSeal
}
```

The proof is minted only from an actual successful HTTPS response made through the exact pinned `*Client` transport. The response TLS state is inspected descriptively after the handshake to record which allowed pin was actually presented.

A proof is valid only when:

- its package-owned seal is exact;
- it retains the exact `*Client` that observed it;
- its digest is non-zero;
- the digest belongs to that client's configured immutable pin-set.

JSON, descriptive hex strings, copied certificates, caller-provided digest bytes, or a proof from another Client cannot reconstruct authority.

## Observation and freshness API

Wave26 should expose focused methods such as:

```go
func (c *Client) ObserveProviderEndpointSPKI(
    ctx context.Context,
) (ProviderEndpointSPKIProof, error)

func (c *Client) ValidateProviderEndpointSPKI(
    ctx context.Context,
    proof ProviderEndpointSPKIProof,
) error
```

Observation performs a lightweight existing CubeAPI control-plane request through the same hardened client. Prefer an existing stable endpoint such as `GET /health` if it already exists; otherwise use an existing authenticated/non-mutating API path already guaranteed by the Cube client contract. No new trust endpoint is created solely to self-report identity.

If a dedicated health path is used, its content is not authority; only the successful TLS handshake and peer SPKI are.

Fresh validation performs another pinned HTTPS request and requires exact equality with the sealed observed digest. This makes a previously valid proof stale after the server rotates to another pin in the accepted overlap set. A caller must explicitly obtain a fresh proof for the newly active key before minting new Wave26 authority.

This deliberate exact-key freshness prevents an old proof from silently surviving endpoint-key rotation merely because both keys are configured.

## Rotation semantics

Suppose configured pins are `{A, B}` during a planned rotation.

- old endpoint presents A: observation mints proof A;
- endpoint rotates to B: ordinary CubeAPI requests still succeed because B is configured;
- validation of old proof A fails as stale because fresh observation is B;
- a new observation mints proof B;
- after all clients have migrated, configuration may remove A.

Wave26 therefore distinguishes "trusted by current configuration" from "same endpoint key as the proof that participated in this authority mint".

## Wave26 bridge

Wave26 adds an opaque authority above Wave25, conceptually:

```go
type RealmResourceProviderEndpointAuthority struct {
    provider RealmResourceProviderIncarnationAuthority
    endpoint ProviderEndpointSPKIProof
    seal     *realmResourceProviderEndpointAuthoritySeal
}
```

Minting requires:

1. a structurally valid Wave25 `RealmResourceProviderIncarnationAuthority`;
2. fresh revalidation of its embedded Wave24 Realm/resource/epoch authority through the existing Wave25 mint path;
3. fresh revalidation of the exact provider incarnation through the exact Cube `*Client`;
4. a valid sealed endpoint SPKI proof from that same `*Client`;
5. a fresh endpoint observation with exact digest equality;
6. exact ResourceBinding sandbox equality across all embedded authorities.

The bridge must not accept a Wave25 authority merely by shape. It must reconstruct freshness through existing validators so stale Realm, stale Cubelet epoch, remote same-ID recreation, cross-client proof reuse, and endpoint-key rotation remain fail-closed.

## Error taxonomy

Wave26 should distinguish at least:

- invalid endpoint pin configuration;
- endpoint pin mismatch during TLS handshake;
- endpoint authority unavailable (no HTTPS/pins/no usable TLS peer state);
- invalid/forged endpoint proof;
- stale endpoint proof after accepted key rotation;
- invalid Wave26 bridge authority.

Errors may wrap transport errors but callers must be able to use `errors.Is` for the Wave26 trust failures above.

## No authority laundering

The following are explicitly forbidden substitutes for SPKI authority:

- API URL hostname hash;
- certificate serial number alone;
- certificate fingerprint of raw leaf DER instead of the specified SPKI hash;
- HTTP response headers such as `Server`, `X-Provider-ID`, or a self-reported key hash;
- API key value;
- Wave25 provider incarnation ID;
- sandbox ID;
- Realm/World ID;
- request ID, client ID, timestamp, host ID, process ID, or Cubelet epoch token;
- automatic trust-on-first-use persistence.

## Backwards compatibility

Existing Cube clients without `EndpointSPKIPins` continue to operate under the prior HTTPS/loopback rules. They can use Wave25 and earlier functionality but cannot mint Wave26 endpoint authority.

Wave26 does not require CubeAPI server-side protocol changes unless a stable existing non-mutating observation route is absent. If no such route exists, adding a minimal health route is allowed only as a liveness trigger for the TLS handshake; response content must not become identity evidence.

## Trust model and explicit non-claims

Wave26 proves only that the exact configured Cube client successfully communicated, under ordinary TLS verification plus explicit SPKI pinning, with a peer presenting the exact allowed public key represented by the proof.

Wave26 does **not** claim:

- hardware/TEE attestation;
- possession of a private key beyond what the completed TLS handshake already establishes;
- that a public CA certificate alone is a provider-global identity;
- that compromise of an explicitly pinned private key is detectable;
- that SPKI identity proves task outcome, OOM causality, resource enforcement, filesystem state, guest state, or sandbox correctness;
- that endpoint identity replaces the Wave25 sandbox incarnation or Wave24 local realization epoch;
- generic `LIVE_PASS` promotion.

## TDD closure requirements

Wave26 is not code-closed until committed RED -> GREEN evidence proves at minimum:

1. malformed, uppercase, short, duplicate and all-zero configured pins fail closed;
2. an empty pin-set cannot mint Wave26 proof;
3. pinned authority requires HTTPS;
4. remote TLS with the exact configured leaf SPKI succeeds;
5. a different otherwise-valid TLS certificate/key fails during handshake before request handling receives credentials/body;
6. ordinary hostname/chain validation is still required;
7. `InsecureSkipVerify=true` is rejected for pinned clients;
8. non-`*http.Transport` and TLS-dial bypass hooks are rejected for pinned clients;
9. explicit private RootCAs remain supported;
10. endpoint proof is sealed, non-serializable authority bound to the exact `*Client`;
11. proof from another client is invalid even if the pin text is the same;
12. with overlap pins `{A,B}`, proof A becomes stale after endpoint changes to B while ordinary pinned requests remain allowed;
13. fresh proof B succeeds after rotation;
14. exact fresh Wave25 authority + exact fresh endpoint proof mints Wave26 bridge authority;
15. stale Realm, stale Wave24 epoch, stale Wave25 incarnation, wrong ResourceBinding, cross-client endpoint proof, or rotated endpoint proof fails closed;
16. a static Wave26 contract forbids hostname/cert-serial/header/API-key/request-ID/timestamp/TOFU shortcuts and requires handshake-time verification;
17. exact-final-head CI is green across Wave26, NolaneWorld checks, Build, Unit, Format, Docs, DCO, and all relevant prior trust contracts.

Autonomously-by: ChatGPT:GPT-5.6-Sol
