# Wave 31 — Realm Runtime CPU Policy Authority

Date: 2026-09-09
Base: Wave30 exact-final `3657256ea40528bf673a8ca4d51c69233d161b45`
Branch: `gpt/wave31-realm-physical-cpu-policy-authority`
Status: approved direction in chat; written-spec review pending; implementation not started

## 1. Purpose

Wave30 proves a narrow runtime fact: a package-owned CPU intervention executed in the exact runtime cgroup and was associated with fresh cgroup-v2 CFS throttling evidence while the exact helper demonstrably consumed CPU. Wave30 intentionally does **not** prove that the effective cgroup CPU limit matches a Realm-requested kernel policy.

The current Realm `ResourceBudget.CPUUnits` cannot close that gap. It is an **accounting budget**. The NolaneWorld contract explicitly states that accounting budgets are never presented as proof of kernel enforcement. Treating `CPUUnits` as cores, milliCPU, quota, shares, or any other kernel control would therefore launder an accounting number into physical enforcement policy.

Wave31 introduces a separate, explicit host-owned runtime CPU bandwidth limit intent and one opaque authority over the current Realm policy that contains it.

The Wave31 claim is intentionally narrow:

> **The current package-owned Realm state, at one exact Realm revision and canonical Realm policy digest, explicitly requests one exact runtime CPU bandwidth limit in milliCPU.**

Wave31 does not propagate that limit to Cube/Cubelet, does not prove `cpu.max` equivalence, does not invoke Wave30, and does not upgrade any trusted resource-enforcement verdict.

## 2. Terminology: runtime CPU limit, not physical-core affinity

The design direction discussed in chat was a separate “physical CPU policy.” The production name is deliberately more precise: **runtime CPU limit**.

The new value represents CPU-time bandwidth, expressed in milliCPU:

- `1000` milliCPU = one CPU-equivalent of scheduler bandwidth;
- `500` milliCPU = one-half CPU-equivalent;
- `2500` milliCPU = two-and-a-half CPU-equivalents.

It does **not** mean:

- a count of physical cores;
- CPU affinity or cpuset placement;
- an exclusive-core reservation;
- a scheduling weight/share;
- a minimum reservation or performance guarantee;
- a topology/NUMA claim.

Future cgroup-v2 equivalence must compare the exact rational bandwidth represented by `cpu.max` with this milliCPU intent. It must not infer physical-core pinning from the value.

## 3. Chosen architecture

Three approaches were considered.

### A. Reinterpret `ResourceBudget.CPUUnits` as kernel CPU quantity

Rejected.

This would contradict the established v8 contract that `ResourceBudget` is accounting-only. There is no existing semantic definition that says one `CPUUnit` equals one core, one milliCPU, one CFS quota quantum, or one share. Any such mapping would be invented after the fact and could create a false enforcement claim.

### B. Add an optional explicit runtime CPU limit to `realm.Spec` and mint an opaque current-policy authority

Selected.

The Realm Spec is already host-owned policy state, its exact revision is durable, and `PolicyDigest(spec, revision)` canonically binds the current policy. Adding one optional scalar with explicit units keeps the policy in the correct authority domain and gives later waves a pre-create authority they can propagate through the realization path.

A sealed Realm-level authority is preferred over a world-level authority because Wave32 must consume the policy **before** the world has been created. After realization, later waves can bridge the Realm policy authority to `RealizationAuthority` by requiring exact Realm ID, Realm revision, and policy digest equality.

### C. Create a second sidecar CPU-policy store outside `realm.Spec`

Rejected for Wave31.

A second mutable policy store would create two revision domains and a split-brain problem: Realm revision/policy could move independently from CPU policy. It would also require a new persistence journal, recovery rules, update transaction, and cross-store atomicity contract. None of that is needed for one host-owned optional policy field.

## 4. Realm Spec extension

Wave31 adds exactly one field to `realm.Spec`:

```go
RuntimeCPULimitMilliCPU uint64 `json:"runtime_cpu_limit_millicpu,omitempty"`
```

Semantics:

- `0` means **no runtime CPU kernel-limit intent is configured**;
- any positive `uint64` is a syntactically valid Realm intent;
- positive values are exact milliCPU bandwidth intents as defined above;
- zero must never be inferred from, defaulted from, or substituted with `ResourceBudget.CPUUnits`.

Wave31 deliberately imposes no arbitrary product maximum. The authority certifies current host policy intent, not host/provider support. A later propagation/enforcement wave may honestly return unsupported/unavailable when a positive value cannot be represented or enforced by a concrete substrate.

`Spec.Validate()` continues to require the existing Realm identity, world-count, lease, network-profile, and accounting-budget constraints. It does not require `RuntimeCPULimitMilliCPU > 0`, because legacy Realms without a kernel-limit intent remain valid.

## 5. Backward compatibility and digest continuity

The new JSON field is a scalar with `omitempty` and is appended to the existing `Spec` schema.

For every pre-Wave31 Realm whose new field is zero:

1. canonical `encoding/json` output for `Spec` remains byte-identical;
2. `PolicyDigest(spec, revision)` remains byte-identical to its pre-Wave31 value;
3. existing durable Realm records that omit the field decode with zero;
4. existing Realm creation/update behavior remains valid;
5. no CPU policy authority is available.

For a Realm with a positive runtime CPU limit:

1. the new field appears in canonical Spec JSON;
2. the existing `PolicyDigest` v1 algorithm includes that field because it hashes the canonical full `Spec`;
3. changing the limit through `Controller.Update` changes the Realm revision and policy digest;
4. changing any other Realm policy field also changes the revision/policy binding and makes an older Wave31 authority stale.

Wave31 must **not** change the `PolicyDigest` domain separator. That would invalidate all legacy policy digests even when the new optional field is absent.

## 6. Separation from accounting budget

`ResourceBudget` remains unchanged:

```go
type ResourceBudget struct {
    CPUUnits  uint64 `json:"cpu_units"`
    MemoryMiB uint64 `json:"memory_mib"`
    DiskMiB   uint64 `json:"disk_mib"`
}
```

Wave31 establishes explicit non-equivalence:

- `CPUUnits` is accounting capacity;
- `RuntimeCPULimitMilliCPU` is host policy intent for runtime CPU bandwidth;
- neither value is derived from the other;
- changing `CPUUnits` while runtime CPU limit is zero must never make Wave31 authority available;
- a positive runtime CPU limit may coexist with any otherwise-valid positive accounting `CPUUnits` value.

No capability or report may call `CPUUnits` “requested CPU cores,” “requested quota,” “requested milliCPU,” or equivalent wording.

## 7. Opaque authority

Wave31 adds one public capability in package `realm`:

```go
type RuntimeCPUPolicyAuthority struct {
    // package-private state only
}
```

Its zero value is invalid. It cannot be reconstructed through JSON, public field assignment, a caller-supplied digest, or a descriptive binding.

The descriptive projection is:

```go
type RuntimeCPUPolicyBinding struct {
    RealmID        ID
    RealmRevision  uint64
    PolicyDigest   string
    LimitMilliCPU  uint64
    Digest         string
}
```

`RuntimeCPUPolicyBinding` is descriptive only and carries no authority.

The authority retains:

- the exact package-owned Store instance used as the trust root;
- the sealed descriptive binding;
- a package-private seal pointer.

The Store trust boundary follows the established `RealizationAuthority` pattern: only exact package-owned `*MemoryStore` and `*DurableStore` implementations are accepted. An external `Store` implementation, wrapper, proxy, or embedding type cannot mint authority merely by satisfying public methods.

## 8. Wave31 authority digest

The descriptive authority digest format is:

`realm-runtime-cpu-policy-v31:<64-lowercase-hex>`

Domain separator:

`nolane.realm-runtime-cpu-policy.v31\x00`

The digest canonically binds exactly:

- Realm ID;
- Realm revision;
- canonical Realm `PolicyDigest`;
- exact positive milliCPU limit.

The digest is deterministic for identical authority-owned inputs. It does not replace the Realm `PolicyDigest`; it gives downstream propagation receipts one explicit component digest for the CPU-policy authority.

The raw hash input must use one unambiguous canonical representation owned by package `realm`. JSON is acceptable only with a private fixed-field canonical struct and the exact domain prefix above. Map serialization is forbidden.

## 9. Minting API

The package-owned minting API is:

```go
func (c *Controller) CurrentRuntimeCPUPolicyAuthority(
    ctx context.Context,
    realmID ID,
) (RuntimeCPUPolicyAuthority, error)
```

Minting succeeds only when all conditions hold:

1. `Controller` and context are valid;
2. the Controller is backed by an exact package-owned `*MemoryStore` or `*DurableStore`;
3. the current Realm exists;
4. the Realm is not closed;
5. Realm revision is non-zero;
6. stored `Spec.ID` exactly equals `realmID`;
7. current `Spec.Validate()` succeeds;
8. `RuntimeCPULimitMilliCPU > 0`;
9. current `PolicyDigest(spec, revision)` succeeds;
10. the binding and Wave31 digest validate exactly.

The caller supplies only `realmID`. It does not supply revision, policy digest, milliCPU value, Store, Realm record, or Wave31 digest.

## 10. Validation API and freshness

The validation API is:

```go
func (c *Controller) ValidateRuntimeCPUPolicyAuthority(
    ctx context.Context,
    authority RuntimeCPUPolicyAuthority,
) (RuntimeCPUPolicyBinding, error)
```

Validation re-reads the current package-owned Realm state. It must require:

- exact authority seal;
- internally valid binding and Wave31 digest;
- exact Store-instance identity between authority and validating Controller;
- current Realm still exists and is open;
- current Realm revision exactly equals the authority revision;
- current stored Realm ID/spec ID exactly match;
- current Spec still validates;
- current runtime CPU limit remains positive and exactly equals the authority value;
- freshly recomputed `PolicyDigest` exactly equals the authority value;
- freshly recomputed Wave31 digest exactly equals the authority digest.

Any Realm update, even one that leaves the milliCPU number unchanged, makes the old authority stale because the authority is bound to the exact Realm revision and full policy digest.

`Binding()` may validate the internal seal/digest and return the descriptive projection, but it is not a freshness check. Only `Controller.ValidateRuntimeCPUPolicyAuthority` re-reads current authoritative state.

## 11. Error classes

Wave31 defines three package errors.

### `ErrRuntimeCPUPolicyUnavailable`

Used when the Controller cannot currently mint/validate a policy authority because the authority source is unavailable, including:

- no positive runtime CPU limit is configured;
- Realm does not exist or is closed at mint time;
- Controller Store is not an exact package-owned trust root;
- stored current Realm state is not a valid mint source.

### `ErrInvalidRuntimeCPUPolicyAuthority`

Used for malformed or foreign authority, including:

- zero authority;
- broken seal;
- malformed binding;
- invalid Wave31 digest format/content;
- authority presented to a different package-owned Store instance;
- reconstructed/deserialized authority.

### `ErrStaleRuntimeCPUPolicyAuthority`

Used when an authority was validly minted from this exact Store but current authoritative Realm state has moved, including:

- Realm revision changed;
- full Realm policy digest changed;
- runtime CPU limit changed or was cleared;
- Realm was closed after minting;
- current Realm record no longer matches the sealed authority binding.

Context cancellation/deadline returns the context error directly.

## 12. Public anti-shortcut contract

Wave31 public APIs must not accept or expose any mechanism that lets a caller manufacture the claim.

The minting/validation surface must not accept:

- `ResourceBudget` or `CPUUnits` as a CPU-policy input;
- caller-supplied milliCPU separate from authoritative Realm state;
- caller-supplied Realm revision;
- caller-supplied Realm `PolicyDigest`;
- caller-supplied Wave31 digest;
- arbitrary Store as a constructor parameter for the authority;
- world/substrate handle;
- cgroup path;
- `cpu.max` bytes;
- quota/period values;
- PID/process evidence;
- Wave28 readback;
- Wave30 CPU intervention authority;
- `TrustedReport`;
- `LIVE_PASS` target.

Package-private tests may construct malformed authority state to prove fail-closed validation, but no production public constructor for `RuntimeCPUPolicyAuthority` exists.

## 13. Required TDD coverage

### Realm Spec and compatibility

1. legacy Spec with zero runtime CPU field marshals without `runtime_cpu_limit_millicpu`;
2. a frozen legacy Spec/revision retains its exact pre-Wave31 `PolicyDigest`;
3. positive runtime CPU limit appears in canonical JSON;
4. positive runtime CPU limit changes `PolicyDigest` relative to the same legacy Spec/revision;
5. changing only the runtime CPU limit changes `PolicyDigest`;
6. legacy Realm durable state without the field loads with zero and remains valid;
7. positive runtime CPU limit survives MemoryStore and DurableStore round-trip/update paths.

### Authority happy path

8. trusted MemoryStore current Realm with positive limit mints authority;
9. trusted DurableStore current Realm with positive limit mints authority;
10. returned binding contains exact Realm ID/revision/policy digest/limit;
11. Wave31 digest is deterministic and canonical lowercase hex;
12. immediate validation against the same Controller/Store succeeds.

### Accounting separation

13. positive `ResourceBudget.CPUUnits` with zero runtime CPU limit returns unavailable;
14. changing accounting `CPUUnits` does not synthesize a runtime CPU limit;
15. positive runtime CPU limit is accepted independently of the exact accounting CPUUnits value as long as the existing budget is valid.

### Fail closed / freshness

16. zero authority is invalid;
17. JSON round-trip cannot restore authority;
18. external Store implementation cannot mint authority;
19. wrapper/embedding around a trusted Store cannot mint authority;
20. authority cannot validate against a different trusted Store instance with identical descriptive records;
21. Realm revision update makes old authority stale even when milliCPU remains unchanged;
22. runtime CPU limit change makes old authority stale;
23. runtime CPU limit clear-to-zero makes old authority stale;
24. unrelated Realm policy change makes old authority stale;
25. Realm close makes old authority stale;
26. internal digest tamper fails invalid;
27. internal binding tamper fails invalid;
28. context cancellation fails before mint/validation.

### Public API/static contract

29. `RuntimeCPUPolicyAuthority` has no exported fields and no public constructor accepting descriptive state;
30. minting API accepts only context plus Realm ID;
31. validation API accepts only context plus opaque authority;
32. source contains no mapping from `ResourceBudget.CPUUnits` to runtime milliCPU;
33. Wave31 production source contains no cgroup, quota/period, PID, Wave28, Wave30, `TrustedReport`, or `LIVE_PASS` integration.

## 14. Scope and expected files

Wave31 implementation is expected to remain Realm-local plus dedicated tests/contracts/docs.

Expected production changes:

- modify `NolaneWorld/realm/model.go` only to append the optional `RuntimeCPULimitMilliCPU` Spec field;
- create `NolaneWorld/realm/runtime_cpu_policy_authority.go` for the opaque authority, package-owned Store trust root, digest, minting, and validation.

Expected test/CI additions:

- Realm behavioral tests for compatibility, authority, store isolation, freshness, and durability;
- `tests/wave31_realm_runtime_cpu_policy_contract.py` static anti-shortcut contract;
- `.github/workflows/wave31-realm-runtime-cpu-policy-contract.yml` dedicated workflow;
- RED and closure evidence under `docs/superpowers/verification/`.

Wave31 must not modify:

- `NolaneWorld/substrate/cube` production behavior;
- `NolaneWorld/fabric` create semantics;
- provider request schemas;
- Cubelet resource application;
- `resourceproof` trusted producer logic;
- `TrustedReport` or aggregate resource verdict semantics.

A change outside that scope requires a new design review rather than silent expansion.

## 15. Relationship to Wave30

Wave31 and Wave30 prove different halves of a future CPU enforcement statement.

Wave30 proves:

- exact runtime/cgroup identity;
- a bounded package-owned CPU intervention;
- exact helper CPU runtime growth;
- quiet-before / throttled-during causal association under the effective finite cgroup limit.

Wave31 proves:

- exact current host Realm CPU bandwidth **intent**;
- exact Realm revision/full policy binding;
- explicit milliCPU semantics independent from accounting budget.

Neither authority alone proves requested-policy enforcement.

Wave31 therefore must not accept Wave30 authority as an input and must not mention Wave30 evidence in its authority digest.

## 16. Planned next trust seams

### Wave32 — Exact CPU policy propagation authority

Propagate a fresh Wave31 policy authority through the world-create/provider boundary without caller-controlled unit conversion. The propagation receipt must bind the Wave31 digest, exact Realm revision/policy digest, world identity, and exact provider request representation.

Wave32 must not yet claim effective kernel equality merely because a request field was sent.

### Wave33 — Requested/effective CPU bandwidth equivalence authority

Bridge:

1. fresh Wave31 policy intent;
2. exact Wave32 propagation receipt;
3. fresh Wave27/Wave28 runtime/cgroup identity and `cpu.max` readback;
4. Wave30 causal throttle intervention.

Requested/effective equivalence must compare exact ratios without assuming a fixed CFS period. Conceptually:

`quotaMicros / periodMicros == limitMilliCPU / 1000`

Implementation must use overflow-safe exact arithmetic/cross-multiplication and fail on rounding ambiguity rather than silently accepting an approximation.

Only then may Nolane claim that the exact runtime was causally CPU-throttled under a limit equal to the exact requested Realm CPU bandwidth policy.

### Wave34 — CPU-only trusted evidence projection

A later projection may consume a fresh Wave33 authority into a CPU-specific trusted evidence component. It must not mark aggregate resource enforcement verified while memory/disk proof remains absent.

## 17. Explicit nonclaims

Wave31 does **not** prove or claim:

- that any world-create request carries the runtime CPU limit;
- that Cube/Cubelet receives the limit;
- that cgroup `cpu.max` equals the requested limit;
- CPU throttling causality;
- CPU affinity, cpuset, exclusive cores, NUMA placement, or physical-core identity;
- CPU reservation/guaranteed performance;
- memory enforcement or OOM causality;
- disk enforcement;
- task outcome;
- guest correctness;
- hypervisor/TEE/hardware attestation;
- `TrustedReport` resource enforcement;
- aggregate `ResourceEnforcementAvailable`/verified status;
- `LIVE_PASS`.

A Wave31 authority says only that an exact current host-owned Realm policy contains one exact positive runtime CPU bandwidth limit intent.

## 18. Success criteria

Wave31 is code-closed only when all of the following are true on one exact head SHA:

1. legacy zero-field Realm JSON and frozen `PolicyDigest` compatibility are proven;
2. explicit positive milliCPU intent is canonically included in current Realm policy digest;
3. package-owned MemoryStore and DurableStore can mint/validate the opaque current-policy authority;
4. external/wrapped stores cannot mint it;
5. zero/deserialized/tampered/cross-store authorities fail closed;
6. Realm revision/policy/limit drift makes old authority stale;
7. accounting `CPUUnits` cannot synthesize or substitute for the runtime CPU limit;
8. dedicated Wave31 behavioral/static contract is GREEN;
9. existing Realm tests, Wave30 regression surface, `go vet ./...`, `go test ./...`, and focused race tests are GREEN;
10. exact-head PR workflows applicable to the stacked PR are GREEN;
11. closure evidence records exact base SHA, exact final SHA, scope diff, TDD RED lineage, and explicit nonclaims.

Until those conditions are met, Wave31 is not complete and must not be presented as requested-policy enforcement.
