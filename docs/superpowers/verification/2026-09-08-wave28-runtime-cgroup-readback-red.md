# Wave 28 RED Verification — Exact Runtime Cgroup Readback Authority

Date: 2026-09-08
Base: Wave27 exact-final `f42d42dec3809bbbd6b8a6b5862c8a3268f6f1b2`
Branch: `gpt/wave28-runtime-cgroup-readback-authority`

## Committed RED lineage

- Initial behavioral RED test: `a05316ac46baf864ce8323ce437149e0a7180e4a`
- Test-only iteration correction before production: `8deeb60ad5ef256b818075424672e4a72f410e23`
- Static anti-shortcut RED contract: `421f66ff029e796e83bbf40db58900687c128488`
- Dedicated workflow head: `4d95c7d62bdb4ac8e75e3b02b4c9d1568ee2b199`
- Dedicated Wave28 run: `34225422787`
- Job: `102058241623`

## RED result

The workflow setup completed successfully through Python, pytest and Go setup. The first production-facing gate then ran:

`go test ./substrate/cube -run 'V28|RuntimeCgroupReadback' -count=1`

It failed at compile time specifically because Wave28 production APIs/types were intentionally absent:

- `undefined: RuntimeCgroupReadbackObserver`
- `undefined: newRuntimeCgroupReadbackObserverForTest`
- `undefined: RuntimeCgroupReadbackAuthority`
- `undefined: ValidateRuntimeCgroupReadbackAuthority`
- `undefined: ErrInvalidRuntimeCgroupReadbackAuthority`

The compiler reported no syntax errors or unrelated fixture/harness failures.

This is the accepted Wave28 RED state. Production implementation begins only after this point.

## Trust constraint preserved by RED

The static contract is already committed and requires Wave28 production to avoid exported/caller-controlled cgroup roots, file sources, pressure runners, `TrustedReport`, `buildTrustedReport`, `CapabilityEvidence`, and `LIVE_PASS`. It also requires package-owned cgroup-v2 readback and fresh Wave27 reconstruction before and after the snapshot.
