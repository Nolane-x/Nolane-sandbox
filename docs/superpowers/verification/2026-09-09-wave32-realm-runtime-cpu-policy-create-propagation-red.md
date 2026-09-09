# Wave32 Realm Runtime CPU Policy Create Propagation — RED Evidence

Date: 2026-09-09

## Exact committed RED

- Branch: `gpt/wave32-realm-cpu-policy-create-propagation-authority`
- Exact RED SHA: `54307e87e1e0cb7c5598562cfc893be2c79f5c36`
- Workflow: `Wave32 Realm Runtime CPU Policy Create Propagation Contract`
- Run ID: `34317682606`
- Job ID: `102357232994`
- Conclusion: `failure` (expected RED)

## Setup provenance

The exact RED run successfully completed:

- checkout
- Python 3.12 setup
- pytest installation
- Go 1.23.12 setup

It then failed at the first production-facing step, `Focused Wave32 fabric tests`.

All later test/static/regression/vet/full/race steps were skipped because the focused RED step failed first.

## Expected compiler failure

The focused fabric package failed to compile specifically because Wave32 production symbols did not yet exist:

- `substrate.RuntimeCPUPolicyCreatePropagation`
- `substrate.RuntimeCPUPolicyCreatePropagationDigestPrefix`
- `ErrRuntimeCPUPolicyPropagationUnavailable`

Representative exact compiler diagnostics:

```text
fabric/runtime_cpu_policy_create_v32_test.go:19:32: undefined: substrate.RuntimeCPUPolicyCreatePropagation
fabric/runtime_cpu_policy_create_v32_test.go:44:30: undefined: substrate.RuntimeCPUPolicyCreatePropagationDigestPrefix
fabric/runtime_cpu_policy_create_v32_test.go:111:21: undefined: ErrRuntimeCPUPolicyPropagationUnavailable
```

This is the intended TDD RED: the committed behavioral contract exists first, and production propagation types/path are absent.

## RED validity

The RED is accepted because:

- workflow YAML parsed and executed;
- dependency/tool setup succeeded;
- failure is not a test syntax/import typo unrelated to the feature;
- failure is exactly missing Wave32 production API required by the tests;
- no Wave32 production implementation existed at this SHA.

Production GREEN work begins only after this evidence point.
