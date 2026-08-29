# Nolane Sandbox Live Substrate Gauntlet v5 Design

**Status:** Approved implementation spec

**Date:** 2026-08-29

## 1. Goal

Release Gauntlet v4 proves Nolane trust invariants deterministically against real trust-plane code. v5 adds a second, non-substitutable evidence class: **live substrate evidence** produced only when the harness crosses CubeAPI, creates a real sandbox, reaches envd inside the guest, observes execution, exercises snapshot/rollback, and observes teardown.

v5 MUST NOT turn missing KVM, missing CubeAPI, missing credentials, missing targets, or a skipped job into PASS.

## 2. Non-negotiable law

> Deterministic proof and live proof are different evidence families. Neither may impersonate the other.

Live state is one of:

- `LIVE_PASS`: all scenarios required by the selected live profile were materially exercised and passed.
- `LIVE_FAIL`: a required live boundary was exercised and violated, or cleanup/verification failed.
- `UNAVAILABLE`: the environment lacked a declared prerequisite, so no live claim is made.

`UNAVAILABLE` is valid diagnostic evidence but **never live-release approval**. `require-live` treats it as a non-zero gate failure.

## 3. Architecture

```text
cmd/nolane-gauntlet-live
        |
        v
 gauntlet/live Runner --------------------+
        |                                 |
        | semantic evidence               | host authority state
        v                                 v
 live.Report / VerifyReport          world.State + authority.Broker
        |
        v
 gauntlet/live/cube Driver
        |
        +--> CubeAPI /health
        +--> POST /sandboxes
        +--> POST /sandboxes/:id/connect
        +--> envd /process.Process/Start
        +--> snapshot / rollback
        +--> PUT /network
        +--> DELETE + observed GET 404
```

The live package owns semantics; the Cube driver owns wire interaction. V4 report hashing and scenario semantics remain unchanged.

## 4. Capability attestation

A Cube endpoint is not `LIVE` merely because `/health` responds or sandbox creation returns an ID.

The minimum live capability attestation requires:

1. CubeAPI health succeeds.
2. A sandbox is created with public traffic and internet disabled by default.
3. `/connect` returns guest data-plane material.
4. envd executes a fixed canary command.
5. Canary exit code is zero and stdout exactly matches the expected canary bytes.
6. Sandbox teardown is issued.
7. Teardown is **observed**: repeated GET reaches not-found, not merely DELETE returning success.

Only then may `GuestExecution=true` and `CleanupObserved=true` appear in a `LIVE_PASS` report.

Raw sandbox IDs, envd tokens, traffic tokens, API keys, and injected credentials MUST NOT enter report evidence. Opaque runtime identifiers are represented by SHA-256 digests when binding is useful.

## 5. Core live scenarios

### 5.1 `live.cube.guest-execution`

Attack/probe path: CubeAPI -> create -> connect -> envd command.

Expected proof:
- control plane observed;
- guest execution observed;
- exact canary observed;
- cleanup observed.

### 5.2 `live.cube.snapshot-authority-monotonicity`

A real sandbox writes state `A`, takes a snapshot, mutates state to `B`, then rolls back. Guest observation MUST return `A` after rollback, proving real execution-state rewind occurred.

In parallel, host-owned authority advances from epoch `e` to `e+1` before rollback. After rollback an effect carrying epoch `e` MUST still be rejected with `world.ErrStaleEpoch`.

This proves the law:

> Execution state may rewind; authority state may not rewind.

### 5.3 `live.cube.cleanup-observed`

Cleanup is a release invariant, not best-effort hygiene. Any created sandbox must enter a cleanup lease. A scenario cannot PASS until every lease is observed absent. Unknown teardown outcome is `LIVE_FAIL` because a hostile guest may remain alive.

## 6. Controlled egress scenarios

Egress denial must not be inferred from a failed connection to an endpoint that was already dead.

A target-dependent scenario therefore has two phases:

1. **Host preflight:** prove the configured target is reachable/valid outside the sandbox.
2. **Guest attempt:** execute the corresponding probe from the live sandbox.

Target configuration is explicit. Missing or failed preflight yields `UNAVAILABLE`, never PASS.

v5 defines target slots for HTTP, TCP, UDP, and DNS. A strict live profile requires every configured/required target; a core profile may omit target-dependent scenarios while still making a narrower `LIVE_PASS` claim whose profile is embedded in evidence.

The report MUST name the exact profile so `core` cannot be mistaken for `full-egress`.

## 7. Network policy safety

The Cube driver exposes typed full-replacement network policy operations. It MUST NOT accept arbitrary JSON from the live CLI.

The safe default remains:
- internet disabled;
- public ingress disabled;
- no credential injection;
- no permissive wildcard rule.

Network updates used by tests must be constructed from typed values and preserve CubeAPI's strict full-replacement semantics.

## 8. Credential boundary

v5 does not place API/envd/traffic credentials into guest environment variables or release evidence.

A future/target-backed credential-injection scenario may prove CubeEgress injection against a controlled HTTPS reflector, but absence of that reflector cannot be presented as injection proof. The v5 core still tests that its own driver redacts every control/data-plane token from evidence and error strings.

## 9. Live evidence

`live.Report` contains only semantic, sanitized fields:
- schema version;
- profile;
- mode (`probe` or `require-live`);
- substrate = `cubesandbox`;
- status;
- stable reason code;
- endpoint SHA-256 fingerprint;
- template SHA-256 fingerprint;
- capability booleans;
- ordered scenario evidence;
- cleanup evidence;
- report digest.

No wall clock is required for the trust digest. Runtime IDs are hashed before inclusion.

`VerifyReport` recomputes all evidence digests and validates status logic. It rejects a report that claims `LIVE_PASS` without guest execution, cleanup observation, all required scenarios, or a live-capable profile.

## 10. Modes

### `probe`

Used on ordinary developer/hosted CI machines. Missing live config yields a verified `UNAVAILABLE` report and exit code 0. Any false `LIVE_PASS`, malformed report, or an exercised security failure remains non-zero.

### `require-live`

Used on a provisioned Cube/KVM runner. `UNAVAILABLE` is non-zero. `LIVE_FAIL` is non-zero. Only verified `LIVE_PASS` exits zero.

## 11. CI

Normal `Nolane World Check` keeps running V4 deterministic evidence and unit/race/vet tests for the V5 harness.

Add `.github/workflows/nolane-live-gauntlet.yml`:
- `workflow_dispatch`;
- a probe job on hosted Linux that proves missing live config cannot fabricate PASS;
- a live job enabled only when repository variable `NOLANE_LIVE_GAUNTLET_ENABLED == 'true'`;
- live job uses `[self-hosted, linux, nolane-kvm]` and environment `nolane-live-gauntlet`;
- secrets are injected only into the process environment and never artifact names or report bodies;
- live report is uploaded as an artifact;
- `require-live` is mandatory in the live job.

A skipped live job is visibly “not executed”; it does not certify the release.

## 12. Failure physics

The harness fails closed on:
- connect response without guest endpoint material;
- malformed or compressed envd stream it cannot verify;
- canary mismatch;
- snapshot failure;
- rollback that does not restore guest state;
- stale host authority accepted after rollback;
- target preflight uncertainty;
- cleanup timeout;
- report mutation/hash mismatch;
- leaked secret material in report serialization;
- a `LIVE_PASS` claim produced without the required exercised markers.

## 13. Scope boundaries

v5 does not modify KVM, RustVMM, CubeNet, CubeEgress, CubeCoW, guest kernel, or CubeAPI server internals.

v5 adds a live proof harness and only the narrow client methods necessary to use existing public Cube APIs. If a needed proof requires changing the security substrate itself, that becomes a separate upstream-compatible task rather than a hidden v5 patch.

## 14. Release claim

After v5 is merged, Nolane Sandbox may claim:

- deterministic V4 trust proof is continuously exercised on hosted CI;
- the repository contains a fail-closed live Cube/KVM proof harness;
- a particular build is **live-substrate verified only when a `LIVE_PASS` v5 artifact from a configured live runner exists**.

The repository MUST NOT claim that hosted CI alone constitutes KVM/live verification.
