# Wave 21 Trust-Closure Amendment

## Status and precedence

This amendment is normative for Wave 21 and takes precedence over conflicting wording in `2026-09-05-nolane-sandbox-guest-kernel-oom-victim-provenance-v21-design.md`.

The self-review of the approved Wave 21 design found four places where permissive wording could weaken fail-closed behavior. This amendment closes them before implementation planning.

## 1. Conflicting host-to-shim binds poison the pending realization

The base design says a second conflicting `BindOOMVictimRealization` can replace the pending token. That behavior is superseded.

For one unconsumed main-start slot:

```text
no pending token + valid T       -> pending T
pending T + exact duplicate T    -> pending T (idempotent)
pending T + different valid T2   -> POISONED
POISONED + any later bind        -> POISONED
```

A POISONED slot causes the next main Task Start to proceed **without** a Wave 21 token. The slot is then cleared. No token from that slot reaches the guest and no Wave 21 evidence can be accepted for that host generation.

Create/Delete/shim teardown clears pending and POISONED state. A consumed slot is also cleared regardless of Task Start success.

This prevents ambient or racing control-plane writes from selecting whichever token arrived last.

## 2. ttrpc request metadata is the only Wave 21 Stats selector

The base design allowed an unspecified equivalent fallback if ttrpc request metadata could not be preserved. That fallback is forbidden.

The only Wave 21 evidence selector on the standard Task Stats method is:

```text
request metadata key: cube-wave21-guest-oom-evidence
value: exact 64-character lowercase realization-token hex
```

Ordinary Stats without that exact metadata key must preserve existing workload metrics behavior.

CubeShim must read the selector from the request context metadata and return the Wave 21 dedicated `Any` only when the selector is canonical and matches an exact cached token for the requested task ID.

Cubelet must attach the selector through containerd/ttrpc context metadata for the internal evidence query. The Go ttrpc client protocol transports request metadata, and Rust ttrpc exposes request metadata through its request context. Wave 21 must exercise this exact path in deterministic cross-language contract tests.

If the repository-pinned client/server versions cannot demonstrate metadata round-trip on a supported target, Wave 21 evidence remains disabled/unknown on that target until that transport is fixed. Implementations may not switch to an unreviewed ID prefix, annotation response channel, Metrics collision, side socket, or tokenless query.

## 3. Collector loss poisons overlapping realizations

The base design says ring-buffer/collector loss makes affected evidence unknown but did not define how "affected" is determined. The deterministic rule is:

- the collector owns a monotonically increasing `loss_epoch`;
- every detected BPF reserve failure, ring-buffer lost-record notification, decoder framing loss, or raw-store overflow increments `loss_epoch`;
- each guest realization records `start_loss_epoch` when its guest-local window opens;
- at finalization it records the current `loss_epoch`;
- if the two values differ, the entire realization is evidence-unavailable and `GetOOMVictimEvidence` returns no positive Wave 21 records for it.

This is deliberately conservative. A loss that could have hidden a victim event prevents the realization from looking complete even when other positive records survived.

Finalized-realization eviction is not a collector loss; it simply makes the evicted realization unavailable.

## 4. Stopped-State host recovery cannot finalize guest evidence by inference

Wave 17 may accept an authoritative stopped Task State when Wait is not the source. Wave 21 does not infer that the guest-local realization has therefore been finalized.

The guest closes `outcome_observed_boot_ns` only from its authoritative main `WaitProcess` path for the exact token-bound container realization.

Therefore:

```text
host Wave17 outcome source == Wait
    + exact token survives
    + CubeShim has/fetches finalized guest evidence
    -> Wave21 finalization attempt may accept positive proof

host Wave17 outcome source == stopped State
    + guest finalized evidence already immutably cached for exact token
    -> Wave21 finalization attempt may accept positive proof

host Wave17 outcome source == stopped State
    + no already-finalized exact-token guest evidence
    -> Wave21 unknown for that generation
```

The controller still performs at most one Wave 21 finalization attempt after accepting the Wave 17 outcome. A later Status/Wait, later guest reconnect, or later cache population cannot repair an already terminal unknown result for that generation.

## Closed invariants after amendment

After this amendment, Wave 21 has no alternate authority path:

```text
Wave17 generation
  + exact host CSPRNG token
  + non-poisoned one-shot pre-Start bind
  + exact guest StartContainer token
  + exact guest main lifetime anchor
  + mark_victim kernel event
  + exact MAIN lifetime or MEMBER cgroup-v2 correlation
  + guest-local time window
  + unchanged collector loss_epoch
  + finalized exact-token guest evidence
  + exact ttrpc metadata selector
  + one terminal Cubelet finalization attempt
  + same-scrape NolaneWorld correlation
```

Any missing link yields unknown. Existing `GetOOMEvent`, containerd `TaskOOM`, exit 137, SIGKILL, cgroup counters, Wave 18, Wave 19, and Wave 20 remain independent evidence and cannot fill a missing Wave 21 link.