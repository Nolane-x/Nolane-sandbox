# Live Realm Proof v9

Live Realm Proof v9 is the evidence bridge between the semantic Freedom Realm profiles introduced in v8 and host capability verification.

It is intentionally narrower than a general security certification. A Realm profile says what the host intends to enforce. A v9 `LIVE_PASS` says that a specific configured Cube execution observed the mandatory profile effects. Only an exact-bound, verified `LIVE_PASS` report may become host-owned `CapabilityEvidenceSource` data.

## Evidence chain

```text
Realm policy
  -> Cube profile projection
  -> live guest canary after profile apply
  -> raw-public negative probe
  -> public-ingress positive control + unauthenticated denial probe
  -> observed cleanup
  -> sealed deterministic v9 report
  -> exact Realm/revision/policy evidence binding
  -> agentruntime capability projection
```

No step may be skipped by treating configuration, availability, or an unverified caller assertion as proof.

## Realm profiles

| Profile | Semantic meaning | Raw Internet | Public inbound | Reality authority |
|---|---|---:|---:|---|
| `R0_INTERNAL_ONLY` | Internal-only Realm | denied | denied | none inherited |
| `R1_PUBLIC_READ` | Public reads through governed Reality path | denied | denied | governed gateway required |
| `R2_SUPPLY_CHAIN` | Supply-chain work through governed Reality/provider paths | denied | denied | typed delegation/provider authority required where applicable |

R1/R2 do not turn on ambient Internet. The semantic profile mapper still sends `allowInternetAccess=false` and `allowPublicTraffic=false` to Cube. Public Reality access belongs to the governed gateway/provider authority path, not to the sandbox network namespace.

## Mandatory live proof

A v9 report can be `LIVE_PASS` only when all of the following are observed for the exact run:

1. the Realm profile was applied through the existing Cube network update path;
2. guest execution still works after the profile transition;
3. a host-preflighted raw-public target is not reachable from the guest;
4. a public ingress canary first succeeds with the host-owned traffic token and then rejects an unauthenticated request;
5. sandbox destruction is observed rather than merely requested.

The ingress positive control is important. A connection failure to a dead service is not evidence of network enforcement. The token-authenticated request must reach the canary before an unauthenticated `403` can count as a restricted-public-ingress observation.

## Fail-honest statuses

- `LIVE_PASS`: all mandatory observations passed. The report is approved.
- `LIVE_FAIL`: a mandatory observation materially contradicted the requested policy, or cleanup became uncertain. The report is not approved.
- `UNAVAILABLE`: the environment or probe could not establish the claim. The report is not approved.

`UNAVAILABLE != PASS` is a release invariant. Ordinary pull-request runners without Cube/KVM configuration deliberately emit a deterministic `UNAVAILABLE` negative-control artifact.

## Capability truth table

| Agent capability claim | Can v9 verify it? | Required evidence |
|---|---:|---|
| Guest execution | Yes | post-profile guest canary PASS |
| Public inbound disabled | Yes | authenticated ingress positive control + unauthenticated denial PASS |
| Network isolation for raw-public/public-ingress dimensions | Yes, narrowly | raw-public denial PASS + public-ingress denial PASS |
| Internal private mesh | Only when a genuine private sibling route is observed | private mesh scenario PASS |
| Public read authority | No | authority belongs to governed Reality/delegation layers |
| Snapshot support | No | not inferred from the Realm-profile proof |
| Filesystem isolation | No | requires independent enforcement evidence |
| Process isolation | No | requires independent enforcement evidence |
| Kernel resource enforcement | No | Realm accounting is not kernel-enforcement proof |
| KVM/hypervisor escape impossibility | No | outside v9's claim surface |

Current Cube v9 integration does not expose a trustworthy private sibling-route primitive to the harness. Therefore internal mesh remains `UNAVAILABLE`; public proxy reachability or traffic-token behavior must not be renamed into mesh proof.

## Exact evidence binding

The immutable evidence source is constructed only from a sealed, verified, approved `LIVE_PASS` report and a host binding containing:

- exact Realm ID;
- exact Realm revision;
- exact canonical policy digest.

A query for a different Realm, revision, or policy receives no evidence. The source has no mutation/rebind method and is safe for concurrent reads.

Evidence strings contain deterministic report/scenario digests rather than endpoint URLs, sandbox handles, target addresses, credentials, envd tokens, or traffic tokens.

## CLI

Safe negative-control on a machine without live Cube configuration:

```bash
go run ./cmd/nolane-realm-gauntlet-live \
  --mode probe \
  --profile R0_INTERNAL_ONLY \
  --raw-public-kind http \
  --raw-public-target https://example.invalid/nolane-v9-negative-control
```

A provisioned release gate uses `--mode require-live`. In that mode both `UNAVAILABLE` and `LIVE_FAIL` are non-success results.

Cube configuration remains host-owned through the existing variables:

- `NOLANE_CUBE_API_URL`
- `NOLANE_CUBE_API_KEY`
- `NOLANE_CUBE_TEMPLATE_ID`
- `NOLANE_CUBE_SANDBOX_DOMAIN`
- `NOLANE_CUBE_PROXY_SCHEME`

The raw-public test target is explicit CLI input and is represented in evidence only by its deterministic digest.

## CI meaning

`Nolane World Check` always runs the v9 CLI without live Cube configuration as a negative control. It generates the artifact twice, requires byte identity, requires `UNAVAILABLE` and `approved=false`, scans plaintext/base64/hex forms of a synthetic Cube credential, and uploads the commit-bound artifact.

That artifact proves fail-honest behavior and deterministic serialization. It is **not** a live/KVM profile verification claim.

A real Realm-profile claim requires a `LIVE_PASS` artifact generated for the exact commit on explicitly configured live infrastructure.

## Historical evidence nondrift

V9 is additive. It does not change the canonical v4, v6, v7, or v8 evidence families. Release closure compares regenerated artifacts against pre-v9 known-good artifacts byte-for-byte.

Known canonical SHA-256 values at v9 closure:

- v4: `94ef192c57f2587d34a8340a8bfd8d297782e121c88ad4aa96792e42bf40c6f4`
- v6: `34705e6ce2128ce884447004257d22fe577ad0b98ef1cf91df0f57ae270148ce`
- v7: `b449afab3d3af299eedf700ce402a070191ce8678af3e2f7c9eabe02ec92a315`
- v8: `d8a6cf3d9bfbcfe1fe7be53452f105a9a8387462df24b862d7413aad5f52afc4`

## Non-claims

V9 does not claim universal isolation, production KMS/HSM correctness, distributed consensus, complete provider coverage, private mesh verification when no private route was observed, or hypervisor escape impossibility.

Its purpose is narrower and stronger: **never let a semantic Realm profile or an unavailable probe masquerade as verified live enforcement.**
