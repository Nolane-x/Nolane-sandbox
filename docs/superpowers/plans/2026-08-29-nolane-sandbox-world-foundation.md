# Nolane Sandbox Trust Kernel v0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first executable Nolane Sandbox Trust Kernel that proves authority non-escalation, rollback-safe external-effect replay protection, independent capability promotion, artifact provenance validation, and explicit network classes without modifying CubeSandbox security internals.

**Architecture:** Add a standalone Go module under `NolaneWorld/`. The module exposes small trust-plane packages and a substrate interface; it has no dependency on guest code and no dependency on Cube internals. The first milestone uses deterministic in-memory stores so invariants can be tested before durable/infrastructure wiring.

**Tech Stack:** Go standard library only; GitHub Actions; SHA-256 content addressing.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-world-design.md`

## Global Constraints

- Do not modify KVM, RustVMM, CubeEgress, CubeNet, CubeCoW, hypervisor, or guest-kernel security code.
- Unknown or malformed security state fails closed.
- Guest state cannot mutate authority epoch, effect history, promotion state, or constitution.
- No production credential material is introduced in this milestone.
- Go module must pass `go test ./...` and `go vet ./...`.
- Commit messages must follow repository `AGENTS.md` and include `Autonomously-by: ChatGPT:GPT-5.6-Sol`.

---

### Task 1: Constitution and Network Classification

**Files:**
- Create: `NolaneWorld/go.mod`
- Create: `NolaneWorld/constitution/laws.go`
- Create: `NolaneWorld/constitution/laws_test.go`
- Create: `NolaneWorld/network/class.go`
- Create: `NolaneWorld/network/class_test.go`

**Interfaces:**
- Produces: `constitution.Law`, `constitution.Laws() []Law`, `constitution.Validate() error`
- Produces: `network.Class`, constants `N0None` through `N5ConsequentialWrite`, `Parse(string) (Class, error)`, `Class.Valid() bool`

- [ ] **Step 1: Write failing tests** proving the 12 stable law IDs exist exactly once, validation rejects duplicate/empty IDs, all six network classes parse, and unknown classes fail.
- [ ] **Step 2: Run** `cd NolaneWorld && go test ./constitution ./network` and verify RED due to missing production packages.
- [ ] **Step 3: Implement minimal law catalog and network class parser** using only the standard library.
- [ ] **Step 4: Re-run tests** and verify GREEN.

### Task 2: World Identity and Monotonic Authority Epoch

**Files:**
- Create: `NolaneWorld/world/identity.go`
- Create: `NolaneWorld/world/identity_test.go`

**Interfaces:**
- Produces: `type ID string`
- Produces: `type Epoch uint64`
- Produces: `NewState(id ID) (*State, error)`
- Produces: `(*State).CurrentEpoch() Epoch`
- Produces: `(*State).AdvanceEpoch() Epoch`
- Produces: `(*State).ValidateEpoch(Epoch) error`
- Sentinel errors: `ErrInvalidWorld`, `ErrInvalidEpoch`, `ErrStaleEpoch`

- [ ] **Step 1: Write failing tests** for empty world IDs, initial epoch 1, monotonic advance, stale rejection, and future epoch rejection.
- [ ] **Step 2: Run** `go test ./world` and verify RED.
- [ ] **Step 3: Implement thread-safe in-memory state** with a mutex and no decrement/restore API.
- [ ] **Step 4: Re-run tests** and verify GREEN.

### Task 3: Authority Broker and External-Effect Ledger

**Files:**
- Create: `NolaneWorld/authority/types.go`
- Create: `NolaneWorld/authority/ledger.go`
- Create: `NolaneWorld/authority/broker.go`
- Create: `NolaneWorld/authority/broker_test.go`

**Interfaces:**
- Consumes: `world.ID`, `world.Epoch`, epoch validation
- Produces: `Intent`, `Decision`, `Receipt`
- Produces: `Policy` interface with `Evaluate(context.Context, Intent) (Decision, error)`
- Produces: `Executor` interface with `Execute(context.Context, Intent) ([]byte, error)`
- Produces: `Ledger` interface and `MemoryLedger`
- Produces: `Broker.Execute(context.Context, Intent) (Receipt, error)`
- Sentinel errors: `ErrInvalidAction`, `ErrActionCollision`, `ErrDenied`, `ErrPolicyFailure`, `ErrExecutionFailure`

- [ ] **Step 1: Write failing tests** proving one execution, exact retry idempotency, altered-payload collision denial, stale-epoch denial, policy-deny no execution, policy-error fail-closed, and cross-world isolation.
- [ ] **Step 2: Run** `go test ./authority` and verify RED.
- [ ] **Step 3: Implement canonical request digest** from length-prefixed fields so concatenation ambiguity is impossible.
- [ ] **Step 4: Implement ledger collision semantics and broker sequence** exactly as specified.
- [ ] **Step 5: Re-run** `go test ./authority` and then `go test ./...`; verify GREEN.

### Task 4: Capability Candidate, Independent Promotion, Registry

**Files:**
- Create: `NolaneWorld/capability/types.go`
- Create: `NolaneWorld/capability/registry.go`
- Create: `NolaneWorld/capability/registry_test.go`

**Interfaces:**
- Produces: `Candidate`, `PromotionRequest`, `PromotionReceipt`, `Record`
- Produces: `Digest([]byte) string`
- Produces: `Registry.Promote(request PromotionRequest) (PromotionReceipt, error)`
- Produces: `Registry.Get(name, version string) (Record, bool)`
- Sentinel errors: `ErrInvalidCandidate`, `ErrSelfPromotion`, `ErrDigestMismatch`, `ErrCapabilityCollision`

- [ ] **Step 1: Write failing tests** for valid promotion, self-promotion rejection, content mismatch rejection, missing verifier evidence, immutable same-version content, and exact duplicate promotion idempotency.
- [ ] **Step 2: Run** `go test ./capability` and verify RED.
- [ ] **Step 3: Implement content-addressed records and promotion receipts** bound to origin world and verification digest.
- [ ] **Step 4: Re-run tests** and verify GREEN.

### Task 5: Artifact Gate

**Files:**
- Create: `NolaneWorld/artifact/gate.go`
- Create: `NolaneWorld/artifact/gate_test.go`

**Interfaces:**
- Produces: `Gate{MaxBytes int64}`
- Produces: `Envelope`, `Receipt`
- Produces: `Gate.Accept(world.ID, logicalName, mediaType string, content []byte) (Receipt, error)`
- Sentinel errors: `ErrInvalidArtifact`, `ErrArtifactTooLarge`

- [ ] **Step 1: Write failing tests** for valid relative names, absolute paths, `..` traversal, NUL bytes, empty content, size limit, and exact-content digest changes.
- [ ] **Step 2: Run** `go test ./artifact` and verify RED.
- [ ] **Step 3: Implement normalization and SHA-256 receipts** without executing or unpacking artifact content.
- [ ] **Step 4: Re-run tests** and verify GREEN.

### Task 6: Substrate Boundary and CI Gate

**Files:**
- Create: `NolaneWorld/substrate/substrate.go`
- Create: `NolaneWorld/substrate/substrate_test.go`
- Create: `NolaneWorld/README.md`
- Create: `.github/workflows/nolane-world-check.yml`

**Interfaces:**
- Produces: `SandboxSubstrate` with `Create`, `Destroy`, `Pause`, `Resume`, `Snapshot`, `Rollback`, `Clone`
- Produces opaque `Handle` and `Snapshot` types that contain no trust-plane credential/policy state.

- [ ] **Step 1: Write compile-time/test contract** proving a fake substrate can implement the interface without importing Cube internals.
- [ ] **Step 2: Run** `go test ./substrate` and verify RED before the interface exists.
- [ ] **Step 3: Implement minimal substrate types/interface and module README** documenting that Cube wiring is intentionally separate.
- [ ] **Step 4: Add CI workflow** running `go test ./...` and `go vet ./...` from `NolaneWorld/`.
- [ ] **Step 5: Run complete verification:** `go test ./...`, `go vet ./...`.
- [ ] **Step 6: Inspect diff for forbidden upstream-core changes**; only `NolaneWorld/`, `docs/superpowers/`, and the new workflow may differ in this milestone.
