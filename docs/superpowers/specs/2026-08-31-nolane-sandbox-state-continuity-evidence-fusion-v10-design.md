# Nolane Sandbox State Continuity / Evidence Fusion v10 — Design

## Status

Approved autonomous continuation after Live Realm Proof v9. Base is `master@a66b5770c2fdff459f25a165c2b25bee5eed5441`.

Repository governance requires transparent contribution provenance. Human DCO is optional; AI-authored commits carry `Autonomously-by: ChatGPT:GPT-5.6-Sol` and MUST NOT fabricate `Signed-off-by`.

## Problem

The current trust chain is intentionally conservative:

- v5 proves live Cube guest execution, snapshot/rollback state restoration, stale-authority rejection, cleanup, and optional egress behavior.
- v9 proves a semantic Realm profile was behaviorally observed on Cube, including guest execution after profile application, raw-public denial, restricted public ingress, cleanup, and optional genuine private mesh.
- `agentruntime.Service` accepts trusted capability verification only from a host-owned immutable `CapabilityEvidenceSource` bound to exact Realm ID, Realm revision, and canonical policy digest.
- the v9 evidence adapter correctly refuses to infer snapshot support from a network-profile proof.

As a result, `Runtime.Capabilities().SnapshotRollback` cannot become `Verified` even when v5 has a real `LIVE_PASS` snapshot proof. Passing a v5 report and a v9 report independently is not sufficient: evidence must prove that both observations belong to the same immutable live provider configuration and must remain independently verifiable without weakening v5 or v9 nondrift.

## Goal

Create a v10 evidence-fusion layer that runs the existing v5 live substrate proof and v9 live Realm proof through the same live driver instance, seals both verified reports into a deterministic composite receipt, and exposes a conservative exact-bound `CapabilityEvidenceSource` that can verify snapshot/rollback together with the existing v9 network claims.

The intended chain is:

`same host-owned Cube driver -> v5 LIVE_PASS + v9 LIVE_PASS -> fingerprint-locked v10 composite receipt -> exact Realm/revision/policy binding -> host CapabilityEvidenceSource -> Runtime.Capabilities`

## Non-goals

V10 does not claim:

- filesystem or process isolation;
- kernel/cgroup resource enforcement;
- public-read authority through the Reality gateway;
- universal private mesh when no genuine sibling route was observed;
- KVM/hypervisor escape impossibility;
- distributed consensus, multi-host scheduling, or complete provider coverage;
- snapshot correctness from an API success alone;
- PASS when either nested proof is `UNAVAILABLE`.

`UNAVAILABLE != PASS` remains a hard release invariant.

## Existing facts carried forward

1. v5 `runSnapshotAuthorityScenario` writes sentinel `A`, snapshots, writes `B`, rolls back, requires `A` to be restored, advances authority epoch, and requires the stale pre-rollback authority epoch to be denied.
2. v5 `LIVE_PASS` requires `SnapshotRollback=true`, guest execution, observed cleanup, a non-empty endpoint digest, a non-empty template digest, and valid scenario digests/markers.
3. v9 `LIVE_PASS` requires a non-empty endpoint digest, exact Realm-profile observations, raw-public denial, restricted public ingress, guest execution after profile application, and observed cleanup.
4. Both v5 and v9 already use the same `live.Driver` abstraction and the Cube implementation exposes an immutable `Fingerprint()` containing endpoint and template digests.
5. V9 intentionally does not include the template digest in its schema, so v10 MUST obtain template binding from the driver used to execute the nested proofs rather than retroactively altering the v9 report family.

## Architecture

### 1. New `capabilityproof` package

Create `NolaneWorld/gauntlet/live/capabilityproof` as a new v10 evidence family. It imports the stable v5 `live` package and v9 `realmproof` package but neither older package imports v10.

This keeps v5/v9 serialization and canonical hashes unchanged.

### 2. Same-driver composite runner

`capabilityproof.Runner` accepts:

- `Mode live.Mode`
- `Profile realm.NetworkProfile`
- `RawPublicTarget live.Target`

It receives one `live.Driver` instance.

For a configured driver it MUST:

1. read the driver fingerprint once before nested execution;
2. run the v5 **core** proof with the same driver instance; core is sufficient because v10 consumes v5 guest/snapshot/cleanup evidence and does not need v5's public-egress profile;
3. run the v9 Realm proof with the same driver instance and requested Realm profile/target;
4. verify both nested reports using their existing verifiers;
5. require v5 endpoint digest to equal the pre-run driver endpoint digest;
6. require v5 template digest to equal the pre-run driver template digest;
7. require v9 endpoint digest to equal the same pre-run driver endpoint digest;
8. reject any empty configured fingerprint component;
9. seal a v10 receipt containing the sanitized nested reports and the locked endpoint/template digests.

The runner MUST NOT infer that two arbitrary externally supplied reports share configuration merely because their endpoint strings happen to match. The trusted correlation comes from one runner invoking both proofs through the same already-constructed driver object and locking its fingerprint.

### 3. Fail-honest status composition

The v10 status is derived conservatively:

- no driver: `UNAVAILABLE`, reason `config_missing`;
- malformed/empty configured fingerprint: `LIVE_FAIL`, reason `fingerprint_invalid`;
- either nested report fails verification or contradicts the locked fingerprint: `LIVE_FAIL`, reason `evidence_mismatch`;
- either nested report is `LIVE_FAIL`: `LIVE_FAIL`, reason identifies the failing component;
- no nested failure but either nested report is `UNAVAILABLE`: `UNAVAILABLE`, reason identifies the unavailable component;
- only when both nested reports are `LIVE_PASS`, both are approved, fingerprints match, and mandatory capability facts are present: `LIVE_PASS`, `approved=true`.

Outer `ModeRequireLive` converts `UNAVAILABLE` to an error; `LIVE_FAIL` is always an error. Probe mode may emit a valid `UNAVAILABLE` receipt without returning failure.

### 4. Deterministic composite receipt

The v10 report contains:

- schema version;
- requested Realm profile and mode;
- substrate identifier;
- status/reason/approved;
- locked endpoint digest;
- locked template digest;
- sanitized full v5 report;
- sanitized full v9 report;
- derived capability bits;
- deterministic v10 digest.

It contains no Cube API key, envd token, traffic token, sandbox handle, endpoint URL, template ID, target plaintext, or WorldID.

`MarshalReport` verifies the full nested structure before serialization and rejects any caller-provided forbidden secret representation.

### 5. Conservative capability derivation

A v10 `LIVE_PASS` derives only the following facts:

- `GuestExecution`: both nested reports prove guest execution on the same locked driver configuration.
- `SnapshotRollback`: v5 snapshot-authority scenario PASS plus `SnapshotRollback=true`.
- `PublicIngressDenied`: v9 public-ingress-denial scenario PASS.
- `NetworkIsolationVerified`: v9 raw-public-denial and public-ingress-denial scenarios PASS.
- `InternalMeshVerified`: only if v9 carries a genuine private-mesh PASS.

It does not derive public read, filesystem isolation, process isolation, or resource enforcement.

### 6. Exact-bound host evidence source

`NewCapabilityEvidenceSource(report, binding)` accepts only a fully verified v10 `LIVE_PASS` report and a binding containing:

- Realm ID;
- Realm revision;
- canonical policy digest.

The resulting source is immutable and returns evidence only for the exact query triple.

Projected `ProviderAttestation` fields:

- guest execution: available + verified;
- snapshot rollback: available + verified;
- public inbound disabled: verified;
- network isolation: verified;
- internal mesh: available + verified only when the v9 nested proof genuinely verified it.

Evidence strings contain only v10/nested scenario/report digests. They MUST NOT contain provider handles, endpoints, template identifiers, WorldIDs, secrets, or target plaintext.

### 7. CLI

Add `cmd/nolane-capability-gauntlet-live` with the same host-owned Cube configuration surface as v9:

- `NOLANE_CUBE_API_URL`
- `NOLANE_CUBE_API_KEY`
- `NOLANE_CUBE_TEMPLATE_ID`
- `NOLANE_CUBE_SANDBOX_DOMAIN`
- `NOLANE_CUBE_PROXY_SCHEME`

Flags:

- `--mode probe|require-live`
- `--profile R0_INTERNAL_ONLY|R1_PUBLIC_READ|R2_SUPPLY_CHAIN`
- `--raw-public-kind http|tcp|udp|dns`
- `--raw-public-target ...`
- `--raw-public-expect ...`
- `--out ...`

Missing Cube configuration produces deterministic `UNAVAILABLE` in probe mode and never creates a false PASS.

### 8. CI and release evidence

Extend `Nolane World Check` with a v10 negative-control step that:

1. injects a synthetic Cube credential;
2. runs the v10 CLI twice without usable Cube URL/template configuration;
3. byte-compares the outputs;
4. requires `status=UNAVAILABLE` and `approved=false`;
5. scans plaintext/base64/hex forms of the synthetic credential;
6. uploads the commit-bound v10 artifact;
7. continues to regenerate v4/v6/v7/v8/v9 evidence unchanged.

Extend the live harness workflow so unit/race/vet includes the v10 package and CLI. A real self-hosted live job may run v10 only when explicitly configured; ordinary PR CI MUST NOT manufacture live PASS.

## Security invariants

1. Capability is not authority.
2. Availability is not verification.
3. A caller-provided v5 or v9 report cannot be used to claim same-driver correlation; only a sealed v10 report produced by the same-driver runner is accepted.
4. `Verified` requires nested verified evidence plus exact Realm/revision/policy binding.
5. Evidence from Realm revision N cannot verify revision N+1.
6. Snapshot rollback does not rewind authority epochs or grants.
7. V10 never converts snapshot availability into public-read/provider authority.
8. Missing infrastructure remains `UNAVAILABLE`.
9. Nested report fingerprint mismatch fails closed.
10. V4–v9 canonical evidence families must not drift.
11. No secret or realization handle may enter v10 evidence.
12. AI contribution provenance remains explicit; no fabricated human DCO is permitted.

## Verification strategy

### RED tests first

- nil driver cannot PASS;
- empty configured driver fingerprint cannot PASS;
- nested v5 `UNAVAILABLE` cannot PASS;
- nested v9 `UNAVAILABLE` cannot PASS;
- nested `LIVE_FAIL` propagates failure;
- fingerprint mismatch fails closed;
- tampered nested v5 report is rejected;
- tampered nested v9 report is rejected;
- forged v10 capability booleans are rejected by `VerifyReport`;
- v10 evidence source rejects non-PASS/unapproved reports;
- binding mismatch returns no evidence;
- v10 source verifies snapshot only when the v5 snapshot-authority scenario passed;
- internal mesh cannot be upgraded without the v9 private-mesh scenario.

### GREEN verification

- focused package tests;
- CLI tests for argument validation, probe-mode negative control, require-live exit code, atomic output, and credential redaction;
- `go test ./...`;
- `go test -race ./...`;
- `go vet ./...`;
- v4/v6/v7/v8/v9 regeneration and nondrift verification;
- v10 negative control generated twice and byte-compared;
- exact-head GitHub Actions before integration;
- fresh post-merge `master` World Check.

## Release/governance

Development uses RED/GREEN commits on `nolane/state-continuity-evidence-fusion-v10`. The full audit history may be preserved on a backup branch before the final candidate is compacted to one provenance-preserving commit.

The integration candidate must carry `Autonomously-by: ChatGPT:GPT-5.6-Sol`. Human DCO is optional under current repository policy and MUST NOT be fabricated by the agent.
