# State Continuity / Evidence Fusion v10 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fuse the existing v5 live snapshot proof and v9 live Realm proof produced by one immutable Cube driver into a deterministic v10 receipt and an exact-bound host capability evidence source that can safely verify snapshot/rollback.

**Architecture:** Add a new `gauntlet/live/capabilityproof` package that orchestrates existing v5 and v9 runners through the same `live.Driver`, locks the driver's endpoint/template fingerprint, embeds and verifies both nested reports, and derives only conservative capability facts. Add a host-only exact-bound evidence adapter, a CLI, deterministic CI negative control, and documentation without changing any v4–v9 report schema or canonical evidence family.

**Tech Stack:** Go 1.23, existing `NolaneWorld/gauntlet/live`, `realmproof`, `agentruntime`, Cube live driver, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-08-31-nolane-sandbox-state-continuity-evidence-fusion-v10-design.md`

## Global Constraints

- `UNAVAILABLE != PASS`.
- Human DCO is optional; AI commits MUST carry `Autonomously-by: ChatGPT:GPT-5.6-Sol` and MUST NOT fabricate `Signed-off-by`.
- V4–v9 report schemas and canonical evidence bytes are immutable for this wave.
- V10 MUST NOT expose Cube API keys, envd/traffic tokens, sandbox handles, endpoint URLs, template IDs, WorldIDs, or raw target plaintext.
- V10 MUST NOT infer public read, filesystem isolation, process isolation, resource enforcement, or universal private mesh.
- `Verified` requires a sealed v10 `LIVE_PASS` plus exact Realm ID/revision/policy binding.

---

### Task 1: RED — Composite report and runner contracts

**Files:**
- Create: `NolaneWorld/gauntlet/live/capabilityproof/types.go`
- Create: `NolaneWorld/gauntlet/live/capabilityproof/runner_test.go`
- Create: `NolaneWorld/gauntlet/live/capabilityproof/report_test.go`

**Interfaces:**
- Consumes: `live.Driver`, `live.Report`, `realmproof.Report`, `realm.NetworkProfile`, `live.Target`.
- Produces:
  - `type Runner struct { Mode live.Mode; Profile realm.NetworkProfile; RawPublicTarget live.Target }`
  - `func (Runner) Run(context.Context, live.Driver) (Report, error)`
  - `func VerifyReport(Report) error`
  - `func MarshalReport(Report, ...string) ([]byte, error)`

- [ ] **Step 1: Define the public v10 types needed by tests**

```go
type ReasonCode string

const (
    ReasonNone                 ReasonCode = "none"
    ReasonConfigMissing        ReasonCode = "config_missing"
    ReasonFingerprintInvalid   ReasonCode = "fingerprint_invalid"
    ReasonEvidenceMismatch     ReasonCode = "evidence_mismatch"
    ReasonSubstrateUnavailable ReasonCode = "substrate_unavailable"
    ReasonRealmUnavailable     ReasonCode = "realm_unavailable"
    ReasonSubstrateFailed      ReasonCode = "substrate_failed"
    ReasonRealmFailed          ReasonCode = "realm_failed"
)

type Capabilities struct {
    GuestExecution       bool `json:"guest_execution"`
    SnapshotRollback     bool `json:"snapshot_rollback"`
    PublicIngressDenied  bool `json:"public_ingress_denied"`
    NetworkIsolation     bool `json:"network_isolation"`
    InternalMeshVerified bool `json:"internal_mesh_verified"`
}

type Report struct {
    SchemaVersion  int                  `json:"schema_version"`
    Profile        realm.NetworkProfile `json:"profile"`
    Mode           live.Mode            `json:"mode"`
    Substrate      string               `json:"substrate"`
    Status         live.Status          `json:"status"`
    Reason         ReasonCode           `json:"reason"`
    Approved       bool                 `json:"approved"`
    EndpointDigest string               `json:"endpoint_digest,omitempty"`
    TemplateDigest string               `json:"template_digest,omitempty"`
    SubstrateProof live.Report          `json:"substrate_proof"`
    RealmProof     realmproof.Report    `json:"realm_proof"`
    Capabilities   Capabilities         `json:"capabilities"`
    Digest         string               `json:"digest"`
}
```

- [ ] **Step 2: Write RED runner tests**

Cover all of these exact behaviors:

```go
func TestRunnerNilDriverIsUnavailable(t *testing.T)
func TestRunnerRequireLiveRejectsUnavailable(t *testing.T)
func TestRunnerRejectsEmptyConfiguredFingerprint(t *testing.T)
func TestRunnerPropagatesSubstrateFailure(t *testing.T)
func TestRunnerPropagatesRealmFailure(t *testing.T)
func TestRunnerRejectsFingerprintMismatch(t *testing.T)
func TestRunnerPassesOnlyWhenBothNestedProofsPass(t *testing.T)
```

Use a fake driver implementing the same live interfaces already used by v5/v9 tests. The PASS fixture must prove snapshot restoration and Realm network semantics, not return pre-sealed reports from caller input.

- [ ] **Step 3: Write RED report tests**

```go
func TestVerifyReportRejectsTamperedNestedSubstrateProof(t *testing.T)
func TestVerifyReportRejectsTamperedNestedRealmProof(t *testing.T)
func TestVerifyReportRejectsForgedCapabilityBits(t *testing.T)
func TestVerifyReportRejectsFingerprintMismatch(t *testing.T)
func TestMarshalReportIsDeterministicAndSecretSafe(t *testing.T)
```

- [ ] **Step 4: Run focused tests and confirm RED**

Run:

```bash
cd NolaneWorld
go test ./gauntlet/live/capabilityproof -count=1
```

Expected: compile/test failure because runner/report implementation is absent.

- [ ] **Step 5: Commit RED proof**

```bash
git add NolaneWorld/gauntlet/live/capabilityproof
git commit -m "test(nolane): prove v10 evidence-fusion contracts

Autonomously-by: ChatGPT:GPT-5.6-Sol"
```

---

### Task 2: GREEN — Same-driver runner and deterministic composite report

**Files:**
- Modify: `NolaneWorld/gauntlet/live/capabilityproof/types.go`
- Create: `NolaneWorld/gauntlet/live/capabilityproof/report.go`
- Create: `NolaneWorld/gauntlet/live/capabilityproof/runner.go`
- Modify tests from Task 1.

**Interfaces:**
- Consumes: the Task 1 public types and existing `live.Runner`, `realmproof.Runner`.
- Produces a fully self-verifying `Report` that embeds the two nested sanitized reports.

- [ ] **Step 1: Implement domain-separated hashing and report sealing**

Use length-prefixed SHA-256 fields under new v10 domains only:

```go
func proofHash(domain string, fields ...string) string
func reportDigest(r Report) string
func sealReport(r *Report) error
func VerifyReport(r Report) error
func MarshalReport(r Report, forbidden ...string) ([]byte, error)
```

`VerifyReport` MUST call both `live.VerifyReport(r.SubstrateProof)` and `realmproof.VerifyReport(r.RealmProof)`.

- [ ] **Step 2: Derive capability bits from nested proof facts, never caller booleans**

For `LIVE_PASS`, require:

```go
r.SubstrateProof.Status == live.StatusLivePass
r.SubstrateProof.Approved
r.SubstrateProof.Capabilities.GuestExecution
r.SubstrateProof.Capabilities.SnapshotRollback
r.SubstrateProof.Capabilities.CleanupObserved
r.RealmProof.Status == live.StatusLivePass
r.RealmProof.Approved
r.RealmProof.Capabilities.GuestExecution
r.RealmProof.Capabilities.RawPublicDenied
r.RealmProof.Capabilities.PublicIngressDenied
```

Set v10 capability bits only from those verified nested facts. Any mismatch between stored bits and derived bits is invalid.

- [ ] **Step 3: Implement the same-driver runner**

Algorithm:

```go
func (r Runner) Run(ctx context.Context, driver live.Driver) (Report, error) {
    mode := normalizedMode(r.Mode)
    profile := normalizedProfile(r.Profile)

    if driver == nil {
        // Run deterministic nil-driver nested probes and return sealed UNAVAILABLE.
    }

    fp := driver.Fingerprint()
    if fp.EndpointDigest == "" || fp.TemplateDigest == "" {
        // LIVE_FAIL / fingerprint_invalid.
    }

    substrateReport, substrateErr := (live.Runner{
        Mode: live.ModeProbe,
        Profile: live.ProfileCore,
    }).Run(ctx, driver)

    realmReport, realmErr := (realmproof.Runner{
        Mode: live.ModeProbe,
        Profile: profile,
        RawPublicTarget: r.RawPublicTarget,
    }).Run(ctx, driver)

    // Validate reports, classify LIVE_FAIL before UNAVAILABLE,
    // bind endpoint/template digests to the pre-run fingerprint,
    // derive capabilities, seal, then apply outer mode error semantics.
}
```

Nested runners use probe mode so v10 can always inspect and seal valid unavailable receipts; outer mode owns `require-live` behavior.

- [ ] **Step 4: Preserve exact status precedence**

Order:

1. invalid/tampered evidence or fingerprint mismatch -> `LIVE_FAIL`;
2. nested `LIVE_FAIL` -> `LIVE_FAIL`;
3. nested `UNAVAILABLE` -> `UNAVAILABLE`;
4. both nested `LIVE_PASS` -> `LIVE_PASS`.

- [ ] **Step 5: Run focused tests**

```bash
cd NolaneWorld
go test ./gauntlet/live/capabilityproof -count=1
```

Expected: PASS.

- [ ] **Step 6: Run live-package regression**

```bash
cd NolaneWorld
go test ./gauntlet/live/... -count=1
```

Expected: PASS with no v5/v9 test changes required.

- [ ] **Step 7: Commit GREEN runner/report**

```bash
git add NolaneWorld/gauntlet/live/capabilityproof
git commit -m "feat(nolane): add v10 same-driver capability proof

Autonomously-by: ChatGPT:GPT-5.6-Sol"
```

---

### Task 3: RED/GREEN — Exact-bound v10 capability evidence source

**Files:**
- Create: `NolaneWorld/gauntlet/live/capabilityproof/evidence_source_test.go`
- Create: `NolaneWorld/gauntlet/live/capabilityproof/evidence_source.go`

**Interfaces:**
- Consumes: verified v10 `Report`, `agentruntime.CapabilityEvidenceQuery`.
- Produces:

```go
type CapabilityEvidenceBinding struct {
    RealmID       realm.ID
    RealmRevision uint64
    PolicyDigest  string
}

func NewCapabilityEvidenceSource(
    report Report,
    binding CapabilityEvidenceBinding,
) (agentruntime.CapabilityEvidenceSource, error)
```

- [ ] **Step 1: Write RED evidence tests**

```go
func TestEvidenceSourceRejectsUnavailableReport(t *testing.T)
func TestEvidenceSourceRejectsTamperedReport(t *testing.T)
func TestEvidenceSourceRejectsInvalidBinding(t *testing.T)
func TestEvidenceSourceReturnsNothingForBindingMismatch(t *testing.T)
func TestEvidenceSourceProjectsSnapshotNetworkAndGuestProof(t *testing.T)
func TestEvidenceSourceDoesNotInventPublicReadFilesystemProcessOrResourceClaims(t *testing.T)
func TestEvidenceSourceCannotUpgradeInternalMeshWithoutV9MeshPass(t *testing.T)
```

- [ ] **Step 2: Confirm RED**

```bash
cd NolaneWorld
go test ./gauntlet/live/capabilityproof -run EvidenceSource -count=1
```

Expected: FAIL because constructor is absent.

- [ ] **Step 3: Implement immutable exact-bound source**

The stored query is exactly:

```go
agentruntime.CapabilityEvidenceQuery{
    RealmID: binding.RealmID,
    RealmRevision: binding.RealmRevision,
    PolicyDigest: binding.PolicyDigest,
}
```

The snapshot attestation sets only:

```go
GuestExecAvailable: true
GuestExecVerified: true
SnapshotAvailable: true
SnapshotVerified: true
PublicInboundDisabled: true
NetworkIsolationVerified: true
```

and optional internal mesh when the nested v9 report genuinely proves it.

Evidence values are digest references such as:

```text
live-capability-v10:snapshot:<digest>
live-capability-v10:network:<digest>
```

No mutable setter or registration method is added.

- [ ] **Step 4: Run focused and agentruntime integration tests**

```bash
cd NolaneWorld
go test ./gauntlet/live/capabilityproof ./agentruntime -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit evidence source**

```bash
git add NolaneWorld/gauntlet/live/capabilityproof
git commit -m "feat(nolane): bind v10 snapshot evidence to Realm truth

Autonomously-by: ChatGPT:GPT-5.6-Sol"
```

---

### Task 4: RED/GREEN — V10 CLI and fail-honest negative control

**Files:**
- Create: `NolaneWorld/cmd/nolane-capability-gauntlet-live/main.go`
- Create: `NolaneWorld/cmd/nolane-capability-gauntlet-live/main_test.go`

**Interfaces:**
- Consumes: Cube environment/config, v10 `Runner` and `MarshalReport`.
- Produces canonical JSON on stdout or `--out` with stable exit semantics.

- [ ] **Step 1: Write RED CLI tests**

Cover:

```go
func TestProbeWithoutCubeConfigWritesUnavailable(t *testing.T)
func TestRequireLiveWithoutCubeConfigReturnsTwo(t *testing.T)
func TestInvalidModeReturnsTwo(t *testing.T)
func TestInvalidProfileReturnsTwo(t *testing.T)
func TestInvalidTargetKindReturnsTwo(t *testing.T)
func TestCredentialEncodingsAreRejectedFromEvidence(t *testing.T)
```

- [ ] **Step 2: Implement CLI**

Follow `cmd/nolane-realm-gauntlet-live` conventions. Construct exactly one `livecube.Driver`; pass that same object to v10 `Runner`.

Exit codes:

- `0`: `LIVE_PASS`, or probe-mode `UNAVAILABLE` written successfully;
- `2`: invalid CLI input or require-live `UNAVAILABLE`;
- `1`: `LIVE_FAIL`, evidence verification failure, or write failure.

- [ ] **Step 3: Run CLI package tests**

```bash
cd NolaneWorld
go test ./cmd/nolane-capability-gauntlet-live -count=1
```

Expected: PASS.

- [ ] **Step 4: Generate negative control twice locally/CI-equivalent**

```bash
cd NolaneWorld
NOLANE_CUBE_API_KEY=SYNTHETIC-V10-CUBE-CREDENTIAL \
  go run ./cmd/nolane-capability-gauntlet-live \
  --mode probe \
  --profile R0_INTERNAL_ONLY \
  --raw-public-kind http \
  --raw-public-target https://example.invalid/nolane-v10-negative-control \
  --out release-evidence/nolane-capability-live-v10-unavailable.json
```

Expected JSON: `status=UNAVAILABLE`, `approved=false`, no synthetic credential encoding.

- [ ] **Step 5: Commit CLI**

```bash
git add NolaneWorld/cmd/nolane-capability-gauntlet-live
git commit -m "feat(nolane): expose v10 capability proof CLI

Autonomously-by: ChatGPT:GPT-5.6-Sol"
```

---

### Task 5: CI, documentation, nondrift, and closure

**Files:**
- Modify: `.github/workflows/nolane-world-check.yml`
- Modify: `.github/workflows/nolane-live-gauntlet.yml`
- Create: `NolaneWorld/STATE-CONTINUITY-EVIDENCE-FUSION-V10.md`
- Modify: `NolaneWorld/README.md`

**Interfaces:**
- Consumes all v10 code and existing release generators.
- Produces deterministic v10 negative-control artifact and release documentation.

- [ ] **Step 1: Extend World Check**

Add a step after v9 generation that runs v10 twice, `cmp`s the files, requires `UNAVAILABLE` and `approved=false`, scans these exact synthetic forms, then removes the repeat file:

```text
SYNTHETIC-V10-CUBE-CREDENTIAL
U1lOVEhFVElDLVYxMC1DVUJFLUNSRURFTlRJQUw=
53594e5448455449432d5631302d435542452d43524544454e5449414c
```

Upload `release-evidence/nolane-capability-live-v10-unavailable.json` as `nolane-capability-live-v10-unavailable-${{ github.sha }}`.

- [ ] **Step 2: Extend live harness workflow**

Include:

```bash
go test -race ./gauntlet/live/... ./cmd/nolane-gauntlet-live ./cmd/nolane-realm-gauntlet-live ./cmd/nolane-capability-gauntlet-live ./substrate/cube
go vet ./gauntlet/live/... ./cmd/nolane-gauntlet-live ./cmd/nolane-realm-gauntlet-live ./cmd/nolane-capability-gauntlet-live ./substrate/cube
```

Add v10 paths to the PR path filter. Do not enable a false live PASS on GitHub-hosted runners.

- [ ] **Step 3: Document v10 truth boundaries**

`STATE-CONTINUITY-EVIDENCE-FUSION-V10.md` must state:

- same-driver correlation mechanism;
- nested v5/v9 evidence requirements;
- exact Realm binding;
- snapshot rollback semantics and stale-authority non-rewind;
- fail-honest statuses;
- capability truth table;
- CLI/CI meaning;
- explicit non-claims.

Update README with a concise v10 section and link.

- [ ] **Step 4: Run full verification**

```bash
cd NolaneWorld
go test ./...
go test -race ./...
go vet ./...
```

Then regenerate v4/v6/v7/v8/v9 evidence using the existing workflow commands and compare historical canonical bytes/hashes. Generate v10 twice and compare byte-for-byte.

- [ ] **Step 5: Commit closure changes**

```bash
git add .github/workflows/nolane-world-check.yml .github/workflows/nolane-live-gauntlet.yml NolaneWorld/README.md NolaneWorld/STATE-CONTINUITY-EVIDENCE-FUSION-V10.md
git commit -m "ci(nolane): close State Continuity Evidence Fusion v10

Autonomously-by: ChatGPT:GPT-5.6-Sol"
```

- [ ] **Step 6: Preserve RED/GREEN history and compact candidate**

Before compaction, create a backup branch pointing to the full implementation head. Build a single direct-child candidate of current `master` with exactly the final tree and one provenance trailer. Force-move only the feature branch, never `master`.

- [ ] **Step 7: Open/refresh PR and require exact-head Actions**

The compact candidate must pass:

- contribution provenance gate;
- Nolane World Check;
- live harness semantics;
- Docs Build Check;
- Format Check amd64/arm64.

Do not treat unrelated Claude reviewer infrastructure failures as code approval or rejection.

- [ ] **Step 8: Audit exact-head artifacts**

Download the v10 artifact and verify:

- exact branch/head binding;
- `UNAVAILABLE` / `approved=false` negative-control status;
- deterministic byte identity expectations;
- no plaintext/base64/hex synthetic credential;
- v4–v9 nondrift.

- [ ] **Step 9: Integrate and verify master**

After exact-head gates are green and review surface has no blocking findings, integrate by fast-forward when the compact commit is a direct child of unchanged `master`. Confirm the PR is `merged=true`, `master` points to the exact v10 SHA, and the fresh push-triggered World Check on that SHA succeeds.
