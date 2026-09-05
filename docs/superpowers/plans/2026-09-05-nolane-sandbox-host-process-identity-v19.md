# Nolane Sandbox Wave 19 — Host Process Identity Execution Record

## Goal

Wave 19 establishes exact, PID-reuse-resistant identity for the CubeShim/VMM host process after trusted cgroup placement, then binds that identity to the controller-local task-realization generation already owned by Waves 16–18.

This is provenance only. Wave 19 does not identify a kernel OOM victim and does not claim that the guest application or main task was OOM-killed.

## Trust boundary

The accepted identity tuple is:

- boot ID from `/proc/sys/kernel/random/boot_id`;
- host PID from CubeBox's runtime endpoint;
- Linux `/proc/<pid>/stat` field 22 (`starttime_ticks`);
- exact cgroup path verified structurally from `/proc/<pid>/cgroup`;
- CubeBox placement timestamp and controller binding timestamp;
- controller-local realization generation.

The following remain forbidden:

- `exit 137 => OOM` inference;
- SIGKILL => OOM inference;
- positive Wave 18 OOM delta => victim inference;
- host PID equality => guest-task identity;
- suffix, substring or basename cgroup matching;
- recovery or persistence that reconstructs a lost host-process identity proof;
- public CubeAPI expansion for this evidence.

## TDD evidence

The dedicated Wave 19 contract was first run against tests with no production Wave 19 implementation. Clean RED run `33951426865` failed only on the intentionally absent Wave 19 APIs after the initial stock-runner CubeBox/native-header harness problem was removed.

The RED contract covered:

1. robust `/proc/<pid>/stat` field-22 parsing, including parenthesized commands;
2. stat-A → cgroup → stat-B PID-reuse sandwich;
3. exact v1/v2 cgroup membership matching;
4. Create/Start/outcome lifecycle fencing and stale-capture rejection;
5. `AddProc`-before-recorder ordering with observational recorder failure semantics;
6. exact Prometheus string-label transport for arbitrary `uint64` values;
7. strict NolaneWorld single-scrape correlation;
8. explicit shape guards against OOM-victim classification fields.

## Implemented producer path

### Package-neutral placement sequencer

`Cubelet/plugins/cube/internals/hostprocess` owns only the primitive sequencing helper and recorder interface. `AddProcAndRecord`:

- validates sandbox ID, cgroup path and nonzero PID;
- calls the supplied `AddProc` authority first;
- never invokes the recorder when `AddProc` fails;
- treats recorder/procfs evidence failure as observational-only after successful placement.

It does not own lifecycle state or construct realization bindings.

### Sandbox controller authority

The cube sandbox controller owns the `/proc` inspector and lifecycle store. Slow procfs reads happen outside the proof-store lock. A pointer lifetime token plus the active task generation is rechecked at commit time, so stale capture work cannot cross Create or a newer Start.

`BeginRealization` clears the old realization binding while retaining lifetime placement. Start revalidates the exact stored `(boot_id,pid,starttime_ticks,cgroup_path)` tuple before accepting a binding for the new generation. Once exact task outcome is accepted, late placement or revalidation cannot repair that closed generation.

### CubeBox integration

CubeBox resolves the existing cube sandbox-controller plugin dependency as the package-neutral host-process placement recorder. The real `cube_container_create.go` cgroup path now calls `hostprocess.AddProcAndRecord` with the exact sandbox ID, cgroup ID and CubeShim endpoint PID. The former direct `setCgroup -> cgroupp.AddProc` helper was removed, so this call site no longer has a parallel non-evidence placement path.

## Exact transport

Resource metrics remains transport-only. It consumes structural visitors and emits one atomic info sample:

```text
cubesandbox_host_process_identity_info{
  sandbox_id,
  generation,
  host_pid,
  starttime_ticks,
  boot_id,
  cgroup_path,
  runtime_role="cube-shim-vmm",
  source="cubebox.cgroup.add_proc",
  placed_at,
  bound_at
} 1
```

Generation, PID and starttime are decimal string labels. Timestamps are UTC RFC3339Nano. Invalid or incomplete evidence emits no sample. Existing public `NewService` remains unchanged.

## NolaneWorld consumer

`TaskTerminationObserver` parses task outcome, Wave 18 realization-OOM evidence and Wave 19 host-process identity from the same management scrape.

The identity sample is optional, but if present it is strict/fail-closed:

- exactly ten labels and numeric value exactly one;
- canonical unsigned decimal generation/PID/starttime;
- canonical lowercase boot UUID;
- canonical absolute cgroup path;
- exact runtime role/source constants;
- UTC RFC3339Nano timestamps with `placed_at <= bound_at`;
- exact target sandbox and task-outcome generation;
- exact cgroup match when Wave 18 OOM evidence is present;
- duplicate or malformed target samples fail closed.

The resulting `HostSandboxProcessIdentityProof` is provenance only. It adds no OOM victim/classification API.

## CI contract and regression gates

`.github/workflows/cube-host-process-identity-contract.yml` runs the pure-Go placement contract, sandbox controller contract, resource-metrics transport contract and NolaneWorld correlation contract. Direct CubeBox package integration is intentionally verified by the repository's native builder/unit/build matrix because stock `ubuntu-latest` cannot build that package without repository native dependencies such as CubeCow/CubeNet artifacts.

Before the PR is marked ready, the final head must pass:

- Wave 19 host-process identity contract;
- Wave 18 realization OOM contract;
- Wave 17 exact task outcome contract;
- host-resource contract;
- Cubelet unit matrix on amd64 and arm64;
- repository build matrix including Docker smoke builds;
- format and DCO checks;
- NolaneWorld unit/race/vet/evidence gates;
- live substrate harness semantics where available.

Infrastructure-only review-bot failures are reported separately from code/test failures.

## Deferred Wave 20

Wave 20 may establish authoritative kernel OOM-victim event capture with event-time process/cgroup identity and correlate that event to this Wave 19 host identity. Only that stronger evidence may justify a victim-level statement. Wave 19 by itself never does.

Autonomously-by: ChatGPT:GPT-5.6-Sol
