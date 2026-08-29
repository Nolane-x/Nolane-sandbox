# Nolane Sandbox Trust Kernel

`NolaneWorld` is the host-owned trust/evolution boundary for **Nolane Sandbox**.

The execution substrate remains CubeSandbox. Nolane does not fork the hypervisor, KVM/RustVMM integration, CubeEgress, CubeNet, or CubeCoW for ordinary agent behavior. Instead it defines the rules that let an untrusted AI build arbitrary software capabilities inside disposable worlds without being able to mint external authority, certify itself, or turn execution snapshots into trusted state.

## Implemented

### Trust Kernel v0

- compiled 12-law constitution;
- world identity and monotonic authority epochs;
- typed authority intents and fail-closed policy decisions;
- exact duplicate/collision semantics for effect receipts;
- candidate → independent promotion → trusted registry;
- exact content and manifest binding;
- artifact admission/provenance receipts;
- explicit network authority classes `N0` through `N5`;
- narrow `SandboxSubstrate` interface.

### Runtime Integration v1

- terminal world authority revocation;
- host-owned World Manager for create/pause/resume/snapshot/rollback/destroy/clone;
- rollback epoch advance before execution-state restoration;
- fail-closed CubeAPI adapter with HTTPS/loopback policy, no redirects, bounded responses, no public network by default;
- Capability Forge using fresh validator worlds rather than origin clones;
- host-hashed validation evidence and teardown-before-promotion;
- validator panic/failure cleanup;
- crash-recoverable append-only effect JournalLedger;
- uncertain-outcome replay denial and host reconciliation;
- single-writer OS locking for the durable effect journal.

### Persistence v2

- host-only `AuthorityControl` separated from broker-facing authority reads/execution;
- per-world append-only authority journals with SHA-256 hash chains;
- epoch advances and terminal close fsynced before becoming visible in memory;
- hashed authority filenames so raw WorldID values never become host paths;
- strict replay that rejects malformed, unknown, out-of-order, identity-mismatched, or hash-mismatched records;
- lifetime single-writer authority file locks;
- hash-chained lifecycle catalog for `creating -> ready -> terminal -> destroyed`;
- persistent World Manager recovery;
- incomplete create/clone recovery terminally revokes authority and reports a possible orphan instead of guessing;
- Manager shutdown serializes against in-flight lifecycle mutations.


### Capability Persistence v3

- `capability.Store` decouples Forge from in-memory registry implementation;
- every promotion binds exact content, manifest, and validation-evidence bytes;
- `DurableRegistry` stores all trusted material in SHA-256 content-addressed storage;
- promotion trust is an append-only, fsynced, hash-chained journal;
- exact retry across restart is idempotent and never appends a second promotion;
- candidate-ID and name/version collision rules survive restart;
- recovery verifies every promotion record and every referenced CAS blob before trusting registry state;
- missing, non-regular, tampered, malformed, or hash-mismatched state fails closed;
- orphan CAS blobs alone never create trusted capability records;
- single-writer OS locking prevents split-brain local registry writers;
- Forge persists the exact validator evidence only after clean validator teardown.


### Release Gauntlet v4

- deterministic adversarial scenario contract with stable IDs, explicit invariants, attack descriptions, expected defenses, and required proof markers;
- runner-owned append-only probes so scenarios cannot directly declare themselves passed;
- fail-closed proof-of-exercise requiring attack, trust-boundary, denial, and all required markers;
- panic/timeout/error conversion into stable machine-readable failure codes;
- deterministic SHA-256 scenario evidence and release report digests with domain-separated length-prefixed fields;
- self-contained evidence carries the exact runner policy and required markers so `VerifyReport` does not have to trust the runner implicitly;
- standard adversarial suite for stale authority epochs, terminal worlds, action-ID rebinding, artifact path traversal, trusted capability CAS tamper, and promotion-journal tamper;
- property/fuzz seeds for report mutation detection and deterministic ordering;
- `nolane-gauntlet` CLI emits verified JSON suitable for CI artifact retention;
- core gauntlet remains independent of Cube security internals so a future live Cube/KVM adapter can reuse the same evidence contract.

### Live Substrate Gauntlet v5

- a second, non-substitutable evidence family for real CubeSandbox execution rather than deterministic in-process proof;
- explicit `LIVE_PASS`, `LIVE_FAIL`, and `UNAVAILABLE` states so absent KVM/Cube infrastructure can never masquerade as a passing live check;
- capability attestation crosses CubeAPI health/create/connect and envd guest execution before claiming live guest capability;
- real snapshot proof requires guest state `A -> B -> rollback -> A`, while host authority advances independently and still rejects the pre-rollback epoch;
- teardown is release evidence: DELETE alone is insufficient, and the harness polls until sandbox absence is observed;
- typed Cube network policy and controlled HTTP/TCP/UDP/DNS egress probes require host preflight before a guest connection failure may count as denial;
- API, envd, traffic, and target expectation credentials are excluded from live reports; runtime identifiers are represented by SHA-256 digests;
- `nolane-gauntlet-live --mode probe` emits verified `UNAVAILABLE` evidence on ordinary machines; `--mode require-live` fails closed unless the selected live profile materially passes;
- the self-hosted live workflow is manual, master-only, and gated by `NOLANE_LIVE_GAUNTLET_ENABLED`, so pull requests never receive live-runner secrets.

A V5 harness being present in the repository is **not** itself a live/KVM verification claim. A build is live-substrate verified only when a `LIVE_PASS` artifact exists from the configured `nolane-kvm` runner for that exact commit.

## Security model

Assume the model, agent, guest root, downloaded packages, webpages, generated code, snapshots, and exported files are hostile.

The Trust Kernel owns authority state outside the guest. A guest snapshot is execution state only; it is never a source of truth for authority epochs, external-effect history, promotion receipts, or trusted capability records.

The central invariant is:

> **Unbounded capability creation; bounded authority, promotion, truth, and persistence.**

A second invariant is intentionally conservative:

> **When the real-world outcome is uncertain, do not execute it again automatically.**

## What is not production-complete yet

Live Substrate Gauntlet v5 still does not claim a perfect sandbox or complete production boundary. The local journals remain single-host crash-recovery primitives, not distributed consensus and not protection against rollback of the entire host storage device. V4 continuously proves deterministic trust invariants; V5 provides the fail-closed harness for real Cube/KVM guest, snapshot, cleanup, and controlled egress evidence. Until a `LIVE_PASS` artifact exists for an exact commit, that commit must not be described as live-substrate verified. Remaining product gates include durable general artifact-receipt/quarantine storage, KMS/typed credential brokering, reconciled external adapters, target-backed credential-injection proof, and a hostile artifact corpus.

See:

- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-world-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-runtime-integration-v1-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-persistence-v2-design.md`

## Verify

```bash
cd NolaneWorld
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/nolane-gauntlet --out release-evidence/nolane-gauntlet-v4.json

# Safe on machines without Cube/KVM: must say UNAVAILABLE, never PASS.
go run ./cmd/nolane-gauntlet-live --mode probe --profile core

# Provisioned live runner only: UNAVAILABLE and LIVE_FAIL are non-zero.
go run ./cmd/nolane-gauntlet-live --mode require-live --profile core --out release-evidence/nolane-live-v5.json
```
