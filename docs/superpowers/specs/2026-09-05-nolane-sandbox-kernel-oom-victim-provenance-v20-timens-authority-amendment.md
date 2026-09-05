# Wave 20 Time-Namespace Authority Amendment

## Status

This amendment is normative for Wave 20 and supersedes any statement in the original Wave 20 design or implementation plan that treats `/proc/self/timens_offsets` as automatically describing the Cubelet process's current time namespace.

The Wave 20 trust claim is unchanged: Linux victim marking may be correlated to Wave 19 process identity only when the `/proc/<pid>/stat` start-time bridge can be reproduced exactly and without guessing.

## Kernel fact corrected

Linux exposes two distinct time-namespace references in `nsproxy`:

```text
time_ns

time_ns_for_children
```

The proc implementation that renders `timens_offsets` obtains the namespace through `timens_for_children_get()`, which reads `nsproxy->time_ns_for_children`.

By contrast, `/proc/<pid>/stat` start-time rendering applies the reader's current time namespace through `current->nsproxy->time_ns` before converting `task->start_boottime` to clock ticks.

Those namespaces normally match, but they can differ after operations such as `unshare(CLONE_NEWTIME)` before the process enters or propagates the new namespace. Therefore reading `/proc/self/timens_offsets` without proving namespace equality is not sufficient authority for Wave 20.

## Correct authority rule

When `/proc/self/timens_offsets` exists, the collector must also read:

```text
/proc/self/ns/time
/proc/self/ns/time_for_children
```

The two namespace handles must:

1. both be readable;
2. both have canonical `time:[inode]` form;
3. identify the exact same namespace.

Only then may the `boottime` row from `/proc/self/timens_offsets` be used to convert kernel `task->start_boottime` into the Wave 19 `/proc/<pid>/stat` `starttime_ticks` domain.

If the current and for-children namespace handles differ, Wave 20 process-lifetime correlation is unavailable. It must not assume zero offset, use the for-children offset as a substitute, or fall back to PID-only correlation.

If `timens_offsets` is absent, zero offset is allowed only when both current and for-children time-namespace handles are also absent in a way that demonstrates the running kernel does not expose time namespaces. Partial availability is capability ambiguity and therefore Wave 20 unknown.

## Failure semantics

Time-namespace authority failure is observational only:

```text
collector unavailable -> no Wave 20 victim proof -> (false, false)
```

It must never fail Cubelet initialization, sandbox `Start`, authoritative `Wait`, or stopped `State` processing.

## Verification contract

The deterministic Wave 20 suite must prove:

- equal canonical `time:[inode]` handles permit offset authority;
- different handles reject offset authority;
- empty or malformed handles reject authority;
- no rejected namespace state can synthesize victim proof;
- all pre-existing exact start-time arithmetic, overflow, amd64/arm64 and workload-noninterference tests remain green.

This amendment narrows evidence availability in an unusual namespace transition state, but strengthens correctness: Wave 20 prefers unknown evidence over a potentially wrong process-lifetime match.
