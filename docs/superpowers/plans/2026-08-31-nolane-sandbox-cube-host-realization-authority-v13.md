# Nolane Sandbox v13 — Cube Host Realization Authority

## Goal

Close the identity gap left by v12 without laundering guest data or caller-supplied labels into host enforcement proof.

v12 deliberately left the Linux cgroup observer fail-closed because the Cube adapter had no concrete host-realization authority. CubeSandbox does expose a host-side Cubelet resource endpoint (`/v1/metrics/resource`) whose sampler reads runtime/cgroup state on the node. v13 binds that observation surface to the exact sandbox identity owned by a `GuestSession`, but does **not** upgrade the resulting snapshot into `resourceproof.TrustedReport`.

## Trust model

1. `GuestSession` owns the realized Cube sandbox ID in unexported state.
2. `ResourceBinding` copies that identity into an opaque adapter-owned value; its state is not caller-writable outside package `cube`.
3. `HostResourceObserver` reads only the configured Cubelet management endpoint and selects metrics whose `sandbox_id` exactly matches the binding.
4. Missing, duplicate, malformed, non-finite, negative, or mismatched samples fail closed.
5. The Cubelet endpoint is an **operator-selected management-plane trust root**, not cryptographic attestation. A process that deliberately configures a malicious endpoint can feed malicious observations, therefore this layer remains observational and does not mint resourceproof authority.
6. `cubesandbox_host_sandbox_memory_failures_total` is a host counter, but it is not equivalent to authoritative task status `OOMKilled`. v13 never creates that semantic upgrade.

## Implemented surface

- `cube.ResourceBinding`
  - minted from a concrete `GuestSession`;
  - exposes the bound sandbox ID read-only;
  - zero/nil bindings fail closed when observed.
- `cube.HostResourceObserver`
  - validates the management URL;
  - requests `/v1/metrics/resource` with bounded response size and context cancellation;
  - accepts only an exact sandbox match;
  - rejects duplicate required metrics;
  - requires the complete v13 metric set.
- `cube.HostResourceSnapshot`
  - CPU limit in cores;
  - CPU throttled periods and throttled microseconds;
  - memory limit and working set;
  - memory failure count;
  - capture time and bound sandbox ID;
  - deliberately no `OOMKilled`, exit-reason, cgroup-path, or trusted-proof field.

## Required metrics

- `cubesandbox_host_sandbox_cpu_limit`
- `cubesandbox_host_sandbox_cpu_throttled_periods_total`
- `cubesandbox_host_sandbox_cpu_throttled_useconds_total`
- `cubesandbox_host_sandbox_memory_limit`
- `cubesandbox_host_sandbox_memory_working_set_bytes`
- `cubesandbox_host_sandbox_memory_failures_total`

All required metrics must be present exactly once for the bound `sandbox_id`.

## Non-goals / deferred authority

v13 must not:

- call `resourceproof.buildTrustedReport` from Cubelet metric data;
- claim a memory OOM solely from `memory_failures_total` or guest exit code 137;
- claim exact CFS quota/period from a CPU-limit-in-cores gauge;
- treat an arbitrary HTTP endpoint as cryptographic provenance;
- enable the deferred CLI/runtime LIVE_PASS path.

The next elevation step requires a host-side task-status primitive (or equivalent runtime authority) that can bind an exact sandbox/workload termination to `OOMKilled`, plus an explicit operator trust root for the management endpoint. Only then may a later wave bridge this observation layer into `resourceproof.TrustedReport`.

## Verification contract

The v13 candidate is acceptable only when the repository's fresh exact-head checks are green, including formatting and the Go contract/gauntlet suites that exercise `NolaneWorld`. Tests added by v13 cover:

- binding to the exact guest sandbox ID;
- exact-sandbox metric selection;
- CPU and memory counter parsing;
- duplicate metric rejection;
- missing/mismatched sandbox rejection.

No merge is allowed while an applicable required check is failing or still unresolved.