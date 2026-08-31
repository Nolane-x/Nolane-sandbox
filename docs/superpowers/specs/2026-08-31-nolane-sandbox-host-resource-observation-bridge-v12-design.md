# Nolane Sandbox Host Resource Observation Bridge v12 — Design

## Status
Implementation design for the v12 wave, revised after RED uncovered the v11 provenance boundary. v12 builds on Typed Resource Enforcement Proof v11 without changing v11 report semantics or schema version.

## Problem
v11 can verify typed CPU and memory enforcement only from a package-owned `TrustedReport`. The repository lacked the mechanical host observer that normalizes Linux cgroup readback/counters and causal pressure outcomes into the exact v11 scalar proof contract.

The first design placed those mechanics in a child package. RED review showed that this is unsafe as an authority boundary: a child package cannot construct the unexported state of `resourceproof.TrustedReport`, while exporting a constructor that accepts caller-provided observations/callbacks would reintroduce provenance laundering.

## Goal
Add a host-observation bridge that:

- normalizes real Linux cgroup v1/v2 CPU and memory state;
- requires causal throttling and OOM evidence around host-controlled pressure;
- preserves the package-owned trusted-report mint boundary;
- emits only the scalar v11 observation contract;
- never exposes host locators, task handles, provider handles, credentials, or write authority to the agent/runtime capability surface.

## Corrected authority architecture

### Semantic and authority ownership
`resourceproof` owns both verification semantics and the authority-bearing observer composition. The production observer implementation therefore lives in the parent `resourceproof` package.

Parsing helpers may be split later, but no child/sibling package is allowed to receive a public path that converts caller-controlled host observations into `TrustedReport`.

### Why there is no generic public live constructor in v12
The current Cube live API does not expose an opaque host realization handle that simultaneously binds:

1. the exact Realm/revision/policy/runtime realization;
2. the corresponding host cgroup;
3. host-controlled CPU/memory pressure execution; and
4. authoritative task termination reason (`OOMKilled`, not merely exit 137).

A CLI that accepts arbitrary `runtime_digest`, cgroup path, file source, or pressure callback would look live while allowing the caller to self-assert the provenance relation. v12 intentionally does not add such a surface.

The next host-integration wave must begin with an opaque Cube-host realization binding contract. Until that exists, missing integration remains `UNAVAILABLE`, never a fabricated live PASS.

## Host-only mechanical inputs
The package-owned observer composes:

- a private cgroup filesystem root/path;
- a read-only file source abstraction;
- a host-owned CPU/memory pressure runner;
- authoritative memory-task status;
- exact requested resource limits; and
- the already-defined v11 binding.

These abstractions are test seams/mechanical dependencies, not public provenance tokens. The constructor and trusted observation operation remain unexported.

## Cgroup dialects

### cgroup v2
- CPU limit: `cpu.max` (`quota period`), rejecting `max` for a bounded request;
- CPU throttling: `cpu.stat`, requiring a positive `nr_throttled` or `throttled_usec` delta;
- memory limit: `memory.max`, rejecting `max` for a bounded request;
- OOM counter: `memory.events`, preferring `oom_kill` and falling back to `oom` only as a counter; the task must still be authoritatively `OOMKilled/137`.

### cgroup v1
- CPU limit: `cpu.cfs_quota_us` + `cpu.cfs_period_us`, rejecting unlimited quota (`-1`);
- CPU throttling: `cpu.stat`; `throttled_time` nanoseconds are normalized to microseconds before entering v11;
- memory limit: `memory.limit_in_bytes`; Linux effectively-unlimited sentinel values are rejected for bounded proof;
- OOM/failure counter: `memory.failcnt`, which still cannot substitute for authoritative task OOM status.

## Causal sequence
For one exact realization:

1. validate mode, binding and requested bounded limits;
2. read exact effective CPU and memory limits;
3. snapshot CPU throttling and memory OOM counters;
4. invoke host-owned CPU pressure;
5. snapshot CPU throttling again;
6. invoke host-owned memory pressure and obtain authoritative task status;
7. snapshot memory OOM counter again;
8. build v11 scalar observations;
9. delegate semantic classification to the existing v11 package-owned verifier.

Readback mismatch is not rewritten as infrastructure failure: the observed effective value is carried into v11 so the semantic verifier returns the appropriate mismatch reason.

Any unavailable read, malformed/unlimited bounded value, dialect ambiguity, pressure failure, task-status uncertainty, invalid binding, or cancellation fails honest and cannot become PASS.

## Security invariants
1. **Readback before trust.** Requested limits alone are not evidence.
2. **Causal counters.** CPU pressure without a throttle delta is not CPU proof.
3. **Authoritative OOM.** Exit 137 without OOM status plus counter delta is not memory proof.
4. **No provenance laundering.** Copyable labels, plain reports, arbitrary paths, callbacks, or CLI flags cannot mint trusted provenance.
5. **Package-owned mint.** `buildTrustedReport`, observer construction and trusted observation remain inside `resourceproof`.
6. **No locator leakage.** Cgroup roots, task IDs/handles, tokens, endpoints and provider handles never enter canonical reports/evidence.
7. **No new agent authority.** Agent runtime receives only immutable capability evidence after trusted verification.
8. **No disk overclaim.** v12 does not prove disk enforcement.
9. **No aggregate overclaim.** CPU+memory proof alone cannot elevate broad `ResourceEnforcement` while disk remains unverified.
10. **UNAVAILABLE != PASS.** Missing live host integration remains unavailable.
11. **Historical nondrift.** v4–v11 canonical evidence semantics remain unchanged.

## RED contracts
The v12 RED suite proves:

- cgroup v2 bounded CPU/memory normalization;
- cgroup v1 bounded CPU/memory normalization, including ns→µs throttled-time conversion;
- malformed/unlimited bounded limits fail closed;
- pressure without throttle delta cannot verify CPU;
- voluntary exit 137 cannot verify memory;
- OOM counter without authoritative task OOM status cannot verify memory;
- exact readback mismatch is delegated to v11;
- observer/pressure errors are unavailable, never PASS;
- private host locator/sentinel strings do not appear in trusted serialization;
- existing v11 disk and aggregate non-overclaim semantics remain unchanged.

## Non-claims
v12 does not claim Cube-host realization binding, generic production CLI live proof, disk quota enforcement, network bandwidth enforcement, PID limits, universal cgroup correctness, or live KVM execution when the required opaque host authority is absent.
