# Nolane Sandbox Provider Authority Runtime v7 Design

## Status

Approved architecture for implementation on `nolane/provider-authority-runtime-v7`.

## Purpose

Delegated Authority Plane v6 establishes the host-owned protocol for typed external effects: the world supplies a delegation ID, operation, canonical resource, payload, authority epoch, and action ID; the host-owned grant chooses the adapter and opaque secret handle; credential bytes never enter the agent protocol.

V7 turns that protocol into a production-oriented provider runtime without weakening the v6 boundary.

The first provider is GitHub because it exercises consequential writes, provider-side identity, idempotency, reconciliation, HTTP transport security, and credential handling while still allowing a small typed authority surface.

V7 is deliberately **not** a generic credential proxy and deliberately **not** a generic HTTP tool. It adds:

1. a host-local secret-broker Vault suitable for placing a KMS/HSM/secret-manager agent outside the Nolane process;
2. a hardened provider HTTP transport whose destination is host configuration, never agent data;
3. a provider-specific typed GitHub adapter;
4. provider-aware idempotency/reconciliation rules;
5. a separate deterministic Provider Authority Gauntlet v7.

V7 does not modify KVM, RustVMM, CubeEgress, CubeNet, CubeCoW, or CubeAPI security internals.

## Central invariant

> **The world may choose the requested typed effect. The host chooses the delegation, provider, endpoint, credential source, and reconciliation strategy.**

V7 adds two stronger corollaries:

> **A provider credential is leased, never handed out.**

> **A consequential provider write is either provider-idempotent, independently reconcilable, or forbidden.**

## Threat model

Assume hostile or compromised: model/agent output, guest root, generated code, downloaded dependencies, intent payloads/resources, provider bodies/errors/headers/redirects, DNS answers, stale snapshots/action IDs, and malformed/oversized secret-broker or provider responses.

Trusted administrative inputs are the Trust Plane process, host configuration, secret-agent socket path, expected secret-agent peer UID, provider endpoint, and provider TLS trust roots.

A compromised external provider can lie about its own state. V7 prevents provider data from silently widening authority or leaking host credentials into agent-visible evidence; it does not make the provider itself trustworthy.

## Non-substitution rule

V7 is an additional evidence/trust layer. It does not replace v4 deterministic release evidence, v5 live Cube/KVM evidence, v6 delegated-authority evidence, or the v6 uncertainty-safe effect ledger.

V4 and v6 standard evidence families must remain byte-stable unless their own version changes.

## Architecture

```text
UNTRUSTED WORLD / AGENT
        |
        | delegation_id + typed operation + canonical resource + payload
        v
+--------------------------------+
| Delegated Authority Plane v6   |
| epoch / scope / revocation     |
| uncertainty-safe action ledger |
+----------------+---------------+
                 |
                 | grant-selected AdapterKind + SecretHandle
                 v
+--------------------------------+
| Provider Authority Runtime v7  |
| typed provider adapter         |
| fixed provider endpoint        |
| strict payload/resource codec  |
| provider action marker         |
| read-only reconciliation       |
+---------+----------------------+
          |
          | Vault.Use(handle, callback)
          v
+--------------------------------+
| BrokerVault                    |
| fixed Unix socket              |
| peer UID pin                   |
| strict bounded protocol        |
+---------+----------------------+
          |
          | opaque handle only
          v
+--------------------------------+
| Host Secret Agent              |
| KMS/HSM/Vault/secret-manager   |
+--------------------------------+

GitHub adapter -> fixed HTTPS provider API -> REAL WORLD
```

The agent never receives provider base URL, credential bytes, `SecretHandle`, HTTP method choice, authentication headers, arbitrary headers/API paths, secret-agent socket path, or KMS/HSM backend configuration.

## `delegation` compatibility helpers

V6 owns construction of `delegation.Secret`: its material field is private, so a Vault in another package cannot fabricate one directly.

V7 adds exactly two narrow host-side helpers:

```go
func ValidateSecretHandle(SecretHandle) error
func WithSecretLease(material []byte, fn func(Secret) error) error
```

`ValidateSecretHandle` exposes the exact existing v6 handle validation rule; it must not normalize or accept anything v6 would reject.

`WithSecretLease`:

- rejects empty material and nil callback;
- creates a private working copy owned by `delegation`;
- invokes `fn(Secret{material: workingCopy})`;
- zeroes the working copy before return;
- never stores, hashes, logs, or serializes secret bytes.

`BrokerVault` zeroes its own decoded broker buffer after `WithSecretLease` returns, so both explicitly owned copies are wiped on the best-effort Go-memory basis acknowledged by the security model.

Neither helper enters `Intent`, `Grant`, receipt, guest protocol, or agent authority.

## `credential/broker`

`credential/broker` implements `delegation.Vault` and owns the production-oriented host-local credential boundary.

It knows only an opaque `SecretHandle` plus host configuration. It does not receive world IDs, grants, operations, resources, payloads, provider URLs, or action IDs.

### Linux Unix-socket transport

Initial production transport is Unix domain socket on Linux.

Configuration:

```go
type Config struct {
    SocketPath      string // absolute host path
    ExpectedPeerUID uint32
    MaxSecretBytes  int // default/hard maximum <= 1 MiB
}
```

Rules:

- socket path must be absolute, clean, non-empty, and host configured;
- socket path is never derived from handle/agent bytes;
- client verifies peer credentials before sending the handle;
- actual peer UID must equal `ExpectedPeerUID`;
- mismatch fails closed;
- filesystem socket permissions remain a second operator-controlled local boundary;
- context bounds connect/read/write lifetime;
- unsupported platforms compile a fail-closed implementation returning `ErrBrokerUnsupported`; peer authentication is never silently skipped.

A malicious process under the exact same trusted host UID is considered a host-boundary compromise and is not solved by v7 UID pinning.

### Framing and canonical schema

Each direction carries exactly one big-endian 32-bit length-prefixed canonical JSON object.

Hard maximum request frame: 4 KiB.
Hard maximum response frame: 1 MiB + 4 KiB protocol overhead.

Request:

```json
{"version":1,"handle":"opaque-host-secret-handle"}
```

Success:

```json
{"version":1,"secret_b64":"..."}
```

Failure:

```json
{"version":1,"error_code":"not_found"}
```

Allowed `error_code` values are exactly:

```text
not_found
denied
unavailable
```

All map to stable broker/secret-unavailable outcomes; broker free-form text is forbidden.

Replay/parser rules:

- unknown fields rejected;
- duplicate fields rejected;
- alternate whitespace/key-order/non-canonical encodings rejected by strict decode plus canonical re-encode equality;
- trailing bytes/second JSON value rejected;
- malformed base64 rejected;
- empty decoded secret rejected;
- decoded secret over configured/hard maximum rejected;
- `secret_b64` and `error_code` are mutually exclusive;
- response bytes are never persisted.

### Vault state machine

`BrokerVault.Use(ctx, handle, fn)`:

1. `delegation.ValidateSecretHandle(handle)`;
2. connect to configured Unix socket;
3. verify peer UID;
4. send canonical handle-only request;
5. read bounded canonical response;
6. decode secret into a fresh broker-owned buffer;
7. call `delegation.WithSecretLease(decoded, fn)`;
8. zero broker-owned decoded buffer;
9. return stable errors only.

Broker failure occurs before provider entry. The v6 Plane therefore retains its explicit `authority.MarkNoEffect(...)` safe-retry semantics when `Vault.Use` never invokes the provider callback.

## `providerhttp`

`providerhttp` owns hardened HTTP mechanics but is **not** a delegation adapter and cannot be registered as an `AdapterKind`.

### Configuration

```go
type Config struct {
    BaseURL string
    RootCAs *x509.CertPool // optional host-owned roots
    Timeout time.Duration
}
```

Production constructor builds its own `http.Client` and `http.Transport`. It does not accept an arbitrary caller `http.Client`, `RoundTripper`, proxy function, redirect function, or dialer.

This prevents a caller from silently restoring environment proxies, credential-forwarding redirects, or origin-rebinding behavior.

Transport invariants:

- scheme must be exactly `https` for **all** endpoints, including loopback/test endpoints;
- tests use TLS test servers plus explicit test roots;
- no URL userinfo, query, or fragment in configured base URL;
- fixed base path is allowed for GitHub Enterprise, e.g. `/api/v3`, and is canonicalized once at construction;
- redirects are always disabled;
- `Proxy` is nil; environment proxy variables are ignored;
- bounded response reader per request;
- finite positive timeout required/defaulted;
- provider route is constructed only from fixed route templates plus validated typed components;
- final normalized URL must retain exact configured scheme, host, port, and base-path prefix.

Authentication material is inserted only inside the trusted adapter request immediately before send and never placed in URL/query/userinfo.

Raw response bodies, raw error text, headers, and auth metadata never become adapter evidence.

## `providers/github`

V7 adds one adapter kind:

```text
github.v1
```

Supported operations are exactly:

```text
github.repo.contents.write
github.issue.comment.create
github.pull_request.comment.create
```

`github.pull_request.comment.create` means a top-level pull-request conversation comment via GitHub's issue-comment surface, not a line-level review comment.

No operation accepts an arbitrary URL, method, host, auth header, request header, or provider path.

### Canonical resources

Resources remain strings in the v6 ABI but GitHub v7 parses into typed values and requires byte-for-byte canonical reserialization.

V7 intentionally supports a conservative syntax subset.

Owner/repository component:

```text
[A-Za-z0-9][A-Za-z0-9._-]{0,99}
```

Branch:

```text
[A-Za-z0-9._/-]{1,255}
```

Additional branch constraints: no leading/trailing `/`, no `//`, no `.`/`..` path component.

Content path segment:

```text
[A-Za-z0-9._+-]{1,255}
```

Additional content-path constraints: total path <= 1024 bytes; `/` separates segments; no empty, `.` or `..` segment.

These regexes deliberately exclude `@`, `:`, `%`, backslash, whitespace, and control characters, making the resource delimiters unambiguous.

Repository contents:

```text
github:repo:<owner>/<repo>:contents:<path>@<branch>
```

Issue comment:

```text
github:repo:<owner>/<repo>:issue:<positive-base10-number>
```

Pull-request conversation comment:

```text
github:repo:<owner>/<repo>:pull:<positive-base10-number>
```

Issue/PR number range is `1..2147483647` in v7.

Broader legal GitHub naming syntax requires a future versioned resource encoding, not permissive parsing.

## Strict operation payloads

Each operation has a dedicated strict canonical JSON decoder. Unknown fields, duplicate fields, non-canonical encoding, trailing values, invalid textual UTF-8, and oversized payloads are rejected before provider entry.

### `github.repo.contents.write`

```json
{"content_b64":"...","commit_message":"...","expected_blob_sha":"optional"}
```

Limits/rules:

- decoded content <= 1 MiB;
- commit message 1..4096 UTF-8 bytes;
- commit message rejects NUL/control characters except newline/tab where explicitly normalized by the adapter;
- `expected_blob_sha`, when present, is exactly 40 lowercase hex characters in v7;
- existing-file update requires `expected_blob_sha`; omission never silently becomes overwrite;
- create may omit it;
- user commit message cannot contain reserved `nolane-provider-v7:` marker text.

### Comment creation payload

Both comment operations use:

```json
{"body":"comment text"}
```

Rules:

- body 1..65536 UTF-8 bytes before adapter marker;
- NUL rejected;
- reserved `nolane-provider-v7:` marker text rejected from user body.

## Provider action marker

From v6 `AdapterRequest.IdempotencyKey`, v7 derives:

```text
nolane-provider-v7:<sha256(idempotency-key)>
```

No secret material participates.

Comments append:

```html
<!-- nolane-provider-v7:<digest> -->
```

Content writes append a fixed machine marker line to the commit message.

The marker is provider-observable provenance that distinguishes this action from unrelated identical visible content.

## GitHub execution

No v7 write path has an internal automatic retry loop.

### Comment create

1. validate canonical resource/payload;
2. derive action marker;
3. append reserved hidden marker;
4. issue exactly one POST to the fixed provider route;
5. any error after request execution begins returns adapter failure; v6 leaves the action pending/uncertain;
6. success returns sanitized typed evidence only.

### Repository contents write

1. validate canonical resource/payload;
2. derive marker and append to commit message;
3. construct exact GitHub write request for configured branch and create/update SHA semantics;
4. issue exactly one provider write;
5. never retry after ambiguous provider/transport outcome;
6. success returns sanitized provider IDs only.

## Reconciliation

Reconciliation is read-only provider observation and never calls a write endpoint.

### Comments

The adapter scans at most 10 provider pages of at most 100 comments each for the exact action marker.

- exact marker observed -> `ReconcileObserved`;
- direct trustworthy provider observation that canonical parent resource does not exist -> `ReconcileAbsent`;
- marker missing from bounded scan -> `ReconcileUnknown`, never false absence;
- ambiguous transport/provider response -> sanitized reconciliation failure/unknown according to available trustworthy observation.

Bounded search proves presence, not absence.

### Repository contents

The adapter scans at most 10 read-only commit-list pages of at most 100 commits each, constrained to the canonical branch/path where provider API permits, and verifies the exact action marker.

- exact marker observed -> `ReconcileObserved`;
- ordinary non-observation -> `ReconcileUnknown`;
- current file missing/different does not prove action absence.

This intentionally loses availability rather than risk a duplicate write.

## Typed adapter evidence

Evidence contains only deterministic/allowlisted values:

- provider kind/version;
- operation;
- SHA-256 of canonical resource;
- provider comment/commit/object ID when successfully parsed and bounded;
- SHA-256 of action marker;
- response status class.

Provider object URL is omitted in v7 evidence to avoid accidental origin/query/privacy leakage.

Raw response body, provider error body, headers, credential material, authorization value, socket path, TLS details, and host diagnostics are excluded.

V6's exact-secret leak detector remains a second independent check on returned evidence.

## Error taxonomy

Credential boundary stable errors:

- `ErrBrokerUnavailable`
- `ErrBrokerPeerMismatch`
- `ErrBrokerUnsupported`
- `ErrBrokerProtocol`
- `ErrBrokerResponseTooLarge`

Provider stable errors:

- `ErrInvalidProviderConfig`
- `ErrInvalidProviderResource`
- `ErrInvalidProviderPayload`
- `ErrProviderTransport`
- `ErrProviderRejected`
- `ErrProviderResponse`

At the Delegated Authority Plane boundary these remain sanitized to existing v6 stable errors such as `ErrSecretUnavailable`, `ErrAdapterFailure`, and `ErrReconcileFailure`; raw external text is never wrapped into agent-facing errors.

## GitHub credential expectations

V7 does not mint tokens. The host secret agent supplies a credential for the host-selected handle.

Operational guidance:

- prefer fine-grained PATs or GitHub App installation tokens;
- provider repository/permission scope should be no broader than the host grant where provider primitives allow;
- rotate underlying bytes behind an opaque handle without changing request digests when operationally appropriate;
- provider credential scope supplements but never replaces exact Nolane grant checks.

## Secret-agent compromise boundary

The host secret agent is trusted to return the correct credential for a handle. V7 cannot detect a malicious trusted agent intentionally returning different credential bytes.

Mitigations are peer UID pinning, socket permissions, one opaque handle per host grant, narrow provider token scope, host-fixed adapter/endpoint, and fail-closed provider authorization errors.

Hardware-attested broker identity is a future strengthening, not a v7 merge requirement.

## Provider Authority Gauntlet v7

V7 adds a separate deterministic evidence family using controlled Unix broker + controlled TLS provider endpoints with synthetic credentials.

Every registered scenario is mandatory and must prove it exercised the intended boundary.

Minimum mandatory scenarios:

1. broker peer mismatch denied using an intentionally wrong expected UID;
2. broker unknown-field/non-canonical response denied;
3. broker oversized response denied before callback/provider entry;
4. synthetic secret plaintext/base64/hex absent from evidence;
5. provider endpoint cannot be rebound by intent/resource/payload;
6. redirect denied and synthetic credential never reaches redirect target;
7. generic authenticated HTTP remains unregistrable as v6 adapter;
8. GitHub resource delimiter/path ambiguity rejected;
9. strict operation payload unknown/duplicate/trailing/non-canonical fields denied before provider entry;
10. comment effect-then-transport-failure leaves action pending and produces one POST only;
11. comment reconciliation observes marker without re-execution;
12. bounded comment non-observation is `unknown`, not false `absent`;
13. content-write effect-then-failure leaves action pending and produces one write only;
14. content reconciliation observes exact commit marker and performs no write;
15. provider raw error/body containing synthetic secret/diagnostics is sanitized;
16. action-ID/resource rebinding still collides under provider adapter;
17. stale/revoked delegation cannot reach provider write;
18. concurrent independent broker leases do not cross secret buffers;
19. v6 evidence remains byte-stable;
20. v4 evidence remains byte-stable.

## Deterministic v7 evidence

CLI:

```bash
go run ./cmd/nolane-provider-gauntlet --out release-evidence/nolane-provider-v7.json
```

CI runs it twice and requires byte-for-byte equality before upload.

Report bytes must exclude synthetic secret plaintext, base64 and hex forms; broker socket temp path; raw Authorization header; raw provider body; nondeterministic timestamps/random IDs.

V7 uses its own suite/version and does not extend v4/v6 standard suites.

## CI trust boundary

Pull-request CI uses synthetic credentials and local controlled endpoints only. No production provider secret is exposed to PR-triggered code.

A later live-provider workflow must be manual/protected, trusted-branch-only, exact-commit, and distinguish `LIVE_PROVIDER_PASS` from `UNAVAILABLE`. Its absence does not block deterministic v7 but means no live production-provider claim may be made.

## Implementation boundary

Expected new code:

```text
NolaneWorld/credential/broker/
NolaneWorld/providerhttp/
NolaneWorld/providers/github/
NolaneWorld/gauntlet/provider/
NolaneWorld/cmd/nolane-provider-gauntlet/
docs/superpowers/specs/
docs/superpowers/plans/
.github/workflows/nolane-world-check.yml
NolaneWorld/README.md
```

Allowed v6 change: only exact `ValidateSecretHandle` / `WithSecretLease` helpers and tests required by the cross-package Vault implementation. No change may give intents control over adapter kind, secret handle/bytes, or provider endpoint.

Cube security-core implementation remains untouched.

## TDD requirements

Implementation proceeds RED -> GREEN for each boundary.

Required RED contracts:

- exact handle-validator parity with existing v6 rules;
- cross-package `WithSecretLease` construction and working-copy wipe;
- peer UID mismatch;
- broker unknown field/non-canonical/trailing frame;
- broker response/decoded-secret bounds;
- provider HTTPS-only/base URL policy;
- redirect credential-forwarding denial;
- endpoint non-rebinding;
- exact GitHub resource regex/delimiter parser;
- strict operation payload codec and numeric/size limits;
- reserved action-marker injection denial;
- no internal write retry;
- comment uncertain-effect + reconciliation;
- content uncertain-effect + reconciliation;
- provider response/error sanitization;
- concurrent Vault lease isolation under race detector;
- deterministic v7 evidence;
- v6/v4 evidence non-drift.

## Security properties intentionally not claimed

V7 does not claim:

- host secret agent/KMS compromise resistance;
- hardware-backed memory secrecy inside the Go process;
- guaranteed compiler/runtime zeroization beyond best-effort owned-buffer wiping;
- distributed consensus or whole-host rollback resistance;
- that GitHub itself cannot lie or be compromised;
- live production-provider verification without a separate exact-commit live artifact;
- line-level code-review comments;
- AWS/email adapters;
- generic authenticated HTTP;
- support for every GitHub-valid path/ref syntax.

## Release gate

V7 may merge only when the exact PR head proves:

1. all Go unit tests pass;
2. `go test -race ./...` passes;
3. `go vet ./...` passes;
4. Docs Build passes;
5. Format Check passes on required architectures;
6. Provider Authority Gauntlet v7 passes every mandatory scenario;
7. v7 evidence is byte-identical across two executions;
8. synthetic secret plaintext/base64/hex is absent from v7 evidence;
9. v6 deterministic evidence remains byte-stable;
10. v4 deterministic evidence remains byte-stable;
11. provider write tests prove no internal automatic retry after provider entry;
12. reconciliation tests prove observation never performs a write;
13. broker tests prove peer mismatch, malformed/non-canonical response, and bounds fail closed;
14. provider tests prove HTTPS-only construction and redirects cannot forward credentials;
15. strict resource/payload tests reject ambiguity before provider entry;
16. final diff does not modify Cube/KVM/RustVMM/CubeNet/CubeEgress/CubeCoW security-core implementation;
17. agent-generated commits contain `Autonomously-by: ChatGPT:GPT-5.6-Sol`;
18. no agent-generated human `Signed-off-by` exists.

## Exit criterion

V7 is complete when Nolane Sandbox can take a v6 delegated intent, resolve a credential through the authenticated host-local broker, execute one of the three explicitly supported GitHub operations against a host-configured HTTPS provider endpoint, quarantine ambiguous outcomes without replay, reconcile by provider-observable action markers, and emit deterministic secret-free adversarial evidence proving those boundaries.

V7 completion does **not** certify a real external KMS/HSM or production GitHub account. Those require environment-specific live evidence in a later gate.