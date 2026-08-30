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
- v4 remains a separate deterministic evidence family and its standard suite is not extended by later live/authority layers.

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

### Delegated Authority Plane v6

- agent-visible external-action intents contain a delegation ID, operation, exact resource, payload, world ID/epoch, and action ID — **never a credential handle, credential bytes, or adapter selection**;
- immutable host grants bind one world/epoch, one typed adapter kind, one canonical resource, an explicit operation allowlist, one host-only `SecretHandle`, and an expiry;
- grant IDs cannot be rebound, revocation is monotonic, and revoked grants remain historically queryable for safe reconciliation;
- `Resolver` is read-only while host administration uses the separate `Controller` mutation interface, so the execution path cannot issue or revoke its own grants;
- `JournalStore` persists issue/revoke transitions with mode `0600`, sequence numbers, SHA-256 hash chaining, fsync-before-trust, strict replay, and single-writer locking;
- `Vault.Use` leases a fresh credential buffer only inside trusted host adapter callbacks and wipes the working buffer when the callback returns;
- `MemoryVault` is test/development infrastructure, **not** a production KMS;
- typed adapter registration rejects generic/raw/authenticated HTTP transports, preventing a credential proxy from becoming arbitrary delegated authority;
- delegated execution resolves the adapter and secret handle from the immutable host grant, binds the grant/handle into the action request digest, and reuses the existing exactly-once effect ledger;
- any provider failure after adapter execution begins remains `pending`/uncertain, so automatic retry cannot duplicate a real-world side effect;
- if provider evidence echoes exact credential bytes, the plane emits the stable `ErrSecretLeak` sentinel, creates no success receipt, and leaves the action uncertain;
- `Reconcile` is observation-only: it can inspect a historical pending effect after grant revocation, expiry, or world epoch advance, but it never calls `Adapter.Execute`;
- an `observed` reconciliation may close the pending receipt; `absent` and `unknown` remain non-replayable;
- the Delegated Authority Gauntlet v6 contains 15 mandatory scenarios covering resource/operation rebinding, stale epoch, revocation, expiry, adapter confusion, generic authenticated HTTP, action collision, uncertain replay, observed/absent reconciliation, secret echo/absence, and durable journal restart/tamper;
- `nolane-authority-gauntlet` emits a deterministic, secret-free JSON evidence artifact without altering the Release Gauntlet v4 standard evidence family.

### Provider Authority Runtime v7

- `BrokerVault` turns a host-only opaque `SecretHandle` into a callback-scoped credential lease over a bounded Unix-socket protocol; on Linux it verifies `SO_PEERCRED` **before** sending the handle to the broker;
- broker messages use one canonical length-prefixed JSON representation, reject unknown/trailing/alternate encodings, enforce credential-size limits, and fail closed on unsupported peer-authentication platforms;
- `providerhttp` owns its HTTPS client and transport, disables environment proxying and redirects, requires TLS 1.2+, pins one configured origin/base path, validates path segments, and bounds provider responses;
- host-controlled provider query values are encoded only after the origin/path has been fixed, so query construction cannot rebind the authenticated destination;
- the first production-shaped typed provider adapter is `github.v1`, limited to `github.repo.contents.write`, `github.issue.comment.create`, and `github.pull_request.comment.create`;
- GitHub resources and payloads are strict canonical ABIs: parse → typed value → serialize must round-trip exactly, rejecting percent-encoding ambiguity, traversal, delimiter confusion, duplicate/unknown JSON fields, and alternate encodings before provider entry;
- authenticated writes use fixed GitHub routes, attach a deterministic `nolane-provider-v7:<sha256(request-digest)>` marker, and never retry inside the adapter;
- provider response bodies are untrusted: successful evidence is rebuilt only from allowlisted bounded object IDs/commit SHAs, while provider failures collapse to stable sentinels without returning raw diagnostics;
- ambiguous writes remain pending in the existing effect journal, so a retry cannot issue a second provider mutation;
- GitHub reconciliation is read-only: comments and commits are searched by exact action marker through bounded GET pagination, `Observed` can resolve a pending journal entry, and bounded non-observation remains `Unknown` rather than manufacturing absence;
- write responses remain capped at 64 KiB while reconciliation has its own bounded 8 MiB page budget; those limits are independently enforced at transport and decoder boundaries;
- Provider Authority Gauntlet v7 contains 20 mandatory scenarios using real Unix broker sockets, TLS provider servers, typed adapters, the Delegated Authority Plane, and the durable effect journal;
- v7 evidence is generated twice and byte-compared, is scanned for plaintext/base64/hex forms of the synthetic credential, and includes hash guards proving the canonical V4 and V6 evidence families did not drift.

`BrokerVault` is the host-local trust boundary toward a real secret backend. V7 proves the broker protocol, peer binding, lease semantics, typed provider runtime, uncertainty behavior, reconciliation, and deterministic evidence; it does **not** claim that every broker implementation is a KMS/HSM, or that an unverified external secret backend inherits Nolane trust automatically.

### Freedom Plane / Reality Membrane v8

- host-owned `Freedom Realm` policy binds a stable `realm://` identity, revision, resource accounting budget, default lease, world limit, and one of the explicit R0-R2 network profiles;
- `Local Capacity Fabric` owns resource reservations, realization revisions, lease generations, exact operation-ID replay/collision semantics, checkpoint/resume fences, and terminal-world denial;
- reusable baselines are sanitized, identity-free templates only: they cannot carry a World identity or checkpoint ownership, and every Acquire still creates the caller's exact fresh `WorldID`;
- the agent-facing `Runtime` is semantic rather than substrate-shaped: `Enter`, `Acquire`, `Exec`, `Spawn`, `Checkpoint`, `Resume`, `RegisterService`, `Capabilities`, and `Release` expose no Cube handle, sandbox token, credential, or Realm administration authority;
- bounded Exec validates command/time/output limits before guest entry, binds action IDs to canonical request digests, returns bounded observations, and quarantines uncertain outcomes instead of replaying them automatically;
- checkpoint rollback may restore guest execution state, but host authority epoch, realization revision, and lease generation move monotonically forward;
- the internal `service://` registry is realization-revision and generation fenced; old readiness never survives a new realization or explicit recovery fence;
- durable Realm state is append-only, strict, hash-chained, fsynced and single-writer locked; host restart replays historical truth first, while Fabric separately invalidates stale handles/readiness without rewriting history;
- fail-honest capability reporting distinguishes `verified`, `available-unproven`, `unavailable`, and `not-applicable`; accounting budgets are never presented as proof of kernel enforcement;
- R0 is internal-only; R1 public-read and R2 supply-chain require a governed Reality gateway and do **not** imply raw Internet access; N3-N5 remain behind the v6/v7 delegated typed-provider authority path;
- the Cube profile mapper accepts only semantic Realm profiles and fails closed rather than accepting agent-supplied raw network policy;
- optional `TimedRuntime` performance hooks emit only semantic operation labels plus durations for Acquire/Exec/Spawn/Checkpoint/Resume; request IDs, World IDs, handles, error text, tokens, credentials, and payloads are excluded from the metric sample ABI;
- Freedom Gauntlet v8 contains exactly 20 stable scenarios exercising real package boundaries for identity, leases, uncertainty, recovery, services, capabilities, Reality crossing, historical evidence nondrift, and `UNAVAILABLE != PASS`;
- CI generates v8 evidence twice, requires byte identity, scans plaintext/base64/hex forms of the synthetic credential, and publishes a commit-bound `nolane-freedom-v8` artifact without changing the v4/v6/v7 commands.

The deterministic v8 suite proves the host-layer contracts it exercises. It **does not** prove live inter-VM mesh behavior, KVM/hypervisor escape impossibility, kernel resource enforcement, or that an R1/R2 profile has actually reached the public Internet. Those require independent live/substrate evidence; absence of such evidence must remain `UNAVAILABLE` or unverified rather than being upgraded into a PASS claim.

## Security model

Assume the model, agent, guest root, downloaded packages, webpages, generated code, snapshots, exported files, and agent-provided external-action payloads are hostile.

The Trust Kernel owns authority state outside the guest. A guest snapshot is execution state only; it is never a source of truth for authority epochs, external-effect history, delegation grants/revocation, promotion receipts, trusted capability records, Realm policy, lease generations, or recovery readiness.

The central invariant is:

> **Unbounded capability creation; bounded authority, promotion, truth, and persistence.**

A second invariant is intentionally conservative:

> **When the real-world outcome is uncertain, do not execute it again automatically.**

V6 strengthens that rule for credentials:

> **The agent requests a typed effect; the host grant chooses the authority and credential. The agent never receives credential material.**

V7 strengthens the provider boundary further:

> **Credentials may cross only from a verified host broker into a typed adapter on a pinned provider route; provider output never defines its own authority or trusted evidence.**

V8 separates internal freedom from external authority:

> **Capability inside a Realm does not imply Reality authority. Host policy, leases, evidence, and typed delegation remain outside guest control.**

## What is not production-complete yet

Freedom Plane v8 provides the semantic Realm/runtime/fabric/membrane contracts, durable recovery fencing, fail-honest capability projection, deterministic adversarial evidence, and secret-free timing hooks. Deterministic v8 alone still does **not** establish live inter-VM service reachability, production-grade kernel resource enforcement, KVM escape impossibility, distributed multi-host Realm consensus, or a production scheduler/provider reconciler for every supported substrate.

Provider Authority Runtime v7 supplies a host-local credential-broker boundary, hardened HTTPS provider transport, a narrow typed GitHub adapter, uncertainty-safe provider writes, read-only positive reconciliation, and deterministic 20-scenario provider evidence. It still does **not** make `MemoryVault` a production secret store, attest arbitrary broker/KMS/HSM implementations, expose generic authenticated HTTP, implement the full GitHub API, provide distributed consensus, or protect against rollback of the entire host storage device.

Live Substrate Gauntlet v5 likewise remains a harness until a `LIVE_PASS` artifact exists for the exact commit on a configured `nolane-kvm` runner. Remaining product gates include a production KMS/HSM-backed broker deployment with backend-specific attestation and rotation policy, additional provider-specific typed adapters where needed, target-backed credential-injection proof on a live KVM runner, live Realm mesh/profile proof, durable general artifact-receipt/quarantine storage, hostile artifact corpus testing, and eventually a durable multi-host authority/effect substrate where required.

None of these deterministic host-layer suites is a claim that the full product is “unescapable” or that every external provider/backend is trusted by default.

See:

- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-world-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-runtime-integration-v1-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-persistence-v2-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-capability-persistence-v3-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-release-gauntlet-v4-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-live-substrate-gauntlet-v5-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-delegated-authority-plane-v6-design.md`
- `../docs/superpowers/specs/2026-08-29-nolane-sandbox-provider-authority-runtime-v7-design.md`
- `../docs/superpowers/specs/2026-08-30-nolane-sandbox-freedom-plane-reality-membrane-v8-design.md`

## Verify

```bash
cd NolaneWorld
go test ./...
go test -race ./...
go vet ./...

# Deterministic in-process trust evidence.
go run ./cmd/nolane-gauntlet --out release-evidence/nolane-gauntlet-v4.json

# Deterministic delegated-authority evidence. Synthetic secret bytes must not appear.
go run ./cmd/nolane-authority-gauntlet --out release-evidence/nolane-authority-v6.json

# Deterministic typed-provider evidence. CI runs this twice, compares bytes,
# and rejects plaintext/base64/hex forms of the synthetic credential.
go run ./cmd/nolane-provider-gauntlet --out release-evidence/nolane-provider-v7.json

# Deterministic Freedom Plane evidence. CI runs this twice, compares bytes,
# scans plaintext/base64/hex credential forms, and uploads exact-head evidence.
go run ./cmd/nolane-freedom-gauntlet --out release-evidence/nolane-freedom-v8.json

# Safe on machines without Cube/KVM: must say UNAVAILABLE, never PASS.
go run ./cmd/nolane-gauntlet-live --mode probe --profile core

# Provisioned live runner only: UNAVAILABLE and LIVE_FAIL are non-zero.
go run ./cmd/nolane-gauntlet-live --mode require-live --profile core --out release-evidence/nolane-live-v5.json
```
