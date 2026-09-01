# Nolane Sandbox v15 — Exact Host OOM Provenance

## Goal

Close the OOM evidence gap left by v14 without expanding the trust surface. v15 must transport a sandbox-assignment-scoped OOM-kill counter from the Linux cgroup ABI to NolaneWorld while preserving the distinction between direct evidence and heuristics.

## Non-goals

- Do not classify exit code 137 as OOM.
- Do not reinterpret `memory_failures_total` as OOM.
- Do not add a new public CubeAPI endpoint.
- Do not expose free-form termination messages.
- Do not yet claim a task-level `OOMKilled` boolean; the counter is evidence that a later outcome proof may bind to a specific termination.

## Evidence sources

- cgroup v1: `memory.oom_control` via containerd cgroup1 `MemoryOomControl.OomKill`.
- cgroup v2: `memory.events` via containerd cgroup2 `MemoryEvents.OomKill`.

Both sources are kernel-observed counters. If the relevant source object is missing, the sample is not evidence of zero OOM kills. Presence must therefore be explicit.

## Contract

Extend `handle.UsageSnapshot` with:

- `MemoryOOMKillsKnown bool`
- `MemoryOOMKillsTotal uint64`

The total is meaningful only when `Known` is true.

The assignment baseline persists the same presence bit and counter. Host normalization subtracts the persisted counter only when both baseline and current sample are known. If OOM evidence presence differs between the persisted baseline and the current sample, the OOM dimension alone becomes unknown while unrelated CPU/memory proof remains available. If both sides are known but the counter regresses, normalization fails closed because the supposedly continuous cgroup evidence epoch is inconsistent.

`HostSandboxSnapshot` carries the normalized evidence. Prometheus exports `cubesandbox_host_sandbox_memory_oom_kills_total` only when the value is known. Absence therefore means unknown/unavailable, not zero.

`NolaneWorld/substrate/cube.HostResourceSnapshot` gains an optional OOM-kill observation. The observer accepts the metric when present and rejects duplicates, fractional values, non-finite values, negative values, and integer samples outside the exact binary64 transport range. Prometheus scalar samples are transported as `float64`, so an integer observation can only be called exact when it is at most `2^53 - 1`; larger decimal integers may round before semantic validation and therefore fail closed. This same exact-integer parser protects CPU quota/period, counters, and byte-valued host observations that claim exactness. The OOM metric remains optional so older Cubelet producers remain compatible; absence remains unknown.

## Trust invariants

1. `MemoryFailures` and `MemoryOOMKills` remain separate facts.
2. No exit-code heuristic may synthesize OOM evidence.
3. No missing metric may synthesize zero/false.
4. Counter normalization is bound to the persisted cgroup assignment baseline.
5. Missing OOM continuity fails closed only for the OOM dimension; it must not erase unrelated valid host evidence.
6. A known OOM counter regression fails closed as a continuity violation.
7. An integer transported through Prometheus may be called exact only inside the binary64 safe-integer range `0..2^53-1`.
8. Producer and consumer tests include positive evidence, missing evidence, regression, duplicate, fractional, safe-range, and compatibility cases.

## Follow-up boundary

After v15 closes, the next bounded trust wave may bind CubeBox/containerd lifecycle fields (`ExitCode`, `Reason`, timestamps) to this kernel OOM evidence to construct task-outcome proof. That wave must keep synthetic NotFound timestamps and exit-137 heuristics out of the proof path.

Autonomously-by: ChatGPT:GPT-5.6-Sol
