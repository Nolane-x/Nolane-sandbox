# Nolane Sandbox Wave 16 — Exact Task Outcome Proof

## Purpose

Wave 16 closes the lifecycle-truth gap left after Wave 15. CubeSandbox currently exposes operational lifecycle state that may be synthesized for reconciliation convenience. In particular, the Cube sandbox controller can manufacture `ExitedAt: time.Now()` when the shim is missing, and the pre-Wave-16 `Wait` path manufactured both an exit timestamp and exit status in that path. Those values can be useful operationally but are not runtime evidence and must never be promoted into a truth/proof surface.

Wave 16 therefore introduces a proof plane that is separate from operational `Status`.

## Trust boundary

Runtime authority outranks narrative and reconciliation state.

A task outcome is proven only when it is derived from one of these successful runtime responses for the same sandbox realization:

1. `task.WaitResponse` returned successfully by the containerd shim task service.
2. `task.StateResponse` whose runtime status is `STOPPED` and that carries a valid runtime exit timestamp.

The following are explicitly **not** proof:

- shim/task `NotFound`;
- `time.Now()` synthesized by CubeSandbox;
- a failed `Wait` response, even if the RPC implementation also returned a partial response;
- a task-service resolver error;
- a non-stopped `State` response;
- a stopped response with no valid runtime exit timestamp;
- CubeBox `Status.FinishedAt` by itself;
- exit code `137` interpreted as OOM;
- Wave 15 `memory_failures_total` or any other heuristic interpreted as OOM.

## Architecture

### Separate proof registry

The Cube sandbox controller owns an in-memory, concurrency-safe task-outcome proof registry. It is intentionally separate from `cubebox.Status` and from containerd's operational `sandbox.ControllerStatus`.

Each exact proof contains:

- `SandboxID` — the sandbox/task identity presented to the authoritative runtime call;
- `Generation` — controller-local realization generation used to prevent stale proof reuse after a new start;
- `ExitCode uint32` — the exact runtime exit status, without semantic reinterpretation;
- `ExitedAt time.Time` — the exact runtime timestamp supplied by the shim;
- `Source` — either `containerd.task.wait` or `containerd.task.state`.

The registry is intentionally not persisted. Persisting terminal proof without a durable runtime realization identity could allow an old task's exit to survive into a new task with the same sandbox ID. After a controller restart, proof is unknown until a fresh authoritative `STOPPED` state or successful `Wait` is observed.

### Realization fencing and restart recovery

`Start` begins a new controller-local realization generation and clears any prior proof for the sandbox.

`Create` is stronger than a simple proof clear: it clears both the accepted proof and active local generation, then places a recovery fence. The fence prevents a late response from the previous runtime realization from being adopted as proof in the interval between `Create` and `Start`. `Start` removes that fence by beginning the new realization.

A controller-local store that is genuinely fresh after process restart has no `Create` fence. In that case, a fresh authoritative successful `Wait` or exact `STOPPED State` observation may recover generation `1` and bind the observed outcome. This is reconstruction from current runtime authority, not persistence of prior proof.

Within one generation:

- the first valid outcome becomes immutable evidence;
- an identical repeated outcome is idempotent;
- a conflicting `ExitCode` or `ExitedAt` is rejected as a provenance conflict rather than rebound.

A source change alone does not rewrite an already accepted outcome. For example, an exact `State` observation followed by an identical exact `Wait` observation remains the same outcome; Wave 16 does not manufacture a stronger provenance narrative by mutation.

### Producer behavior

`Wait` is hardened because its return value itself represents a terminal outcome:

- shim/task `NotFound` returns an error and zero `sandbox.ExitStatus` instead of fabricated `ExitStatus=1` / `ExitedAt=now`;
- any task-service resolver error returns an error and zero outcome;
- any failed task `Wait` returns an error and discards any partial response;
- a successful response must carry a valid runtime `ExitedAt` before it can be returned as an exact terminal outcome and recorded as proof;
- exact exit code `137` remains `137`; no OOM interpretation is attached.

`Status` remains an operational compatibility surface:

- its existing NotFound reconciliation behavior may synthesize `ExitedAt: time.Now()`, but that path never writes the proof registry;
- non-stopped runtime state never produces terminal proof;
- `STOPPED` state without a valid runtime timestamp remains usable operationally but stays unproven;
- exact `STOPPED` runtime state may reconstruct or confirm proof;
- an exact `STOPPED` observation that conflicts with an already bound outcome fails closed instead of silently rebinding proof;
- during the `Create`→`Start` fence interval, operational `Status` may still be returned, but exact-outcome recovery remains blocked.

### Runtime service seam

Wave 16 narrows the controller's internal dependency boundary to the task RPCs needed by this proof path (`State`, `Wait`, and existing `Stats`) and an endpoint resolver. Production resolution still uses the real shim/ttrpc task client. The seam exists so public `Wait` and `Status` behavior can be tested without fabricating proof through helper-only tests.

### Proof access

The concrete sandbox controller implements an optional `TaskOutcomeProofProvider` interface. Downstream components must explicitly opt into that proof interface; they must not reconstruct proof from operational status fields.

This is a deliberate type-level boundary between "what the system currently reports for operation" and "what the runtime actually proved."

## Reason semantics

The containerd task `Wait` / `State` responses available at this boundary provide exit status and exit time, but not an authoritative textual termination reason. Wave 16 therefore does **not** invent a `Reason` and does not treat an empty string as a proven reason.

A later transport/correlation wave may add reason evidence only if an authoritative producer exists and its realization can be bound to the same proof.

## OOM semantics

Wave 16 does not classify OOM.

`ExitCode == 137` is preserved exactly as the numeric code `137`; it is not promoted to `OOMKilled=true` and is not correlated to Wave 15 automatically. Any future OOM attribution must prove that the Wave 15 kernel OOM-kill counter transition and this task outcome belong to the same sandbox realization and compatible evidence interval.

## Failure semantics

Proof is dimensionally fail-closed:

- missing proof means unknown, not successful exit and not zero exit code;
- invalid runtime timestamps produce no proof;
- resolver and Wait errors expose no partial proof-like terminal outcome;
- conflicting terminal observations are errors;
- synthetic operational timestamps cannot populate proof;
- controller restart discards in-memory proof rather than silently persisting stale authority;
- fresh runtime authority may reconstruct proof after restart only when no Create fence forbids adoption.

## Verification contract

Wave 16 adds a permanent path-scoped GitHub Actions gate, `Cube Task Outcome Contract`, because the repository's ordinary `cubelet-pkg-test` target runs only `go test -short ./pkg/...` and does not execute `Cubelet/plugins/cube/internals/sandbox`.

The contract runs:

```bash
cd Cubelet
go test ./plugins/cube/internals/sandbox -run 'TaskOutcome|OutcomeCandidate' -count=1
```

The permanent gate prevents the proof tests from becoming historical-only evidence.

Wave 16 verification demonstrates:

1. fresh registry lookup is unknown;
2. exact exit code and runtime timestamp are preserved, including exit code `137` without OOM classification;
3. identical repeated proof is idempotent;
4. conflicting terminal outcome is rejected;
5. new realization invalidates previous proof;
6. `Create` clears authority and fences restart-style recovery until `Start`;
7. fresh controller-local state may recover from fresh authoritative runtime evidence after restart;
8. successful `WaitResponse` with valid runtime time becomes proof;
9. NotFound, resolver errors, failed Wait, nil/invalid response, and missing timestamp cannot become proof or successful terminal outcome;
10. only exact `STOPPED` `StateResponse` can become terminal proof;
11. operational NotFound and incomplete STOPPED status remain separate from proof;
12. exact State/Wait conflicts fail closed;
13. package contract, repository unit tests, formatting and build gates must remain green on the final candidate.

## Non-goals

Wave 16 does not:

- redesign CubeBox lifecycle state;
- persist outcome proof across controller restart;
- add OOM heuristics;
- infer a textual termination reason;
- transport task outcome into NolaneWorld yet;
- correlate task outcome with Wave 15 OOM evidence yet.

Those are separate proof/transport questions and must not expand this closure wave.
