# Nolane Sandbox Capability Persistence v3 Implementation Plan

**Goal:** Make trusted capability material and promotion provenance survive Trust Plane restart with exact-byte binding and fail-closed recovery.

**Constraints:** Keep Cube security core unchanged; no guest-owned trusted storage; no receipt may be considered valid if referenced content/manifest/evidence is missing or altered; all local persistence is single-writer.

### Task 1 — Exact evidence contract
- [x] Add `VerificationEvidence` to PromotionRequest.
- [x] Require non-empty exact evidence and SHA-256 match.
- [x] Add bounded promotion metadata.
- [x] Introduce `capability.Store` and make Forge depend on it.
- [x] Pass host-copied validator evidence through Forge.

### Task 2 — Durable CAS
- [x] Add SHA-256 CAS for content, manifest and evidence.
- [x] Never derive blob paths from agent-controlled names/IDs.
- [x] Exclusive-create and fsync new blobs; verify rather than overwrite existing blobs.
- [x] Treat orphan blobs as untrusted garbage only.

### Task 3 — Durable promotion journal
- [x] Append-only hash-chained JSONL journal.
- [x] Fsync journal before in-memory trust transition.
- [x] Strict replay and receipt/candidate recomputation.
- [x] Verify every referenced blob during recovery.
- [x] Persist collision/idempotency semantics across restart.
- [x] Add lifetime single-writer lock.

### Task 4 — Adversarial contracts
- [x] Journal tamper and malformed-tail tests.
- [x] Missing/tampered blob tests.
- [x] Orphan blob non-trust test.
- [x] Exact retry/no-second-record test.
- [x] Restart collision tests.
- [x] Forge + DurableRegistry integration test.
- [x] Demonstrate RED when DurableRegistry implementation is absent.

### Task 5 — Verification
- [x] Local `go test ./...`.
- [x] Local `go test -race ./...`.
- [x] Local `go vet ./...`.
- [ ] Commit exact v3 tree to `nolane/world-foundation-v0`.
- [ ] Verify GitHub Nolane World Check on exact commit.
- [ ] Verify diff remains isolated from Cube security core.

DCO remains a human attestation gate and is never synthesized by the agent.
