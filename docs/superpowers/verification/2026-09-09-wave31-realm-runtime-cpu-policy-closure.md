# Wave31 Realm Runtime CPU Policy Authority — Closure Evidence

Date: 2026-09-09
Repository: `Nolane-x/Nolane-sandbox`
Branch: `gpt/wave31-realm-physical-cpu-policy-authority`
Draft PR: #42
Base branch: `gpt/wave30-exact-cpu-throttle-intervention-authority`
Base exact SHA: `3657256ea40528bf673a8ca4d51c69233d161b45`

## 1. Claim boundary

Wave31 adds one explicit Realm runtime CPU bandwidth intent, `RuntimeCPULimitMilliCPU`, and one opaque package-owned authority over the exact current Realm revision and full Realm `PolicyDigest` that carries that positive intent.

Wave31 does not reinterpret `ResourceBudget.CPUUnits`. The established accounting budget remains accounting-only and cannot mint, default, derive, or substitute the runtime CPU policy intent.

Wave31 does **not** claim or implement:

- propagation through Cube/fabric/provider create boundaries;
- `cpu.max` or quota/period equivalence;
- cgroup placement/readback;
- Wave30 causal-throttling equality with requested Realm policy;
- memory or disk enforcement;
- `resourceproof` trusted-producer integration;
- `TrustedReport`;
- aggregate resource-enforcement verification;
- `LIVE_PASS`;
- physical-core affinity, cpuset/exclusive cores, NUMA placement, or performance guarantees.

## 2. Backward-compatibility contract

The Realm field is exactly:

```go
RuntimeCPULimitMilliCPU uint64 `json:"runtime_cpu_limit_millicpu,omitempty"`
```

Zero means no runtime CPU kernel-limit intent. `Spec.Validate()` does not require a positive value. The existing `PolicyDigest` algorithm and domain remain unchanged.

A pre-Wave31 `validSpec()` at revision `7` is frozen to the exact legacy digest:

`ff2a38e09715c134c0590d2c286afb4d2d19480eebf04acdcea2a057723c55b1`

The Wave31 compatibility tests require zero-valued legacy Specs to omit `runtime_cpu_limit_millicpu` and preserve this digest exactly.

## 3. Canonical committed RED

Test/contract-only candidate:

`8a3522ad15102c37d237a87d9f3fa050f5a30ffc`

Dedicated workflow:

- run: `34315037426`
- job: `102349364158`
- conclusion: `failure`
- failing step: `Focused Wave31 tests`
- checkout/Python/pytest/Go setup: all success

Expected production-missing compile failures included:

```text
spec.RuntimeCPULimitMilliCPU undefined
ctl.CurrentRuntimeCPUPolicyAuthority undefined
ErrRuntimeCPUPolicyUnavailable undefined
ctl.ValidateRuntimeCPUPolicyAuthority undefined
```

No Wave31 production field or authority source existed at this SHA. The later static/regression/vet/full/race steps were skipped after the intended focused compile RED.

Detailed RED record:

`docs/superpowers/verification/2026-09-09-wave31-realm-runtime-cpu-policy-red.md`

## 4. Initial GREEN and static-contract correction

Initial authority production candidate:

`abfef9b55df62d55d6a71ba27db177ef9fc51ef3`

Dedicated run:

`34315237972`

Behavioral Wave31 tests passed. The static contract failed because it required exactly one ASCII space between the Go field name/type/tag while `gofmt` aligned the struct with multiple spaces. This was a static-test parser bug, not a production trust failure.

The static assertion was changed to a whitespace-tolerant regex without changing production semantics.

Post-correction candidate:

`0e33fa11a6c5b61ca6b128170e332d98375093d4`

Dedicated run:

`34315318546`

Result: full success across focused Wave31, static anti-shortcut, prior Realm/V23 authority regression, `go vet ./...`, full `go test ./...`, and focused race.

## 5. Review-driven RED and hardening

Whole-diff trust review found one structural integrity gap after the first full GREEN: `RuntimeCPUPolicyAuthority.Binding()` validated a non-nil bound Store but did not itself require the Store's exact dynamic type to be `*MemoryStore` or `*DurableStore`.

A package-internal marker-bearing wrapper was therefore constructed in a review test to prove the gap.

Review RED candidate:

`9ab5c300f4a17908ef32225d7532bd75ad2c306b`

Dedicated run:

`34315526680`

Exact failing behavioral contract:

`TestV31BindingRejectsNonExactPackageStore`

The test failed because a wrapper Store still produced `Binding() == valid`.

Production was then hardened with an exact dynamic type check in the authority's own structural validation. Only non-nil exact `*MemoryStore` and exact `*DurableStore` bound Store values are accepted; wrappers/embeddings are rejected even if they can satisfy the package-private marker.

## 6. Exact code-head

Code-head SHA after review hardening:

`ac6c2e5cbfb270ac103d8ef7db982c3f9508bd07`

Dedicated push workflow:

- workflow: `Wave31 Realm Runtime CPU Policy Contract`
- run: `34315601174`
- job: `102351035366`
- conclusion: `success`

All dedicated steps succeeded:

- Focused Wave31 tests — SUCCESS
- Static Wave31 contract — SUCCESS
- Prior Realm authority regression — SUCCESS
- `go vet ./...` — SUCCESS
- full `go test ./...` — SUCCESS
- focused Wave31 race — SUCCESS

## 7. Code-head PR integration evidence

PR #42 was opened as draft with exact:

- base SHA: `3657256ea40528bf673a8ca4d51c69233d161b45`
- head SHA: `ac6c2e5cbfb270ac103d8ef7db982c3f9508bd07`
- mergeable: true after GitHub completed mergeability calculation
- state: open
- draft: true
- merged: false

All nine applicable PR verification workflows completed SUCCESS on this exact code-head:

1. DCO Check — `34315712515` — SUCCESS
2. Cube Kernel OOM Victim Contract — `34315712581` — SUCCESS
3. Nolane World Check — `34315712513` — SUCCESS
4. Docs Build Check — `34315712586` — SUCCESS
5. Wave31 Realm Runtime CPU Policy Contract — `34315712508` — SUCCESS
6. Wave 23 Realm Cube Realization Contract — `34315712565` — SUCCESS
7. Deploy VitePress site to GitHub Pages — `34315712582` — SUCCESS
8. Format Check — `34315712560` — SUCCESS
9. Cube Guest Kernel OOM Victim Contract — `34315712518` — SUCCESS

The Format workflow succeeded on both amd64 and arm64, including builder formatting, web formatting, and no-uncommitted-format-change checks.

The Guest Kernel OOM workflow completed SUCCESS after its deepest Cubelet job exercised `Verify CubeBox live pre-Start binding in canonical builder`.

Code-head verification therefore consisted of ten successful code/integration surfaces: one dedicated push contract plus nine PR integration workflows.

## 8. Code-head ancestry and scope audit

Compare:

`3657256ea40528bf673a8ca4d51c69233d161b45...ac6c2e5cbfb270ac103d8ef7db982c3f9508bd07`

Audit result:

- merge base: exactly `3657256ea40528bf673a8ca4d51c69233d161b45`
- `ahead_by=14`
- `behind_by=0`
- changed files: 10

Exact code-head file scope:

1. `.github/workflows/wave31-realm-runtime-cpu-policy-contract.yml`
2. `NolaneWorld/realm/model.go`
3. `NolaneWorld/realm/runtime_cpu_policy_authority.go`
4. `NolaneWorld/realm/runtime_cpu_policy_authority_external_v31_test.go`
5. `NolaneWorld/realm/runtime_cpu_policy_authority_v31_review_test.go`
6. `NolaneWorld/realm/runtime_cpu_policy_authority_v31_test.go`
7. `docs/superpowers/plans/2026-09-09-realm-runtime-cpu-policy-authority-v31.md`
8. `docs/superpowers/specs/2026-09-09-realm-runtime-cpu-policy-authority-v31-design.md`
9. `docs/superpowers/verification/2026-09-09-wave31-realm-runtime-cpu-policy-red.md`
10. `tests/wave31_realm_runtime_cpu_policy_contract.py`

Production changes are limited to `realm/model.go` and `realm/runtime_cpu_policy_authority.go`. No Cube, fabric, provider, Cubelet, cgroup, Wave30, or `resourceproof` production behavior was modified.

## 9. Review surface audit

At code-head on PR #42:

- submitted reviews: 0
- inline review threads: 0
- PR conversation comments: 0

The direct whole-diff trust review that found the Store structural-validation gap was converted into committed review-driven RED evidence before the production hardening fix.

## 10. Auxiliary Claude automation

Auxiliary run:

- workflow: `Claude Auto Review`
- run: `34315712501`
- job: `102351376984`
- conclusion: `failure`

This automation did **not** execute a code review. It failed at:

`Generate GitHub App token`

with exact error:

`[@octokit/auth-app] appId option is required`

The following steps were skipped:

- `Prefetch PR review inputs`
- `Run Claude review`
- `Publish sticky review comment`

Therefore this is a repository credential/configuration failure in an auxiliary review workflow, not a Wave31 code finding and not a substitute for the successful Wave31 code/integration gates.

## 11. Exact-final verification requirement

This closure document itself changes the branch SHA. The code-head evidence above is lineage only after this commit.

Wave31 is not code-closed until the new exact-final closure SHA independently passes:

1. a fresh dedicated `Wave31 Realm Runtime CPU Policy Contract` push run;
2. every applicable PR verification workflow on PR #42;
3. a refreshed ancestry/scope audit with this closure document as the only additional expected file;
4. refreshed PR state/review-surface checks;
5. explicit accounting of any auxiliary review automation outcome.

PR #42 must remain draft/open/unmerged unless a human explicitly authorizes merge.
