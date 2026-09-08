# Wave 27 Runtime Realization Provenance Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind the exact current host runtime process realization to the fully fresh Wave26 identity chain, derive an authoritative runtime provenance digest, and project the complete descriptive resourceproof binding without minting resource enforcement or LIVE_PASS.

**Architecture:** Reuse the existing Cubelet resource-metrics trust boundary and existing parsers for Wave24 realization epoch and Wave19 host process identity. Add a dedicated same-scrape `RuntimeRealizationObserver` that seals the exact epoch+process pair, then add a Wave27 bridge that reconstructs Wave26 freshness, proves the same epoch/process realization, and derives a deterministic runtime digest. Keep the bridge in `substrate/cube`; add only a narrow `resourceproof` binding projection adapter after authority exists.

**Tech Stack:** Go 1.23 standard library, existing NolaneWorld Realm/Cube authority types, existing Cubelet Prometheus text format, Python static contract, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-08-runtime-realization-provenance-authority-v27-design.md`

## Global Constraints

- Stack exactly on Wave26 closure SHA `2aff6aaa846fce77c91a36038ca61b15d96b1a67`.
- No production code before committed RED evidence.
- Reuse existing canonical parsers for realization epoch and host process identity; do not create weaker duplicate parsing logic.
- Runtime observation must bind epoch + host process identity from one bounded Cubelet metrics scrape.
- Proofs are sealed, non-serializable authority and exact-observer-bound.
- Wave27 bridge must reconstruct Wave26 freshness through existing validators rather than accepting a sealed object by shape alone.
- Runtime digest namespace is exactly `runtime-realization-v27:` followed by 64 lowercase hex characters.
- No PID-only, timestamp-only, sandbox-hash, SPKI-only, caller-supplied digest, TOFU, or descriptive reconstruction shortcuts.
- Wave27 does not mint `resourceproof.TrustedReport` and does not claim CPU, memory, disk, aggregate resource enforcement, OOM causality, task outcome, hardware attestation, or LIVE_PASS.
- Every commit message includes `Autonomously-by: ChatGPT:GPT-5.6-Sol`.
- PR remains draft/unmerged unless the user explicitly authorizes merge.

---

### Task 1: Same-scrape runtime realization observer and sealed proof

**Files:**
- Create: `NolaneWorld/substrate/cube/runtime_realization.go`
- Create: `NolaneWorld/substrate/cube/runtime_realization_v27_test.go`

**Interfaces:**
- Consumes: `ResourceBinding`, `RealizationEpochProof`, `exactRealizationEpochFromSample`, `HostSandboxProcessIdentityProof`, `exactHostSandboxProcessIdentityFromSample`, `hostResourceMetricsPath`.
- Produces:
  - `RuntimeRealizationConfig { BaseURL string; HTTPClient *http.Client }`
  - `RuntimeRealizationObserver`
  - `RuntimeRealizationProof`
  - `NewRuntimeRealizationObserver(RuntimeRealizationConfig) (*RuntimeRealizationObserver, error)`
  - `(*RuntimeRealizationObserver).Observe(context.Context, ResourceBinding) (RuntimeRealizationProof, error)`
  - `(*RuntimeRealizationObserver).ValidateCurrent(context.Context, ResourceBinding, RuntimeRealizationProof) error`
  - `ErrRuntimeRealizationUnavailable`, `ErrInvalidRuntimeRealizationProof`, `ErrStaleRuntimeRealizationProof`.

- [ ] **Step 1: Write failing exact-success and same-scrape tests**

Create a single `httptest.Server` response containing one canonical `cubesandbox_realization_epoch_info` line and one canonical `cubesandbox_host_process_identity_info` line for the same sandbox/generation. Require `Observe` to return a valid sealed proof whose descriptive epoch/process copies match the samples.

- [ ] **Step 2: Write failing malformed/ambiguity matrix**

Require failure for missing epoch, duplicate epoch, missing process identity, duplicate process identity, wrong sandbox, generation mismatch, malformed zero epoch token, and malformed process identity.

- [ ] **Step 3: Write failing opacity/context/freshness tests**

Require zero proof invalid, JSON round-trip unable to restore validity, proof from observer A invalid under observer B, proof stale after epoch token change with generation reused, stale after PID/starttime change, and stale after boot-ID change.

- [ ] **Step 4: Commit RED tests only**

Commit the test file before production code.

- [ ] **Step 5: Verify RED**

Use the dedicated Wave27 workflow/static contract from Task 4 or focused GitHub Actions run; expected failure is missing `RuntimeRealization*` API/implementation, not malformed test fixtures.

- [ ] **Step 6: Implement minimal observer/proof**

Use exactly one GET to `hostResourceMetricsPath`. Parse only the two Wave27 authority metric families for the requested sandbox, reject duplicates/ambiguity, mint a sealed internal `RealizationEpochProof` bound to the exact runtime observer authority context plus canonical process identity, and retain no caller-provided authority fields.

- [ ] **Step 7: Implement fresh exact validation**

`ValidateCurrent` rejects invalid/cross-observer proofs, performs a new same-scrape observation, and requires exact equality of epoch token/generation and every process identity field.

- [ ] **Step 8: Run focused tests GREEN and commit**

Commit production only after the same RED tests turn GREEN.

---

### Task 2: Wave27 bridge and runtime digest

**Files:**
- Create: `NolaneWorld/substrate/cube/realm_resource_runtime_authority.go`
- Create: `NolaneWorld/substrate/cube/realm_resource_runtime_authority_v27_test.go`

**Interfaces:**
- Consumes: `realm.Controller`, `realm.RealizationAuthority`, `RealmResourceProviderEndpointAuthority`, `ResourceBinding`, `RealizationEpochObserver`, `*Client`, `ProviderEndpointSPKIProof`, `RuntimeRealizationObserver`, `RuntimeRealizationProof`.
- Produces:
  - sealed `RealmResourceRuntimeAuthority`
  - `ValidateRealmResourceRuntimeAuthority(...)`
  - `RuntimeDigest() (string, bool)`
  - `RealizationBinding() (realm.RealizationBinding, bool)` descriptive accessor
  - `ErrInvalidRealmResourceRuntimeAuthority`.

- [ ] **Step 1: Write failing exact-success bridge test**

Construct the full fresh Wave23→26 authority using existing Wave26 test helpers/patterns, add an exact fresh runtime proof for the same Wave24 epoch/process generation, then require valid Wave27 authority and canonical runtime digest.

- [ ] **Step 2: Write failing adversarial bridge matrix**

Require fail closed for zero runtime proof, cross-observer proof, wrong ResourceBinding, stale Realm, stale Wave24 epoch, stale Wave25 provider incarnation, rotated Wave26 endpoint proof, runtime epoch mismatch against embedded Wave24 authority, and runtime process replacement.

- [ ] **Step 3: Write failing digest semantics tests**

Require deterministic digest for identical authority, exact namespace/hex shape, and digest change when any authoritative runtime dimension changes while a new fresh authority is legitimately minted.

- [ ] **Step 4: Commit RED bridge tests**

Commit before production bridge code.

- [ ] **Step 5: Verify RED**

Expected failure is missing Wave27 bridge API.

- [ ] **Step 6: Implement full Wave26 freshness reconstruction**

Extract Wave26 nested provider/endpoint inputs through existing accessors, re-run `ValidateRealmResourceProviderEndpointAuthority(...)` with current controller, realization, resource, epoch observer, client and endpoint proof, then require equivalence with the supplied Wave26 authority.

- [ ] **Step 7: Bind runtime proof to embedded Wave24 epoch**

Validate runtime proof through the exact runtime observer and require exact sandbox/generation/token equality with the Wave24 epoch proof embedded under Wave25/Wave26.

- [ ] **Step 8: Derive deterministic runtime digest**

Canonicalize authority-owned fields only and hash with domain separator `nolane.runtime-realization.v27\x00`; expose only `runtime-realization-v27:<hex>`.

- [ ] **Step 9: Run focused bridge tests GREEN and commit**

---

### Task 3: Complete resourceproof.Binding projection without TrustedReport minting

**Files:**
- Create: `NolaneWorld/gauntlet/live/resourceproof/runtime_binding_v27.go`
- Create: `NolaneWorld/gauntlet/live/resourceproof/runtime_binding_v27_test.go`

**Interfaces:**
- Consumes: valid `cube.RealmResourceRuntimeAuthority` descriptive accessors only after its seal validation.
- Produces: `BindingFromRuntimeAuthority(cube.RealmResourceRuntimeAuthority) (Binding, error)` and `ErrRuntimeBindingUnavailable`.

- [ ] **Step 1: Write failing projection tests**

Require exact Realm ID/revision/policy/realization fields and Wave27 runtime digest; zero authority must fail.

- [ ] **Step 2: Write failing no-promotion test**

Require the projected `Binding` to remain descriptive: passing it to public `BuildReport` with copied `SourceLiveHost` observations must remain `UNAVAILABLE`, never `LIVE_PASS`.

- [ ] **Step 3: Commit RED tests**

- [ ] **Step 4: Verify RED**

Expected failure is missing binding projection API.

- [ ] **Step 5: Implement minimal projection helper**

Map fields only from valid Wave27 authority accessors. Do not add any public constructor for Wave27 authority or `TrustedReport`.

- [ ] **Step 6: Run focused tests GREEN and commit**

---

### Task 4: Static contract, CI and exact-head closure

**Files:**
- Create: `tests/wave27_runtime_realization_contract.py`
- Create: `.github/workflows/wave27-runtime-realization-contract.yml`
- Create: `docs/superpowers/verification/2026-09-08-wave27-runtime-realization-provenance-authority-closure.md`

**Interfaces:**
- Consumes: all Task 1–3 files.
- Produces: dedicated Wave27 CI evidence and closure record.

- [ ] **Step 1: Add static contract**

Require exact production symbols, same-scrape parser use, domain-separated runtime digest, sealed authority shapes, and absence of public field-based constructors/forbidden shortcut strings.

- [ ] **Step 2: Add dedicated workflow**

Run Python contract, focused Wave27 Go tests, prior Wave23–26 trust tests, `go vet ./...`, `go test ./...`, and race coverage appropriate to NolaneWorld.

- [ ] **Step 3: Verify workflow on implementation head**

Require dedicated Wave27 job GREEN plus repository-wide applicable checks.

- [ ] **Step 4: Write closure record**

Record base SHA, committed RED SHA(s), final exact implementation SHA, focused/full commands, CI run IDs, and explicit non-claims.

- [ ] **Step 5: Create draft PR**

Open Wave27 against the same base lineage as Wave26, keep draft/unmerged, and include RED→GREEN lineage plus exact-head evidence.

- [ ] **Step 6: Final exact-head verification**

Re-fetch PR head and workflow runs; any newer commit invalidates prior evidence and must be re-verified.

Autonomously-by: ChatGPT:GPT-5.6-Sol
