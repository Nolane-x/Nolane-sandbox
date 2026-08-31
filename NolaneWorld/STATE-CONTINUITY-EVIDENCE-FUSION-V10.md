# State Continuity / Evidence Fusion v10

## Purpose

V10 closes a specific capability-truth gap between two already-independent live evidence families:

- **Live Substrate Gauntlet v5** proves guest execution, real `A -> B -> rollback -> A` state restoration, stale-authority denial after rollback, and observed teardown on a configured Cube substrate.
- **Live Realm Proof v9** proves that one semantic Realm profile was behaviorally exercised on Cube, including guest execution after profile application, raw-public denial, restricted public-ingress denial, cleanup, and optional genuine private mesh.

Neither proof is allowed to silently imply facts owned by the other. In particular, v9 intentionally cannot claim snapshot/rollback verification merely because the underlying provider happens to expose a snapshot API.

V10 fuses those proof families without changing either older report schema.

## Trust chain

The trusted chain is:

```text
one host-owned live.Driver instance
    |
    +--> v5 core proof ----+
    |                      |
    +--> v9 Realm proof ---+--> locked v10 composite receipt
                                   |
                                   +--> exact Realm ID
                                   +--> exact Realm revision
                                   +--> exact policy digest
                                   |
                                   +--> immutable CapabilityEvidenceSource
                                           |
                                           +--> Runtime.Capabilities
```

The critical property is **same-driver execution**, not merely similar text fields in two reports.

## Same-driver correlation

`capabilityproof.Runner` receives one `live.Driver` object. For configured infrastructure it:

1. reads the driver fingerprint before nested execution;
2. rejects an empty endpoint or template digest;
3. executes v5 core proof through that exact driver instance;
4. executes v9 Realm proof through that same driver instance;
5. independently verifies both nested reports;
6. requires v5 endpoint digest to equal the locked endpoint digest;
7. requires v5 template digest to equal the locked template digest;
8. requires v9 endpoint digest to equal the same locked endpoint digest;
9. derives capability facts only from nested verified evidence;
10. seals the full composite into a domain-separated deterministic digest.

A pair of reports supplied independently by a caller cannot manufacture this relationship. V10 does not expose an API that accepts arbitrary v5/v9 reports and declares them correlated.

A rotating or rebound provider fingerprint therefore fails closed as `LIVE_FAIL/evidence_mismatch`.

## Why the template digest matters

The v9 schema intentionally does not carry Cube template identity because v9 is a Realm-profile evidence family. V10 does **not** modify v9 to add that field.

Instead, the v10 runner locks the provider fingerprint itself before invoking either proof. V5 already binds both endpoint and template digests. The outer v10 receipt therefore obtains template correlation from the shared live driver and the verified v5 nested report while preserving v9 nondrift.

## Status semantics

V10 uses the same high-level live status vocabulary:

| Status | Meaning |
|---|---|
| `LIVE_PASS` | Both nested proof families passed, both reports verified, fingerprint binding matched, and all mandatory v10 facts were present. |
| `LIVE_FAIL` | A live violation was observed, a configured fingerprint was invalid, or evidence/fingerprint binding contradicted itself. |
| `UNAVAILABLE` | Required live infrastructure or one nested proof was unavailable and no stronger failure was observed. |

`UNAVAILABLE != PASS` is a hard invariant.

Failure precedence is conservative:

1. invalid evidence or fingerprint mismatch -> `LIVE_FAIL`;
2. nested `LIVE_FAIL` -> `LIVE_FAIL`;
3. nested `UNAVAILABLE` -> `UNAVAILABLE`;
4. only two verified nested `LIVE_PASS` reports may produce v10 `LIVE_PASS`.

Non-PASS v10 reports carry **zero derived capability bits**.

## Snapshot/rollback proof

Snapshot capability is not inferred from API availability.

The v5 nested proof must contain a passing snapshot-authority scenario that materially demonstrates:

```text
guest sentinel A
    -> snapshot
    -> guest sentinel B
    -> rollback(snapshot)
    -> guest sentinel A restored
```

At the same time, host authority remains monotonic. Rollback does not rewind an authority epoch, grant, effect receipt, Realm revision, lease generation, or other host-owned trust state. The stale pre-rollback authority epoch must remain denied.

Only that verified v5 scenario, fused with a verified same-driver v9 Realm proof, may set v10 `SnapshotRollback=true`.

## Derived capability truth

For a v10 `LIVE_PASS`, the composite may derive only:

| Runtime fact | V10 may verify? | Required evidence |
|---|---:|---|
| Guest execution | Yes | Passing v5 guest execution + passing v9 guest-after-profile on the same locked driver. |
| Snapshot rollback | Yes | Passing v5 snapshot-authority scenario plus v5 snapshot capability. |
| Public inbound disabled | Yes | Passing v9 restricted public-ingress denial. |
| Network isolation for observed v9 dimensions | Yes | Passing v9 raw-public denial + public-ingress denial. |
| Internal mesh | Conditional | Only a genuine passing v9 private-mesh scenario. |
| Public read through Reality gateway | **No** | V10 does not prove governed Reality-gateway reachability. |
| Filesystem isolation | **No** | Not established by v5/v9 fusion. |
| Process isolation | **No** | Not established by v5/v9 fusion. |
| Kernel/cgroup resource enforcement | **No** | Accounting budgets are not enforcement evidence. |

Capability remains distinct from authority. A verified snapshot capability does not grant external provider access, public read, credentials, network delegation, or Realm administration.

## Exact Realm binding

`NewCapabilityEvidenceSource` accepts only a fully verified v10 `LIVE_PASS` receipt and a host binding containing:

- exact `RealmID`;
- exact nonzero Realm revision;
- exact canonical policy digest.

The resulting source is immutable. Its `Snapshot` method returns evidence only when the entire query triple matches exactly.

Therefore:

```text
Realm A evidence != Realm B evidence
revision N evidence != revision N+1 evidence
policy P evidence != policy Q evidence
```

There is no wildcard, latest-revision fallback, mutable registration method, or agent-facing evidence setter.

## Evidence references

The capability adapter emits only digest references such as:

```text
live-capability-v10:guest:<digest>
live-capability-v10:snapshot:<digest>
live-capability-v10:public-inbound:<digest>
live-capability-v10:network:<digest>
```

These references do not contain Cube API keys, envd/traffic tokens, Cube sandbox handles, endpoint URLs, template identifiers, World IDs, raw network targets, or provider credentials.

## CLI

The host-facing CLI is:

```bash
go run ./cmd/nolane-capability-gauntlet-live \
  --mode probe \
  --profile R0_INTERNAL_ONLY \
  --raw-public-kind http \
  --raw-public-target https://example.invalid/nolane-v10-probe \
  --out release-evidence/nolane-capability-live-v10.json
```

Cube configuration is host-owned through `NOLANE_CUBE_API_URL`, `NOLANE_CUBE_API_KEY`, `NOLANE_CUBE_TEMPLATE_ID`, `NOLANE_CUBE_SANDBOX_DOMAIN`, and `NOLANE_CUBE_PROXY_SCHEME`.

The CLI constructs at most one Cube live driver and passes that same object into the v10 runner.

Exit semantics:

- `0`: verified `LIVE_PASS`, or a verified probe-mode `UNAVAILABLE` artifact;
- `2`: invalid invocation or require-live `UNAVAILABLE`;
- `1`: `LIVE_FAIL`, evidence verification failure, or output failure.

`--out` writes atomically.

## CI meaning

Ordinary GitHub-hosted CI is **not** treated as live Cube/KVM proof.

`Nolane World Check` deliberately runs v10 without usable Cube URL/template configuration and requires a deterministic negative-control receipt:

```text
status   = UNAVAILABLE
approved = false
reason   = config_missing
```

The artifact is generated twice and byte-compared. CI scans plaintext, base64, and hex forms of the synthetic credential before upload.

The Live Substrate workflow also race-tests and vets the v5/v9/v10 live packages and CLIs, and confirms that absent live infrastructure cannot become a PASS claim.

A real v10 capability claim requires a `LIVE_PASS` v10 receipt produced for the exact commit on explicitly configured live infrastructure. Repository presence, unit tests, or an `UNAVAILABLE` artifact are not substitutes.

## Historical nondrift

V10 is an additive evidence family. It does not alter the v4, v6, v7, v8, or v9 command/report schemas.

The release workflow continues to regenerate those historical evidence artifacts before generating v10. Their canonical payloads must remain unchanged unless a separately reviewed migration explicitly versions that evidence family.

## Non-claims

V10 does **not** prove KVM/hypervisor escape impossibility, universal filesystem/process isolation, kernel/cgroup resource enforcement, production KMS/HSM correctness, Reality-gateway public-read authority, public inbound availability, universal private mesh without a genuine observed sibling route, distributed consensus, multi-host scheduler correctness, or correctness of every possible Cube/provider deployment.

It also does not claim that snapshot rollback restores or rewinds host authority.

The narrow claim is stronger and auditable:

> When a v10 receipt is `LIVE_PASS`, the existing v5 state-continuity proof and v9 Realm-network proof were both verified through one fingerprint-locked live driver configuration, and only those observed facts may be projected into exact-bound runtime capability truth.
