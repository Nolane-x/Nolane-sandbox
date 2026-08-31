# Nolane Sandbox — Cube Exact CPU Readback Bridge v14 Design

Date: 2026-08-31
Base: `43bbbcf48d7539e0ca55139bef911da77f5e3a30`
Status: RED design candidate

## 1. Problem

Wave 13 bound host resource observations to an opaque Cube sandbox identity and corrected its consumer to the real CubeSandbox metric names and units. That bridge still cannot carry an exact CPU enforcement readback into the v11 resource-proof trust kernel.

The reason is not missing host data. Cubelet's host sampler already reads and retains the exact cgroup CPU limit tuple:

- `CPULimitQuotaUS`
- `CPULimitPeriodUS`
- `CPULimitUnlimited`

The loss occurs at the management export boundary. `/v1/metrics/resource` currently emits only `cubesandbox_host_sandbox_cpu_limit_cores`, computed as `quota / period`. A ratio is not an invertible representation of the original limit tuple: `25000/50000`, `50000/100000`, and `100000/200000` all serialize as `0.5` cores while representing distinct exact readbacks.

Therefore `cpu_limit_cores` is useful telemetry but cannot be laundered into the exact quota/period authority required by `resourceproof`.

## 2. Objective

Preserve the exact host cgroup CPU limit tuple across the real CubeSandbox producer/consumer boundary without widening trust or leaking host-private identifiers.

The bridge will export two additional host-scoped Prometheus gauges:

- `cubesandbox_host_sandbox_cpu_limit_quota_microseconds`
- `cubesandbox_host_sandbox_cpu_limit_period_microseconds`

Both are labelled only by `sandbox_id`, matching the existing host metric identity surface.

`NolaneWorld/substrate/cube.HostResourceObserver` will require and parse both values as exact positive integers and retain them in `HostResourceSnapshot` as:

- `CPULimitQuotaUS uint64`
- `CPULimitPeriodUS uint64`

The existing `CPULimitCores` remains observational compatibility data, but it is derived telemetry. It must agree with `quota / period`; disagreement is a fail-closed producer-contract violation.

## 3. Authority model

### Source authority

The source of exact CPU readback remains Cubelet's host-side cgroup reader. v14 does not reconstruct quota or period from cores and does not accept caller-supplied values.

### Identity authority

The consumer continues to select metrics using the opaque `ResourceBinding` minted from the concrete Cube `GuestSession` sandbox identity. A caller cannot rewrite the binding's private sandbox state.

### Trust boundary

`HostResourceSnapshot` remains observational host data. v14 does **not** mint `resourceproof.TrustedReport`, does not enable a runtime/CLI LIVE_PASS path, and does not claim broad `ResourceEnforcement` merely because exact CPU readback is now transportable.

The Cubelet management endpoint remains an operator-selected management trust root, not cryptographic attestation.

## 4. Producer contract

For a finite CPU limit, Cubelet must export all three related views for the same `sandbox_id`:

1. exact quota in microseconds;
2. exact period in microseconds;
3. derived cores ratio.

Quota and period are gauges because they represent the current configured limit, not cumulative counters.

For an unlimited CPU limit, Cubelet must omit the finite limit metrics, preserving the existing typed unlimited semantics. The Nolane observer therefore cannot accidentally interpret an unlimited sandbox as a finite bounded CPU proof.

No cgroup path, task handle, containerd handle, provider token, or other host-private locator is added to the metrics surface.

## 5. Consumer invariants

The observer must fail closed when any of the following is true:

- quota is missing;
- period is missing;
- either exact scalar is zero, fractional, negative, non-finite, duplicated, or outside `uint64` range;
- the metric belongs to another sandbox;
- multiple samples exist for the bound sandbox;
- the derived cores value materially disagrees with `quota / period`;
- the management response is malformed or unavailable.

The observer must preserve quota and period exactly as integers. Equal core ratios must never be used to infer an exact tuple.

## 6. Compatibility

Existing metric names remain present. v14 is additive at the producer surface and additive at `HostResourceSnapshot`.

The v13 truth statements remain valid:

- memory failures are not `OOMKilled`;
- guest exit code 137 is not host OOM provenance;
- current memory is cgroup charge, not a fabricated working-set estimate;
- disk is not proven;
- no broad resource-enforcement capability may be asserted from this observation alone.

## 7. OOM/task outcome is explicitly out of scope

CubeBox status contains `ExitCode`, `Reason`, and lifecycle timestamps, but v14 does not yet treat them as authoritative OOM evidence. Their write path and provenance must be traced independently before any `OOMKilled` claim can enter the resource-proof bridge.

That investigation is the next trust wave, not a reason to weaken v14 by coupling an unproven source into this change.

## 8. RED proof

RED must exist on both sides of the boundary before production implementation:

- Cubelet producer test requires exact quota/period metrics and fails because current Prometheus export only emits cores.
- NolaneWorld consumer tests require exact scalar preservation, reject cores-only responses, reject malformed exact scalars, and reject ratio mismatch.

The RED failure must be attributable to missing v14 production semantics, not provenance, formatting, runner, or fixture errors.

## 9. GREEN and closure gates

Before merge, the exact candidate SHA must have fresh success for every applicable gate, including:

- Cubelet Unit Test Check on amd64 and arm64;
- Nolane World unit tests;
- Nolane World race tests;
- `go vet`;
- Format Check;
- DCO / AI provenance;
- live gauntlet and docs gates when triggered.

Merge must be locked to the exact reviewed head SHA. After merge, push-to-master checks for both Cubelet and NolaneWorld must be re-verified before v14 is declared closed.

## 10. Completion criterion

v14 is complete only when a finite Cube sandbox CPU limit can travel from the real host cgroup sampler through Cubelet's management endpoint into an opaque-sandbox-bound Nolane observation with the exact quota and period preserved, while all old truth boundaries remain intact.
