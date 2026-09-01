# Nolane Sandbox v15 — Exact Host OOM Provenance Implementation Plan

> Execute RED → GREEN. Keep v15 bounded to exact kernel OOM-kill evidence; task lifecycle outcome remains the next trust wave.

## Task 1 — RED: cgroup evidence contract

Add v1/v2 tests that require `MemoryOOMKillsKnown` plus exact `MemoryOOMKillsTotal`, including missing-source cases. Confirm current production code cannot satisfy the contract.

## Task 2 — GREEN: cgroup readers

Extend `handle.UsageSnapshot` with OOM presence and counter fields. Populate them only from direct cgroup v1 `MemoryOomControl.OomKill` or cgroup v2 `MemoryEvents.OomKill` evidence.

Focused verification:

```bash
go test ./plugins/cube/internals/cgroup/handle/v1 ./plugins/cube/internals/cgroup/handle/v2
```

(run from `Cubelet`)

## Task 3 — Assignment baseline and normalized host proof

Persist OOM evidence presence/counter in `HostMetricsBaseline`, capture it at cgroup assignment, include it in lazy baseline creation, and normalize it alongside other monotonic counters. Known-state drift and counter regression must fail closed.

Add focused sampler/baseline tests.

## Task 4 — Producer transport

Add `cubesandbox_host_sandbox_memory_oom_kills_total{sandbox_id=...}` to the resource metrics producer. Emit it only when the normalized OOM signal is known. Keep `memory_failures_total` unchanged and separate.

Add Prometheus positive and omission tests.

## Task 5 — NolaneWorld consumer

Extend `HostResourceSnapshot` with optional OOM-kill evidence without making it a task-level `OOMKilled` verdict. Parse the optional metric strictly; duplicate, fractional, non-finite, negative, or overflow values fail closed. Older producers without the metric remain valid and represent unknown OOM evidence.

Add v15 observer tests.

## Task 6 — Verification and closure

Run focused Cubelet and NolaneWorld tests, then repository CI. Inspect PR review surface and workflow results. Only mark the PR ready/merge when fresh evidence is green.

After merge, record exact head SHA and leave task-level outcome/OOM correlation for the next bounded wave.

Autonomously-by: ChatGPT:GPT-5.6-Sol
