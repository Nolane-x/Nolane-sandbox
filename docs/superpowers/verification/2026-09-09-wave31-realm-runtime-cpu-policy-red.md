# Wave31 Realm Runtime CPU Policy Authority — RED Evidence

Date: 2026-09-09
Base: Wave30 exact-final `3657256ea40528bf673a8ca4d51c69233d161b45`
Branch: `gpt/wave31-realm-physical-cpu-policy-authority`

## Test-only / contract RED candidate

Exact SHA:

`8a3522ad15102c37d237a87d9f3fa050f5a30ffc`

Dedicated workflow:

- workflow: `Wave31 Realm Runtime CPU Policy Contract`
- run ID: `34315037426`
- job ID: `102349364158`
- conclusion: `failure`
- failing step: `Focused Wave31 tests`
- later static/regression/vet/full/race steps: skipped because the focused RED failed first

Setup was clean:

- checkout: success
- Python 3.12 setup: success
- pytest install: success
- Go 1.23.12 setup: success

## Canonical missing-production failure

The focused command was:

```text
go test ./realm -run 'V31|RuntimeCPUPolicy' -count=1
```

The compiler failed on the intended absent Wave31 production surface, including:

```text
realm/runtime_cpu_policy_authority_v31_test.go:17:7: spec.RuntimeCPULimitMilliCPU undefined (type Spec has no field or method RuntimeCPULimitMilliCPU)
realm/runtime_cpu_policy_authority_v31_test.go:59:12: withLimit.RuntimeCPULimitMilliCPU undefined (type Spec has no field or method RuntimeCPULimitMilliCPU)
realm/runtime_cpu_policy_authority_v31_test.go:85:24: ctl.CurrentRuntimeCPUPolicyAuthority undefined (type *Controller has no field or method CurrentRuntimeCPUPolicyAuthority)
realm/runtime_cpu_policy_authority_v31_test.go:86:21: undefined: ErrRuntimeCPUPolicyUnavailable
realm/runtime_cpu_policy_authority_v31_test.go:133:24: ctl.ValidateRuntimeCPUPolicyAuthority undefined (type *Controller has no field or method ValidateRuntimeCPUPolicyAuthority)
realm/runtime_cpu_policy_authority_v31_test.go:171:27: recovered.Spec.RuntimeCPULimitMilliCPU undefined (type Spec has no field or method RuntimeCPULimitMilliCPU)
```

The Go package ended with:

```text
FAIL github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm [build failed]
```

This is a valid TDD RED: the new tests were compiled against the exact pre-Wave31 production tree and failed because the requested field/authority/error API did not yet exist. No production Wave31 source existed at this SHA.

## Locked behaviors before GREEN

The committed RED suite requires:

- zero-value runtime CPU intent omitted from legacy JSON;
- frozen pre-Wave31 `PolicyDigest` continuity;
- positive milliCPU intent changes policy digest;
- accounting `CPUUnits` cannot mint runtime CPU policy authority;
- MemoryStore and DurableStore support only explicit positive intent;
- deterministic sealed authority digest;
- zero/tampered/serialized authority rejection;
- exact Store-instance isolation;
- revision, limit, unrelated policy, and close transitions stale old authority;
- context cancellation propagation;
- caller-owned and embedded Store wrappers cannot mint authority;
- static public API anti-shortcut constraints.

Production implementation may begin only after this RED point.
