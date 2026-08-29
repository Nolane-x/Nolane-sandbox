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

Assume all of the following are hostile or compromised:

- model/agent output;
- guest root;
- arbitrary generated code;
- downloaded dependencies;
- intent payload bytes;
- resource strings supplied by the agent;
- provider response bodies;
- provider error text;
- redirects;
- HTTP response headers;
- DNS answers;
- stale world snapshots;
- stale/replayed action IDs;
- malformed secret-broker responses;
- malformed or oversized provider responses.

Host configuration, the Trust Plane process, the local secret-agent socket path, the configured secret-agent peer identity, and configured provider endpoint are trusted administrative inputs.

A compromised external provider can lie about its own state. V7 prevents such provider data from silently widening authority or leaking host credentials into agent-visible evidence; it does not make the provider itself trustworthy.

## Non-substitution rule

V7 is an additional trust layer. It does not replace:

- v4 deterministic release evidence;
- v5 live Cube/KVM evidence;
- v6 delegated-authority evidence;
- the v6 effect ledger uncertainty state machine.

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
|                                |
|  typed provider adapter        |
|  fixed provider endpoint       |
|  strict payload codec          |
|  provider idempotency marker   |
|  reconciliation strategy       |
+---------+----------------------+
          |
          | Vault.Use(handle, callback)
          v
+--------------------------------+
| BrokerVault                    |
| fixed Unix socket              |
| peer UID identity pin          |
| strict bounded protocol        |
+---------+----------------------+
          |
          | handle only
          v
+--------------------------------+
| Host Secret Agent              |
| operator-owned process         |
| KMS/HSM/Vault/secret-manager   |
+--------------------------------+

Provider adapter -> fixed HTTPS provider API -> REAL WORLD
```

The agent never receives:

- provider base URL;
- credential bytes;
- `SecretHandle`;
- HTTP method selection;
- authentication headers;
- arbitrary request headers;
- arbitrary provider path selection;
- secret-agent socket path;
- KMS/HSM backend configuration.

## Package boundaries

### `credential/broker`

Owns the production-oriented host-local credential boundary.

Responsibilities:

- Unix-socket client;
- host-configured absolute socket path;
- peer identity verification;
- strict request/response framing;
- response size bound;
- callback-scoped credential lease;
- working-buffer zeroing;
- stable sanitized error taxonomy;
- no logging of secret bytes or raw broker response content.

It implements `delegation.Vault`.

It does **not** know world IDs, grants, operations, provider URLs, payloads, or external-action targets. Its only trust-bearing input from the plane is an opaque `SecretHandle`.

### `providerhttp`

Owns hardened provider HTTP mechanics shared by provider-specific adapters.

Responsibilities:

- host-configured base URL validation;
- HTTPS required for non-loopback destinations;
- no URL userinfo;
- redirects disabled;
- no environment-derived HTTP proxy;
- bounded response bodies;
- explicit connect/request timeout through supplied `http.Client`/transport;
- provider paths constructed from typed components rather than concatenated agent URLs;
- authentication material added only immediately before sending the trusted host request;
- raw provider response headers/body never returned directly to the agent.

`providerhttp` is **not** itself a delegation adapter and must never be registrable as an `AdapterKind`.

### `providers/github`

Owns the first production typed provider adapter.

V7 registers one adapter kind:

```text
github.v1
```

The adapter supports exactly these operations:

```text
github.repo.contents.write
github.issue.comment.create
github.pull_request.comment.create
```

No operation accepts an arbitrary URL, method, hostname, authorization header, or API path.

### `gauntlet/provider`

Owns deterministic v7 provider-authority evidence. It uses an in-process controlled fake provider plus a controlled fake credential broker. It never requires a real GitHub credential in ordinary CI.

## Secret broker protocol

### Rationale

Embedding AWS/GCP/Azure/Vault SDKs directly in the Nolane trust process would enlarge the trusted computing base and tie the core to one secret backend. V7 instead defines a narrow local broker contract. Operators may place Vault Agent, a KMS/HSM-backed helper, or another secret manager behind that broker without changing the AI-facing protocol.

V7 therefore claims a **production credential boundary**, not that every external KMS backend has been live-certified.

### Transport

Initial production transport is Unix domain socket on Linux.

Requirements:

- socket path is absolute and host configured;
- socket path is never derived from `SecretHandle` or agent data;
- connection is local Unix-domain only;
- client verifies peer credentials before sending the handle;
- expected peer UID is explicit host configuration;
- peer UID mismatch fails closed;
- platforms where equivalent peer verification is unsupported return a stable unsupported error rather than silently skipping authentication;
- socket timeouts are context bounded.

### Framing

Every request and response is one length-prefixed canonical JSON message.

Maximum request: 4 KiB.
Maximum response: 1 MiB plus framing overhead.

Request schema:

```json
{
  "version": 1,
  "handle": "opaque-host-secret-handle"
}
```

Successful response schema:

```json
{
  "version": 1,
  "secret_b64": "..."
}
```

Failure response schema:

```json
{
  "version": 1,
  "error_code": "not_found"
}
```

Rules:

- unknown JSON fields rejected;
- duplicate fields rejected by canonical re-encoding/equality check;
- trailing bytes rejected;
- malformed base64 rejected;
- empty secret rejected;
- oversized decoded secret rejected;
- `secret_b64` and `error_code` are mutually exclusive;
- raw broker error text is not part of the protocol;
- response bytes are not persisted.

### Vault semantics

`BrokerVault.Use(ctx, handle, fn)`:

1. validates the opaque handle using v6 constraints;
2. connects to the configured socket;
3. verifies peer identity;
4. sends the strict handle-only request;
5. reads the bounded strict response;
6. decodes a fresh credential buffer;
7. invokes `fn(delegation.Secret)`;
8. zeroes the credential buffer after callback return;
9. returns stable errors only.

If the broker cannot provide a credential, this happens before provider entry. The v6 Plane may therefore use its existing explicit `authority.MarkNoEffect(...)` path and permit a safe retry after the secret source recovers.

## Provider HTTP transport

### Endpoint ownership

Provider endpoint is host configuration associated with the GitHub adapter instance.

For public GitHub the canonical default is:

```text
https://api.github.com
```

GitHub Enterprise Server may use a host-configured HTTPS base URL, including a fixed base path such as `/api/v3`.

The agent cannot override this endpoint through resource or payload bytes.

### URL construction

Every provider URL is built from:

- validated host-owned base URL;
- provider-owned fixed route template;
- typed validated path components.

Path components are escaped with URL-path semantics. The adapter rejects control characters, backslashes, empty required components, `.`/`..` path segments for repository content paths, percent-encoded ambiguity in canonical resource fields, and values outside documented length bounds.

The result must remain under the configured base scheme/host/base-path boundary.

### Authentication

The GitHub credential is used only as an HTTP authorization value generated inside the adapter callback.

Requirements:

- never in URL/userinfo/query;
- never in log/evidence/error text;
- never copied into an intent or provider receipt;
- redirects disabled so the header cannot be forwarded to another origin;
- provider evidence is checked again by the v6 exact-secret leak detector.

### Response discipline

Write and reconcile paths use small bounded response limits per endpoint.

The adapter returns typed evidence containing only allowlisted fields such as:

- provider kind/version;
- operation;
- canonical resource digest;
- provider object/commit/comment ID when present;
- provider object URL **only if** it is same-provider and credential-free;
- idempotency marker digest;
- response status class.

Raw body text, raw provider error text, raw headers, and authentication metadata are not evidence.

## Canonical GitHub resources

Resources remain strings in the v6 `Grant`/`Intent` ABI, but the GitHub adapter parses them into typed structs and re-serializes them. If exact canonical re-serialization differs from the input string, the adapter rejects the resource.

### Repository contents

```text
github:repo:<owner>/<repo>:contents:<path>@<branch>
```

Constraints:

- owner and repo use a conservative GitHub identifier charset;
- branch is non-empty, bounded, contains no control characters, and is never interpreted as a URL;
- content path uses `/` separators only;
- no empty segment;
- no `.` or `..` segment;
- no backslash;
- no percent-encoded segment in the canonical string.

### Issue comment

```text
github:repo:<owner>/<repo>:issue:<positive-decimal-number>
```

### Pull request conversation comment

```text
github:repo:<owner>/<repo>:pull:<positive-decimal-number>
```

V7 `pull_request.comment.create` means a top-level pull-request conversation comment through GitHub's issue-comment surface. It does not mean a line-level code-review comment.

## Strict payloads

Every operation has a dedicated payload struct and strict JSON decoder. Unknown fields, duplicate fields, trailing values, invalid UTF-8 where textual content is required, and oversized payloads are rejected before provider entry.

### `github.repo.contents.write`

```json
{
  "content_b64": "...",
  "commit_message": "human-readable message",
  "expected_blob_sha": "optional-current-blob-sha"
}
```

Rules:

- decoded content size is bounded by the adapter's v7 limit;
- commit message is bounded and sanitized for control characters;
- `expected_blob_sha`, when supplied, must be lowercase hex with the expected GitHub object length accepted by the adapter;
- the adapter appends a deterministic Nolane action marker to the provider commit message;
- user-provided commit message cannot contain a Nolane action-marker prefix.

### `github.issue.comment.create`

```json
{
  "body": "comment text"
}
```

### `github.pull_request.comment.create`

```json
{
  "body": "comment text"
}
```

Comment body limits are explicit. User body cannot contain the reserved Nolane action-marker prefix.

## Provider idempotency marker

V7 derives a provider marker from the v6 deterministic `AdapterRequest.IdempotencyKey` without secret material:

```text
nolane-provider-v7:<sha256(idempotency-key)>
```

The exact syntax used externally is provider specific and versioned.

For comments the adapter appends a hidden HTML marker:

```html
<!-- nolane-provider-v7:<digest> -->
```

For repository content writes the adapter appends a fixed machine marker line to the commit message.

The marker is not a credential. It intentionally creates provider-observable provenance that lets reconciliation distinguish this action from an unrelated identical user-visible payload.

## GitHub execution strategies

### Comment creation

Before the irreversible POST, the adapter does **not** perform a create-on-retry loop. The v6 ledger already guarantees one provider-entry attempt per action unless independently reconciled.

Execute:

1. validate typed resource/payload;
2. construct marker;
3. create body with reserved marker appended;
4. issue exactly one POST to the provider route;
5. on any provider/transport error after request execution begins, return an adapter failure; v6 leaves the action pending/uncertain;
6. on success return sanitized typed evidence.

### Repository contents write

Execute:

1. validate typed resource/payload;
2. construct action marker in commit message;
3. issue exactly one provider write using the configured branch and optional expected current blob SHA;
4. never automatically repeat a write after an ambiguous transport/provider error;
5. successful provider IDs are sanitized into typed evidence.

No write method in v7 contains an internal automatic retry loop.

## Reconciliation strategies

Reconciliation is provider read-only observation and must never call a write endpoint.

### Comment reconciliation

The adapter queries the provider's comment listing/read surface for the exact repository/issue or pull request and searches a bounded provider result window for the exact v7 action marker.

- exact marker observed -> `ReconcileObserved`;
- provider proves the requested object cannot exist because the parent resource itself does not exist -> `ReconcileAbsent`;
- marker not found in a bounded or paginated search -> `ReconcileUnknown`, **not** absent;
- transport/provider ambiguity -> stable reconcile failure/unknown according to whether any trustworthy observation was obtained.

A bounded search can prove presence, not absence.

### Repository contents reconciliation

The adapter uses read-only commit/content surfaces to search for the exact action marker on the configured branch/path.

- exact marker observed -> `ReconcileObserved`;
- strong provider proof that the target repository/ref did not exist for the action is not reconstructed retroactively, so ordinary non-observation -> `ReconcileUnknown`;
- v7 does not infer `absent` merely because the current file content differs or a short commit window lacks the marker.

This favors availability loss over accidental duplicate write.

## Secret-broker and provider error taxonomy

V7 adds stable errors. Names are illustrative API requirements and may be grouped by package, but raw external text must never escape.

Credential boundary:

- `ErrBrokerUnavailable`
- `ErrBrokerPeerMismatch`
- `ErrBrokerUnsupported`
- `ErrBrokerProtocol`
- `ErrBrokerResponseTooLarge`
- existing `delegation.ErrSecretUnavailable` at the Plane boundary

Provider transport/adapter:

- `ErrInvalidProviderConfig`
- `ErrInvalidProviderResource`
- `ErrInvalidProviderPayload`
- `ErrProviderTransport`
- `ErrProviderRejected`
- `ErrProviderResponse`
- existing `delegation.ErrAdapterFailure` / `ErrReconcileFailure` at the Plane boundary

The delegated plane continues to sanitize provider-specific errors to its v6 stable sentinels.

## GitHub token expectations

V7 does not mint tokens. It consumes a secret supplied by the host secret agent.

Operational guidance:

- prefer fine-grained GitHub tokens or GitHub App installation tokens;
- token repository scope should be no broader than the corresponding host grant;
- token permission scope should be no broader than the adapter operation set;
- token rotation is performed by the secret backend without changing the v6 `SecretHandle` when operationally appropriate;
- rotating secret bytes must not alter request digests because v6 binds the opaque handle, not credential material.

V7 does not claim that token scope alone enforces Nolane policy; Nolane's exact grant checks remain mandatory even when provider credentials are already narrow.

## Secret-agent compromise boundary

The host secret agent is trusted to return the credential for the requested opaque handle. V7 prevents the agent/model from choosing the handle, but it cannot detect a malicious secret agent intentionally returning the wrong credential bytes.

Mitigations:

- peer identity pinned;
- one opaque handle per host grant;
- provider token should itself be narrowly scoped;
- adapter endpoint/provider kind fixed by host configuration;
- external provider authorization failures remain safe failures, not reasons to widen scope.

A future hardware-attested secret-agent protocol may strengthen this boundary; it is not required for v7.

## Provider Authority Gauntlet v7

V7 adds a third deterministic host-authority evidence family beside v4 and v6. The suite uses controlled fake Unix broker/provider endpoints and synthetic credentials.

Every registered scenario is mandatory.

Minimum scenario set:

1. **broker peer mismatch denied** — a socket server with the wrong expected peer identity cannot lease a secret;
2. **broker malformed/unknown-field response denied** — permissive JSON cannot cross the credential boundary;
3. **broker oversized response denied** — size bounds fail before callback/provider entry;
4. **secret absent from broker/provider evidence** — exact synthetic secret marker never appears in report bytes;
5. **provider endpoint cannot be rebound by intent** — hostile payload/resource URL-like text does not change configured origin;
6. **redirect denied with no credential forwarding** — provider 3xx never causes a second-origin request;
7. **generic provider HTTP cannot register as a delegation adapter** — v6 invariant remains enforced;
8. **GitHub resource canonicalization rejects ambiguous path/resource forms**;
9. **strict payload rejects unknown/duplicate/trailing fields before provider entry**;
10. **comment effect-then-transport-failure remains pending** — no automatic second POST;
11. **comment reconciliation observes marker without re-execution**;
12. **bounded comment non-observation returns unknown, not false absence/retry permission**;
13. **content-write effect-then-failure remains pending**;
14. **content reconciliation observes exact commit marker without a write call**;
15. **provider error body containing synthetic secret/raw diagnostics is sanitized from agent-facing error/evidence**;
16. **action-ID/resource rebinding still collides under provider adapter**;
17. **revoked/stale delegation still cannot execute provider write**;
18. **v6 evidence remains byte-stable**;
19. **v4 evidence remains byte-stable**.

The gauntlet must include proof-of-exercise markers showing the attack reached the intended broker/transport/provider boundary. A scenario cannot pass merely because setup failed before exercising the defense.

## Deterministic v7 evidence

CLI:

```bash
go run ./cmd/nolane-provider-gauntlet --out release-evidence/nolane-provider-v7.json
```

CI runs it twice and requires byte-for-byte equality before upload.

The report must not contain:

- synthetic secret bytes;
- base64/hex representation of the synthetic secret;
- broker socket temp path;
- raw Authorization header;
- raw provider response body;
- nondeterministic timestamps or random IDs.

V7 report uses its own suite/version and must not alter v4/v6 evidence generation.

## CI trust boundary

Ordinary pull-request CI uses only synthetic credentials and local controlled endpoints.

No repository/provider production secret is exposed to PR-triggered jobs.

A future live-provider workflow, if added, must follow the same pattern as v5:

- manual or protected-environment trigger;
- trusted branch only;
- no untrusted PR code with provider credentials;
- exact-commit artifact;
- explicit `LIVE_PROVIDER_PASS`/`UNAVAILABLE` distinction.

Such a live-provider workflow is not required for the deterministic v7 merge gate and its absence must not be mislabeled as live provider verification.

## Implementation boundaries

Expected new code is limited to Nolane-owned layers:

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

Small changes to `delegation` are allowed only when required to expose a safe reusable helper or stable contract. No change may give the intent control over adapter kind, secret handle, or provider endpoint.

Cube security-core implementation remains untouched.

## TDD requirements

Implementation proceeds RED -> GREEN for each trust boundary.

Required RED contracts include:

- peer identity mismatch;
- strict broker decoder unknown field/trailing frame;
- broker response bound;
- working secret buffer wipe behavior observable through test-owned references where possible without exposing production mutation APIs;
- provider base URL/redirect policy;
- endpoint non-rebinding;
- canonical GitHub resource parser;
- strict per-operation payload parser;
- no internal write retry;
- comment uncertain-effect reconciliation;
- content uncertain-effect reconciliation;
- provider response/error sanitization;
- reserved marker injection denial;
- deterministic v7 report;
- v6 and v4 evidence non-drift.

Race tests must cover concurrent Vault leases and concurrent independent provider actions.

## Security properties intentionally not claimed

V7 does not claim:

- that the host secret agent or external KMS cannot be compromised;
- hardware-backed memory secrecy inside the Nolane process;
- guaranteed zeroization by the Go compiler/runtime or copies outside the explicitly owned working buffer;
- distributed consensus for grant/effect state;
- whole-host storage rollback resistance;
- that GitHub itself cannot lie or be compromised;
- live production-provider verification without a separate exact-commit live artifact;
- line-level GitHub code-review comments;
- AWS/email adapters in v7;
- generic authenticated HTTP.

## Release gate

V7 may merge only when all of the following hold on the exact PR head:

1. all Go unit tests pass;
2. `go test -race ./...` passes;
3. `go vet ./...` passes;
4. Docs Build passes;
5. Format Check passes on required architectures;
6. Provider Authority Gauntlet v7 passes every mandatory scenario;
7. v7 evidence is byte-identical across two executions;
8. synthetic secret bytes and encoded forms are absent from v7 evidence;
9. v6 deterministic evidence remains byte-stable;
10. v4 deterministic evidence remains byte-stable;
11. provider write tests prove no internal automatic retry after provider entry;
12. reconciliation tests prove observation never performs a write;
13. broker tests prove peer mismatch, malformed response, and oversized response fail closed;
14. provider tests prove redirects do not forward credentials;
15. strict resource/payload tests reject ambiguous encodings before provider entry;
16. final diff does not modify Cube/KVM/RustVMM/CubeNet/CubeEgress/CubeCoW security-core implementation;
17. agent-generated commits contain the required `Autonomously-by: ChatGPT:GPT-5.6-Sol` trailer;
18. no agent-generated human `Signed-off-by` is added.

## Exit criterion

V7 is complete when Nolane Sandbox can take a v6 delegated intent, resolve a credential through the authenticated host-local broker, execute one of the explicitly supported GitHub operations against a host-configured provider endpoint, quarantine ambiguous outcomes without replay, reconcile by provider-observable action markers, and emit deterministic secret-free adversarial evidence proving those boundaries.

Completion of v7 does **not** by itself certify a real external KMS/HSM or a production GitHub account. Those require environment-specific live evidence in a later gate.