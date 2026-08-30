# Nolane Sandbox Live Realm Proof v9 — Implementation Plan

> **Execution:** follow Superpowers TDD. Every behavior change begins with a failing contract test. Do not weaken `UNAVAILABLE != PASS`. Do not add AI DCO sign-offs.

**Goal:** turn live Cube Realm-profile observations into deterministic, exact-bound host capability evidence without exposing substrate credentials or mutable truth authority to agents.

**Base:** stacked on `nolane/capability-truth-authority@7b32ef5e59c7e90c1369539f5a3a912f13868330` until PR #11 lands.

## Task 1 — Realm live report contract

**Files**
- Create: `NolaneWorld/gauntlet/live/realmproof/types.go`
- Create: `NolaneWorld/gauntlet/live/realmproof/report.go`
- Create: `NolaneWorld/gauntlet/live/realmproof/report_test.go`

**RED contracts**
- deterministic marshal/hash for identical reports;
- approved report requires mandatory profile/guest/raw-egress-denial/public-ingress-denial PASS evidence;
- missing mandatory evidence is `UNAVAILABLE`, never PASS;
- optional mesh unavailable cannot become Verified;
- malformed/tampered digest is rejected;
- report contains no raw endpoint, sandbox handle, token or target fields.

**GREEN implementation**
- schema v1 with profile, status, reason, mandatory capabilities, optional mesh outcome, scenario evidence, endpoint/target digests only;
- canonical seal and `VerifyReport`.

**Verify**
- `go test ./gauntlet/live/realmproof`

## Task 2 — Realm-aware live runner

**Files**
- Create: `NolaneWorld/gauntlet/live/realmproof/runner.go`
- Create: `NolaneWorld/gauntlet/live/realmproof/runner_test.go`

**RED contracts**
- nil/unhealthy driver -> UNAVAILABLE;
- missing mandatory optional interface support -> UNAVAILABLE;
- profile apply failure -> LIVE_FAIL;
- guest post-profile failure -> LIVE_FAIL;
- raw public target reachable -> LIVE_FAIL;
- unauthenticated public ingress accepted -> LIVE_FAIL;
- all mandatory observations deny/reach as expected -> LIVE_PASS;
- mesh unsupported -> mesh UNAVAILABLE while mandatory profile report may still pass;
- real private mesh pass -> mesh capability true;
- cleanup uncertainty -> LIVE_FAIL.

**GREEN implementation**
- preserve existing v5 `live.Driver` compatibility;
- add local optional interfaces through type assertions rather than changing v5 Driver/Sandbox;
- preflight target before sandbox mutation;
- always destroy observed live boxes.

**Verify**
- `go test ./gauntlet/live/realmproof`
- `go test ./gauntlet/live/...`

## Task 3 — Cube Realm proof driver

**Files**
- Modify: `NolaneWorld/gauntlet/live/cube/driver.go`
- Modify/Create tests: `NolaneWorld/gauntlet/live/cube/realm_test.go`
- Modify if necessary: `NolaneWorld/substrate/cube/live.go`

**RED contracts**
- `ApplyRealmProfile` uses the exact existing Cube network update path;
- ingress canary starts inside exact sandbox on bounded fixed port;
- external ingress probe deliberately sends neither envd nor traffic token;
- HTTP 403 means restricted public ingress observed;
- 2xx/3xx reaching canary means policy violation;
- connection/control ambiguity is UNAVAILABLE, not denial PASS;
- raw egress negative probe reuses hardened command builder and distinguishes unreachable vs unsupported.

**GREEN implementation**
- expose profile application on Cube live box without leaking handle;
- add `ProbeRawPublic` and `ProbePublicIngressDenied` host-owned driver methods;
- keep traffic/envd secrets private in `GuestSession`.

**Verify**
- `go test ./gauntlet/live/cube ./substrate/cube`

## Task 4 — Immutable host evidence bridge

**Files**
- Create: `NolaneWorld/agentruntime/realm_evidence.go`
- Create: `NolaneWorld/agentruntime/realm_evidence_test.go`

**RED contracts**
- only a sealed approved realmproof report can construct a source;
- source binds exact Realm ID/revision/policy digest;
- wrong query returns no evidence or mismatch fail-closed without rebind;
- guest verification maps only from guest PASS;
- public inbound Verified maps only from real ingress-denial PASS;
- network isolation Verified requires both raw-public-denial + ingress-denial PASS;
- mesh Verified requires genuine mesh PASS;
- PublicRead/Snapshot/Filesystem/Process/ResourceEnforcement remain unverified;
- source contains no mutator and is safe for concurrent reads.

**GREEN implementation**
- immutable constructor returning a `CapabilityEvidenceSource` implementation;
- deterministic evidence strings use report/scenario digests, not raw runtime metadata.

**Verify**
- `go test ./agentruntime`
- `go test -race ./agentruntime`

## Task 5 — CLI and negative-control surface

**Files**
- Create: `NolaneWorld/cmd/nolane-realm-gauntlet-live/main.go`
- Create: `NolaneWorld/cmd/nolane-realm-gauntlet-live/main_test.go`

**Behavior**
- profiles: R0/R1/R2;
- modes: probe / require-live;
- explicit raw-public test target;
- output canonical JSON only;
- missing Cube config -> UNAVAILABLE artifact and non-success only in require-live mode;
- no credentials in stdout/artifact.

**Verify**
- `go test ./cmd/nolane-realm-gauntlet-live`

## Task 6 — CI fail-honest integration

**Files**
- Modify: `.github/workflows/nolane-world-check.yml`

**Behavior**
- always run deterministic unit tests;
- run v9 CLI without live configuration as a negative control and assert `UNAVAILABLE` / `approved=false`;
- never treat that negative control as live proof;
- if a future explicit live workflow supplies Cube configuration, `require-live` may gate a live release independently.

**Verify**
- workflow syntax/format checks;
- exact PR CI.

## Task 7 — Documentation and capability truth table

**Files**
- Modify: `README.md`
- Modify/create: `NolaneWorld/README.md` if present/appropriate.

**Document**
- semantic profile vs live observed proof;
- trusted evidence chain;
- claims v9 can and cannot verify;
- mesh remains UNAVAILABLE on Cube until a genuine private sibling route is observable;
- no universal isolation claim.

## Task 8 — Historical nondrift and release closure

**Checks**
1. `go test ./...`
2. `go test -race ./...`
3. `go vet ./...`
4. regenerate v4, v6, v7, v8 deterministic artifacts;
5. compare v4/v6/v7/v8 canonical JSON with pre-v9 known-good evidence where the repository already pins it;
6. verify v9 synthetic report determinism by double generation + byte compare;
7. scan v9 artifact for synthetic/plain/base64/hex credential markers;
8. Format Check amd64 + arm64;
9. inspect PR reviews/threads and changed-file audit.

## Task 9 — Release hygiene

- preserve full RED/GREEN branch before compaction;
- compact final v9 tree to one unsigned AI-authored commit if history is noisy;
- base the stacked PR on `nolane/capability-truth-authority` while PR #11 is open;
- do not merge while human DCO is red;
- after PR #11 lands, rebase/retarget v9 onto fresh `master`, rerun exact-head verification, then leave one human-DCO candidate.
