# State Continuity / Evidence Fusion v10 Implementation Plan

> **Execution method:** Superpowers planning + TDD + systematic debugging + exact-head verification.

**Goal:** Fuse the existing v5 live snapshot proof and v9 live Realm proof produced by one immutable Cube driver into a deterministic v10 receipt and an exact-bound host capability evidence source that can safely verify snapshot/rollback.

**Architecture:** `gauntlet/live/capabilityproof` orchestrates v5 and v9 through the same `live.Driver`, locks the driver's endpoint/template fingerprint, embeds and verifies both nested reports, derives only conservative capability facts, and exposes an immutable exact-bound `CapabilityEvidenceSource`. A dedicated CLI and two CI surfaces provide fail-honest negative-control evidence without changing v4-v9 evidence schemas.

**Spec:** `docs/superpowers/specs/2026-08-31-nolane-sandbox-state-continuity-evidence-fusion-v10-design.md`

## Global constraints

- `UNAVAILABLE != PASS`.
- Human DCO is optional; AI commits carry `Autonomously-by: ChatGPT:GPT-5.6-Sol` and never fabricate `Signed-off-by`.
- V4-v9 report schemas and canonical evidence bytes are immutable for this wave.
- V10 evidence must not expose Cube API keys, envd/traffic tokens, sandbox handles, endpoint URLs, template IDs, WorldIDs, or raw target plaintext.
- V10 does not infer public read, filesystem isolation, process isolation, resource enforcement, or universal private mesh.
- `Verified` requires a sealed v10 `LIVE_PASS` plus exact Realm ID/revision/policy binding.

---

## Task 1 — Composite contract, RED first

**Files:**
- `NolaneWorld/gauntlet/live/capabilityproof/types.go`
- `NolaneWorld/gauntlet/live/capabilityproof/runner_test.go`
- `NolaneWorld/gauntlet/live/capabilityproof/report_test.go`

- [x] Define v10 report/status/reason/capability types.
- [x] Add fake live driver that materially exercises v5 snapshot state restoration and v9 Realm behavior instead of returning prebuilt reports.
- [x] Add RED tests for nil driver, require-live unavailability, empty fingerprint, substrate failure, Realm failure, fingerprint rotation, dual-pass behavior, nested tamper, forged capability bits, and deterministic secret-safe marshaling.
- [x] Prove RED in GitHub Actions. Failure was exactly missing `Runner.Run`, `VerifyReport`, and `MarshalReport`; existing v5/Cube/v9 packages remained green.

---

## Task 2 — Same-driver runner and deterministic report

**Files:**
- `NolaneWorld/gauntlet/live/capabilityproof/report.go`
- `NolaneWorld/gauntlet/live/capabilityproof/runner.go`

- [x] Add length-prefixed SHA-256 v10 domains.
- [x] Verify both nested evidence families with their existing verifiers.
- [x] Require nested v5 core and v9 Realm reports to be probe-mode evidence while the outer v10 mode owns require-live semantics.
- [x] Lock one pre-run driver fingerprint.
- [x] Require v5 endpoint + template digests and v9 endpoint digest to match the lock.
- [x] Fail closed on fingerprint rotation/rebind.
- [x] Derive capability bits only from verified nested facts.
- [x] Clear all capability bits from every non-PASS composite receipt.
- [x] Preserve precedence: evidence mismatch/failure before unavailability.
- [x] Prove GREEN with live-package race tests and vet.

---

## Task 3 — Exact-bound immutable capability evidence

**Files:**
- `NolaneWorld/gauntlet/live/capabilityproof/evidence_source_test.go`
- `NolaneWorld/gauntlet/live/capabilityproof/evidence_source.go`

- [x] Add RED tests for unavailable/tampered reports, invalid bindings, exact mismatch, snapshot/network/guest projection, no invented claims, and no unsupported mesh upgrade.
- [x] Prove RED in Actions with only the constructor/binding symbols absent.
- [x] Require a sealed v10 `LIVE_PASS` and valid Realm binding at construction.
- [x] Bind to exact Realm ID, nonzero Realm revision, and canonical policy digest.
- [x] Require PASS scenario evidence from v5 guest execution + snapshot authority and v9 guest-after-profile + raw-public denial + public-ingress denial.
- [x] Project only guest execution, snapshot rollback, public-inbound-disabled, observed network isolation, and optional genuinely verified mesh.
- [x] Use digest-only `live-capability-v10:*` evidence references.
- [x] Expose no mutable setter/registration authority.
- [x] Prove GREEN with race tests, vet, and missing-infrastructure negative control.

---

## Task 4 — V10 CLI

**Files:**
- `NolaneWorld/cmd/nolane-capability-gauntlet-live/main_test.go`
- `NolaneWorld/cmd/nolane-capability-gauntlet-live/main.go`

- [x] Add RED tests for probe-mode unavailability, require-live exit semantics, invalid mode/profile/target kind, plaintext/base64/hex credential exclusion, and atomic `--out` behavior.
- [x] Prove RED in full World Check; `capabilityproof` itself stayed green.
- [x] Parse host-owned Cube configuration and validate semantic flags.
- [x] Construct at most one Cube live driver and pass that exact object to the v10 runner.
- [x] Emit verified canonical JSON.
- [x] Use exit code 0 for PASS or probe-mode UNAVAILABLE, 2 for usage/require-live UNAVAILABLE, and 1 for LIVE_FAIL/evidence/output errors.
- [x] Scan caller-provided credential plaintext/base64/hex before evidence output.
- [x] Write files atomically.
- [x] Prove GREEN with `go test ./...`, `go test -race ./...`, `go vet ./...`, and existing v4-v9 generators.

---

## Task 5 — CI, release documentation, nondrift, and integration

**Files:**
- `.github/workflows/nolane-world-check.yml`
- `.github/workflows/nolane-live-gauntlet.yml`
- `NolaneWorld/STATE-CONTINUITY-EVIDENCE-FUSION-V10.md`

- [x] Extend Live Substrate harness path filters to v9/v10 packages, CLIs, specs, plans, and release doc.
- [x] Race-test and vet v5/v9/v10 live packages and all three live CLIs.
- [x] Generate v5 and v10 missing-infrastructure negative controls on ordinary GitHub-hosted runners and require `UNAVAILABLE`, `approved=false`, `config_missing`.
- [x] Keep the self-hosted live/KVM job explicit and manual; do not manufacture a live v10 PASS on hosted runners.
- [x] Extend World Check with deterministic v10 negative-control generation twice + byte comparison.
- [x] Require v10 `UNAVAILABLE`, `approved=false`, `config_missing` in CI.
- [x] Scan exact plaintext/base64/hex forms of `SYNTHETIC-V10-CUBE-CREDENTIAL`.
- [x] Upload commit-bound v10 negative-control evidence without changing v4-v9 generator commands.
- [x] Document same-driver correlation, nested evidence requirements, exact Realm binding, snapshot authority non-rewind, status semantics, capability truth table, CLI/CI meaning, historical nondrift, and explicit non-claims.
- [x] Full World Check is GREEN through unit/race/vet, v4-v9 generation, v10 generation, and all evidence uploads.
- [x] Live harness is GREEN through race/vet and both missing-infrastructure negative controls.
- [ ] Docs Build Check GREEN after removing VitePress-sensitive GitHub Actions moustache syntax from this plan.
- [ ] Format Check GREEN on amd64/arm64.
- [ ] Audit exact-head v4-v10 artifacts and historical canonical hashes.
- [ ] Preserve full RED/GREEN history on a backup branch.
- [ ] Compact final feature candidate to one direct-child commit of unchanged `master` with the exact final tree.
- [ ] Require fresh exact-head provenance, World, Live, Docs, and Format gates on the compact candidate.
- [ ] Audit compact-candidate artifacts again.
- [ ] Confirm no blocking review findings.
- [ ] Integrate by fast-forward if `master` is unchanged and the candidate is its direct child.
- [ ] Confirm PR #13 is merged and `master` points to the exact v10 commit.
- [ ] Require fresh post-merge World Check success on the exact master SHA.

## Release evidence names

Historical artifacts retain their existing names. The new v10 negative-control artifact is conceptually named:

```text
nolane-capability-live-v10-unavailable-<exact-ci-sha>
```

The placeholder above is intentionally documentation-safe; the real GitHub Actions workflow uses its native commit-SHA expression directly.

## Closure rule

Do not call v10 complete merely because code compiles or deterministic CI is green. Closure requires:

1. exact-head technical gates green;
2. exact-head artifact verification and historical nondrift;
3. one compact direct-child integration candidate;
4. merged `master` at the exact candidate SHA;
5. fresh post-merge verification.
