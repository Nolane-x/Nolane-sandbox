# Wave 24 — Cubelet Realization Epoch Authority

## Status

Design contract for Wave 24. Stacked on the exact Wave 23 closure head `a9999891a83cacefce20979e5483a9a3480116a0`.

## Problem

Wave 23 binds a fresh Realm World realization to an opaque Cube `ResourceBinding`, but `ResourceBinding` currently carries only the literal Cube sandbox ID. Cubelet's Wave 17 task-outcome generation is exact while a realization remains in the current in-memory lifecycle, yet `Create` clears Wave 17 state and a later `Start` may legitimately begin again at numeric generation `1`.

Therefore `{sandbox_id, generation}` is not a non-reusable realization identity across a Cubelet `Clear -> BeginRealization` lifecycle. A stale proof could alias a later realization if the same literal sandbox ID is reused and the numeric generation resets.

## Scope

Wave 24 introduces a **Cubelet-local realization epoch authority**. It proves identity under the current Cubelet producer lifecycle. It does **not** claim a provider-global immutable sandbox incarnation, because the generic Cube provider API exposes only a literal sandbox ID and no independently verifiable remote-incarnation identifier.

The authoritative identity tuple is:

```
{ sandbox_id, wave17_generation, realization_epoch_token }
```

where `realization_epoch_token` is a cryptographically random, non-zero, exact 32-byte value minted by Cubelet at the same authoritative lifecycle seam as Wave 17 `BeginRealization`.

## Required properties

1. **Producer-owned minting.** The epoch token is minted by Cubelet, not by NolaneWorld, `GuestSession`, Realm, or a caller.
2. **Exact lifecycle binding.** A token is bound to exactly one current `{sandbox_id, generation}` realization.
3. **Clear invalidates.** `Clear(sandbox_id)` removes current epoch authority. It never reconstructs a token from sandbox ID, generation, timestamps, or process state.
4. **Non-aliasing reuse.** After `Clear`, a later `BeginRealization` for the same sandbox may reset the numeric generation to `1`, but it must receive a different non-zero epoch token.
5. **No silent downgrade.** If secure random token minting fails, the new realization cannot acquire Wave24 authority. Production code must not substitute a zero token, deterministic token, local timestamp, counter, UUID derived from sandbox ID, or Wave21 guest-OOM token.
6. **Read-only validation.** Consumers can ask whether an exact epoch is current without mutating or promoting stale state.
7. **Canonical transport.** The existing trusted Cubelet resource-metrics management path exports the current epoch as one exact metric sample with canonical sandbox ID, canonical non-zero decimal generation, and exactly 64 lowercase hexadecimal token characters.
8. **Fail closed transport.** Malformed, duplicate, zero-token, wrong-generation, wrong-sandbox, missing or non-canonical epoch samples do not mint NolaneWorld authority.
9. **Opaque NolaneWorld proof.** A trusted management observer may mint an in-process sealed epoch proof. Caller-provided fields or JSON cannot mint the proof.
10. **Exact Realm/Cube bridge.** Any Wave24 cross-boundary authority requires all of: fresh Wave23 `realm.RealizationAuthority`, package-owned Cube `ResourceBinding`, trusted current Cubelet epoch proof, exact sandbox equality, and exact current epoch identity.
11. **Restart non-claim.** Loss of Cubelet epoch state invalidates authority. Wave24 does not reconstruct prior epoch authority after producer restart.
12. **No semantic laundering.** Wave24 does not turn a realization epoch into OOM causality, task-exit truth, runtime digest, disk enforcement proof, provider-global identity, or a generic LIVE_PASS verdict.

## Producer lifecycle seam

`Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go` already calls Wave 17 `BeginRealization` at the beginning of `Start`, before resource baseline/collector setup and before runtime start. `Create` calls `Clear` for the prior realization state. Wave24 must bind token creation to this same lifecycle, preferably inside the Wave17 outcome authority object rather than in an independent registry that can drift.

The preferred producer API shape is an opaque/descriptive split such as:

```go
type RealizationEpoch struct {
    Generation uint64
    token      [32]byte
}

func (s *taskOutcomeProofStore) BeginRealizationEpoch(sandboxID string) (RealizationEpoch, error)
func (s *taskOutcomeProofStore) CurrentRealizationEpoch(sandboxID string) (RealizationEpoch, bool)
func (s *taskOutcomeProofStore) IsCurrentRealizationEpoch(sandboxID string, generation uint64, token [32]byte) bool
```

The exact final API may differ if existing call structure requires a smaller compatibility surface, but the trust properties above are mandatory.

## Management transport

Wave17 already exports exact task outcome proof through `Cubelet/plugins/cube/internals/resourcemetrics/task_outcome_prometheus.go`. Wave24 extends the same management metrics trust boundary rather than creating an unrelated endpoint.

Canonical metric shape:

```text
cubesandbox_realization_epoch_info{sandbox_id="<exact>",generation="<canonical-decimal>",token="<64-lowercase-hex>"} 1
```

Only current known epoch authority is exported. The token is an identity nonce, not a credential; secrecy is not its trust property.

## NolaneWorld consumer

NolaneWorld should add a dedicated realization-epoch observer and sealed proof. Existing `ResourceBinding` must not be silently considered stronger simply because a new metric exists.

A Wave24 bridge authority may expose descriptive fields for diagnostics, but those fields are insufficient to reconstruct the sealed capability.

## TDD closure requirements

Wave24 is not complete until committed RED evidence proves at minimum:

- same sandbox + reset generation after `Clear` cannot reuse the old epoch;
- old exact epoch becomes stale immediately after `Clear` or next realization;
- malformed/duplicate/non-canonical epoch transport is rejected;
- caller/descriptive reconstruction cannot mint NolaneWorld epoch proof;
- wrong sandbox or stale Realm authority cannot bridge;
- exact fresh tuple bridges successfully.

Then the implementation must turn those same tests GREEN, with a dedicated Wave24 workflow and fresh exact-head CI evidence.

## Explicit non-claims

Wave24 does not prove that a remote provider never reuses a sandbox ID across a Cubelet restart. That stronger claim requires a provider-supplied immutable incarnation identifier or another durable producer root of trust that is not present in the current generic Cube API.