# Wave 31 Realm Runtime CPU Policy Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional host-owned Realm runtime CPU bandwidth intent in milliCPU and an opaque, freshness-validated authority over that exact current policy, without changing resource-accounting semantics or claiming kernel enforcement.

**Architecture:** Extend `realm.Spec` with one `omitempty` scalar so legacy JSON and `PolicyDigest` remain byte-stable when unset. Add a sealed `RuntimeCPUPolicyAuthority` in package `realm` that can be minted and freshly validated only against exact package-owned `MemoryStore` / `DurableStore` instances, using the existing Realm revision plus full `PolicyDigest` as the policy trust root. Wave31 remains Realm-local and deliberately does not touch Cube, cgroups, Wave30, `resourceproof`, `TrustedReport`, or `LIVE_PASS`.

**Tech Stack:** Go 1.23 standard library, Python 3 static contract tests, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-09-realm-runtime-cpu-policy-authority-v31-design.md`

## Global Constraints

- `ResourceBudget.CPUUnits` remains accounting-only and MUST NOT be mapped or defaulted to milliCPU.
- New field is exactly `RuntimeCPULimitMilliCPU uint64 `json:"runtime_cpu_limit_millicpu,omitempty"``.
- Zero means no runtime CPU kernel-limit intent; legacy Realms remain valid.
- Existing `PolicyDigest` domain/version remains unchanged.
- Every legacy zero-field Spec must preserve canonical JSON and `PolicyDigest` byte-for-byte.
- Runtime CPU policy authority is opaque, sealed, Store-instance-bound, and fresh only after authoritative Realm re-read.
- Only exact package-owned `*MemoryStore` and `*DurableStore` may mint/validate the authority.
- Wave31 does not alter Cube/fabric/provider/Cubelet/resourceproof behavior.
- Wave31 does not prove cgroup propagation, `cpu.max` equivalence, Wave30 causal enforcement, `TrustedReport`, aggregate resource verification, or `LIVE_PASS`.
- Autonomous commits include `Autonomously-by: ChatGPT:GPT-5.6-Sol` and never `Signed-off-by`.

---

### Task 1: Lock compatibility and authority behavior with RED tests

**Files:**
- Create: `NolaneWorld/realm/runtime_cpu_policy_authority_v31_test.go`
- Create: `NolaneWorld/realm/runtime_cpu_policy_authority_external_v31_test.go`
- Create: `docs/superpowers/verification/2026-09-09-wave31-realm-runtime-cpu-policy-red.md`

**Interfaces:**
- Consumes: existing `realm.Spec`, `PolicyDigest`, `MemoryStore`, `DurableStore`, `Controller`.
- Produces desired API:
  - `RuntimeCPUPolicyAuthority`
  - `RuntimeCPUPolicyBinding`
  - `(*Controller).CurrentRuntimeCPUPolicyAuthority(context.Context, ID)`
  - `(*Controller).ValidateRuntimeCPUPolicyAuthority(context.Context, RuntimeCPUPolicyAuthority)`
  - `ErrRuntimeCPUPolicyUnavailable`
  - `ErrInvalidRuntimeCPUPolicyAuthority`
  - `ErrStaleRuntimeCPUPolicyAuthority`

- [ ] **Step 1: Add compatibility RED tests**

Tests must assert:

```go
func TestV31LegacySpecOmitsRuntimeCPULimit(t *testing.T) {
    spec := validTestSpec()
    raw, err := json.Marshal(spec)
    if err != nil { t.Fatal(err) }
    if bytes.Contains(raw, []byte("runtime_cpu_limit_millicpu")) {
        t.Fatalf("legacy spec emitted Wave31 field: %s", raw)
    }
}

func TestV31LegacyPolicyDigestFrozen(t *testing.T) {
    spec := validTestSpec()
    got, err := PolicyDigest(spec, 7)
    if err != nil { t.Fatal(err) }
    const want = "<replace in test with exact pre-Wave31 digest computed from Wave30 source fixture>"
    if got != want { t.Fatalf("legacy policy digest drift: got %q want %q", got, want) }
}

func TestV31PositiveLimitChangesPolicyDigest(t *testing.T) {
    legacy := validTestSpec()
    withLimit := legacy
    withLimit.RuntimeCPULimitMilliCPU = 500
    before, _ := PolicyDigest(legacy, 7)
    after, _ := PolicyDigest(withLimit, 7)
    if before == after { t.Fatal("positive runtime CPU intent must change policy digest") }
}
```

Before committing the test, compute and hard-code the frozen legacy digest from the Wave30 implementation/fixture; do not leave a placeholder literal.

- [ ] **Step 2: Add authority RED tests**

Cover:

```go
func TestV31CurrentRuntimeCPUPolicyAuthorityMemoryStore(t *testing.T)
func TestV31CurrentRuntimeCPUPolicyAuthorityDurableStore(t *testing.T)
func TestV31AccountingCPUUnitsDoNotMintAuthority(t *testing.T)
func TestV31AuthorityDeterministicDigest(t *testing.T)
func TestV31ZeroAuthorityInvalid(t *testing.T)
func TestV31JSONRoundTripCannotRestoreAuthority(t *testing.T)
func TestV31DifferentTrustedStoreInstanceRejected(t *testing.T)
func TestV31RevisionUpdateMakesAuthorityStale(t *testing.T)
func TestV31LimitChangeMakesAuthorityStale(t *testing.T)
func TestV31LimitClearMakesAuthorityStale(t *testing.T)
func TestV31UnrelatedPolicyChangeMakesAuthorityStale(t *testing.T)
func TestV31RealmCloseMakesAuthorityStale(t *testing.T)
func TestV31ContextCancellation(t *testing.T)
```

Use package-private tests for seal/digest tamper and an external-package test for public-surface opacity.

- [ ] **Step 3: Verify RED**

Run through dedicated GitHub Actions after Task 2 workflow exists. Expected compile failure must be specifically missing Wave31 field/types/methods/errors, not YAML/setup/test syntax.

- [ ] **Step 4: Record exact RED lineage**

Write exact test-only SHA, workflow run ID, and missing-symbol failure excerpt to `docs/superpowers/verification/2026-09-09-wave31-realm-runtime-cpu-policy-red.md`.

- [ ] **Step 5: Commit RED tests/evidence**

Commit with autonomous provenance. Production Wave31 code must still be absent.

---

### Task 2: Add static anti-shortcut contract and dedicated workflow

**Files:**
- Create: `tests/wave31_realm_runtime_cpu_policy_contract.py`
- Create: `.github/workflows/wave31-realm-runtime-cpu-policy-contract.yml`

**Interfaces:**
- Consumes: Wave31 source/spec/test paths.
- Produces: a PR/push gate that rejects accounting-to-kernel laundering, public injection shortcuts, and scope expansion.

- [ ] **Step 1: Add static contract**

The Python test must assert exact source invariants:

```python
MODEL = ROOT / "NolaneWorld/realm/model.go"
AUTH = ROOT / "NolaneWorld/realm/runtime_cpu_policy_authority.go"

model = MODEL.read_text()
auth = AUTH.read_text()

assert 'RuntimeCPULimitMilliCPU uint64 `json:"runtime_cpu_limit_millicpu,omitempty"`' in model
assert 'nolane.realm-runtime-cpu-policy.v31\\x00' in auth
assert 'realm-runtime-cpu-policy-v31:' in auth
assert 'CurrentRuntimeCPUPolicyAuthority' in auth
assert 'ValidateRuntimeCPUPolicyAuthority' in auth

for forbidden in [
    "CPUUnits *", "CPUUnits/", "CPUUnits /", "HostPressureRunner",
    "TrustedReport", "LIVE_PASS", "cpu.max", "cgroup.procs",
    "RuntimeCPUThrottleInterventionAuthority", "RuntimeCgroupReadbackAuthority",
]:
    assert forbidden not in auth
```

Also parse exported function signatures sufficiently to ensure minting accepts only `context.Context, ID`, validation accepts only `context.Context, RuntimeCPUPolicyAuthority`, and no exported constructor exists for the authority.

- [ ] **Step 2: Add workflow**

Use path triggers for:

```yaml
- 'NolaneWorld/realm/model.go'
- 'NolaneWorld/realm/runtime_cpu_policy_authority.go'
- 'NolaneWorld/realm/*v31_test.go'
- 'tests/wave31_realm_runtime_cpu_policy_contract.py'
- 'docs/superpowers/specs/2026-09-09-realm-runtime-cpu-policy-authority-v31-design.md'
- 'docs/superpowers/plans/2026-09-09-realm-runtime-cpu-policy-authority-v31.md'
- 'docs/superpowers/verification/2026-09-09-wave31-realm-runtime-cpu-policy-*.md'
```

Jobs/steps must run:

```bash
python -m unittest tests.wave31_realm_runtime_cpu_policy_contract
cd NolaneWorld && go test ./realm -run 'V31' -count=1
cd NolaneWorld && go test ./realm -run 'V23|V31' -count=1
cd NolaneWorld && go vet ./...
cd NolaneWorld && go test ./...
cd NolaneWorld && go test -race ./realm -run 'V31' -count=1
```

- [ ] **Step 3: Commit and observe RED**

The workflow must fail only because Wave31 production symbols/field are absent. If static contract fails before Go due to absent production file, arrange workflow ordering so focused Go RED still yields the intended missing-symbol evidence, or make the static step run after focused Go.

---

### Task 3: Implement the optional Realm runtime CPU intent

**Files:**
- Modify: `NolaneWorld/realm/model.go`

**Interfaces:**
- Consumes: existing `Spec`, existing `PolicyDigest`.
- Produces: optional `RuntimeCPULimitMilliCPU` field while preserving zero-value JSON/digest continuity.

- [ ] **Step 1: Add the minimal field**

Append to `Spec`:

```go
RuntimeCPULimitMilliCPU uint64 `json:"runtime_cpu_limit_millicpu,omitempty"`
```

Do not derive it from `ResourceBudget` and do not make `Spec.Validate()` require it.

- [ ] **Step 2: Run focused compatibility tests**

```bash
cd NolaneWorld
go test ./realm -run 'TestV31Legacy|TestV31PositiveLimit' -count=1
```

Expected: compatibility tests pass; authority tests remain RED until Task 4.

- [ ] **Step 3: Commit field-only GREEN checkpoint**

Commit with autonomous provenance.

---

### Task 4: Implement sealed Runtime CPU Policy Authority

**Files:**
- Create: `NolaneWorld/realm/runtime_cpu_policy_authority.go`

**Interfaces:**
- Consumes: `Controller`, package-owned Store implementations, `PolicyDigest`, positive `RuntimeCPULimitMilliCPU`.
- Produces:

```go
type RuntimeCPUPolicyBinding struct {
    RealmID       ID
    RealmRevision uint64
    PolicyDigest  string
    LimitMilliCPU uint64
    Digest        string
}

type RuntimeCPUPolicyAuthority struct { /* all private */ }

func (a RuntimeCPUPolicyAuthority) Binding() (RuntimeCPUPolicyBinding, bool)
func (c *Controller) CurrentRuntimeCPUPolicyAuthority(ctx context.Context, realmID ID) (RuntimeCPUPolicyAuthority, error)
func (c *Controller) ValidateRuntimeCPUPolicyAuthority(ctx context.Context, authority RuntimeCPUPolicyAuthority) (RuntimeCPUPolicyBinding, error)
```

- [ ] **Step 1: Define errors and seal/store trust root**

Follow the exact `RealizationAuthority` pattern:

```go
var (
    ErrRuntimeCPUPolicyUnavailable = errors.New("realm: runtime CPU policy authority unavailable")
    ErrInvalidRuntimeCPUPolicyAuthority = errors.New("realm: invalid runtime CPU policy authority")
    ErrStaleRuntimeCPUPolicyAuthority = errors.New("realm: stale runtime CPU policy authority")
)
```

Use a dedicated unexported Store marker and an exact type switch that accepts only non-nil `*MemoryStore` or `*DurableStore`.

- [ ] **Step 2: Implement canonical digest**

Use a private fixed-field canonical struct with JSON and exact domain:

```go
const runtimeCPUPolicyDigestPrefix = "realm-runtime-cpu-policy-v31:"
const runtimeCPUPolicyDigestDomain = "nolane.realm-runtime-cpu-policy.v31\x00"
```

Hash exact Realm ID, revision, policy digest, positive milliCPU limit. Validate prefix plus exactly 64 lowercase hex characters and recompute on every `Binding()` / fresh validation.

- [ ] **Step 3: Implement minting**

`CurrentRuntimeCPUPolicyAuthority` must:

```text
ctx check -> exact package-owned Store -> current Realm -> open/revision/id/spec validity -> positive explicit milliCPU -> current PolicyDigest -> canonical Wave31 digest -> seal/store/binding
```

No caller-supplied revision/digest/value.

- [ ] **Step 4: Implement fresh validation**

`ValidateRuntimeCPUPolicyAuthority` must distinguish:

```text
malformed/foreign seal or Store instance -> ErrInvalidRuntimeCPUPolicyAuthority
current same-store Realm drift/close/limit clear/change/revision or policy digest drift -> ErrStaleRuntimeCPUPolicyAuthority
current source structurally unavailable without a previously valid same-store authority only where spec says unavailable
context cancellation -> ctx error
```

- [ ] **Step 5: Run focused tests**

```bash
cd NolaneWorld
go test ./realm -run 'V31' -count=1
go test -race ./realm -run 'V31' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit production GREEN**

Commit with autonomous provenance.

---

### Task 5: Harden durability, external opacity, and regression surfaces

**Files:**
- Modify tests from Task 1 only if review exposes a missing case.
- Do not expand production scope outside `realm/model.go` and `realm/runtime_cpu_policy_authority.go` without new design review.

**Interfaces:**
- Consumes: completed Wave31 authority.
- Produces: proof that MemoryStore/DurableStore and V23 authority behavior do not regress.

- [ ] **Step 1: Run full dedicated gate**

```bash
python -m unittest tests.wave31_realm_runtime_cpu_policy_contract
cd NolaneWorld && go test ./realm -run 'V31' -count=1
cd NolaneWorld && go test ./realm -run 'V23|V31' -count=1
cd NolaneWorld && go vet ./...
cd NolaneWorld && go test ./...
cd NolaneWorld && go test -race ./realm -run 'V31' -count=1
```

- [ ] **Step 2: Whole-diff trust review**

Review for:

```text
legacy digest drift
CPUUnits -> milliCPU inference
arbitrary Store/wrapper acceptance
JSON authority reconstruction
same descriptive state crossing Store instances
stale Realm revision accepted
closed Realm accepted
cgroup/Wave30/resourceproof scope creep
```

Any found defect gets a new review-driven failing test before production fix.

- [ ] **Step 3: Re-run full dedicated gate after every review fix**

No code-head claim until fresh exact-head run is fully green.

---

### Task 6: Code-head audit, draft PR, and exact-final closure

**Files:**
- Create: `docs/superpowers/verification/2026-09-09-wave31-realm-runtime-cpu-policy-closure.md`

**Interfaces:**
- Consumes: exact GREEN Wave31 code-head.
- Produces: auditable stacked draft PR and final exact-SHA closure evidence.

- [ ] **Step 1: Audit ancestry and scope**

Compare Wave30 exact-final `3657256ea40528bf673a8ca4d51c69233d161b45` to Wave31 head.

Required:

```text
merge_base == 3657256ea40528bf673a8ca4d51c69233d161b45
behind_by == 0
production files limited to realm/model.go + realm/runtime_cpu_policy_authority.go
all other changes are Wave31 tests/workflow/spec/plan/verification/static contract
```

- [ ] **Step 2: Open draft PR #42-equivalent**

Base branch: `gpt/wave30-exact-cpu-throttle-intervention-authority`.
Head: `gpt/wave31-realm-physical-cpu-policy-authority`.
PR body must include autonomous provenance and explicit nonclaims.

- [ ] **Step 3: Require all applicable PR workflows green on exact code-head**

Do not substitute push-only results for PR-triggered integration evidence.

- [ ] **Step 4: Commit closure record**

Record exact code-head SHA, RED lineage, dedicated workflow run IDs, PR workflow results, ancestry/scope audit, review state, and any external automation failures that did not execute code review.

- [ ] **Step 5: Re-run exact-final verification**

Closure commit changes SHA. Require fresh dedicated Wave31 gate plus all applicable PR workflows on the closure SHA.

- [ ] **Step 6: Final state**

Leave PR draft/open/unmerged unless the human explicitly authorizes merge.
