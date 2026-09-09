# Wave32 Realm Runtime CPU Policy Create Propagation Closure

## Scope

Wave32 propagates one sealed Wave31 Realm runtime CPU policy authority through the fabric World-create path into the exact Cube provider-create request and returns descriptive propagation evidence. It does not claim provider interpretation or kernel enforcement.

## Exact base

- Wave31 exact-final base: `9219908c5dc67d66bae4a424247114ffa6e6dbe4`
- Branch: `gpt/wave32-realm-cpu-policy-create-propagation-authority`
- PR: `#43`
- Spec: `docs/superpowers/specs/2026-09-09-realm-runtime-cpu-policy-create-propagation-v32-design.md`
- Plan: `docs/superpowers/plans/2026-09-09-realm-runtime-cpu-policy-create-propagation-v32.md`

## Canonical RED lineage

Initial committed RED:

- SHA: `54307e87e1e0cb7c5598562cfc893be2c79f5c36`
- Dedicated run: `34317682606`
- Setup succeeded.
- Focused Wave32 compilation failed because production Wave32 seams did not yet exist, including `substrate.RuntimeCPUPolicyCreatePropagation`, `substrate.RuntimeCPUPolicyCreatePropagationDigestPrefix`, and `ErrRuntimeCPUPolicyPropagationUnavailable`.
- No YAML, test-syntax, or unrelated regression failure was accepted as canonical RED.

Initial implementation GREEN lineage:

- Pre-review implementation head: `66cccf435da31686788acee6c30325398c94c6ba`
- Dedicated run: `34317933223`
- Focused fabric/Cube tests, static contract, prior Realm authority regressions, vet, full NolaneWorld tests, and focused race all succeeded.

## Whole-diff trust review and review-driven RED

Whole-diff review found an Important trust gap despite the initial GREEN: the propagation receipt was bound to Realm policy, World ID, and exact request bytes, but not to the provider-returned sandbox handle. A manager could therefore pair a valid request receipt for one sandbox-create operation with a different returned handle, preventing later waves from proving that propagation evidence belonged to the leased runtime sandbox.

Review-driven RED:

- Fabric test-only SHA: `a48eda0018128b3ef8e3ec83befc45be57b5e428`
- Dedicated run: `34323147321`
- Job: `102374170226`
- Setup succeeded.
- Failure was exact and expected: `unknown field SubstrateHandle in struct literal of type substrate.RuntimeCPUPolicyCreatePropagation`.
- Cube boundary test was then extended at `4f8feba6dc63badf8061e5dd696d8fe1d020ea55` to require the same provider-sandbox binding before production was hardened.

Production hardening lineage:

- `94bea121cb103fe7eae4d67ebe42b9886d4a1059` — add `SubstrateHandle` to the descriptive propagation receipt and structural validation.
- `e7da33fff43c79fea948c8970f43d2a315c0aea1` — populate the receipt only from the actual provider response `sandboxID` returned by the Cube `POST /sandboxes` operation.
- `a316855ab78e7f4d4861f96001cae56dcbfbc6c0` — require fabric receipt handle equality with the exact returned/leased substrate handle.
- Code-head/static lock: `b4b02a95eb96e270498a5c9617ba062329635747`.

The final matching invariant at code-head requires all Wave31 binding fields, exact World ID, and `propagation.SubstrateHandle == handle` before a successful lease is possible.

## Dedicated code-head verification

Exact code-head: `b4b02a95eb96e270498a5c9617ba062329635747`

Dedicated push run: `34323683113` — **SUCCESS**.

Successful steps:

- Focused Wave32 fabric tests
- Focused Wave32 Cube tests
- Static Wave32 anti-shortcut contract
- Prior Realm authority regression
- `go vet ./...`
- full `go test ./... -count=1`
- focused Wave32 race tests

## Exact code-head PR integration verification

All 15 applicable PR workflows on `b4b02a95eb96e270498a5c9617ba062329635747` completed successfully:

1. DCO Check — `34323686790` — SUCCESS
2. Cube Host Process Identity Contract — `34323686951` — SUCCESS
3. Wave 26 Provider Endpoint SPKI Contract — `34323686718` — SUCCESS
4. Wave 25 Provider Incarnation Contract — `34323686742` — SUCCESS
5. Deploy VitePress site to GitHub Pages — `34323686494` — SUCCESS
6. Cube Task Outcome Contract — `34323686641` — SUCCESS
7. Wave 24 Cubelet Realization Epoch Contract — `34323686624` — SUCCESS
8. Cube Realization OOM Contract — `34323686693` — SUCCESS
9. Docs Build Check — `34323686655` — SUCCESS
10. Cube Kernel OOM Victim Contract — `34323686548` — SUCCESS
11. Wave32 Realm Runtime CPU Policy Create Propagation Contract — `34323686696` — SUCCESS
12. Nolane World Check — `34323686670` — SUCCESS
13. Nolane Live Substrate Gauntlet — `34323686684` — SUCCESS
14. Format Check — `34323686606` — SUCCESS
15. Cube Guest Kernel OOM Victim Contract — `34323686701` — SUCCESS

The deep `cubelet-wave21` job in run `34323686701` passed `Verify CubeBox live pre-Start binding in canonical builder`.

## Code-head ancestry and scope audit

Compare Wave31 exact-final `9219908c5dc67d66bae4a424247114ffa6e6dbe4` to code-head `b4b02a95eb96e270498a5c9617ba062329635747`:

- merge base: exact Wave31 final
- `ahead_by=16`
- `behind_by=0`
- 10 changed files

Production scope is limited to:

- `NolaneWorld/fabric/fabric.go`
- `NolaneWorld/substrate/cube/client.go`
- `NolaneWorld/substrate/runtime_cpu_policy_create_propagation.go`

The remaining files are Wave32 tests, static contract, workflow, spec, plan, and verification evidence.

## Review surface audit

At code-head:

- submitted PR reviews: 0
- review threads: 0
- PR comments: 0

A manual whole-diff trust review verified:

- `AcquireRequest` accepts no raw runtime CPU limit, policy digest, descriptive binding, or authority material;
- positive-policy Realm creation cannot silently fall back to legacy `WorldManager.Create`;
- Cube accepts sealed `realm.RuntimeCPUPolicyAuthority`, not a caller-constructed binding or raw milliCPU value;
- the exact JSON bytes hashed for `RequestDigest` are the same bytes sent to `POST /sandboxes`;
- provider metadata is derived only from the validated Wave31 authority binding;
- the returned receipt binds the provider response sandbox handle;
- fabric requires the receipt handle to equal the exact returned/leased substrate handle;
- the same Wave31 authority instance is freshness-checked before and after the external create;
- mismatch or stale evidence cannot yield a successful lease;
- zero-policy Realms preserve the legacy create path;
- Wave32 production contains no `cpu.max`, `TrustedReport`, or `LIVE_PASS` claim laundering.

## Auxiliary automation

`Claude Auto Review` run `34323684876` failed before any code review ran. `Generate GitHub App token` failed with `[@octokit/auth-app] appId option is required`; prefetch, Claude review, and publish steps were skipped. This is a repository credential/configuration issue, not a Wave32 code finding. It is intentionally recorded separately from code/integration gates.

## Explicit nonclaims

Wave32 does **not** claim:

- provider interpretation of the propagated metadata;
- provider-side CPU policy enforcement;
- any milliCPU-to-quota/period conversion;
- `cpu.max` equivalence;
- cgroup membership, placement, or readback;
- equivalence with Wave30 causal CPU throttling evidence;
- memory or disk enforcement;
- aggregate resource verification;
- `TrustedReport` eligibility;
- `LIVE_PASS`.

## Exact-final ceremony requirement

This closure commit changes the branch SHA. Therefore the code-head evidence above is lineage only after this commit. Wave32 may be called code-closed only after fresh dedicated push verification and every applicable PR integration workflow succeed on the new closure exact SHA, followed by a fresh ancestry/scope/review-state audit. PR #43 remains draft/open/unmerged without explicit human merge authorization.
