# Wave 27 — Exact Runtime Realization Provenance Authority Closure

## Status

Wave27 implementation is code-complete relative to its design and implementation plan. This record captures the committed RED→GREEN lineage and the first full pull-request integration pass. Because this document itself changes the branch head, final closure still requires a fresh exact-head CI pass after this commit.

## Lineage

Wave26 base / Wave27 merge base:

`2aff6aaa846fce77c91a36038ca61b15d96b1a67`

Branch:

`gpt/wave27-runtime-realization-provenance-authority`

Draft PR:

`#38 — Wave 27: exact runtime realization provenance authority`

PR base:

`gpt/wave26-provider-endpoint-spki-authority`

Pre-closure-document code head:

`f765d45444d5fcfcf6c93b0af9edf0491a4f76ba`

The base-to-code-head comparison is `ahead_by=14`, `behind_by=0`, with merge base exactly `2aff6aaa846fce77c91a36038ca61b15d96b1a67`.

## Trust seam closed

Wave27 closes only the exact runtime-realization provenance seam left intentionally open by Wave23 and preserved by Waves24–26.

The strongest identity chain after Wave27 is:

```text
fresh Realm realization
+ exact Cube ResourceBinding
+ fresh Cubelet realization epoch
+ fresh provider-persisted sandbox incarnation
+ fresh pinned TLS endpoint SPKI
+ exact current host runtime process realization
```

Wave27 does not reinterpret any identity component as resource enforcement.

## Production behavior

### Same-scrape runtime realization proof

`RuntimeRealizationObserver` performs one bounded request to the existing Cubelet resource-metrics management endpoint and accepts authority only when that same scrape contains exactly one canonical current realization epoch and exactly one canonical host process identity for the requested sandbox.

The observer reuses the existing authoritative parsers:

- `exactRealizationEpochFromSample`
- `exactHostSandboxProcessIdentityFromSample`

It requires exact sandbox/generation equality and therefore does not stitch together an epoch from one scrape with a process identity from another scrape.

`RuntimeRealizationProof` is sealed, has no public field-based constructor, and is bound to the exact `*RuntimeRealizationObserver` that minted it. Fresh validation re-observes the same pair and fails closed after epoch rotation, runtime process replacement, host boot-ID change, or observer-context substitution.

### Wave27 bridge

`ValidateRealmResourceRuntimeAuthority(...)` does not accept Wave26 authority by shape alone. It extracts the nested provider-incarnation and endpoint proofs and re-runs `ValidateRealmResourceProviderEndpointAuthority(...)` through the current Realm controller, realization authority, ResourceBinding, Wave24 epoch observer and exact Cube client.

After exact equivalence with the supplied Wave26 capability is established, it freshly validates the exact runtime proof and requires the runtime proof's sealed Wave24 epoch identity to equal the epoch embedded beneath Wave26.

Only then does it mint sealed `RealmResourceRuntimeAuthority`.

### Runtime digest

A valid Wave27 authority exposes:

```text
runtime-realization-v27:<64-lowercase-hex>
```

The digest is SHA-256 over a canonical JSON document prefixed by the domain separator:

```text
nolane.runtime-realization.v27\x00
```

The authority-owned preimage binds:

- Realm ID and revision;
- policy digest;
- World ID and Realm realization revision;
- substrate handle / sandbox identity;
- Wave17 generation;
- Wave24 realization epoch token;
- Wave25 provider incarnation ID;
- Wave26 endpoint SPKI SHA-256;
- host PID and process start-time ticks;
- host boot ID;
- host cgroup path;
- runtime role and producer source;
- canonical placement and binding timestamps.

`RealmResourceRuntimeAuthority.Valid()` recomputes the digest and requires the same embedded Wave24 epoch, so a copied digest string is not authority.

### resourceproof binding projection

`resourceproof.BindingFromRuntimeAuthority(...)` projects only descriptive:

- Realm ID;
- Realm revision;
- policy digest;
- realization revision;
- Wave27 runtime digest.

The projected `Binding` remains ordinary data. A caller can copy or fabricate a Wave27-looking runtime digest, but public `BuildReport` still treats copied `SourceLiveHost` observations as untrusted and returns `UNAVAILABLE`, never `LIVE_PASS`.

## Committed RED→GREEN evidence

### Harness RED

Commit:

`71f21c07f89a51e7331efc8f2de25509795dc350`

Workflow run:

`34219851159`

The static Wave27 contract executed successfully under pytest and failed because Wave27 production files were intentionally absent. An earlier harness attempt failed because pytest itself was not installed; that infrastructure defect was fixed before counting RED evidence.

### Task 1 — same-scrape runtime proof

Behavioral RED commit:

`e0f839be094e9ba181c951da2e03e1373d7c151a`

Before production code, the opaque-accessor assertions were corrected at:

`1ea6974e330430cb52a08a69a2f13f140367101c`

Production GREEN commit:

`33136c1e622232b1f4a2d7d42b1d95d8c6b901c7`

Checkpoint workflow run:

`34220071717`

The focused Wave27 Cube tests were GREEN while later Task2/Task3 static closure surfaces were still intentionally absent.

### Task 2 — Wave27 bridge and runtime digest

RED commit:

`91619b6dced19bc7f0c033d7cb2b03702ddd4b63`

RED workflow run:

`34220369417`

Focused Go compilation failed specifically because `RealmResourceRuntimeAuthority`, `ValidateRealmResourceRuntimeAuthority` and the Wave27 bridge error type did not yet exist.

GREEN commit:

`d51a117ffb25db959786429627ce9e8e571ed272`

GREEN checkpoint run:

`34220498442`

Focused Wave27 bridge tests passed; the run remained non-green only because Task3 production was still intentionally absent from the static closure contract.

### Task 3 — resourceproof binding projection

RED commit:

`5fce3e836e19fe670cb2c139023b580a26f24331`

RED workflow run:

`34220584152`

Cube Wave27 tests remained GREEN, while resourceproof compilation failed specifically because `BindingFromRuntimeAuthority` and `ErrRuntimeBindingUnavailable` did not yet exist.

GREEN/code-head commit:

`f765d45444d5fcfcf6c93b0af9edf0491a4f76ba`

Dedicated code-head workflow run:

`34220692458`

All dedicated Wave27 gates succeeded:

- focused Wave27 Cube tests;
- focused resourceproof projection tests;
- static anti-shortcut contract;
- prior Wave23–26 trust tests;
- `go vet ./...` in NolaneWorld;
- `go test ./...` in NolaneWorld;
- focused race tests for Wave27/runtime authority surfaces.

## First full PR integration pass on code head

Exact SHA:

`f765d45444d5fcfcf6c93b0af9edf0491a4f76ba`

All 16 applicable pull-request workflows completed `SUCCESS`:

1. Wave27 Runtime Realization Contract — run `34220834151`.
2. Cube Task Outcome Contract — run `34220834176`.
3. Format Check — run `34220834279`, including amd64 and arm64 format/no-diff jobs.
4. DCO Check — run `34220834297`.
5. Cube Host Process Identity Contract — run `34220834141`.
6. Deploy VitePress site to GitHub Pages — run `34220834263`.
7. Cube Guest Kernel OOM Victim Contract — run `34220834182`; all five jobs succeeded, including canonical-builder Cubelet live pre-Start binding verification.
8. Wave 22 Public Live Verdict Contract — run `34220834168`.
9. Wave 26 Provider Endpoint SPKI Contract — run `34220834214`.
10. Wave 25 Provider Incarnation Contract — run `34220834194`.
11. Cube Kernel OOM Victim Contract — run `34220834267`.
12. Nolane Live Substrate Gauntlet — run `34220834262`.
13. Docs Build Check — run `34220834265`.
14. Wave 24 Cubelet Realization Epoch Contract — run `34220834272`.
15. Cube Realization OOM Contract — run `34220834231`.
16. Nolane World Check — run `34220834308`.

No review submissions or inline review threads were present during the integration audit.

## Scope audit

The code-head PR contained exactly 11 Wave27 files before this closure record:

- dedicated workflow;
- same-scrape observer/proof production and tests;
- Wave27 bridge/digest production and tests;
- resourceproof binding projection and tests;
- design;
- implementation plan;
- RED lineage record;
- static trust contract.

No Wave27 production change modified Cubelet, CubeAPI, Realm, Wave24, Wave25 or Wave26 producer semantics. Existing authority layers are consumed through their public/opaque validators rather than weakened or replaced.

## Anti-laundering review

The whole-PR trust review found no new public constructor that can recreate:

- `RuntimeRealizationProof` from descriptive fields;
- `RealmResourceRuntimeAuthority` from descriptive fields;
- Wave27 authority from JSON;
- `resourceproof.TrustedReport` from Wave27 binding data.

The static contract also forbids the designed shortcut families, including PID-only identity, timestamp-only identity, sandbox-ID hash substitutes, endpoint-SPKI-only runtime identity and caller-supplied runtime digest authority.

## Explicit non-claims

Wave27 does **not** prove:

- CPU enforcement;
- memory enforcement;
- disk enforcement;
- aggregate resource enforcement;
- OOM causality or victim identity;
- task success/failure;
- filesystem isolation;
- guest-state correctness;
- image/binary measurement;
- hardware or TEE attestation;
- `resourceproof.TrustedReport` provenance by itself;
- public `LIVE_PASS` by itself.

## Final exact-head requirement

This closure record is itself a new commit. The evidence above proves the implementation/code head and its first complete PR integration pass, but final Wave27 closure requires every applicable pull-request workflow to be re-run and observed `SUCCESS` on the commit containing this document. Any subsequent branch change invalidates that final exact-head evidence and requires another verification pass.

## Next safe trust seam

The next safe wave is a package-owned causal resource producer that consumes fresh Wave27 runtime provenance together with direct CPU/memory readback and authoritative task/OOM evidence before minting `resourceproof.TrustedReport`. That later seam, not Wave27 identity alone, is where production `LIVE_PASS` activation can be considered.

Autonomously-by: ChatGPT:GPT-5.6-Sol
