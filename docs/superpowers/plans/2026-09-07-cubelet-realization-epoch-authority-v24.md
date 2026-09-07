# Wave 24 — Cubelet Realization Epoch Authority Implementation Plan

## Objective

Close the remaining Wave23 literal-sandbox-ID alias seam by adding a producer-owned, non-reusable Cubelet realization epoch identity and carrying it through the existing management metrics trust boundary into a sealed NolaneWorld bridge.

## Phase 1 — Producer authority RED

1. Add focused Cubelet sandbox tests proving the desired epoch lifecycle:
   - first begin produces generation 1 plus non-zero 32-byte token;
   - exact tuple is current;
   - `Clear` invalidates it;
   - re-begin after `Clear` may return generation 1 but must use a different token;
   - forged/zero/old token is not current.
2. Commit tests before production implementation.
3. Observe CI RED for the missing producer API/behavior.

## Phase 2 — Producer authority GREEN

1. Bind epoch token state into the Wave17 task-outcome proof store so generation and token cannot drift.
2. Mint token from `crypto/rand` at the authoritative `BeginRealization` lifecycle seam.
3. Fail closed on token-mint failure; do not create partial Wave24 authority.
4. Ensure `Clear` removes token state.
5. Preserve existing Wave17 generation semantics and downstream Wave18–21 behavior.
6. Turn focused producer tests GREEN.

## Phase 3 — Existing management exporter RED/GREEN

1. Add `cubesandbox_realization_epoch_info` collector tests before implementation.
2. Export only current exact epoch tuples through `resourcemetrics`, using:
   - exact sandbox ID;
   - canonical non-zero decimal generation;
   - exact 64-char lowercase hex token;
   - metric value exactly `1`.
3. Wire the collector into the same management registry as task-outcome evidence.
4. Do not create a separate HTTP endpoint.

## Phase 4 — NolaneWorld observer RED/GREEN

1. Add focused tests for strict parsing of epoch metrics.
2. Implement a trusted `RealizationEpochObserver` over the existing Cubelet management metrics endpoint.
3. Reject malformed, duplicate, zero, uppercase/non-canonical hex, missing, wrong-sandbox, wrong-value or wrong-generation samples.
4. Mint only an opaque package-owned epoch proof; descriptive fields do not reconstruct authority.

## Phase 5 — Realm/Cube bridge RED/GREEN

1. Add tests proving:
   - exact fresh Wave23 Realm authority + exact `ResourceBinding` + exact epoch proof succeeds;
   - wrong sandbox fails;
   - stale Realm authority fails;
   - old epoch after producer re-realization fails;
   - zero/forged/deserialized epoch proof fails.
2. Add an opaque Wave24 bridge authority without changing Wave23 semantics.
3. Keep explicit non-claims around provider-global identity and restart persistence.

## Phase 6 — Dedicated contract and verification

1. Add `tests/wave24_cubelet_realization_epoch_contract.py` to lock trust shape and forbid deterministic/local reconstruction shortcuts.
2. Add `.github/workflows/wave24-cubelet-realization-epoch-contract.yml` with focused Cubelet, resourcemetrics, NolaneWorld and static-contract jobs.
3. Run/fetch exact-head CI.
4. Fix failures minimally and repeat exact-head verification after every production change.
5. Record closure evidence only after dedicated Wave24, NolaneWorld, relevant Cube contracts, Format and DCO are GREEN on the same final head.
6. Keep the PR draft and unmerged unless the user explicitly authorizes merge.

## Non-goals

- provider-global immutable remote incarnation;
- durable epoch recovery across Cubelet restart;
- reuse of Wave21 guest-OOM token as generic incarnation authority;
- treating epoch identity as OOM/task-exit/resource-enforcement truth;
- generic LIVE_PASS or runtime-digest activation.