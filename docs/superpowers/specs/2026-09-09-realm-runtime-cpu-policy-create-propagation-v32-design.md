# Wave32 Design — Realm Runtime CPU Policy Create Propagation Authority

Date: 2026-09-09
Base: Wave31 exact-final `9219908c5dc67d66bae4a424247114ffa6e6dbe4`
Branch: `gpt/wave32-realm-cpu-policy-create-propagation-authority`

## 1. Purpose

Wave31 proves one exact current Realm policy carries a positive runtime CPU bandwidth intent in milliCPU. It deliberately stops before any substrate or provider propagation.

Wave32 closes only the next trust gap: when `fabric.Local.Acquire` creates a World for a Realm whose current policy has a positive `RuntimeCPULimitMilliCPU`, the exact sealed Wave31 authority must cross the fabric -> WorldManager -> Cube provider-create boundary, and the Cube client must emit canonical request-bound propagation evidence for the exact Wave31 binding it received.

Wave32 remains propagation-only. It does not claim that the provider enforces the policy, that a provider field maps to Linux `cpu.max`, or that Wave30 throttling was caused by this policy.

## 2. Existing gap

At the Wave31 base, `fabric.WorldManager` exposes:

```go
Create(context.Context, world.ID) (substrate.Handle, error)
```

and `fabric.Local.Acquire` calls:

```go
f.manager.Create(ctx, req.WorldID)
```

The call contains no Realm runtime CPU policy authority.

The Cube client then posts `/sandboxes` with template/network/world metadata only. Therefore a positive Wave31 Realm CPU policy can exist while creation proceeds through a path that never observes it.

## 3. Approaches considered

### A. Raw milliCPU in `WorldManager.Create`

Rejected. A signature such as `Create(ctx, id, milliCPU)` turns a descriptive scalar into caller-controlled authority material and severs the Wave31 provenance chain.

### B. Context-carried propagation

Rejected. Storing policy data or authority in `context.Context` hides a security-relevant trust boundary and makes bypass/static audit harder.

### C. Explicit typed authority path — selected

Keep the legacy create path for Realms with no runtime CPU intent and introduce an explicit capability-bearing path for positive-intent Realms.

The fabric mints and freshness-validates Wave31 authority from its exact Realm Store. Callers do not supply milliCPU, policy digest, Realm revision, or authority.

The WorldManager must explicitly support the authority-bearing create path. Positive-intent creation fails closed when that capability is unavailable.

## 4. Public/structural API

Wave32 adds a provider-neutral propagation receipt in `substrate`:

```go
type RuntimeCPUPolicyCreatePropagation struct {
    RealmID          string
    RealmRevision    uint64
    PolicyDigest     string
    LimitMilliCPU    uint64
    AuthorityDigest  string
    WorldID          world.ID
    RequestDigest    string
}
```

This structure is descriptive evidence only. It is not authority and cannot mint Wave31 authority.

Fabric retains the existing `WorldManager` interface and adds a separate required capability for positive-intent Realms:

```go
type RuntimeCPUPolicyWorldManager interface {
    CreateWithRuntimeCPUPolicy(
        context.Context,
        world.ID,
        realm.RuntimeCPUPolicyAuthority,
    ) (substrate.Handle, substrate.RuntimeCPUPolicyCreatePropagation, error)
}
```

This avoids breaking the zero-policy legacy path while making positive-policy propagation explicit and fail-closed.

## 5. Fabric authority lifecycle

`fabric.Local` owns a private `*realm.Controller` built from the same Store passed to `NewLocal`.

For a Realm whose current `RuntimeCPULimitMilliCPU == 0`:

1. preserve the existing `WorldManager.Create(ctx, worldID)` path byte-for-byte in meaning;
2. no Wave31 authority is minted;
3. no Wave32 propagation claim exists.

For a Realm whose current `RuntimeCPULimitMilliCPU > 0`:

1. require the manager to implement `RuntimeCPUPolicyWorldManager` before external create side effects;
2. mint `CurrentRuntimeCPUPolicyAuthority(ctx, realmID)` from the exact package-owned Realm Store;
3. validate the authority immediately before the create call;
4. require its binding RealmID/revision to equal the exact Acquire RealmID/revision and the current Realm record;
5. call `CreateWithRuntimeCPUPolicy(ctx, worldID, authority)`;
6. validate the returned propagation receipt against the pre-call Wave31 binding and requested WorldID;
7. revalidate the same Wave31 authority immediately after the create call;
8. if post-call validation is stale/invalid, fail as outcome-uncertain and do not mint a successful lease;
9. only after all checks pass may the normal lease/completed-operation path continue.

The pre/post bracket does not make Realm update and provider create atomic. It establishes a fail-closed success rule: a successful Acquire cannot be returned if the Realm policy drifted across the provider-create operation.

## 6. Exact receipt checks

A valid propagation receipt must match the Wave31 binding exactly:

- RealmID
- RealmRevision
- PolicyDigest
- LimitMilliCPU
- Wave31 authority Digest

and must match the exact requested `world.ID`.

`RequestDigest` must be canonical lowercase SHA-256 hex and must be derived by the Cube client from the exact canonical JSON request body sent to `POST /sandboxes`, under a Wave32 domain separator.

A caller cannot provide or override the receipt in `AcquireRequest`.

## 7. Cube provider-create boundary

`cube.Client.CreateWithRuntimeCPUPolicy` accepts only:

- context
- world ID
- sealed Wave31 authority

It extracts the Wave31 binding through `authority.Binding()`. Invalid/zero/reconstructed authority fails before HTTP.

The provider create body keeps all existing fields and adds canonical Nolane metadata derived only from that binding:

- `nolane.realm.id`
- `nolane.realm.revision`
- `nolane.realm.policy_digest`
- `nolane.realm.runtime_cpu_policy_digest`
- `nolane.realm.runtime_cpu_limit_millicpu`

The exact request body is marshaled once to canonical JSON for the request-digest calculation and HTTP send path. Tests must capture the actual HTTP body and prove the receipt digest corresponds to that body.

These metadata fields are propagation evidence. Wave32 makes no claim that the provider interprets them as scheduling/enforcement controls.

## 8. Request digest

Prefix/domain:

```text
runtime-cpu-policy-create-propagation-v32:<64 lowercase hex>
nolane.runtime-cpu-policy-create-propagation.v32\x00
```

The hash input is the exact canonical JSON bytes sent as the provider create body.

Raw HTTP credentials, provider response bodies, API keys, and transient headers are not included.

## 9. Failure taxonomy

Wave32 adds fabric/cube errors sufficient to distinguish:

- positive Realm policy but manager lacks the typed propagation path;
- invalid/mismatched propagation receipt;
- invalid structurally reconstructed Wave31 authority at Cube ingress;
- stale Wave31 authority before or after create;
- ordinary provider create failure/outcome uncertainty.

No failure may silently fall back from the authority-bearing path to legacy `Create` for a positive CPU policy.

## 10. Compatibility

Wave32 must preserve:

- all Wave31 Realm JSON/digest compatibility;
- zero-policy Realm creation through legacy `Create`;
- existing `WorldManager` implementations for zero-policy tests/consumers;
- existing Cube create body semantics except for authority-bearing creation;
- all Wave23–Wave31 authority behavior.

## 11. Security/trust invariants

Wave32 tests and static contracts must lock:

1. callers cannot put raw milliCPU/policy digests/authority into `AcquireRequest`;
2. positive policy never uses legacy `WorldManager.Create`;
3. missing typed manager capability fails before provider create;
4. fabric derives authority only from its same Realm Store via `realm.Controller`;
5. pre-create Wave31 freshness is mandatory;
6. returned receipt must exactly match the Wave31 binding and WorldID;
7. post-create Wave31 freshness is mandatory before successful lease completion;
8. Cube rejects invalid/reconstructed authority before HTTP;
9. Cube request metadata is derived from Wave31 binding, not caller arguments;
10. receipt request digest covers the exact body sent;
11. no Wave32 API claims `cpu.max`, quota, period, throttling, OOM, TrustedReport, or LIVE_PASS.

## 12. TDD ceremony

Before production code:

- commit focused fabric behavioral tests;
- commit Cube HTTP-boundary tests;
- commit external opacity/anti-forgery tests where applicable;
- commit static Python anti-shortcut contract;
- commit dedicated Wave32 GitHub Actions workflow;
- run the committed exact RED SHA and require failure only because Wave32 production symbols/behavior do not exist.

Then implement the minimum GREEN, run focused/static/prior-regression/vet/full/race gates, perform whole-diff trust review, harden through review-driven RED if needed, and only then open a stacked draft PR.

## 13. CI and closure

Wave32 final closure requires:

- dedicated Wave32 exact-head SUCCESS;
- all applicable PR workflows SUCCESS on the same exact head;
- exact ancestry from Wave31 final with `behind_by=0`;
- scoped diff audit;
- no unresolved reviews/threads/comments;
- closure evidence committed;
- all applicable CI rerun and SUCCESS on the closure exact-final SHA.

The PR remains draft/open/unmerged unless explicit human authorization changes that.

## 14. Explicit nonclaims

Wave32 does **not** prove or implement:

- provider interpretation of the propagated metadata;
- vCPU allocation or scheduling guarantees;
- milliCPU -> quota/period conversion;
- `cpu.max` equivalence;
- cgroup placement/readback;
- requested-policy -> Wave30 causal throttling equivalence;
- memory/disk enforcement;
- cpuset/affinity/exclusive cores/NUMA;
- TrustedReport;
- aggregate resource-enforcement verification;
- LIVE_PASS.

Those require later evidence layers. In particular, Wave33 may only connect requested policy to effective kernel CPU configuration after Wave32 makes request provenance explicit.
