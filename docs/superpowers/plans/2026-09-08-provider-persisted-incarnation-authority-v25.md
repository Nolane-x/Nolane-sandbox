# Wave 25 Provider-Persisted Sandbox Incarnation Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a provider-persisted sandbox incarnation identity that survives Cubelet/CubeAPI process restarts, rejects remote literal-sandbox-ID reuse, and composes with Wave24 into a stronger sealed Realm/Cube authority.

**Architecture:** CubeAPI mints one canonical UUIDv4 at the trusted remote create boundary, persists it in a reserved CubeMaster annotation, and only projects that stored value on later GET/connect/resume operations. NolaneWorld observes the value through its existing hardened Cube `Client`, mints a sealed proof bound to that exact client authority context, freshly re-observes it before use, and composes it above the already-fresh Wave24 `RealmResourceEpochAuthority`.

**Tech Stack:** Rust 2021 (`CubeAPI`, `uuid` crate, serde, axum), Go (`NolaneWorld/substrate/cube`), Python static contracts, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-08-provider-persisted-incarnation-authority-v25-design.md`

## Global Constraints

- Base exactly on Wave24 closure `e29a956310b0e420a7b57662567307376322ed8d`.
- Reserved provider annotation key is exactly `nolane.provider.incarnation.v1`.
- Provider incarnation is a canonical lowercase hyphenated UUIDv4 minted independently of request ID, sandbox ID, host ID, timestamps, metadata, Wave24 token, Realm ID and World ID.
- Public `NewSandbox` must not gain a caller-controlled annotation/incarnation authority input.
- Missing or malformed persisted incarnation is unavailable; never reconstruct/backfill it.
- Wave25 does not replace Wave24; strongest bridge requires fresh Wave24 local authority plus fresh provider incarnation authority.
- No cryptographic endpoint-attestation claim and no semantic promotion into OOM/task-outcome/resource/LIVE_PASS truth.
- Every contribution commit must include `Autonomously-by: ChatGPT:GPT-5.6-Sol`.
- PR remains draft/unmerged unless explicitly authorized.

---

## File Structure

### CubeAPI producer

- Modify `CubeAPI/src/models/mod.rs`
  - additive optional read-only `incarnationID` on `Sandbox` and `SandboxDetail` response DTOs only.
- Create `CubeAPI/src/services/provider_incarnation.rs`
  - reserved annotation key;
  - UUIDv4 minting;
  - strict canonical UUIDv4 parsing from stored annotations.
- Modify `CubeAPI/src/services/mod.rs`
  - module declaration/export as required by existing module style.
- Modify `CubeAPI/src/services/sandboxes.rs`
  - mint once during create;
  - insert reserved internal annotation before CubeMaster create;
  - project persisted identity in get/connect/resume;
  - never remint outside create.
- Add focused Rust tests beside the producer helpers/service tests.

### NolaneWorld observer/proof

- Create `NolaneWorld/substrate/cube/provider_incarnation.go`
  - private GET response shape;
  - strict canonical UUIDv4 validator;
  - sealed `ProviderIncarnationProof` bound to exact `*Client`;
  - `ObserveProviderIncarnation` and `ValidateProviderIncarnation`.
- Create `NolaneWorld/substrate/cube/provider_incarnation_v25_test.go`
  - strict transport/proof/context/freshness tests.

### Wave25 bridge

- Create `NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority.go`
  - sealed authority containing fresh Wave24 authority + provider proof.
- Create `NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority_v25_test.go`
  - exact bridge, remote recreation, stale/local mismatch and reconstruction tests.

### Contracts / CI

- Create `tests/wave25_provider_incarnation_contract.py`
  - static trust-shape and forbidden-shortcut checks.
- Create `.github/workflows/wave25-provider-incarnation-contract.yml`
  - focused CubeAPI, NolaneWorld and static-contract gates.

---

### Task 1: CubeAPI Provider-Incarnation RED

**Files:**
- Create: `CubeAPI/src/services/provider_incarnation.rs`
- Modify: `CubeAPI/src/services/mod.rs`
- Test: `CubeAPI/src/services/provider_incarnation.rs`
- Test/Modify: `CubeAPI/src/services/sandboxes.rs`

**Interfaces:**
- Produces later:

```rust
pub(crate) const PROVIDER_INCARNATION_ANNOTATION: &str =
    "nolane.provider.incarnation.v1";

pub(crate) fn new_provider_incarnation_id() -> String;

pub(crate) fn provider_incarnation_from_annotations(
    annotations: &std::collections::HashMap<String, String>,
) -> Option<String>;
```

- [ ] **Step 1: Write helper RED tests before implementation**

Add tests requiring:

```rust
#[test]
fn provider_incarnation_mint_is_canonical_uuid_v4_and_non_repeating() {
    let first = new_provider_incarnation_id();
    let second = new_provider_incarnation_id();
    assert_ne!(first, second);
    let parsed = uuid::Uuid::parse_str(&first).unwrap();
    assert_eq!(parsed.get_version_num(), 4);
    assert_eq!(parsed.hyphenated().to_string(), first);
}

#[test]
fn provider_incarnation_reader_rejects_missing_malformed_uppercase_and_non_v4() {
    let mut annotations = std::collections::HashMap::new();
    assert_eq!(provider_incarnation_from_annotations(&annotations), None);

    annotations.insert(PROVIDER_INCARNATION_ANNOTATION.into(), "not-a-uuid".into());
    assert_eq!(provider_incarnation_from_annotations(&annotations), None);

    annotations.insert(
        PROVIDER_INCARNATION_ANNOTATION.into(),
        "550E8400-E29B-41D4-A716-446655440000".into(),
    );
    assert_eq!(provider_incarnation_from_annotations(&annotations), None);

    annotations.insert(
        PROVIDER_INCARNATION_ANNOTATION.into(),
        "550e8400-e29b-11d4-a716-446655440000".into(),
    );
    assert_eq!(provider_incarnation_from_annotations(&annotations), None);
}
```

- [ ] **Step 2: Add service-level RED assertions at create/read seams**

Extend existing CubeAPI sandbox service test infrastructure so a create captures the outbound `CreateSandboxRequest` and proves:

```rust
let raw = request
    .annotations
    .get(PROVIDER_INCARNATION_ANNOTATION)
    .expect("provider incarnation annotation missing");
let parsed = Uuid::parse_str(raw).unwrap();
assert_eq!(parsed.get_version_num(), 4);
assert_eq!(parsed.hyphenated().to_string(), *raw);
```

Also add a negative test proving client metadata named `nolane.provider.incarnation.v1` remains in labels and cannot replace the internal annotation value.

- [ ] **Step 3: Run focused tests and record genuine RED**

Run from `CubeAPI`:

```bash
cargo test provider_incarnation -- --nocapture
```

Expected RED: missing helper/module/service wiring, not environment/setup failure.

- [ ] **Step 4: Commit RED evidence**

```bash
git add CubeAPI/src/services/provider_incarnation.rs CubeAPI/src/services/mod.rs CubeAPI/src/services/sandboxes.rs
git commit -m $'test(wave25): require provider-persisted incarnation minting\n\nAutonomously-by: ChatGPT:GPT-5.6-Sol'
```

---

### Task 2: CubeAPI Producer GREEN and Additive API Projection

**Files:**
- Modify: `CubeAPI/src/services/provider_incarnation.rs`
- Modify: `CubeAPI/src/models/mod.rs`
- Modify: `CubeAPI/src/services/sandboxes.rs`
- Test: focused CubeAPI tests in those files

**Interfaces:**
- Consumes Task 1 helper signatures.
- Produces response fields:

```rust
#[serde(
    rename = "incarnationID",
    skip_serializing_if = "Option::is_none"
)]
pub incarnation_id: Option<String>,
```

on `Sandbox` and `SandboxDetail` only.

- [ ] **Step 1: Implement strict helper**

Use the existing `uuid` dependency only:

```rust
pub(crate) fn new_provider_incarnation_id() -> String {
    Uuid::new_v4().hyphenated().to_string()
}

pub(crate) fn provider_incarnation_from_annotations(
    annotations: &HashMap<String, String>,
) -> Option<String> {
    let raw = annotations.get(PROVIDER_INCARNATION_ANNOTATION)?;
    let parsed = Uuid::parse_str(raw).ok()?;
    if parsed.get_version_num() != 4 || parsed.hyphenated().to_string() != *raw {
        return None;
    }
    Some(raw.clone())
}
```

- [ ] **Step 2: Mint exactly once at create**

Immediately before constructing `CreateSandboxRequest`:

```rust
let provider_incarnation_id = new_provider_incarnation_id();
annotations.insert(
    PROVIDER_INCARNATION_ANNOTATION.to_string(),
    provider_incarnation_id.clone(),
);
```

Do not derive this from `new_request_id()` and do not call `new_provider_incarnation_id()` in get/connect/resume.

- [ ] **Step 3: Add read-only response fields**

Add optional `incarnation_id` fields to `Sandbox` and `SandboxDetail` with exact wire name `incarnationID`. Update every constructor so:

- create response uses the exact value just inserted and accepted by the successful CubeMaster create;
- get/connect/resume use `provider_incarnation_from_annotations(&d.annotations)`;
- legacy/malformed annotation yields `None`.

Update `sandbox_response` signature to take:

```rust
incarnation_id: Option<String>
```

and never manufacture a fallback inside that function.

- [ ] **Step 4: Add persistence/non-remint tests**

Use a fixed sandbox detail fixture with annotation:

```rust
"nolane.provider.incarnation.v1" -> "550e8400-e29b-41d4-a716-446655440000"
```

and prove get/connect/resume preserve that exact value. Add a missing/malformed fixture and prove the response omits `incarnationID` instead of replacing it.

- [ ] **Step 5: Run CubeAPI verification**

```bash
cargo fmt -- --check
cargo test provider_incarnation -- --nocapture
cargo test sandboxes -- --nocapture
cargo test
```

Expected: PASS.

- [ ] **Step 6: Commit GREEN**

```bash
git add CubeAPI/src/models/mod.rs CubeAPI/src/services/provider_incarnation.rs CubeAPI/src/services/sandboxes.rs
git commit -m $'feat(wave25): persist provider sandbox incarnation authority\n\nAutonomously-by: ChatGPT:GPT-5.6-Sol'
```

---

### Task 3: NolaneWorld Provider Observer RED

**Files:**
- Create: `NolaneWorld/substrate/cube/provider_incarnation_v25_test.go`

**Interfaces:**
- Requires future production API:

```go
type ProviderIncarnationProof struct { /* sealed/private */ }

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

- [ ] **Step 1: Write exact observation RED**

Use `httptest.Server` returning:

```json
{
  "sandboxID":"sandbox-v25",
  "incarnationID":"550e8400-e29b-41d4-a716-446655440000"
}
```

Require returned proof to be valid and expose diagnostics only through safe accessors:

```go
sandboxID, ok := proof.SandboxID()
incarnationID, ok2 := proof.IncarnationID()
if !ok || !ok2 || sandboxID != "sandbox-v25" ||
    incarnationID != "550e8400-e29b-41d4-a716-446655440000" {
    t.Fatal("exact provider incarnation was not sealed")
}
```

- [ ] **Step 2: Write strict negative matrix**

Require rejection of:

```text
missing incarnationID
empty sandboxID
wrong sandboxID
uppercase UUID
UUIDv1
malformed UUID
non-canonical UUID text
HTTP non-2xx
response larger than Client.maxBytes
```

- [ ] **Step 3: Write unforgeability/context RED**

Construct package-local descriptive fields without the seal and require invalid. JSON round-trip must not restore a proof. A proof minted by `clientA` must be invalid when passed to `clientB.ValidateProviderIncarnation`, even when both mock endpoints return byte-identical JSON.

- [ ] **Step 4: Write remote recreation freshness RED**

Use a mutable test server:

1. observe token A for `sandbox-v25`;
2. mutate server to token B with the same sandbox ID;
3. `ValidateProviderIncarnation(..., oldProof)` must return `ErrStaleProviderIncarnationProof`.

- [ ] **Step 5: Run RED**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V25.*ProviderIncarnation' -count=1
```

Expected RED: missing Wave25 proof/observer APIs.

- [ ] **Step 6: Commit RED**

```bash
git add NolaneWorld/substrate/cube/provider_incarnation_v25_test.go
git commit -m $'test(wave25): require sealed provider incarnation observation\n\nAutonomously-by: ChatGPT:GPT-5.6-Sol'
```

---

### Task 4: NolaneWorld Provider Observer GREEN

**Files:**
- Create: `NolaneWorld/substrate/cube/provider_incarnation.go`
- Test: `NolaneWorld/substrate/cube/provider_incarnation_v25_test.go`

**Interfaces:**
- Produces:

```go
var (
    ErrProviderIncarnationUnavailable = errors.New("cube provider incarnation observation unavailable")
    ErrInvalidProviderIncarnationProof = errors.New("cube: invalid provider incarnation proof")
    ErrStaleProviderIncarnationProof = errors.New("cube: stale provider incarnation proof")
)
```

and the Task 3 method signatures.

- [ ] **Step 1: Implement sealed proof bound to exact client**

```go
type providerIncarnationProofSeal struct{}
var currentProviderIncarnationProofSeal = &providerIncarnationProofSeal{}

type ProviderIncarnationProof struct {
    sandboxID     string
    incarnationID string
    client        *Client
    seal          *providerIncarnationProofSeal
}
```

`Valid()` requires current seal, non-nil client, canonical exact sandbox ID and canonical UUIDv4 incarnation.

- [ ] **Step 2: Implement strict canonical UUIDv4 validation without adding dependencies**

Use `encoding/hex` and fixed UUID positions. Require exactly 36 lowercase characters, hyphens at `8,13,18,23`, hex elsewhere, version nibble `raw[14] == '4'`, and RFC4122 variant nibble at index 19 in `8,9,a,b`.

- [ ] **Step 3: Implement fresh GET observation using existing Client trust root**

Request:

```go
GET c.apiURL + "/sandboxes/" + url.PathEscape(resource.sandboxID)
```

with the same `Accept`, API-key, response-size and hardened-client semantics as `doJSON`. Prefer factoring a small private `doJSON` reuse rather than creating a second HTTP trust policy.

Decode only the required additive fields:

```go
var out struct {
    SandboxID     string `json:"sandboxID"`
    IncarnationID string `json:"incarnationID"`
}
```

Then mint the sealed proof only after exact binding/canonical validation.

- [ ] **Step 4: Implement freshness validation**

```go
func (c *Client) ValidateProviderIncarnation(
    ctx context.Context,
    resource ResourceBinding,
    proof ProviderIncarnationProof,
) error {
    if !proof.Valid() || proof.client != c {
        return ErrInvalidProviderIncarnationProof
    }
    current, err := c.ObserveProviderIncarnation(ctx, resource)
    if err != nil {
        return err
    }
    if !sameProviderIncarnationProof(current, proof) {
        return ErrStaleProviderIncarnationProof
    }
    return nil
}
```

- [ ] **Step 5: Run focused and package tests**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V25.*ProviderIncarnation' -count=1
go test ./substrate/cube -count=1
go vet ./substrate/cube
```

Expected: PASS.

- [ ] **Step 6: Commit GREEN**

```bash
git add NolaneWorld/substrate/cube/provider_incarnation.go NolaneWorld/substrate/cube/provider_incarnation_v25_test.go
git commit -m $'feat(wave25): seal provider-persisted incarnation proof\n\nAutonomously-by: ChatGPT:GPT-5.6-Sol'
```

---

### Task 5: Wave25 Realm/Cube Bridge RED

**Files:**
- Create: `NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority_v25_test.go`

**Interfaces:**
- Requires future API:

```go
type RealmResourceProviderIncarnationAuthority struct { /* sealed/private */ }

func ValidateRealmResourceProviderIncarnationAuthority(
    ctx context.Context,
    local RealmResourceEpochAuthority,
    resource ResourceBinding,
    client *Client,
    provider ProviderIncarnationProof,
) (RealmResourceProviderIncarnationAuthority, error)
```

- [ ] **Step 1: Write exact success RED**

Reuse Wave24 test helpers to mint a valid fresh `RealmResourceEpochAuthority`, observe an exact provider proof for the same sandbox, then require a valid Wave25 authority.

- [ ] **Step 2: Write failure matrix**

Require failure for:

```text
zero/forged/deserialized Wave24 authority
wrong ResourceBinding sandbox
zero/forged/deserialized provider proof
provider proof from different Client
stale provider token after remote same-ID recreation
stale Wave24 epoch after local Clear -> Start
sandbox mismatch between Wave24 and provider proofs
```

- [ ] **Step 3: Run RED**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V25.*RealmResourceProvider' -count=1
```

Expected RED: missing Wave25 bridge API.

- [ ] **Step 4: Commit RED**

```bash
git add NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority_v25_test.go
git commit -m $'test(wave25): require provider-bound Realm Cube authority\n\nAutonomously-by: ChatGPT:GPT-5.6-Sol'
```

---

### Task 6: Wave25 Realm/Cube Bridge GREEN

**Files:**
- Create: `NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority.go`
- Test: `NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority_v25_test.go`

**Interfaces:**
- Consumes exact Task 5 signature.
- Authority stores:

```go
type RealmResourceProviderIncarnationAuthority struct {
    local    RealmResourceEpochAuthority
    provider ProviderIncarnationProof
    seal     *realmResourceProviderIncarnationAuthoritySeal
}
```

- [ ] **Step 1: Implement validation order**

The mint function must:

1. reject invalid local Wave24 authority;
2. require exact `ResourceBinding` sandbox equality with the Wave24 authority;
3. reject invalid provider proof / nil client;
4. require provider proof sandbox equality;
5. call `client.ValidateProviderIncarnation(ctx, resource, provider)` for a fresh provider read;
6. mint the Wave25 seal only after all checks pass.

Do not convert descriptive fields back into either Wave24 or provider authority.

- [ ] **Step 2: Add safe diagnostic accessors**

Expose only read-only methods such as:

```go
func (a RealmResourceProviderIncarnationAuthority) SandboxID() (string, bool)
func (a RealmResourceProviderIncarnationAuthority) Local() (RealmResourceEpochAuthority, bool)
func (a RealmResourceProviderIncarnationAuthority) Provider() (ProviderIncarnationProof, bool)
```

- [ ] **Step 3: Run focused/full NolaneWorld tests**

```bash
cd NolaneWorld
go test ./substrate/cube -run 'V25' -count=1
go test ./... -count=1
go vet ./...
```

Expected: PASS.

- [ ] **Step 4: Commit GREEN**

```bash
git add NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority.go NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority_v25_test.go
git commit -m $'feat(wave25): bind Realm Cube authority to provider incarnation\n\nAutonomously-by: ChatGPT:GPT-5.6-Sol'
```

---

### Task 7: Static Trust Contract and Dedicated CI

**Files:**
- Create: `tests/wave25_provider_incarnation_contract.py`
- Create: `.github/workflows/wave25-provider-incarnation-contract.yml`

**Interfaces:**
- Static contract reads the exact Wave25 producer, consumer and bridge files.

- [ ] **Step 1: Add static required-shape checks**

Require literals/structure proving:

```text
nolane.provider.incarnation.v1
Uuid::new_v4()
CreateSandboxRequest annotations insertion
incarnationID response projection
ProviderIncarnationProof seal
proof.client == current Client context
fresh ObserveProviderIncarnation in validation
RealmResourceEpochAuthority retained by Wave25 bridge
ValidateProviderIncarnation called at bridge time
```

- [ ] **Step 2: Add forbidden-shortcut checks**

Fail if Wave25 production identity code derives authority from any of:

```text
startedAt / create_at / chrono::Utc::now
sandbox ID hashing
clientID / host_id
CubeMaster request_id
Wave24 realization token
Realm ID / World ID
caller metadata as incarnation authority
public NewProviderIncarnationProof constructor
serialized authority fields
```

The static test must distinguish harmless operational uses elsewhere from actual provider-incarnation producer code; scope checks to Wave25 files/seams rather than banning repository-wide strings.

- [ ] **Step 3: Add dedicated workflow**

The workflow must run on Wave25 paths and include:

```yaml
- CubeAPI focused provider-incarnation tests
- NolaneWorld focused V25 tests
- python3 tests/wave25_provider_incarnation_contract.py
```

Use the repository's existing Rust/Go setup conventions and exact version files/toolchains rather than inventing new version pins.

- [ ] **Step 4: Run static contract locally or via CI**

```bash
python3 tests/wave25_provider_incarnation_contract.py
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tests/wave25_provider_incarnation_contract.py .github/workflows/wave25-provider-incarnation-contract.yml
git commit -m $'ci(wave25): gate provider incarnation trust boundary\n\nAutonomously-by: ChatGPT:GPT-5.6-Sol'
```

---

### Task 8: Exact-Head Closure Verification

**Files:**
- Optionally create closure evidence only if repository convention requires it; do not create a metadata-only source commit after exact-head verification unless necessary.
- Update PR body after verification without changing branch HEAD.

- [ ] **Step 1: Run/fetch all Wave25-relevant checks on one exact HEAD**

Required green set:

```text
Wave25 Provider Incarnation Contract
Nolane World Check
Nolane Live Substrate Gauntlet
Build Check
Unit Test Check
Format Check
Docs Build Check
DCO Check
Wave24 Cubelet Realization Epoch Contract
Cube Guest Kernel OOM Victim Contract
Cube Task Outcome Contract
Cube Host Resource Contract
Cube Host Process Identity Contract
Cube Kernel OOM Victim Contract
Cube Realization OOM Contract
```

- [ ] **Step 2: Treat every failure by root cause**

If any production change is required, create a new HEAD and restart exact-head verification. Never cite CI from an older SHA as final closure evidence.

- [ ] **Step 3: Compact provenance only if needed**

If experimental TDD commits violate DCO, preserve a backup branch, squash/rewrite onto exact Wave24 base with unchanged final tree, and rerun every final-head gate.

- [ ] **Step 4: Update PR closure metadata**

Record:

```text
exact Wave24 base SHA
exact Wave25 closure SHA
trust boundary closed
explicit non-claims
all exact-head workflow conclusions
backup branch if history was compacted
PR remains draft/unmerged
```

- [ ] **Step 5: Re-fetch PR state**

Verify:

```text
state=open
draft=true
merged=false
head_sha=<verified Wave25 SHA>
```

Only then call Wave25 code-closed.

## Self-Review

- Spec coverage: producer mint/persist/read, caller non-nomination, legacy fail-closed, strict NolaneWorld observer, exact-client context, fresh provider revalidation, Wave24 composition, remote same-ID recreation, static anti-shortcut contract, and exact-head closure are each mapped to explicit tasks.
- Placeholder scan: no TBD/TODO/"similar to" implementation gaps remain.
- Type consistency: producer response field is `incarnationID`; Go proof uses `incarnationID`; observer and bridge signatures use `ProviderIncarnationProof`; Wave25 bridge composes `RealmResourceEpochAuthority` rather than recreating Wave24 descriptive state.
- YAGNI: no new database, signing PKI, generic provider abstraction, or persistence service is introduced; the existing CubeMaster annotation channel is reused.

Autonomously-by: ChatGPT:GPT-5.6-Sol
