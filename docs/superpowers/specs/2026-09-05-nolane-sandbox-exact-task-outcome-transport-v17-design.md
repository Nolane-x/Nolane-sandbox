# Nolane Sandbox Wave 17 — Exact Task Outcome Transport

## Purpose

Wave 16 created an exact, fail-closed task-outcome proof plane inside the Cube sandbox controller, but deliberately stopped at the controller boundary. NolaneWorld therefore could not consume exact runtime exit evidence without reconstructing it from weaker operational state.

Wave 17 closes that transport gap without changing the authority of the underlying proof and without introducing OOM attribution.

The narrow claim is:

> If NolaneWorld accepts a Wave 17 task-outcome observation, every accepted field is an exact transport of one currently accepted Wave 16 `TaskOutcomeProof` for the same sandbox binding.

## Trust boundary

Wave 17 does not create task-outcome truth. The only truth producer remains Wave 16:

- successful containerd task `WaitResponse`; or
- exact `STOPPED` containerd task `StateResponse` with a valid runtime exit timestamp.

Wave 17 may only enumerate and serialize proofs already accepted by that plane. It must not call operational `Status`, infer an exit from CubeBox state, manufacture timestamps, reinterpret exit code `137`, or recover a proof on behalf of a consumer.

The transport path is host-side only:

```text
containerd task runtime
        |
        v
Wave 16 TaskOutcomeProof store
        |
        v
controller-local exact proof snapshot
        |
        v
package-neutral proof visitor
        |
        v
Cubelet resource-metrics management surface
        |
        v
NolaneWorld TaskOutcomeObserver
        |
        v
exact ResourceBinding-scoped observation
```

## Producer contract

### Deterministic proof enumeration

The concrete Cube sandbox controller adds a controller-local `TaskOutcomeProofLister` interface:

```go
type TaskOutcomeProofLister interface {
    ListTaskOutcomeProofs() []TaskOutcomeProof
}
```

The returned slice is a detached snapshot, sorted by `SandboxID` and then `Generation`. It contains only currently accepted proofs. `Create`, `Start`, proof clearing, and realization fencing retain Wave 16 semantics; Wave 17 does not persist or resurrect cleared proofs.

Enumeration is intentionally separate from resource sampler availability. A terminal proof must not disappear merely because host or guest resource sampling is stale, disabled, or unavailable.

### Package-neutral transport bridge

The resource-metrics package must not import the sandbox package. This is not merely a style preference: the sandbox package already has integration tests that import resource-metrics, so a reverse production dependency creates an import cycle under Go's package graph.

The concrete controller therefore exposes the exact accepted snapshot through a structural visitor whose signature contains only primitive and standard-library types:

```go
func (c *controllerLocal) VisitTaskOutcomeProofs(
    visit func(
        sandboxID string,
        generation uint64,
        exitCode uint32,
        exitedAt time.Time,
        source string,
    ),
)
```

The bridge is transport-only. It does not perform runtime lookup, operational reconciliation, proof recovery, or classification. It visits the already-accepted Wave 16 proofs in the deterministic lister order.

The resource-metrics package consumes this method through a local structural interface. This preserves a one-way dependency graph while retaining compile-time method-shape compatibility at the concrete controller boundary.

### Atomic exact info metric

Each accepted proof is exported as exactly one Prometheus info-style sample:

```text
cubesandbox_task_outcome_info{
  sandbox_id="sandbox-123",
  generation="18446744073709551615",
  source="containerd.task.wait",
  exit_code="137",
  exited_at="2026-09-05T04:05:06.123456789Z"
} 1
```

All proof payload fields are labels, not floating-point sample values:

- `sandbox_id` — exact Wave 16 sandbox identity;
- `generation` — canonical base-10 unsigned integer string;
- `source` — exact Wave 16 source enum;
- `exit_code` — canonical base-10 `uint32` string;
- `exited_at` — canonical UTC `RFC3339Nano` string.

The metric value is exactly `1` and carries no additional semantics.

This representation is deliberate. Prometheus samples are binary64, so carrying an arbitrary `uint64` generation as a sample value could round values above `2^53-1`. String labels preserve the full Wave 16 generation and nanosecond timestamp exactly.

The producer emits no sample when no proof exists. Absence means unknown, not exit code zero and not success.

Invalid data presented to the exporter is not promoted into a metric. The real Cube controller is expected to provide only Wave 16-accepted proofs; exporter validation is a final defensive boundary, not an alternate proof authority.

## Consumer contract

NolaneWorld adds a dedicated `TaskOutcomeObserver`. It reuses the existing Cubelet management metrics endpoint and the opaque `ResourceBinding` minted from a concrete `GuestSession`.

The observer accepts a task-outcome sample only when all of the following are true:

1. the sample is `cubesandbox_task_outcome_info`;
2. `sandbox_id` exactly equals the bound sandbox ID;
3. exactly the five required labels are present and no additional labels are accepted;
4. the metric value numerically equals exactly `1` and is finite;
5. `generation` is canonical base-10, parses as a nonzero `uint64`, and round-trips textually;
6. `exit_code` is canonical base-10 and parses exactly as `uint32`;
7. `source` is exactly `containerd.task.wait` or `containerd.task.state`;
8. `exited_at` parses as `RFC3339Nano`, is nonzero, and is already canonical UTC text;
9. exactly one matching sample exists.

A missing matching sample returns `(zero, false, nil)`: proof is unknown.

Malformed, partial, duplicate, conflicting, or over-wide matching samples fail closed with `ErrTaskOutcomeUnavailable`. Samples for other sandboxes are ignored.

The observer reads at most 1 MiB from the management response and does not fall back to operational status or host resource counters when exact task-outcome transport is missing.

## Independence from resource metrics

Wave 17 reuses the existing management endpoint and Prometheus encoder, but task outcome is not a resource metric semantically.

The management handler therefore has two independent inputs:

- the existing `SandboxResourceCache` for sampled resource data;
- the controller's package-neutral exact-proof visitor for terminal task proof.

A nil/empty resource cache must not suppress a valid task-outcome proof. Conversely, a healthy resource sample must never manufacture a task-outcome sample.

## Exactness and lifecycle semantics

Wave 17 preserves Wave 16 lifecycle semantics exactly:

- only the currently accepted proof for each sandbox is exported;
- a new `Start` removes the prior realization proof until a fresh authoritative outcome exists;
- a `Create` fence prevents late prior-realization evidence from reappearing;
- controller restart still loses in-memory proof until fresh runtime authority reconstructs it;
- identical proof observation is stable;
- conflicting runtime outcomes remain rejected before transport.

The transport generation is controller-local evidence, not a globally durable task identity.

## OOM boundary

Wave 17 does **not** set or infer `OOMKilled`.

Exit code `137` remains exact numeric task evidence only. Wave 15's `MemoryOOMKills` remains independent kernel evidence.

A direct correlation such as:

```text
exit_code == 137 && memory_oom_kills_total > 0 => OOMKilled
```

is forbidden.

The Wave 15 OOM counter is currently scoped to the sandbox/cgroup assignment baseline, while Wave 16 task outcome is scoped to a controller-local task realization. Multiple task realizations may occur inside one sandbox assignment, so a positive assignment-scoped OOM counter could belong to an earlier realization.

A later correlation wave must first establish a realization-scoped OOM baseline or another authoritative realization binding. Until then, exact outcome and exact OOM evidence remain separate facts.

## Failure semantics

Wave 17 is fail-closed and dimension-local:

- no proof -> unknown;
- invalid binding -> error;
- management endpoint failure -> unavailable;
- malformed target metric -> unavailable;
- duplicate target metric -> unavailable;
- invalid generation/exit/source/time -> unavailable;
- unrelated resource evidence remains independently usable;
- no consumer may synthesize task outcome from missing Wave 17 evidence.

## Verification contract

Wave 17 upgrades the existing `Cube Task Outcome Contract` into a permanent cross-module gate. It triggers when the sandbox proof plane, resource-metrics transport, NolaneWorld Cube consumer, Wave 17 spec/plan, or the gate itself changes.

The focused contract runs:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox ./plugins/cube/internals/resourcemetrics \
  -run 'TaskOutcome|OutcomeCandidate' -count=1

cd ../NolaneWorld
go test ./substrate/cube -run 'TaskOutcome' -count=1
```

Broader repository gates remain required as fresh evidence on the final candidate, including Cubelet/CubeCow unit coverage, NolaneWorld unit/race/vet/evidence generation, build, formatting, DCO metadata validation, and other path-triggered trust checks.

Verification must demonstrate:

1. proof enumeration is deterministic, detached, and excludes cleared proofs;
2. transport works even when the resource cache is absent;
3. arbitrary `uint64` generation values round-trip exactly, including `math.MaxUint64` and values above binary64's safe integer range;
4. exit code `137` round-trips without OOM interpretation;
5. nanosecond exit timestamps round-trip exactly;
6. no proof emits no task-outcome metric;
7. NolaneWorld accepts one complete exact sample for the bound sandbox;
8. missing proof stays unknown;
9. wrong-sandbox samples are ignored;
10. duplicate, partial, malformed, unsupported-source, invalid-time, and non-unit target samples fail closed;
11. the sandbox-to-resource-metrics package graph remains cycle-free;
12. existing Wave 15 host-resource observation remains compatible;
13. repository unit, race, vet, formatting, build, and trust gates remain green on the final candidate.

## Non-goals

Wave 17 does not:

- persist Wave 16 task proofs;
- create a new public CubeAPI endpoint;
- expose a textual termination reason;
- classify OOM;
- correlate Wave 15 OOM evidence to a task realization;
- alter CubeBox operational lifecycle semantics;
- widen guest authority or expose management credentials to the guest.

Those remain separately reviewable trust changes.

## Next trust closure

The next safe correlation step is not `137 => OOM`. A future wave should bind OOM evidence to the same task realization as the accepted task-outcome generation, for example by establishing a realization-scoped OOM baseline or an equally authoritative runtime/cgroup realization binding. Only after that proof exists should NolaneWorld expose an attributed OOM outcome.

Autonomously-by: ChatGPT:GPT-5.6-Sol
