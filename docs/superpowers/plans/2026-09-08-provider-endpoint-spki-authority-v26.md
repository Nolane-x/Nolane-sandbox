# Wave 26 Provider Endpoint SPKI Identity Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fail-closed TLS SPKI endpoint identity authority to the NolaneWorld Cube control-plane client and compose it above Wave25 provider-incarnation authority.

**Architecture:** Extend the existing Cube client with canonical configured SHA-256 SPKI pins and harden its `http.Transport` so pin membership is checked during the TLS handshake in addition to normal certificate-chain/hostname verification. Mint a sealed endpoint proof only from a successful pinned HTTPS `/health` observation, re-observe exact key freshness before use, and compose that proof above freshly revalidated Wave25 authority.

**Tech Stack:** Go standard library (`crypto/sha256`, `crypto/tls`, `crypto/x509` test helpers, `net/http`, `httptest`), existing NolaneWorld Cube/Realm authority types, Python static contract, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-08-provider-endpoint-spki-authority-v26-design.md`

## Global Constraints

- Stack exactly on Wave25 closure SHA `cbf47fa8b0c652052aa12b7a092ff411c1fc24cb`.
- SPKI pin format is exactly 64 lowercase hex characters representing SHA-256 of `RawSubjectPublicKeyInfo`.
- Pinning must happen during TLS handshake before request credentials/body are sent.
- Normal certificate-chain and hostname verification remain mandatory.
- `InsecureSkipVerify=true`, non-`*http.Transport` pinned transports, and `DialTLS`/`DialTLSContext` bypass hooks fail closed.
- Empty pin configuration keeps prior client behavior but cannot mint Wave26 endpoint authority.
- Wave26 proof is sealed, non-serializable, exact-`*Client` bound, and exact-key fresh.
- Rotation is explicit overlap: `{A,B}` accepts transport for either key, but proof A becomes stale when current endpoint presents B.
- No server self-reported identity header or TOFU behavior.
- Wave25/Wave24/Realm freshness remains mandatory.
- Every commit message includes `Autonomously-by: ChatGPT:GPT-5.6-Sol`.
- PR remains draft/unmerged unless the user explicitly authorizes merge.

---

### Task 1: Canonical pin configuration and handshake-time transport hardening

**Files:**
- Create: `NolaneWorld/substrate/cube/provider_endpoint_spki_transport.go`
- Create: `NolaneWorld/substrate/cube/provider_endpoint_spki_transport_v26_test.go`
- Modify: `NolaneWorld/substrate/cube/client.go`

**Interfaces:**
- Consumes: `Config.HTTPClient`, `Config.APIURL`, existing `hardenedHTTPClient` behavior.
- Produces:
  - `Config.EndpointSPKIPins []string`
  - `Client.endpointSPKIPins map[[32]byte]struct{}`
  - `parseEndpointSPKIPins([]string) (map[[32]byte]struct{}, error)`
  - `hardenedPinnedHTTPClient(*http.Client, time.Duration, map[[32]byte]struct{}) (*http.Client, error)`
  - typed errors `ErrInvalidEndpointSPKIConfig`, `ErrEndpointSPKIMismatch`, `ErrEndpointTLSAuthorityUnavailable`.

- [ ] **Step 1: Write failing configuration tests**

Add table tests that call `New` with short, uppercase, non-hex, duplicate, all-zero pins and require `errors.Is(err, ErrInvalidEndpointSPKIConfig)`.

Also verify a valid 64-lowercase-hex pin is accepted when paired with an HTTPS URL and a safe `*http.Transport`.

- [ ] **Step 2: Write failing transport-policy tests**

Add tests requiring pinned `New` to reject:

```go
&http.Client{Transport: roundTripperFunc(...)}
&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
&http.Client{Transport: &http.Transport{DialTLSContext: func(...) (...) { ... }}}
```

and reject pin authority configuration on `http://127.0.0.1` while preserving ordinary unpinned loopback HTTP construction.

- [ ] **Step 3: Run focused tests and capture RED**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V26|EndpointSPKI' -count=1
```

Expected: compile/test failure because Wave26 configuration, errors and transport hardening do not yet exist.

- [ ] **Step 4: Implement canonical pin parsing**

Use `hex.DecodeString`, require exact length 64, lowercase canonical text, decoded length 32, non-zero digest, and no duplicate digest.

- [ ] **Step 5: Implement pinned transport cloning**

For non-empty pins:

```go
base := http.DefaultTransport.(*http.Transport).Clone()
// or clone the caller's *http.Transport

tlsConfig := base.TLSClientConfig.Clone() // when non-nil
// reject InsecureSkipVerify and TLS dial bypass hooks
originalVerify := tlsConfig.VerifyConnection
tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
    if originalVerify != nil {
        if err := originalVerify(state); err != nil { return err }
    }
    if len(state.PeerCertificates) == 0 { return ErrEndpointTLSAuthorityUnavailable }
    digest := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
    if _, ok := pins[digest]; !ok { return ErrEndpointSPKIMismatch }
    return nil
}
```

Do not set `InsecureSkipVerify`; standard verification remains active.

- [ ] **Step 6: Integrate into `New`**

Parse pins before client construction. For non-empty pins require API scheme `https`; construct the control-plane `http` client with `hardenedPinnedHTTPClient`. Leave `dataHTTP` semantics unchanged unless it shares the control-plane client; Wave26 authority applies only to CubeAPI control-plane calls.

- [ ] **Step 7: Run focused tests and verify GREEN**

Run the same focused Go command and require pass.

- [ ] **Step 8: Commit**

```text
feat(wave26): enforce Cube endpoint SPKI pins during TLS handshake

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 2: Sealed endpoint observation and exact-key freshness

**Files:**
- Create: `NolaneWorld/substrate/cube/provider_endpoint_spki.go`
- Create: `NolaneWorld/substrate/cube/provider_endpoint_spki_v26_test.go`
- Modify: `NolaneWorld/substrate/cube/client.go` only if a low-level response helper is required.

**Interfaces:**
- Consumes: `Client.endpointSPKIPins`, hardened control-plane `http.Client`, existing `/health` route.
- Produces:
  - `ProviderEndpointSPKIProof`
  - `(*Client).ObserveProviderEndpointSPKI(context.Context) (ProviderEndpointSPKIProof, error)`
  - `(*Client).ValidateProviderEndpointSPKI(context.Context, ProviderEndpointSPKIProof) error`
  - `sameProviderEndpointSPKIProof`
  - errors `ErrInvalidProviderEndpointSPKIProof`, `ErrStaleProviderEndpointSPKIProof`.

- [ ] **Step 1: Build TLS test helpers**

Use `httptest.NewTLSServer`, its trusted client/root pool, and compute the test server leaf digest with:

```go
digest := sha256.Sum256(ts.Certificate().RawSubjectPublicKeyInfo)
hexPin := hex.EncodeToString(digest[:])
```

Create a mutable delegating server/client arrangement for rotation tests so endpoint A and B can be selected while the configured pin-set contains both.

- [ ] **Step 2: Write failing observation tests**

Require:

- exact pin + valid TLS -> sealed proof;
- no pins -> `ErrEndpointTLSAuthorityUnavailable`;
- unpinned peer key -> handshake returns `ErrEndpointSPKIMismatch` and handler records that request credentials/body were not delivered;
- proof exposes canonical lowercase digest only through a descriptive accessor;
- JSON marshal/unmarshal or zero-value proof cannot forge `Valid()`.

- [ ] **Step 3: Write failing client-binding and rotation tests**

Require proof from client A to fail validation under client B even with identical configured pin text.

With overlap pins `{A,B}`, validate proof A while A is active, switch endpoint to B, require old proof A to return `ErrStaleProviderEndpointSPKIProof`, then freshly observe B and validate B successfully.

- [ ] **Step 4: Run focused tests and capture RED**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V26|EndpointSPKI' -count=1
```

- [ ] **Step 5: Implement observation from actual TLS response state**

Send `GET /health` through the exact hardened client. Require a 2xx response, non-nil `resp.TLS`, at least one peer certificate, compute the leaf SPKI digest, and require it is still in the immutable configured set before minting:

```go
return ProviderEndpointSPKIProof{
    spkiSHA256: digest,
    client: c,
    seal: currentProviderEndpointSPKIProofSeal,
}, nil
```

Response content is ignored as identity evidence.

- [ ] **Step 6: Implement fresh validation**

Reject invalid or cross-client proofs. Re-run `ObserveProviderEndpointSPKI`; exact digest mismatch is `ErrStaleProviderEndpointSPKIProof` even when the new digest is another configured rotation pin.

- [ ] **Step 7: Run focused tests and verify GREEN**

- [ ] **Step 8: Commit**

```text
feat(wave26): seal and refresh provider endpoint SPKI proof

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 3: Compose endpoint authority above Wave25

**Files:**
- Create: `NolaneWorld/substrate/cube/realm_resource_provider_endpoint_authority.go`
- Create: `NolaneWorld/substrate/cube/realm_resource_provider_endpoint_authority_v26_test.go`

**Interfaces:**
- Consumes:
  - `realm.Controller`
  - `realm.RealizationAuthority`
  - `RealmResourceProviderIncarnationAuthority`
  - `ResourceBinding`
  - `RealizationEpochObserver`
  - exact `*Client`
  - `ProviderEndpointSPKIProof`
- Produces:
  - sealed `RealmResourceProviderEndpointAuthority`
  - `ValidateRealmResourceProviderEndpointAuthority(...)`
  - `ErrInvalidRealmResourceProviderEndpointAuthority`.

- [ ] **Step 1: Write failing exact-success bridge test**

Construct fresh Wave23/24/25 authority using existing test patterns, then require exact fresh endpoint proof from the same client to mint a valid Wave26 bridge.

- [ ] **Step 2: Write failing adversarial matrix**

Require fail-closed behavior for:

- zero/forged/deserialized endpoint proof;
- cross-client endpoint proof;
- wrong `ResourceBinding`;
- stale Realm authority;
- stale Wave24 epoch;
- stale Wave25 remote incarnation after same-ID recreation;
- rotated endpoint key where old proof is stale.

- [ ] **Step 3: Run focused bridge tests and capture RED**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'RealmResourceProviderEndpoint.*V26|V26.*RealmResourceProviderEndpoint' -count=1
```

- [ ] **Step 4: Implement bridge freshness reconstruction**

Do not trust the Wave25 authority by shape. Extract its embedded local/provider proofs and call existing `ValidateRealmResourceProviderIncarnationAuthority(...)` with the current controller, realization, resource, epoch observer and client. Require equivalence with the supplied Wave25 authority. Then require `client.ValidateProviderEndpointSPKI(ctx, endpointProof)` and exact client binding before minting the sealed Wave26 capability.

- [ ] **Step 5: Run bridge and full Wave26 focused tests GREEN**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V26|EndpointSPKI' -count=1
go vet ./substrate/cube
```

- [ ] **Step 6: Commit**

```text
feat(wave26): bridge fresh provider endpoint identity authority

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 4: Dedicated static trust-shape contract and CI

**Files:**
- Create: `tests/wave26_provider_endpoint_spki_contract.py`
- Create: `.github/workflows/wave26-provider-endpoint-spki-contract.yml`

**Interfaces:**
- Consumes: Wave26 Go implementation and test files.
- Produces: deterministic anti-shortcut contract and dedicated Wave26 CI gate.

- [ ] **Step 1: Write static contract**

Assert source contains:

- `sha256.Sum256(...RawSubjectPublicKeyInfo)`;
- `VerifyConnection` handshake hook;
- preserved original verify callback;
- explicit rejection of `InsecureSkipVerify`;
- explicit rejection of custom TLS dial hooks/non-transport RoundTrippers;
- exact `/health` observation path;
- sealed proof with no JSON tags/public constructor;
- fresh `ObserveProviderEndpointSPKI` inside validation;
- Wave26 bridge invokes Wave25 validator and endpoint validator.

Also forbid authority shortcuts in Wave26 files such as `Header.Get`, `Server`, `X-Provider-ID`, `time.Now`, `requestID`, `clientID`, `sandboxID` hashing, `InsecureSkipVerify = true`, and TOFU persistence.

- [ ] **Step 2: Run static contract locally/CI-compatible**

```bash
python3 tests/wave26_provider_endpoint_spki_contract.py
```

- [ ] **Step 3: Add dedicated GitHub Actions workflow**

Trigger on Wave26 paths. Use the repository's NolaneWorld Go version. Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V26|EndpointSPKI' -count=1
go vet ./substrate/cube
cd ..
python3 tests/wave26_provider_endpoint_spki_contract.py
```

Include amd64/arm64 jobs where the existing contract workflows already establish that pattern.

- [ ] **Step 4: Commit**

```text
ci(wave26): lock provider endpoint SPKI trust contract

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 5: Exact-final-head verification and draft PR closure

**Files:**
- Modify only if failures require code/test/workflow corrections.
- PR metadata: new Wave26 draft PR stacked on `gpt/wave25-provider-incarnation-authority`.

**Interfaces:**
- Consumes: final Wave26 branch HEAD and GitHub Actions.
- Produces: one exact-final-head SHA with all required CI green and a closure PR body documenting claims/non-claims.

- [ ] **Step 1: Create draft PR**

Title:

```text
Wave 26: provider endpoint SPKI identity authority
```

Base: `gpt/wave25-provider-incarnation-authority`.

- [ ] **Step 2: Verify exact diff and commit provenance**

Require branch `ahead_by >= 1`, `behind_by = 0`, no unrelated files, and DCO-compatible provenance on every Wave26 commit.

- [ ] **Step 3: Wait only through current tool execution for CI and inspect failures**

Check Wave26 dedicated workflow plus Nolane World Check, Build, Unit, Format, Docs, DCO and all prior Wave24/Wave25 trust gates against the same exact final SHA. Fix only evidence-backed failures.

- [ ] **Step 4: Re-run exact-final-head verification after every fix**

Do not claim closure from a superseded SHA.

- [ ] **Step 5: Update PR body to code-closed only when all required exact-head gates are green**

Document endpoint pin semantics, rotation behavior, anti-bypass transport rules, exact closure SHA, and explicit non-claims.

- [ ] **Step 6: Keep PR draft/unmerged**

Merge requires explicit user authorization.

Autonomously-by: ChatGPT:GPT-5.6-Sol
