# Provider Authority Runtime v7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn Delegated Authority Plane v6 into a production-oriented provider runtime with a host-local secret broker, hardened provider transport, a typed GitHub adapter, provider reconciliation, and deterministic v7 adversarial evidence without weakening v4/v6 contracts.

**Architecture:** Keep `delegation` as the authority protocol and add only two narrow secret-construction helpers there. Implement an external `credential/broker` Vault over authenticated Unix sockets, a non-adapter `providerhttp` package that owns fixed HTTPS transport, and a `providers/github` typed adapter whose routes and operations are fixed by host code. Reuse the v6 uncertainty ledger so provider writes are entered once and ambiguous outcomes require read-only reconciliation.

**Tech Stack:** Go 1.23, standard library only, Linux Unix sockets/`SO_PEERCRED`, `crypto/tls`, `crypto/x509`, `net/http`, existing Nolane `delegation`, `authority`, and `gauntlet` packages, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-08-29-nolane-sandbox-provider-authority-runtime-v7-design.md`

## Global Constraints

- Agent protocol still contains no `SecretHandle`, provider endpoint, adapter choice, HTTP method, auth header, or raw provider path.
- `BrokerVault` receives only an opaque `SecretHandle` plus host configuration.
- Broker peer UID verification is mandatory on Linux; unsupported platforms fail closed.
- Provider base URLs are HTTPS-only, including tests; tests use TLS servers and explicit roots.
- Provider transport builds its own `http.Client`/`Transport`; arbitrary caller transports, proxies, redirect handlers, and dialers are not accepted.
- GitHub adapter kind is exactly `github.v1` and supports only `github.repo.contents.write`, `github.issue.comment.create`, and `github.pull_request.comment.create`.
- No provider write path internally retries after provider entry.
- Bounded non-observation never proves absence.
- Raw broker/provider response text, headers, credentials, socket paths, and TLS diagnostics never enter agent-visible evidence.
- V4 and v6 standard evidence bytes must remain unchanged.
- Cube/KVM/RustVMM/CubeNet/CubeEgress/CubeCoW/CubeAPI security-core implementation must not be modified.
- All AI commits/PRs include `Autonomously-by: ChatGPT:GPT-5.6-Sol` and never add `Signed-off-by`.

---

### Task 1: Delegation secret-lease compatibility helpers

**Files:**
- Modify: `NolaneWorld/delegation/vault.go`
- Test: `NolaneWorld/delegation/contracts_test.go`

**Interfaces:**
- Produces: `func ValidateSecretHandle(SecretHandle) error`
- Produces: `func WithSecretLease([]byte, func(Secret) error) error`
- Preserves: existing `Vault`, `Secret`, and `MemoryVault` behavior.

- [ ] **Step 1: Write failing helper contracts**

Add tests equivalent to:

```go
func TestValidateSecretHandleMatchesV6Rule(t *testing.T) {
    if err := ValidateSecretHandle("kms/github/repo-a"); err != nil { t.Fatal(err) }
    for _, bad := range []SecretHandle{"", " leading", "trailing ", SecretHandle(strings.Repeat("x", 513))} {
        if err := ValidateSecretHandle(bad); !errors.Is(err, ErrSecretUnavailable) { t.Fatalf("handle=%q err=%v", bad, err) }
    }
}

func TestWithSecretLeaseCopiesAndZeroesWorkingMaterial(t *testing.T) {
    original := []byte("SYNTHETIC-V7-SECRET")
    var leaked []byte
    err := WithSecretLease(original, func(s Secret) error {
        leaked = s.Bytes()
        if string(leaked) != string(original) { t.Fatal("wrong lease") }
        return nil
    })
    if err != nil { t.Fatal(err) }
    if bytes.Equal(leaked, original) { t.Fatal("lease buffer was not wiped") }
    if string(original) != "SYNTHETIC-V7-SECRET" { t.Fatal("caller buffer mutated") }
}
```

- [ ] **Step 2: Run focused tests and prove RED**

Run: `cd NolaneWorld && go test ./delegation -run 'TestValidateSecretHandle|TestWithSecretLease'`

Expected: compile failure because both helpers do not exist.

- [ ] **Step 3: Implement minimal host-only helpers**

`ValidateSecretHandle` delegates to the existing exact v6 `strict(string(handle), 512)` rule and returns `ErrSecretUnavailable` on failure. `WithSecretLease` rejects empty input/nil callback, clones material, defers `zero(working)`, and constructs `Secret` only inside the `delegation` package.

- [ ] **Step 4: Run focused tests and race test**

Run: `go test -race ./delegation`

Expected: PASS.

- [ ] **Step 5: Commit**

Commit message: `feat(nolane): add host secret lease helpers` plus required autonomous trailer.

---

### Task 2: Strict host-local BrokerVault

**Files:**
- Create: `NolaneWorld/credential/broker/broker.go`
- Create: `NolaneWorld/credential/broker/protocol.go`
- Create: `NolaneWorld/credential/broker/peer_linux.go`
- Create: `NolaneWorld/credential/broker/peer_other.go`
- Create: `NolaneWorld/credential/broker/broker_test.go`
- Create: `NolaneWorld/credential/broker/protocol_test.go`

**Interfaces:**

```go
type Config struct {
    SocketPath      string
    ExpectedPeerUID uint32
    MaxSecretBytes  int
}

func New(Config) (*Vault, error)
func (v *Vault) Use(context.Context, delegation.SecretHandle, func(delegation.Secret) error) error
```

Stable package errors: `ErrBrokerUnavailable`, `ErrBrokerPeerMismatch`, `ErrBrokerUnsupported`, `ErrBrokerProtocol`, `ErrBrokerResponseTooLarge`.

- [ ] **Step 1: Write RED protocol tests**

Cover exact canonical response, unknown field, duplicate field, alternate whitespace/key order, trailing frame bytes, malformed base64, empty secret, oversized secret, both `secret_b64`+`error_code`, unknown error code, and frame length above bounds.

Canonical decoder contract:

```go
raw := []byte(`{"version":1,"secret_b64":"U0VDUkVU"}`)
secret, err := decodeResponse(raw, 1<<20)
if err != nil || string(secret) != "SECRET" { t.Fatalf("secret=%q err=%v", secret, err) }
```

- [ ] **Step 2: Prove RED**

Run: `go test ./credential/broker`

Expected: package/functions missing.

- [ ] **Step 3: Implement canonical 32-bit big-endian single-message framing**

Request JSON must marshal exactly as:

```json
{"version":1,"handle":"<opaque>"}
```

Read one bounded response frame and require EOF after it; strict JSON decoder uses `DisallowUnknownFields`, rejects a second JSON value, re-marshals the decoded struct, and requires byte equality with received JSON.

- [ ] **Step 4: Add Linux peer-identity RED tests**

Start a Unix listener under `t.TempDir()`, connect as the test process, require `ExpectedPeerUID == uint32(os.Getuid())` to succeed, and a deliberately different UID to return `ErrBrokerPeerMismatch` before the handle is processed.

- [ ] **Step 5: Implement peer verification**

On Linux use `(*net.UnixConn).SyscallConn()` and `syscall.GetsockoptUcred(..., SOL_SOCKET, SO_PEERCRED)`. Non-Linux builds return `ErrBrokerUnsupported`; never skip peer verification.

- [ ] **Step 6: Implement Vault state machine**

Validate absolute/clean socket path, validate handle with `delegation.ValidateSecretHandle`, connect with context, verify UID, send canonical request, close request write side, read bounded response to EOF, decode into broker-owned bytes, `defer zero`, then call `delegation.WithSecretLease(decoded, fn)`.

- [ ] **Step 7: Test sanitization and data minimization**

The fake broker records the request and proves it contains only `version` and `handle`; tests verify world/resource/action/provider data never reaches the broker. Errors returned to callers are stable package sentinels and never include broker-supplied text.

- [ ] **Step 8: Run focused race/vet and commit**

Run: `go test -race ./credential/broker && go vet ./credential/broker`

Commit: `feat(nolane): add authenticated secret broker vault` plus trailer.

---

### Task 3: Hardened non-adapter provider HTTP runtime

**Files:**
- Create: `NolaneWorld/providerhttp/client.go`
- Create: `NolaneWorld/providerhttp/url.go`
- Create: `NolaneWorld/providerhttp/client_test.go`

**Interfaces:**

```go
type Config struct {
    BaseURL string
    RootCAs *x509.CertPool
    Timeout time.Duration
}

type Client struct { /* private client/base */ }
func New(Config) (*Client, error)
func (c *Client) Do(context.Context, string, []string, http.Header, []byte, int64) (status int, body []byte, err error)
```

`Do` accepts a host-code fixed method, already-validated path components/route segments, trusted headers, body, and response cap; it never accepts an absolute URL.

- [ ] **Step 1: Write RED constructor/transport tests**

Reject `http://`, userinfo, query, fragment, malformed/noncanonical base paths, zero/negative timeout, and base-path escapes. Verify environment `HTTP_PROXY`/`HTTPS_PROXY` are ignored.

- [ ] **Step 2: Prove RED**

Run: `go test ./providerhttp`

- [ ] **Step 3: Implement owned HTTPS transport**

Create `http.Transport{Proxy:nil, TLSClientConfig:&tls.Config{MinVersion:tls.VersionTLS12, RootCAs:cfg.RootCAs}}` and an `http.Client` with finite timeout plus `CheckRedirect` returning `http.ErrUseLastResponse`. Do not expose the transport/client.

- [ ] **Step 4: Implement route construction and origin pin**

Build provider URLs only beneath the immutable base scheme/host/base-path. Escape path components with path semantics and reject anything that changes scheme, host, port, or base prefix.

- [ ] **Step 5: TLS tests**

Use `httptest.NewTLSServer`, build an explicit `x509.CertPool` from the server certificate, and prove a redirect to another TLS server is not followed and never receives authorization headers.

- [ ] **Step 6: Bounded body tests**

A response larger than the caller cap returns a stable transport response error without exposing the body. Raw provider body text is never embedded in returned error text.

- [ ] **Step 7: Race/vet and commit**

Run: `go test -race ./providerhttp && go vet ./providerhttp`

Commit: `feat(nolane): add pinned provider HTTPS runtime` plus trailer.

---

### Task 4: GitHub canonical resource and payload codecs

**Files:**
- Create: `NolaneWorld/providers/github/types.go`
- Create: `NolaneWorld/providers/github/resource.go`
- Create: `NolaneWorld/providers/github/payload.go`
- Create: `NolaneWorld/providers/github/codec_test.go`

**Interfaces:**

```go
const Kind delegation.AdapterKind = "github.v1"
const (
    OpContentsWrite delegation.Operation = "github.repo.contents.write"
    OpIssueComment delegation.Operation = "github.issue.comment.create"
    OpPullComment delegation.Operation = "github.pull_request.comment.create"
)
```

Typed resources: `ContentsResource`, `IssueResource`, `PullResource`. Typed payloads: `ContentsWritePayload`, `CommentPayload`.

- [ ] **Step 1: RED resource tests**

Test exact canonical round-trip for the three resource forms and reject percent encoding, `@`/`:` ambiguity, backslash, empty/dot/dotdot path segments, invalid owner/repo charset, branch `//`, branch leading/trailing slash, and issue/PR numbers outside `1..2147483647`.

- [ ] **Step 2: Implement parser + canonical serializer**

Parsing is successful only if serializing the typed value reproduces the input byte-for-byte.

- [ ] **Step 3: RED payload tests**

Reject unknown/duplicate fields, alternate noncanonical JSON encoding, trailing JSON, invalid base64, decoded content >1 MiB, empty/oversized commit message, invalid `expected_blob_sha`, empty/oversized comment body, NUL, and user-supplied `nolane-provider-v7:` marker.

- [ ] **Step 4: Implement strict payload codecs**

Use dedicated structs per operation, strict decode plus canonical re-encode equality, UTF-8 validation, and exact numerical/size limits from the spec.

- [ ] **Step 5: Marker tests/implementation**

```go
func actionMarker(idempotencyKey string) string {
    sum := sha256.Sum256([]byte(idempotencyKey))
    return "nolane-provider-v7:" + hex.EncodeToString(sum[:])
}
```

Prove secret bytes do not participate.

- [ ] **Step 6: Race/vet and commit**

Run: `go test -race ./providers/github && go vet ./providers/github`

Commit: `feat(nolane): add canonical GitHub provider codecs` plus trailer.

---

### Task 5: Typed GitHub adapter execution

**Files:**
- Create: `NolaneWorld/providers/github/adapter.go`
- Create: `NolaneWorld/providers/github/evidence.go`
- Create: `NolaneWorld/providers/github/execute_test.go`

**Interfaces:**

```go
type Config struct {
    BaseURL string
    RootCAs *x509.CertPool
    Timeout time.Duration
}
func New(Config) (*Adapter, error)
func (*Adapter) Kind() delegation.AdapterKind
func (*Adapter) Execute(context.Context, delegation.AdapterRequest, delegation.Secret) (delegation.Effect, error)
```

- [ ] **Step 1: RED adapter-kind/operation tests**

`Kind()` must equal `github.v1`. Any operation outside the exact three-operation set returns a provider error before network entry.

- [ ] **Step 2: RED issue/PR comment tests**

Controlled TLS provider asserts one POST to `/repos/{owner}/{repo}/issues/{n}/comments`, correct bearer authorization, canonical JSON body with hidden action marker, and no second request on server 500/connection ambiguity.

- [ ] **Step 3: RED contents-write tests**

Controlled TLS provider asserts one PUT to `/repos/{owner}/{repo}/contents/{path}`, branch and optional SHA semantics, deterministic marker in commit message, and no retry. Add create/update fixtures; update without expected SHA is rejected before network entry.

- [ ] **Step 4: Implement execution routes**

Use only `providerhttp.Client`; paths come from typed resources, method is adapter-owned, auth is injected inside `Execute`, and no internal retry loop exists.

- [ ] **Step 5: Implement sanitized evidence**

Return canonical JSON evidence containing provider version/kind, operation, SHA-256 resource digest, bounded parsed provider ID/SHA, marker digest, and status class. Omit object URLs, raw body, headers, token, host diagnostics, and errors.

- [ ] **Step 6: Secret-leak and redirect tests**

Provider responses intentionally echo plaintext/base64 token and malicious headers; returned evidence must not contain those bytes. Cross-origin redirect is not followed.

- [ ] **Step 7: Race/vet and commit**

Run: `go test -race ./providers/github && go vet ./providers/github`

Commit: `feat(nolane): add typed GitHub provider writes` plus trailer.

---

### Task 6: Read-only GitHub reconciliation

**Files:**
- Modify: `NolaneWorld/providers/github/adapter.go`
- Create: `NolaneWorld/providers/github/reconcile.go`
- Create: `NolaneWorld/providers/github/reconcile_test.go`

**Interfaces:**
- Implements existing `delegation.Adapter.Reconcile`.
- Never calls write routes.

- [ ] **Step 1: RED comment reconciliation tests**

Fake provider exposes paginated GET comment lists. Exact marker on any page 1..10 -> `ReconcileObserved`; ten full/partial pages without marker -> `ReconcileUnknown`; transport ambiguity -> sanitized error. Assert POST/PUT counters remain zero.

- [ ] **Step 2: RED contents reconciliation tests**

GET at most ten commit-list pages constrained to canonical branch/path. Exact marker in commit message -> observed; no marker -> unknown. Current file absence is never treated as proof the historic write was absent.

- [ ] **Step 3: Implement bounded read-only scans**

`per_page=100`, maximum ten pages, fixed routes/query keys owned by the adapter. No caller/agent-supplied arbitrary query or URL.

- [ ] **Step 4: Prove v6 uncertainty integration**

Through a real `delegation.Plane` + `authority.MemoryLedger`/`JournalLedger`, force an Execute transport error after provider entry, verify retry is `ErrActionUncertain`, then reconcile marker observed and prove completed receipt without second write.

- [ ] **Step 5: Race/vet and commit**

Run: `go test -race ./providers/github ./delegation ./authority && go vet ./providers/github ./delegation ./authority`

Commit: `feat(nolane): add GitHub provider reconciliation` plus trailer.

---

### Task 7: Provider Authority Gauntlet v7 and CLI

**Files:**
- Create: `NolaneWorld/gauntlet/provider/scenarios.go`
- Create: `NolaneWorld/gauntlet/provider/scenarios_test.go`
- Create: `NolaneWorld/cmd/nolane-provider-gauntlet/main.go`
- Create: `NolaneWorld/cmd/nolane-provider-gauntlet/main_test.go`

**Interfaces:**

```go
const SyntheticSecret = "SYNTHETIC-V7-SECRET"
func Standard() []gauntlet.Scenario
func RunStandard(context.Context, gauntlet.Policy) (gauntlet.Report, error)
```

- [ ] **Step 1: Add mandatory adversarial scenarios**

At minimum: broker peer UID mismatch; broker unknown/duplicate/noncanonical response; oversized broker response; provider HTTP downgrade; provider redirect credential forwarding; endpoint rebinding attempt; resource delimiter/path traversal; payload unknown/duplicate field; operation escalation; provider error sanitization; ambiguous write no-retry; comment reconciliation observed; bounded non-observation -> unknown; secret plaintext/base64/hex absence; v6 uncertain replay integration.

Each scenario must emit attack/boundary/denial-or-observation markers through the existing `gauntlet.Probe`; no scenario self-declares PASS.

- [ ] **Step 2: Use controlled real boundaries**

Use a real temp Unix socket broker and TLS `httptest` provider. Synthetic token is fixed for deterministic semantics, but runtime ports/paths must never appear in evidence details; record stable digests/labels only.

- [ ] **Step 3: Add deterministic CLI**

Follow the existing v6 CLI shape. `nolane-provider-gauntlet --out ...` runs standard suite, calls `gauntlet.MarshalReport`, rejects unapproved reports, and checks raw JSON does not contain plaintext, base64, or hex forms of the synthetic token.

- [ ] **Step 4: Determinism tests**

Run the suite twice and require byte-identical JSON. Runtime TLS ports, temp directories, socket paths, UIDs, timestamps, provider request IDs, and raw errors cannot enter event details/report bytes.

- [ ] **Step 5: Run focused tests and fuzz seeds**

Run: `go test -race ./gauntlet/provider ./cmd/nolane-provider-gauntlet`

Add fuzz/property seeds for broker response parser and GitHub canonical resource/payload codecs if not already covered by table tests.

- [ ] **Step 6: Commit**

Commit: `feat(nolane): add Provider Authority Gauntlet v7` plus trailer.

---

### Task 8: CI, README, and full release verification

**Files:**
- Modify: `.github/workflows/nolane-world-check.yml`
- Modify: `NolaneWorld/README.md`

**Interfaces:**
- Existing V4/V6 generation commands remain unchanged.
- Adds v7 evidence artifact <code v-pre>nolane-provider-v7-${{ github.sha }}</code>.

- [ ] **Step 1: Extend CI without changing V4/V6 commands**

Add:

```bash
go run ./cmd/nolane-provider-gauntlet --out release-evidence/nolane-provider-v7.json
go run ./cmd/nolane-provider-gauntlet --out release-evidence/nolane-provider-v7-repeat.json
cmp release-evidence/nolane-provider-v7.json release-evidence/nolane-provider-v7-repeat.json
```

Then negative-grep plaintext/base64/hex synthetic secret forms and upload only the verified first report.

- [ ] **Step 2: Update README**

Document v7 as provider runtime, BrokerVault production boundary, GitHub adapter operations, limits/non-goals, verification command, and explicit statement that a host-local broker is a boundary to KMS/HSM but does not itself live-certify every backend.

- [ ] **Step 3: Fresh full local/repository suite**

Run:

```bash
cd NolaneWorld
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/nolane-gauntlet --out /tmp/v4.json
go run ./cmd/nolane-authority-gauntlet --out /tmp/v6.json
go run ./cmd/nolane-provider-gauntlet --out /tmp/v7.json
```

- [ ] **Step 4: Prove evidence non-regression**

V4 raw JSON SHA-256 must remain `94ef192c57f2587d34a8340a8bfd8d297782e121c88ad4aa96792e42bf40c6f4`. Compare v6 report bytes to the v6 artifact from `master`/pre-v7 exact command; any drift blocks merge unless v6 is explicitly versioned separately, which is outside this plan.

- [ ] **Step 5: Diff audit**

Compare branch to `master`. Allowed changes are only `NolaneWorld/delegation` compatibility helper, new `credential`, `providerhttp`, `providers/github`, `gauntlet/provider`, v7 CLI, README, v7 spec/plan, and Nolane workflow. Any Cube security-core change blocks release.

- [ ] **Step 6: Open PR**

PR body records architecture, tests, evidence digests, and `Autonomously-by: ChatGPT:GPT-5.6-Sol`; no `Signed-off-by`.

- [ ] **Step 7: Exact-head GitHub gate**

Require Nolane World Check, Docs Build, Format amd64/arm64, V7 gauntlet, deterministic compare, and secret-negative checks to pass. DCO remains the human attestation gate and is never forged.

- [ ] **Step 8: Squash merge with head lock and post-merge verification**

Merge only with `expected_head_sha`. Then verify `master` contains v7 and its push-triggered Nolane World Check regenerates V4/V6/V7 evidence successfully.
