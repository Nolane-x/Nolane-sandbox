# Nolane Sandbox Host Resource Observation Bridge v12 — Design

## Status
Approved implementation design for the v12 wave. This wave builds on Typed Resource Enforcement Proof v11 without changing v11 proof semantics.

## Problem
v11 can verify typed CPU and memory enforcement only when it receives trusted host observations. The repository still lacks a production host observer that obtains those observations from the Linux cgroup substrate. Without that bridge, hosted fixtures remain `UNAVAILABLE` and live CPU/memory claims cannot be minted from the real host.

## Goal
Add a host-only observer that converts real cgroup readback/counters plus host-controlled pressure/task status into the exact scalar `resourceproof.Observation` contract used by v11.

The bridge MUST NOT expose cgroup paths, task handles, provider handles, secrets, or write authority to the agent/runtime capability surface.

## Architecture

### Dependency direction
`resourceproof` remains the semantic verifier. A new child package `resourceproof/hostobserver` owns Linux/cgroup observation mechanics and depends only on the Go standard library plus the parent proof contract.

It MUST NOT import the Cubelet module or its large dependency graph. Cubelet/live-host integration supplies only host-owned callbacks/roots to the observer.

### Host-only inputs
The observer receives:

- a private cgroup filesystem root/path;
- a read-only file source abstraction;
- a host-owned pressure runner for CPU and memory probes;
- a host-owned task-status source that distinguishes authoritative OOM termination from voluntary exit 137;
- the exact v11 `RunSpec` / requested limits.

None of these host locators or handles are copied into canonical proof documents.

### Cgroup dialects
Support Linux cgroup v2 and v1 normalization.

For v2:

- CPU limit: `cpu.max` (`quota period`);
- CPU throttling: `cpu.stat`, requiring a positive `nr_throttled` or `throttled_usec` delta;
- memory limit: `memory.max`;
- OOM counter: `memory.events` (`oom_kill` preferred; `oom` may be recorded separately but cannot substitute for an authoritative kill when the task did not terminate as OOMKilled).

For v1:

- CPU limit: `cpu.cfs_quota_us` + `cpu.cfs_period_us`;
- CPU throttling: `cpu.stat`, requiring a positive `nr_throttled` / throttled-time delta;
- memory limit: `memory.limit_in_bytes`;
- OOM/failure observation: normalized from cgroup memory counters plus authoritative task status. A counter alone cannot manufacture `OOMKilled`.

### Causal sequence
For one exact realization:

1. read exact CPU and memory limits;
2. snapshot CPU throttling and memory OOM counters;
3. invoke host-owned CPU pressure;
4. snapshot CPU throttling again;
5. invoke host-owned memory pressure;
6. obtain authoritative task exit status;
7. snapshot memory OOM counter again;
8. emit only typed scalar observations to v11.

Any unavailable read, dialect ambiguity, stale realization, pressure uncertainty, parse error, or task-status uncertainty fails honest: no live PASS is manufactured.

## Security invariants

1. **Readback before trust.** Requested limits alone are not evidence.
2. **Causal counters.** CPU pressure without a throttle delta is not CPU proof.
3. **Authoritative OOM.** Exit 137 without host OOM status + counter delta is not memory proof.
4. **No provenance laundering.** Fixture/synthetic observations cannot be labelled live by copying a string.
5. **No locator leakage.** Cgroup roots, task IDs/handles, tokens, endpoints and provider handles never enter canonical reports/evidence.
6. **No new agent authority.** Agent-facing runtime receives only immutable capability evidence produced after v11 verification.
7. **No disk overclaim.** v12 does not prove disk enforcement.
8. **No aggregate overclaim.** CPU+memory proof alone cannot elevate broad `ResourceEnforcement` while the Realm budget still includes disk.
9. **UNAVAILABLE != PASS.** Missing live cgroup/task infrastructure stays unavailable.
10. **Historical nondrift.** v4–v11 canonical evidence semantics remain unchanged.

## RED contracts

The first implementation tests must prove:

- v2 CPU/memory files normalize correctly;
- v1 CPU/memory files normalize correctly;
- malformed or unlimited limits fail closed for bounded-proof requests;
- pressure without throttle delta cannot verify CPU;
- voluntary exit 137 without authoritative OOM cannot verify memory;
- OOM counter without task OOM status cannot verify memory;
- exact readback mismatch is carried to v11 and fails there;
- observer errors produce `UNAVAILABLE`, never PASS;
- host locator/secret sentinel strings cannot appear in marshalled reports/evidence;
- CPU+memory proof leaves Disk and broad ResourceEnforcement non-verified.

## Non-claims

v12 does not prove disk quota enforcement, network bandwidth enforcement, PID limits, universal cgroup correctness, or live KVM execution when the required host runner is absent.
