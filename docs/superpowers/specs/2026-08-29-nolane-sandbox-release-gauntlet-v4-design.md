# Nolane Sandbox Release Gauntlet v4 Design

**Status:** Implementation spec

**Date:** 2026-08-29

**Depends on:** Agent-World Foundation v0–v3 merged at `abb8e35cb5e2e98362299ec2dca74f435d949ef4`

## 1. Goal

Release Gauntlet v4 turns the security claims already implemented by `NolaneWorld` into executable, machine-verifiable release evidence.

The release invariant is:

> **A release is not trusted because its implementation claims an invariant; it is trusted only when required adversarial scenarios exercise that invariant and produce verifiable evidence that the defense held.**

The gauntlet must remain outside CubeSandbox security internals. It attacks through public Nolane interfaces and a narrow future adapter for live Cube/KVM execution.

## 2. Non-goals

V4 does not claim perfect sandbox security, replace external penetration testing, fuzz the KVM kernel, or prove absence of unknown vulnerabilities. It does not add new authority to an agent. It does not require a live Cube/KVM host for deterministic CI.

## 3. Architecture

V4 has four layers:

1. **Scenario Contract** — declarative identity, invariant, severity, required proof markers, and executable attack function.
2. **Deterministic Runner** — stable ordering, timeout handling, panic recovery, proof-of-exercise validation, fail-closed release decision.
3. **Evidence Envelope** — per-scenario SHA-256 evidence digests and one release report digest with strict verification.
4. **Hybrid adapters** — deterministic in-process scenarios now; live Cube/KVM adapter later through the same scenario/result boundary.

Property/fuzz tests stress the runner and evidence verifier without changing release semantics.

## 4. Scenario Contract

A scenario is defined by:

```go
type ScenarioSpec struct {
    ID              string
    Invariant       string
    Attack          string
    ExpectedDefense string
    Severity        Severity
    RequiredMarkers []string
}

type Scenario interface {
    Spec() ScenarioSpec
    Run(context.Context, *Probe) error
}
```

`ID` is globally unique and stable. `Invariant`, `Attack`, and `ExpectedDefense` are non-empty release evidence fields. Required markers name observable points that must be exercised.

A scenario never returns `PASS` directly. It can only execute operations and record observations through a runner-owned `Probe`.

## 5. Proof of Exercise

The runner creates the `Probe`; scenario code cannot forge the final evidence digest.

Probe events are append-only:

```go
type Event struct {
    Marker string
    Kind   EventKind
    Detail string
}
```

Allowed kinds are `attack`, `boundary`, `denial`, and `observation`.

A scenario passes only when all conditions hold:

- scenario returned no error;
- at least one `attack` event exists;
- at least one `boundary` event exists;
- at least one `denial` event exists;
- every `RequiredMarker` appeared;
- no duplicate scenario ID exists in the suite;
- the scenario did not panic or time out.

This prevents a vacuous test that simply returns success without touching the defense path.

## 6. Runner Semantics

`Runner.Run` sorts scenarios by `ScenarioSpec.ID` before execution. Report order is deterministic independent of registration order.

Each scenario receives a bounded context. Panic is recovered and becomes a failed scenario. Missing markers, missing attack/boundary/denial event classes, duplicate IDs, malformed specs, timeout, or execution errors all fail closed.

One failed required scenario means `Report.Approved == false`.

V4 has no optional/skipped required scenarios. A scenario that cannot execute is a release failure, not a silent skip.

## 7. Evidence Model

Every scenario produces `ScenarioEvidence` containing only deterministic fields:

- scenario ID;
- invariant;
- attack;
- expected defense;
- severity;
- outcome;
- normalized events;
- failure code/message if any;
- evidence digest.

Wall-clock timestamps are excluded from hashes. A caller may attach an external timestamp after verification, but it is not trust-bearing.

The scenario evidence digest uses length-prefixed SHA-256 fields with domain separator `nolane.gauntlet.scenario.v1`.

The release report digest binds:

- schema version;
- product ID `nolane-sandbox`;
- sorted scenario evidence digests;
- approved bit;
- runner policy digest.

Domain separator: `nolane.gauntlet.report.v1`.

`VerifyReport` recomputes every scenario digest, validates deterministic order, validates outcome semantics, recomputes the report digest, and rejects any mutation.

## 8. Deterministic Built-in Scenarios

V4 ships scenarios that attack existing trust boundaries rather than mocks alone.

### 8.1 Stale authority epoch

- create world state at epoch 1;
- advance authority to epoch 2;
- submit an epoch-1 intent through `authority.Broker`;
- prove executor invocation count remains zero;
- record stale-epoch denial.

### 8.2 Terminal authority

- close the world authority;
- submit an otherwise valid intent;
- prove executor invocation count remains zero;
- record terminal-world denial.

### 8.3 Action-ID rebinding

- execute action ID `a` with request A;
- submit action ID `a` with materially different request B;
- require `ErrActionCollision`;
- prove executor ran exactly once.

### 8.4 Artifact path traversal

- submit traversal/absolute/backslash/dot-component logical names to `artifact.Gate`;
- every hostile name must be rejected;
- no accepted receipt may be produced.

### 8.5 Capability trusted-blob tamper

- promote a valid capability into `DurableRegistry`;
- close registry;
- modify exact content CAS blob;
- reopening registry must return `ErrRegistryCorrupt`;
- the tampered capability must never become readable trusted material.

### 8.6 Capability journal tamper

- promote a valid capability;
- close registry;
- mutate a trust-bearing byte in `promotions.jsonl`;
- reopening must fail closed with `ErrRegistryCorrupt`.

## 9. Policy and Release Gate

`Policy` contains:

```go
type Policy struct {
    ScenarioTimeout time.Duration
    ProductID       string
}
```

Validation requires `ProductID == "nolane-sandbox"` and a positive bounded timeout.

The release gate is deliberately simple in V4: all registered scenarios are required. No percentage score can hide one failed high-severity invariant.

## 10. Property and Fuzz Layer

Go fuzz targets prove:

- report verifier rejects mutation of any trust-bearing report field;
- registration order never changes report digest;
- arbitrary malformed scenario IDs/markers/spec fields are rejected rather than normalized into trusted evidence;
- duplicate markers do not satisfy missing required markers;
- event strings cannot create hash ambiguities because all fields are length-prefixed.

Fuzz tests are supplemental; deterministic contract tests remain release blockers.

## 11. Future Live Adapter

V4 defines a narrow extension point for future live substrate attacks:

```go
type Environment interface {
    Name() string
}
```

The core runner does not import Cube packages. A future `gauntlet/livecube` adapter may construct scenarios that call `substrate.SandboxSubstrate` and real egress endpoints while emitting the same Probe events.

This keeps deterministic CI useful on ordinary GitHub runners and avoids coupling release evidence to Cube implementation details.

## 12. Required Tests

Tests must prove:

- vacuous scenario cannot pass;
- panic cannot pass;
- timeout cannot pass;
- missing required marker cannot pass;
- duplicate scenario ID fails suite construction/run;
- registration order does not change evidence digest;
- report mutation is detected;
- exact valid report verifies;
- one scenario failure rejects entire release;
- stale epoch, terminal world, action collision, traversal, CAS tamper, and journal tamper built-ins all survive their attacks;
- negative controls demonstrate the gauntlet actually detects a defense failure.

## 13. Release Output

The v4 CLI/library output is a JSON report produced by `MarshalReport` after `VerifyReport` succeeds. The report is suitable for CI artifact retention and future signed release attestations.

Signing/KMS is intentionally deferred. V4 establishes deterministic evidence bytes first so later signing does not certify unstable or ambiguous material.

## 14. Security Boundary

The gauntlet is not a source of authority. It receives no guest credentials, cloud tokens, or production secrets. Its output can veto release but cannot grant runtime authority.

The new invariant is:

> **Verification may deny promotion; verification may never mint authority.**
