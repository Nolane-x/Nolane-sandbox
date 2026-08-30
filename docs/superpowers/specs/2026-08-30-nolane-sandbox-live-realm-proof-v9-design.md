# Nolane Sandbox Live Realm Proof v9 — Design

## Status

Approved continuation of Freedom Plane / Reality Membrane v8. This wave is stacked on the capability-truth hardening candidate (`7b32ef5e59c7e90c1369539f5a3a912f13868330`) until PR #11 receives human DCO and lands.

## Problem

v8 defines semantic Realm profiles and maps them to Cube network policy, but the strongest release evidence is still split:

- v8 proves the semantic policy and fail-closed mapping deterministically.
- v5 proves selected Cube guest/snapshot/egress behavior against a live substrate when infrastructure is configured.
- `Runtime.Capabilities` now accepts trusted verification only from a host-owned `CapabilityEvidenceSource` bound to exact Realm ID, Realm revision, and canonical policy digest.

What is missing is a trustworthy bridge from live Realm-profile observations to that host evidence authority. A successful network-policy API call is not sufficient evidence that ingress or egress enforcement actually occurred.

## Goal

Create a fail-honest Live Realm Proof layer that can produce exact-bound host capability evidence from real Cube observations without exposing substrate handles, tokens, or mutable evidence authority to the agent-facing runtime.

The intended chain is:

`Cube/KVM observation -> deterministic live Realm receipt -> exact Realm/revision/policy binding -> host CapabilityEvidenceSource -> Runtime.Capabilities`

## Non-goals

v9 does not claim:

- universal KVM or kernel escape-proof isolation;
- multi-host scheduling, consensus, or distributed Realm orchestration;
- public-read verification unless a governed Reality gateway is actually exercised;
- internal-mesh verification unless a genuine private sibling-to-sibling route is exercised;
- resource-enforcement verification without measured hard-enforcement evidence;
- PASS from configuration inspection alone.

`UNAVAILABLE != PASS` remains a hard invariant.

## Existing substrate facts

1. `ApplyRealmProfile` maps R0/R1/R2 through `membrane.Plan` and sets both raw Internet and public traffic according to the fail-closed semantic plan.
2. CubeAPI `PUT /sandboxes/:sandboxID/network` forwards updates into CubeMaster.
3. CubeMaster applies the network update to Cubelet before persisting the canonical sandbox spec. Therefore a successful update proves that the node accepted the policy, but not that every requested property was behaviorally enforced.
4. CubeProxy enforces `AllowPublicTraffic=false` on both cache-fill and cache-hit paths: a request without the exact sandbox traffic token receives HTTP 403. This gives v9 a real negative-observation surface for public ingress.
5. The v5 live runner already has explicit `LIVE_PASS`, `LIVE_FAIL`, and `UNAVAILABLE` states and must remain the common live substrate foundation.

## Architecture

### 1. Optional Realm-aware live interfaces

The v5 `Driver` and `Sandbox` interfaces remain source-compatible. v9 adds optional interfaces discovered by type assertion:

- `RealmProfileSandbox`: apply a Realm network profile to the exact live sandbox.
- `PublicIngressProber`: establish a canary service inside the sandbox and probe it externally without privileged traffic credentials.
- `InternalMeshProber`: exercise an actual private sibling-to-sibling route when the substrate exposes one.

Unsupported optional interfaces produce `UNAVAILABLE`, never PASS.

### 2. Realm proof runner

A dedicated Realm runner composes the existing live substrate rather than changing v5 release semantics.

For each requested Realm profile it must:

1. verify control-plane health;
2. create a live sandbox;
3. apply the exact Realm profile;
4. prove guest execution still works after the policy transition;
5. probe a configured raw-public target and require it to be unreachable for R0/R1/R2;
6. start a deterministic HTTP canary and require unauthenticated external ingress to be denied (Cube: HTTP 403);
7. optionally exercise genuine internal mesh with two siblings; if unsupported, record `UNAVAILABLE` for that claim rather than rejecting otherwise valid profile proof;
8. observe cleanup.

A profile proof is `LIVE_PASS` only for the mandatory profile invariants. Optional mesh status is carried independently so it cannot be silently upgraded.

### 3. Deterministic receipt

The Realm live report contains no sandbox ID, substrate handle, traffic token, envd token, endpoint URL, or target plaintext. It stores only stable digests, scenario IDs, outcomes, reason codes, and non-secret markers.

The report is sealed with deterministic canonical JSON hashing. Identical synthetic observations must generate byte-identical reports.

### 4. Host evidence bridge

A new immutable adapter implements `agentruntime.CapabilityEvidenceSource` from a sealed Realm proof plus its exact binding:

- Realm ID
- Realm revision
- canonical policy digest

The adapter rejects malformed/unapproved reports and binding mismatches.

Claim translation is conservative:

- `GuestExecVerified`: only from a passed live guest-after-profile observation.
- `PublicInboundDisabled`: only from a passed unauthenticated external-ingress-denial observation.
- `InternalMeshVerified`: only from a passed genuine private-mesh observation.
- `NetworkIsolationVerified`: only when the mandatory raw-public-denial and public-ingress-denial observations both pass for the exact profile.
- `PublicReadVerified`: never produced by this runner; governed Reality gateway proof is a separate authority surface.
- `SnapshotVerified`: not inferred from this profile runner; v5 snapshot evidence may be composed only by an explicit future evidence combiner.
- filesystem/process/resource-enforcement claims are not inferred.

No agent-facing method can register or mutate this evidence.

### 5. Cube implementation

The Cube live driver will implement:

- profile application through the existing `cubewire.Client.ApplyRealmProfile` path;
- raw public egress negative probing through the existing guest command probe machinery;
- public-ingress denial by starting a bounded HTTP canary on a fixed high port and issuing an external request to the sandbox proxy **without** envd/traffic access-token headers. HTTP 403 is the expected restricted-ingress observation.

The privileged traffic token remains private inside the Cube driver and is never included in reports.

If Cube cannot expose a true private sibling endpoint without leaking realization authority into the agent surface, the Cube driver will not claim internal mesh in v9. The optional interface makes that limitation explicit and leaves a future substrate extension well-defined.

## Failure semantics

- Missing driver/config: `UNAVAILABLE`.
- Driver lacks mandatory Realm-profile or ingress-probe support: `UNAVAILABLE`.
- Configured public target cannot be preflighted externally: `UNAVAILABLE`.
- Applying the profile fails: `LIVE_FAIL`.
- Raw public target remains reachable after a profile that forbids raw Internet: `LIVE_FAIL`.
- Unauthenticated public ingress reaches the canary instead of being denied: `LIVE_FAIL`.
- Probe tooling missing inside guest: `UNAVAILABLE`.
- Cleanup cannot be observed: `LIVE_FAIL` with cleanup reason.
- Optional private mesh unsupported: mesh claim remains unavailable; it is never synthesized from public proxy reachability.

## Security invariants

1. Capability is not authority and availability is not verification.
2. Caller-supplied capability attestations cannot become trusted evidence.
3. Live proof does not carry credentials or realization handles.
4. Realm profiles never grant N3-N5 delegated provider authority.
5. No profile grants public inbound or raw public Internet.
6. `Verified` requires exact live evidence and exact Realm binding.
7. Evidence from revision N cannot verify revision N+1.
8. Missing live infrastructure remains `UNAVAILABLE`.
9. Historical v4-v8 deterministic evidence must not drift as a side effect of v9.

## Verification strategy

- RED contract tests first for unsupported driver, ingress bypass, egress bypass, binding mismatch, forged report, and mesh non-upgrade.
- Fake-driver deterministic tests for every status transition.
- Cube HTTP tests proving the external ingress probe sends no traffic/envd credential headers and interprets 403 as denial rather than success.
- Existing v5 live tests remain green.
- `go test ./...`, `go test -race ./...`, `go vet ./...` on the exact stacked candidate.
- v4/v6/v7/v8 deterministic evidence regeneration and nondrift checks.
- normal CI exercises the negative-control path only unless real Cube live infrastructure is explicitly configured; CI must not fabricate live PASS.

## Release/governance

v9 is developed on a branch stacked on PR #11 until the host-evidence hardening is human-DCO-complete. AI commits remain unsigned as required by repository `AGENTS.md`. RED/GREEN history may be preserved on an audit branch and compacted to one unsigned AI commit for human review at closure.
