# Wave32 Realm Runtime CPU Policy Create Propagation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Propagate sealed Wave31 Realm runtime CPU policy authority through fabric World creation into the exact Cube provider-create HTTP request and return canonical propagation evidence, without claiming enforcement.

**Architecture:** Preserve legacy `WorldManager.Create` for zero-policy Realms. Positive-policy Realms use a separate typed `RuntimeCPUPolicyWorldManager` capability that accepts only sealed `realm.RuntimeCPUPolicyAuthority`; Cube derives provider metadata and a request digest from that authority, and fabric brackets the external create with Wave31 freshness checks before accepting a successful lease.

**Tech Stack:** Go 1.x, standard library HTTP/JSON/SHA-256, Python pytest static contracts, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-09-realm-runtime-cpu-policy-create-propagation-v32-design.md`

## Global Constraints

- Base exact SHA: `9219908c5dc67d66bae4a424247114ffa6e6dbe4`.
- Branch: `gpt/wave32-realm-cpu-policy-create-propagation-authority`.
- Do not put raw runtime milliCPU, Realm policy digest, or Wave31 authority into `fabric.AcquireRequest`.
- Positive CPU policy must never silently fall back to legacy `WorldManager.Create`.
- Zero CPU policy must preserve the legacy create path.
- Provider metadata is propagation evidence only; no `cpu.max`, quota/period, cgroup, throttling-causality, TrustedReport, or LIVE_PASS claim.
- Every autonomous commit must include `Autonomously-by: ChatGPT:GPT-5.6-Sol` and must not include `Signed-off-by`.

---

## File Structure

Create:
- `NolaneWorld/substrate/runtime_cpu_policy_create_propagation.go` — provider-neutral descriptive propagation receipt and canonical validation helpers.
- `NolaneWorld/fabric/runtime_cpu_policy_create_v32_test.go` — positive-policy routing, fail-closed manager capability, receipt matching, freshness bracket, and zero-policy compatibility tests.
- `NolaneWorld/substrate/cube/runtime_cpu_policy_create_v32_test.go` — actual HTTP-body propagation and reconstructed-authority rejection tests.
- `tests/wave32_realm_runtime_cpu_policy_create_propagation_contract.py` — static anti-shortcut contract.
- `.github/workflows/wave32-realm-runtime-cpu-policy-create-propagation-contract.yml` — dedicated focused/static/regression/vet/full/race gate.
- `docs/superpowers/verification/2026-09-09-wave32-realm-runtime-cpu-policy-create-propagation-red.md` — committed RED lineage.
- `docs/superpowers/verification/2026-09-09-wave32-realm-runtime-cpu-policy-create-propagation-closure.md` — final closure evidence.

Modify:
- `NolaneWorld/fabric/fabric.go` — typed positive-policy manager capability, same-Store Realm controller, pre/post freshness bracket, receipt validation.
- `NolaneWorld/substrate/cube/client.go` — authority-bearing create method, canonical request-body construction and request digest.

No other production file should change unless a committed RED demonstrates a missing required seam.

---

### Task 1: Commit Wave32 RED contract

**Files:**
- Create: `NolaneWorld/fabric/runtime_cpu_policy_create_v32_test.go`
- Create: `NolaneWorld/substrate/cube/runtime_cpu_policy_create_v32_test.go`
- Create: `tests/wave32_realm_runtime_cpu_policy_create_propagation_contract.py`
- Create: `.github/workflows/wave32-realm-runtime-cpu-policy-create-propagation-contract.yml`

**Interfaces expected by tests:**

```go
type RuntimeCPUPolicyWorldManager interface {
    CreateWithRuntimeCPUPolicy(
        context.Context,
        world.ID,
        realm.RuntimeCPUPolicyAuthority,
    ) (substrate.Handle, substrate.RuntimeCPUPolicyCreatePropagation, error)
}

func (c *Client) CreateWithRuntimeCPUPolicy(
    ctx context.Context,
    id world.ID,
    authority realm.RuntimeCPUPolicyAuthority,
) (substrate.Handle, substrate.RuntimeCPUPolicyCreatePropagation, error)
```

Receipt expected by tests:

```go
type RuntimeCPUPolicyCreatePropagation struct {
    RealmID         string
    RealmRevision   uint64
    PolicyDigest    string
    LimitMilliCPU   uint64
    AuthorityDigest string
    WorldID         world.ID
    RequestDigest   string
}
```

- [ ] **Step 1: Add fabric RED tests**

Tests must prove:
- a positive-policy Realm calls `CreateWithRuntimeCPUPolicy` exactly once and never calls legacy `Create`;
- manager lacking the typed capability fails closed before legacy/external create;
- manager returning a receipt with any RealmID/revision/policy digest/limit/authority digest/WorldID mismatch makes Acquire fail without a successful lease;
- a Realm policy update performed inside the typed manager call makes post-create freshness fail and Acquire return outcome uncertainty;
- zero-policy Realm still uses legacy `Create` and does not require the typed capability;
- `AcquireRequest` remains free of runtime CPU policy fields.

Production change that makes these tests pass: add the typed manager interface, private same-Store Realm controller, authority mint/validate bracket, and exact receipt checks.

- [ ] **Step 2: Add Cube RED tests**

Use `httptest.Server` and a real `realm.MemoryStore` + `realm.Controller` to mint Wave31 authority. Tests must prove:
- actual `POST /sandboxes` JSON contains exact authority-derived metadata values;
- returned receipt exactly matches the Wave31 binding and world ID;
- `RequestDigest` is `runtime-cpu-policy-create-propagation-v32:<64 lowercase hex>` and changes when the exact provider body changes;
- a zero/reconstructed `realm.RuntimeCPUPolicyAuthority` fails before HTTP and produces no request.

Production change that makes these tests pass: implement the authority-bearing Cube create path and exact request-body digest.

- [ ] **Step 3: Add static anti-shortcut contract**

The Python contract must assert:
- `AcquireRequest` source does not contain `RuntimeCPULimitMilliCPU`, `RuntimeCPUPolicyAuthority`, `PolicyDigest`, or `LimitMilliCPU` fields;
- `RuntimeCPUPolicyWorldManager` consumes `realm.RuntimeCPUPolicyAuthority`, not `uint64`;
- positive-policy code contains no fallback call to legacy `Create`;
- Cube authority-bearing create method calls `authority.Binding()`;
- canonical provider metadata keys from the spec are present;
- Wave32 digest prefix/domain are present;
- no Wave32 production file contains `cpu.max`, `TrustedReport`, or `LIVE_PASS`.

- [ ] **Step 4: Add dedicated workflow**

Run in order:

```bash
go test ./fabric -run 'TestV32' -count=1
go test ./substrate/cube -run 'TestV32' -count=1
python -m pytest ../tests/wave32_realm_runtime_cpu_policy_create_propagation_contract.py -q
go test ./realm -run 'TestV31|TestV23' -count=1
go vet ./...
go test ./...
go test -race ./fabric ./substrate/cube -run 'TestV32' -count=1
```

Working directory for Go commands: `NolaneWorld`.

- [ ] **Step 5: Commit RED harness**

Commit message:

```text
test(wave32): lock CPU policy create propagation contract

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

- [ ] **Step 6: Run exact committed RED SHA**

Expected: setup succeeds; focused Wave32 compile/tests fail because `RuntimeCPUPolicyCreatePropagation`, `RuntimeCPUPolicyWorldManager`, and `CreateWithRuntimeCPUPolicy` do not yet exist. Static/vet/full/race may be skipped by workflow failure ordering.

Do not accept YAML, syntax, import, or unrelated regression failures as canonical RED.

---

### Task 2: Add provider-neutral propagation receipt and Cube GREEN

**Files:**
- Create: `NolaneWorld/substrate/runtime_cpu_policy_create_propagation.go`
- Modify: `NolaneWorld/substrate/cube/client.go`
- Test: `NolaneWorld/substrate/cube/runtime_cpu_policy_create_v32_test.go`

**Produces:**

```go
const RuntimeCPUPolicyCreatePropagationDigestPrefix = "runtime-cpu-policy-create-propagation-v32:"
```

with a validation method/helper that rejects empty/noncanonical fields and malformed request digests.

- [ ] **Step 1: Implement receipt validation**

Require non-empty RealmID, nonzero RealmRevision/LimitMilliCPU, canonical lowercase 64-hex policy digest, Wave31 authority digest prefix `realm-runtime-cpu-policy-v31:`, non-empty WorldID, and Wave32 request-digest prefix plus lowercase 64 hex.

- [ ] **Step 2: Refactor Cube create-body construction minimally**

Create one internal helper that returns canonical JSON bytes for the provider create body. Legacy `Create` must preserve its existing body semantics.

- [ ] **Step 3: Implement `CreateWithRuntimeCPUPolicy`**

Call `authority.Binding()` first. If invalid, return a Cube Wave32 invalid-authority error before HTTP.

Add exact metadata derived only from the binding:

```text
nolane.realm.id
nolane.realm.revision
nolane.realm.policy_digest
nolane.realm.runtime_cpu_policy_digest
nolane.realm.runtime_cpu_limit_millicpu
```

Hash the exact JSON bytes to be sent with domain `nolane.runtime-cpu-policy-create-propagation.v32\x00`; send those same bytes in `POST /sandboxes`; validate provider response; return handle plus receipt.

- [ ] **Step 4: Run focused Cube tests**

Run:

```bash
cd NolaneWorld
go test ./substrate/cube -run 'TestV32' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Cube GREEN**

Commit message:

```text
feat(wave32): propagate CPU policy at Cube create boundary

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 3: Add fabric authority routing and freshness bracket

**Files:**
- Modify: `NolaneWorld/fabric/fabric.go`
- Test: `NolaneWorld/fabric/runtime_cpu_policy_create_v32_test.go`

- [ ] **Step 1: Add `RuntimeCPUPolicyWorldManager`**

Do not alter the existing `WorldManager.Create` signature.

- [ ] **Step 2: Add private same-Store Realm controller to `Local`**

`NewLocal` must call `realm.NewController(store)` and retain the returned controller. No caller supplies a controller or authority.

- [ ] **Step 3: Route positive policy through authority path**

Before create side effects for a positive current Realm policy:
- assert manager implements `RuntimeCPUPolicyWorldManager`;
- mint current authority from `f.realmController`;
- validate it;
- require binding RealmID/revision equals Acquire/current Realm state.

Zero-policy path remains `f.manager.Create(ctx, req.WorldID)`.

- [ ] **Step 4: Validate receipt and post-create freshness**

After typed create returns:
- validate receipt structurally;
- compare all receipt binding fields and WorldID to pre-call Wave31 binding;
- re-run `ValidateRuntimeCPUPolicyAuthority` on the same authority;
- if stale/invalid/mismatch, record uncertain operation and return `ErrOutcomeUncertain` without successful lease.

- [ ] **Step 5: Run focused fabric tests**

```bash
cd NolaneWorld
go test ./fabric -run 'TestV32' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit fabric GREEN**

```text
feat(wave32): require sealed CPU policy create propagation

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

---

### Task 4: Full GREEN and trust review

**Files:**
- Update static/workflow tests only if a demonstrated harness defect exists.
- Create review-driven RED test file only if whole-diff review finds a real untested trust gap.

- [ ] **Step 1: Run dedicated Wave32 workflow on exact production head**

Require success for focused fabric, focused Cube, static, prior Realm authority regression, vet, full suite, race.

- [ ] **Step 2: Audit whole diff against Wave31 exact-final**

Require merge base exact `9219908c5dc67d66bae4a424247114ffa6e6dbe4` and `behind_by=0`.

- [ ] **Step 3: Manual trust review**

Check specifically:
- no raw caller policy path;
- no positive-policy fallback;
- Cube cannot accept descriptive binding in place of authority;
- HTTP receipt digest covers bytes actually sent;
- pre/post freshness is the same authority instance;
- mismatch/staleness never yields a successful lease;
- zero-policy compatibility remains intact;
- no enforcement claim laundering.

If a real gap is found, add a committed review-driven RED test, verify RED, patch only the root cause, then rerun the full dedicated gate.

---

### Task 5: Draft PR and exact-final closure

**Files:**
- Create: `docs/superpowers/verification/2026-09-09-wave32-realm-runtime-cpu-policy-create-propagation-closure.md`

- [ ] **Step 1: Open stacked draft PR**

Head: `gpt/wave32-realm-cpu-policy-create-propagation-authority`

Base: `gpt/wave31-realm-physical-cpu-policy-authority`

Title:

```text
Wave 32: Realm runtime CPU policy create propagation authority
```

Include exact base/head, canonical RED, any review-driven RED, dedicated GREEN, architecture, trust invariants, and explicit nonclaims.

- [ ] **Step 2: Observe every applicable PR workflow on exact code head**

Do not infer from prior Wave31 runs.

- [ ] **Step 3: Audit reviews/threads/comments**

No unresolved code finding may remain.

- [ ] **Step 4: Commit closure record**

Record exact code head, all run IDs/conclusions, ancestry/scope audit, review findings, and auxiliary automation failures separately from code/integration gates.

Commit:

```text
docs(wave32): record exact code-head closure

Autonomously-by: ChatGPT:GPT-5.6-Sol
```

- [ ] **Step 5: Verify exact-final closure SHA fresh**

Rerun/observe dedicated push and every applicable PR integration workflow on the closure SHA. Re-audit ancestry/scope and PR state.

Only then call Wave32 code-closed. Keep PR draft/open/unmerged without explicit human merge authorization.
