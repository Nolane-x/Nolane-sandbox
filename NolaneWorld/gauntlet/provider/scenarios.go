package providergauntlet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	broker "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/credential/broker"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	delegationgauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/delegation"
	v4scenarios "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/scenarios"
	githubprovider "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/providers/github"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

const SyntheticSecret = "SYNTHETIC-V7-SECRET"

const (
	v4EvidenceSHA256 = "94ef192c57f2587d34a8340a8bfd8d297782e121c88ad4aa96792e42bf40c6f4"
	v6EvidenceSHA256 = "34705e6ce2128ce884447004257d22fe577ad0b98ef1cf91df0f57ae270148ce"
)

var providerNow = time.Date(2026, 8, 30, 1, 0, 0, 0, time.UTC)

func RunStandard(ctx context.Context, policy gauntlet.Policy) (gauntlet.Report, error) {
	return gauntlet.NewRunner(policy).Run(ctx, Standard())
}

func Standard() []gauntlet.Scenario {
	return []gauntlet.Scenario{
		providerScenario("provider.v7.action-resource-collision", "An action ID cannot be rebound across provider resources.", "Execute an authorized provider action and reuse its action ID under a second exact grant/resource.", "The effect ledger rejects the changed request digest before a second provider write.", "provider-action-collision-denied", actionResourceCollision),
		providerScenario("provider.v7.broker-noncanonical", "The credential broker accepts one canonical response encoding only.", "Return unknown-field and alternate-encoding broker responses over a real Unix socket.", "The BrokerVault rejects both before a secret lease callback can run.", "broker-noncanonical-denied", brokerNoncanonical),
		providerScenario("provider.v7.broker-oversized", "Credential material is bounded before trusted adapter entry.", "Return a valid broker response whose decoded secret exceeds the configured lease bound.", "The BrokerVault rejects the response and never enters the callback.", "broker-oversized-denied", brokerOversized),
		providerScenario("provider.v7.broker-peer-mismatch", "A Unix credential broker must match the host-pinned peer UID before the handle is disclosed.", "Connect to a real Unix broker while intentionally pinning the wrong peer UID.", "SO_PEERCRED verification rejects the peer before the request frame is sent.", "broker-peer-denied", brokerPeerMismatch),
		providerScenario("provider.v7.comment-ambiguous-once", "An ambiguous comment write is never automatically executed twice.", "Let the TLS provider receive the POST and then drop the connection before a response is trusted.", "The durable effect journal leaves the action pending and a retry is quarantined.", "comment-one-post-only", commentAmbiguousOnce),
		providerScenario("provider.v7.comment-reconcile-observed", "Provider observation can close an uncertain comment without re-execution.", "Persist the provider marker, fail the write response, then reconcile the historical pending action.", "Read-only reconciliation observes the exact marker and resolves the journal with one POST total.", "comment-observed-no-rewrite", commentReconcileObserved),
		providerScenario("provider.v7.comment-reconcile-unknown", "Bounded comment search proves presence, not absence.", "Scan ten provider pages without the exact action marker.", "The adapter returns unknown and never converts bounded non-observation into absence.", "comment-nonobservation-unknown", commentReconcileUnknown),
		providerScenario("provider.v7.concurrent-secret-leases", "Independent broker leases cannot cross credential buffers.", "Resolve two opaque handles concurrently through one real Unix broker.", "Each callback receives only its own fresh lease bytes.", "broker-leases-isolated", concurrentSecretLeases),
		providerScenario("provider.v7.contents-ambiguous-once", "An ambiguous contents write is never automatically executed twice.", "Let the TLS provider receive the PUT and drop the connection before a response is trusted.", "The effect journal remains pending and retry cannot issue a second PUT.", "contents-one-put-only", contentsAmbiguousOnce),
		providerScenario("provider.v7.contents-reconcile-observed", "Repository contents reconciliation is read-only and marker-bound.", "Expose a commit carrying the exact provider action marker on the canonical branch/path.", "The adapter observes the commit by GET and emits sanitized evidence without any write.", "contents-marker-observed", contentsReconcileObserved),
		providerScenario("provider.v7.endpoint-rebinding", "Agent-controlled bytes cannot rebind the provider origin.", "Embed an alternate HTTPS origin in a provider resource/payload attempt.", "Canonical resource parsing rejects it before either provider endpoint is entered.", "endpoint-rebind-denied", endpointRebinding),
		providerScenario("provider.v7.generic-auth-http", "Generic authenticated HTTP is never a trusted delegated adapter.", "Attempt to register an authenticated-http adapter beside typed provider support.", "The v6 adapter registry rejects the generic transport kind.", "generic-auth-http-denied", genericAuthenticatedHTTP),
		providerScenario("provider.v7.payload-canonicality", "Provider operation payloads have one strict canonical JSON representation.", "Submit unknown, duplicate, trailing, and alternate-encoding fields.", "The typed GitHub adapter rejects each payload before provider entry.", "provider-payload-denied", payloadCanonicality),
		providerScenario("provider.v7.provider-error-sanitized", "Raw provider failures and credential echoes never become agent-visible errors.", "Return a provider error body containing the synthetic credential and diagnostics.", "The adapter returns only a stable sentinel and discards raw response text.", "provider-error-sanitized", providerErrorSanitized),
		providerScenario("provider.v7.redirect-credential-contained", "Credentials cannot follow provider redirects.", "Return a cross-origin redirect from the pinned TLS provider after Authorization is attached.", "The owned HTTP client refuses redirect following and the target receives no request.", "redirect-credential-contained", redirectCredentialContained),
		providerScenario("provider.v7.resource-canonicality", "Provider resources require exact canonical typed syntax.", "Submit delimiter ambiguity, traversal, percent-encoding, and noncanonical numbering.", "The adapter rejects every malformed resource before network entry.", "provider-resource-denied", resourceCanonicality),
		providerScenario("provider.v7.secret-evidence-absence", "Successful provider evidence never contains credential bytes or reversible credential encodings.", "Make the provider echo synthetic credentials inside its raw success body.", "The adapter parses only allowlisted IDs and rebuilds sanitized canonical evidence.", "provider-secret-absent", secretEvidenceAbsence),
		providerScenario("provider.v7.stale-revoked-denied", "Stale or revoked delegated authority cannot reach a provider write.", "Attempt one write after epoch advance and another after host revocation.", "The Delegated Authority Plane denies both before Adapter.Execute.", "stale-revoked-provider-denied", staleRevokedDenied),
		providerScenario("provider.v7.v4-evidence-stable", "Provider runtime work cannot silently change Release Gauntlet v4 evidence.", "Regenerate v4 with its exact historical policy.", "The raw canonical report hash matches the pre-v7 release contract.", "v4-evidence-unchanged", v4EvidenceStable),
		providerScenario("provider.v7.v6-evidence-stable", "Provider runtime work cannot silently change Delegated Authority v6 evidence.", "Regenerate v6 with its exact historical policy.", "The raw canonical report hash matches the master pre-v7 artifact.", "v6-evidence-unchanged", v6EvidenceStable),
	}
}

func providerScenario(id, invariant, attack, defense, marker string, fn func(context.Context, *gauntlet.Probe) error) gauntlet.Scenario {
	return gauntlet.ScenarioFunc{Definition: gauntlet.ScenarioSpec{ID: id, Invariant: invariant, Attack: attack, ExpectedDefense: defense, Severity: gauntlet.SeverityCritical, RequiredMarkers: []string{marker}}, Execute: fn}
}

func record(p *gauntlet.Probe, kind gauntlet.EventKind, marker, detail string) error {
	return p.Record(kind, marker, detail)
}

func proof(p *gauntlet.Probe, boundary, denial, detail string) error {
	if err := record(p, gauntlet.EventBoundary, boundary, detail); err != nil {
		return err
	}
	return record(p, gauntlet.EventDenial, denial, detail)
}

func markerFor(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "nolane-provider-v7:" + hex.EncodeToString(sum[:])
}

func rootsFor(server *httptest.Server) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return pool
}

func newGitHubAdapter(server *httptest.Server) (*githubprovider.Adapter, error) {
	return githubprovider.New(githubprovider.Config{BaseURL: server.URL, RootCAs: rootsFor(server), Timeout: 2 * time.Second})
}

func useSyntheticSecret(fn func(delegation.Secret) error) error {
	return delegation.WithSecretLease([]byte(SyntheticSecret), fn)
}

func baseGrant(id delegation.ID, resource string, operation delegation.Operation) delegation.Grant {
	return delegation.Grant{ID: id, WorldID: "provider-v7-world", AuthorityEpoch: 1, Adapter: githubprovider.Kind, Resource: resource, Operations: []delegation.Operation{operation}, SecretHandle: delegation.SecretHandle("provider-v7-secret"), IssuedAt: providerNow.Add(-time.Hour), ExpiresAt: providerNow.Add(time.Hour)}
}

func baseIntent(grant delegation.Grant, actionID string, payload []byte) delegation.Intent {
	return delegation.Intent{WorldID: grant.WorldID, AuthorityEpoch: grant.AuthorityEpoch, ActionID: actionID, DelegationID: grant.ID, Operation: grant.Operations[0], Resource: grant.Resource, Payload: payload}
}

func makePlane(adapter delegation.Adapter, grants []delegation.Grant, ledger authority.InspectableLedger) (*delegation.Plane, *world.State, *delegation.MemoryStore, error) {
	state, err := world.NewState("provider-v7-world")
	if err != nil {
		return nil, nil, nil, err
	}
	store := delegation.NewMemoryStore()
	for _, grant := range grants {
		if err := store.Issue(grant); err != nil {
			return nil, nil, nil, err
		}
	}
	vault := delegation.NewMemoryVault()
	if err := vault.Put("provider-v7-secret", []byte(SyntheticSecret)); err != nil {
		return nil, nil, nil, err
	}
	registry, err := delegation.NewRegistry(adapter)
	if err != nil {
		return nil, nil, nil, err
	}
	plane, err := delegation.NewPlane(state, store, vault, registry, ledger, func() time.Time { return providerNow })
	if err != nil {
		return nil, nil, nil, err
	}
	return plane, state, store, nil
}

func openResolvingLedger() (*authority.JournalLedger, func(), error) {
	dir, err := os.MkdirTemp("", "nolane-provider-v7-ledger-")
	if err != nil {
		return nil, nil, err
	}
	ledger, err := authority.OpenJournalLedger(filepath.Join(dir, "effects.jsonl"))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, err
	}
	return ledger, func() { _ = ledger.Close(); _ = os.RemoveAll(dir) }, nil
}

type brokerResponder func([]byte) []byte

func startBroker(responder brokerResponder, accepts int) (string, func(), error) {
	if accepts <= 0 {
		return "", nil, errors.New("invalid broker accept count")
	}
	dir, err := os.MkdirTemp("", "nolane-provider-v7-broker-")
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "broker.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < accepts; i++ {
			conn, err := ln.Accept()
			if err != nil {
				break
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				raw, _ := io.ReadAll(c)
				if responder != nil {
					if out := responder(raw); len(out) != 0 {
						_, _ = c.Write(out)
					}
				}
			}(conn)
		}
		wg.Wait()
	}()
	cleanup := func() {
		_ = ln.Close()
		<-done
		_ = os.RemoveAll(dir)
	}
	return path, cleanup, nil
}

func framed(body []byte) []byte {
	out := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(out[:4], uint32(len(body)))
	copy(out[4:], body)
	return out
}

func validBrokerResponse(secret string) []byte {
	msg := struct {
		Version   int    `json:"version"`
		SecretB64 string `json:"secret_b64,omitempty"`
	}{Version: 1, SecretB64: base64.StdEncoding.EncodeToString([]byte(secret))}
	raw, _ := json.Marshal(msg)
	return framed(raw)
}

func requestHandle(raw []byte) string {
	if len(raw) < 5 {
		return ""
	}
	n := int(binary.BigEndian.Uint32(raw[:4]))
	if n <= 0 || 4+n > len(raw) {
		return ""
	}
	var msg struct {
		Version int    `json:"version"`
		Handle  string `json:"handle"`
	}
	if json.Unmarshal(raw[4:4+n], &msg) != nil || msg.Version != 1 {
		return ""
	}
	return msg.Handle
}

func brokerPeerMismatch(ctx context.Context, p *gauntlet.Probe) error {
	uid, ok := currentUID()
	if !ok {
		return broker.ErrBrokerUnsupported
	}
	path, cleanup, err := startBroker(nil, 1)
	if err != nil {
		return err
	}
	defer cleanup()
	wrong := uid + 1
	if wrong == uid {
		wrong = uid - 1
	}
	vault, err := broker.New(broker.Config{SocketPath: path, ExpectedPeerUID: wrong, MaxSecretBytes: 64})
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "broker-peer-mismatch-attempt", "client intentionally pinned a peer UID different from the Unix broker process"); err != nil {
		return err
	}
	called := false
	err = vault.Use(ctx, "github-token", func(delegation.Secret) error { called = true; return nil })
	if !errors.Is(err, broker.ErrBrokerPeerMismatch) || called {
		return errors.New("broker peer mismatch was not denied before callback")
	}
	return proof(p, "unix-peer-credential-boundary", "broker-peer-denied", "SO_PEERCRED rejected the mismatched host peer before secret leasing")
}

func brokerNoncanonical(ctx context.Context, p *gauntlet.Probe) error {
	uid, ok := currentUID()
	if !ok {
		return broker.ErrBrokerUnsupported
	}
	var response atomic.Int64
	path, cleanup, err := startBroker(func([]byte) []byte {
		if response.Add(1) == 1 {
			return framed([]byte(`{"version":1,"secret_b64":"U0FGRQ==","extra":1}`))
		}
		return framed([]byte(`{"version":1, "secret_b64":"U0FGRQ=="}`))
	}, 2)
	if err != nil {
		return err
	}
	defer cleanup()
	vault, err := broker.New(broker.Config{SocketPath: path, ExpectedPeerUID: uid, MaxSecretBytes: 64})
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "broker-noncanonical-attempt", "broker returned an unknown field and then an alternate JSON encoding"); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		called := false
		err = vault.Use(ctx, "github-token", func(delegation.Secret) error { called = true; return nil })
		if !errors.Is(err, broker.ErrBrokerProtocol) || called {
			return errors.New("noncanonical broker response reached callback")
		}
	}
	return proof(p, "canonical-broker-protocol", "broker-noncanonical-denied", "strict response replay rejected both alternate encodings")
}

func brokerOversized(ctx context.Context, p *gauntlet.Probe) error {
	uid, ok := currentUID()
	if !ok {
		return broker.ErrBrokerUnsupported
	}
	path, cleanup, err := startBroker(func([]byte) []byte { return validBrokerResponse(strings.Repeat("A", 32)) }, 1)
	if err != nil {
		return err
	}
	defer cleanup()
	vault, err := broker.New(broker.Config{SocketPath: path, ExpectedPeerUID: uid, MaxSecretBytes: 8})
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "broker-oversized-attempt", "broker returned decoded credential bytes beyond the configured lease bound"); err != nil {
		return err
	}
	called := false
	err = vault.Use(ctx, "github-token", func(delegation.Secret) error { called = true; return nil })
	if !errors.Is(err, broker.ErrBrokerResponseTooLarge) || called {
		return errors.New("oversized broker secret reached callback")
	}
	return proof(p, "broker-secret-size-boundary", "broker-oversized-denied", "oversized credential material was rejected before adapter entry")
}

func concurrentSecretLeases(ctx context.Context, p *gauntlet.Probe) error {
	uid, ok := currentUID()
	if !ok {
		return broker.ErrBrokerUnsupported
	}
	path, cleanup, err := startBroker(func(raw []byte) []byte {
		switch requestHandle(raw) {
		case "handle-a":
			return validBrokerResponse("LEASE-A")
		case "handle-b":
			return validBrokerResponse("LEASE-B")
		default:
			return framed([]byte(`{"version":1,"error_code":"not_found"}`))
		}
	}, 2)
	if err != nil {
		return err
	}
	defer cleanup()
	vault, err := broker.New(broker.Config{SocketPath: path, ExpectedPeerUID: uid, MaxSecretBytes: 64})
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "concurrent-broker-leases", "two opaque handles were resolved concurrently through the same Unix broker"); err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	got := map[string]string{}
	var firstErr error
	for _, handle := range []delegation.SecretHandle{"handle-a", "handle-b"} {
		h := handle
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := vault.Use(ctx, h, func(secret delegation.Secret) error {
				mu.Lock()
				got[string(h)] = string(secret.Bytes())
				mu.Unlock()
				return nil
			})
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if firstErr != nil || got["handle-a"] != "LEASE-A" || got["handle-b"] != "LEASE-B" {
		return errors.New("concurrent broker lease buffers crossed or failed")
	}
	return proof(p, "fresh-secret-lease-boundary", "broker-leases-isolated", "each callback observed only the credential mapped to its own opaque handle")
}

func endpointRebinding(ctx context.Context, p *gauntlet.Probe) error {
	var pinned, alternate atomic.Int64
	serverA := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { pinned.Add(1) }))
	defer serverA.Close()
	serverB := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { alternate.Add(1) }))
	defer serverB.Close()
	adapter, err := newGitHubAdapter(serverA)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "endpoint-rebind-attempt", "an alternate HTTPS origin was embedded where a canonical provider resource was required"); err != nil {
		return err
	}
	request := delegation.AdapterRequest{Operation: githubprovider.OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:" + serverB.URL, Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "endpoint-rebind"}
	err = useSyntheticSecret(func(secret delegation.Secret) error { _, err := adapter.Execute(ctx, request, secret); return err })
	if !errors.Is(err, githubprovider.ErrInvalidProviderResource) || pinned.Load() != 0 || alternate.Load() != 0 {
		return errors.New("provider endpoint rebinding was not rejected before network")
	}
	return proof(p, "typed-provider-origin-boundary", "endpoint-rebind-denied", "resource canonicalization prevented agent bytes from becoming a provider origin")
}

func redirectCredentialContained(ctx context.Context, p *gauntlet.Probe) error {
	var targetCalls atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			targetCalls.Add(1000)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	adapter, err := newGitHubAdapter(source)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "cross-origin-provider-redirect", "pinned provider returned a redirect toward a second TLS origin after authentication was attached"); err != nil {
		return err
	}
	request := delegation.AdapterRequest{Operation: githubprovider.OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "redirect"}
	err = useSyntheticSecret(func(secret delegation.Secret) error { _, err := adapter.Execute(ctx, request, secret); return err })
	if !errors.Is(err, githubprovider.ErrProviderRejected) || targetCalls.Load() != 0 {
		return errors.New("redirect target was reached or redirect was accepted")
	}
	return proof(p, "owned-http-redirect-boundary", "redirect-credential-contained", "redirect following remained disabled and no credential-bearing request reached the target")
}

func resourceCanonicality(ctx context.Context, p *gauntlet.Probe) error {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "provider-resource-ambiguity", "malformed GitHub resources exercised delimiter, traversal, percent, and numbering ambiguity"); err != nil {
		return err
	}
	cases := []delegation.AdapterRequest{
		{Operation: githubprovider.OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:042", Payload: []byte(`{"body":"x"}`), IdempotencyKey: "r1"},
		{Operation: githubprovider.OpIssueComment, Resource: "github:repo:Nolane-x/Nolane%2fsandbox:issue:42", Payload: []byte(`{"body":"x"}`), IdempotencyKey: "r2"},
		{Operation: githubprovider.OpContentsWrite, Resource: "github:repo:Nolane-x/Nolane-sandbox:contents:docs/../x@main", Payload: []byte(`{"content_b64":"eA==","commit_message":"x"}`), IdempotencyKey: "r3"},
		{Operation: githubprovider.OpContentsWrite, Resource: "github:repo:Nolane-x/Nolane-sandbox:contents:docs/x@bad@branch", Payload: []byte(`{"content_b64":"eA==","commit_message":"x"}`), IdempotencyKey: "r4"},
	}
	for _, request := range cases {
		err := useSyntheticSecret(func(secret delegation.Secret) error { _, err := adapter.Execute(ctx, request, secret); return err })
		if !errors.Is(err, githubprovider.ErrInvalidProviderResource) {
			return errors.New("ambiguous provider resource was accepted")
		}
	}
	if calls.Load() != 0 {
		return errors.New("malformed provider resource reached network")
	}
	return proof(p, "canonical-provider-resource-boundary", "provider-resource-denied", "all noncanonical resource forms were rejected before provider entry")
}

func payloadCanonicality(ctx context.Context, p *gauntlet.Probe) error {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "provider-payload-ambiguity", "strict payload decoder received unknown, duplicate, trailing, and alternate JSON encodings"); err != nil {
		return err
	}
	payloads := [][]byte{
		[]byte(`{"body":"x","extra":1}`),
		[]byte(`{"body":"x","body":"y"}`),
		[]byte(`{"body":"x"}{}`),
		[]byte(`{ "body":"x"}`),
	}
	for i, payload := range payloads {
		request := delegation.AdapterRequest{Operation: githubprovider.OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Payload: payload, IdempotencyKey: "payload-" + strconv.Itoa(i)}
		err := useSyntheticSecret(func(secret delegation.Secret) error { _, err := adapter.Execute(ctx, request, secret); return err })
		if !errors.Is(err, githubprovider.ErrInvalidProviderPayload) {
			return errors.New("noncanonical provider payload was accepted")
		}
	}
	if calls.Load() != 0 {
		return errors.New("noncanonical provider payload reached network")
	}
	return proof(p, "canonical-provider-payload-boundary", "provider-payload-denied", "strict JSON parsing rejected every alternate payload before provider entry")
}

func providerErrorSanitized(ctx context.Context, p *gauntlet.Probe) error {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("provider diagnostics " + SyntheticSecret))
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "provider-error-secret-echo", "provider returned a failure body containing credential-shaped diagnostics"); err != nil {
		return err
	}
	request := delegation.AdapterRequest{Operation: githubprovider.OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "provider-error"}
	err = useSyntheticSecret(func(secret delegation.Secret) error { _, err := adapter.Execute(ctx, request, secret); return err })
	if !errors.Is(err, githubprovider.ErrProviderRejected) || strings.Contains(err.Error(), SyntheticSecret) || strings.Contains(err.Error(), "diagnostics") {
		return errors.New("provider error text escaped sanitization")
	}
	return proof(p, "provider-error-sanitization-boundary", "provider-error-sanitized", "raw provider body was discarded and only a stable sentinel remained")
}

func secretEvidenceAbsence(ctx context.Context, p *gauntlet.Probe) error {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 123, "body": SyntheticSecret, "diagnostic": base64.StdEncoding.EncodeToString([]byte(SyntheticSecret))})
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "provider-success-secret-echo", "provider success body echoed plaintext and encoded credential material beside a valid object ID"); err != nil {
		return err
	}
	request := delegation.AdapterRequest{Operation: githubprovider.OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "secret-evidence"}
	var effect delegation.Effect
	err = useSyntheticSecret(func(secret delegation.Secret) error { var inner error; effect, inner = adapter.Execute(ctx, request, secret); return inner })
	if err != nil {
		return err
	}
	forms := []string{SyntheticSecret, base64.StdEncoding.EncodeToString([]byte(SyntheticSecret)), hex.EncodeToString([]byte(SyntheticSecret))}
	for _, form := range forms {
		if bytes.Contains(effect.Evidence, []byte(form)) {
			return errors.New("credential representation appeared in sanitized provider evidence")
		}
	}
	return proof(p, "allowlisted-provider-evidence-boundary", "provider-secret-absent", "adapter rebuilt evidence from bounded provider IDs instead of returning raw response bytes")
}

func genericAuthenticatedHTTP(_ context.Context, p *gauntlet.Probe) error {
	if err := record(p, gauntlet.EventAttack, "generic-auth-http-registration", "generic authenticated HTTP adapter kind was presented to the delegated registry"); err != nil {
		return err
	}
	_, err := delegation.NewRegistry(dummyAdapter{kind: "authenticated-http"})
	if !errors.Is(err, delegation.ErrGenericAdapter) {
		return errors.New("generic authenticated HTTP registered")
	}
	return proof(p, "typed-adapter-registry-boundary", "generic-auth-http-denied", "generic credential-bearing transport remained forbidden")
}

func commentAmbiguousOnce(ctx context.Context, p *gauntlet.Probe) error {
	var posts atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = io.ReadAll(r.Body)
		if hijacker, ok := w.(http.Hijacker); ok {
			conn, _, err := hijacker.Hijack()
			if err == nil {
				_ = conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	ledger, cleanup, err := openResolvingLedger()
	if err != nil {
		return err
	}
	defer cleanup()
	grant := baseGrant("comment-once", "github:repo:Nolane-x/Nolane-sandbox:issue:42", githubprovider.OpIssueComment)
	plane, _, _, err := makePlane(adapter, []delegation.Grant{grant}, ledger)
	if err != nil {
		return err
	}
	intent := baseIntent(grant, "comment-once-action", []byte(`{"body":"hello"}`))
	if err := record(p, gauntlet.EventAttack, "comment-effect-response-loss", "provider accepted the comment request then the trusted response channel became ambiguous"); err != nil {
		return err
	}
	_, first := plane.Execute(ctx, intent)
	if !errors.Is(first, delegation.ErrAdapterFailure) {
		return errors.New("ambiguous provider comment did not fail closed")
	}
	_, second := plane.Execute(ctx, intent)
	if !errors.Is(second, authority.ErrActionUncertain) || posts.Load() != 1 {
		return errors.New("ambiguous comment action was executed more than once")
	}
	return proof(p, "pending-effect-journal-boundary", "comment-one-post-only", "pending uncertainty quarantined the retry and provider POST count remained one")
}

func contentsAmbiguousOnce(ctx context.Context, p *gauntlet.Probe) error {
	var puts atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		puts.Add(1)
		_, _ = io.ReadAll(r.Body)
		if hijacker, ok := w.(http.Hijacker); ok {
			conn, _, err := hijacker.Hijack()
			if err == nil {
				_ = conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	ledger, cleanup, err := openResolvingLedger()
	if err != nil {
		return err
	}
	defer cleanup()
	grant := baseGrant("contents-once", "github:repo:Nolane-x/Nolane-sandbox:contents:docs/spec-v7.md@main", githubprovider.OpContentsWrite)
	plane, _, _, err := makePlane(adapter, []delegation.Grant{grant}, ledger)
	if err != nil {
		return err
	}
	intent := baseIntent(grant, "contents-once-action", []byte(`{"content_b64":"aGVsbG8=","commit_message":"update"}`))
	if err := record(p, gauntlet.EventAttack, "contents-effect-response-loss", "provider accepted the contents request then the trusted response channel became ambiguous"); err != nil {
		return err
	}
	_, first := plane.Execute(ctx, intent)
	if !errors.Is(first, delegation.ErrAdapterFailure) {
		return errors.New("ambiguous provider contents write did not fail closed")
	}
	_, second := plane.Execute(ctx, intent)
	if !errors.Is(second, authority.ErrActionUncertain) || puts.Load() != 1 {
		return errors.New("ambiguous contents action was executed more than once")
	}
	return proof(p, "pending-contents-journal-boundary", "contents-one-put-only", "pending uncertainty quarantined the retry and provider PUT count remained one")
}

func commentReconcileObserved(ctx context.Context, p *gauntlet.Probe) error {
	var posts, gets atomic.Int64
	var storedBody string
	var mu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			posts.Add(1)
			var body struct{ Body string `json:"body"` }
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			storedBody = body.Body
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
		case http.MethodGet:
			gets.Add(1)
			mu.Lock()
			body := storedBody
			mu.Unlock()
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 909, "body": body}})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	ledger, cleanup, err := openResolvingLedger()
	if err != nil {
		return err
	}
	defer cleanup()
	grant := baseGrant("comment-reconcile", "github:repo:Nolane-x/Nolane-sandbox:issue:42", githubprovider.OpIssueComment)
	plane, _, _, err := makePlane(adapter, []delegation.Grant{grant}, ledger)
	if err != nil {
		return err
	}
	intent := baseIntent(grant, "comment-reconcile-action", []byte(`{"body":"hello"}`))
	if err := record(p, gauntlet.EventAttack, "historical-comment-reconcile", "provider effect marker existed although the original write response was not trusted"); err != nil {
		return err
	}
	_, _ = plane.Execute(ctx, intent)
	receipt, err := plane.Reconcile(ctx, intent)
	if err != nil || receipt.EffectDigest == "" || posts.Load() != 1 || gets.Load() < 1 {
		return errors.New("comment reconciliation did not resolve the pending action safely")
	}
	return proof(p, "read-only-comment-reconciliation", "comment-observed-no-rewrite", "exact provider marker resolved the pending receipt without a second POST")
}

func commentReconcileUnknown(ctx context.Context, p *gauntlet.Probe) error {
	var gets atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "bounded-comment-nonobservation", "ten bounded provider pages omitted the exact action marker"); err != nil {
		return err
	}
	request := delegation.AdapterRequest{Operation: githubprovider.OpPullComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:pull:7", Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "comment-unknown"}
	var result delegation.ReconcileResult
	err = useSyntheticSecret(func(secret delegation.Secret) error { var inner error; result, inner = adapter.Reconcile(ctx, request, secret); return inner })
	if err != nil || result.State != delegation.ReconcileUnknown || gets.Load() != 10 {
		return errors.New("bounded comment non-observation did not remain unknown")
	}
	return proof(p, "bounded-presence-only-reconciliation", "comment-nonobservation-unknown", "ten-page scan preserved uncertainty instead of manufacturing absence")
}

func contentsReconcileObserved(ctx context.Context, p *gauntlet.Probe) error {
	var gets, writes atomic.Int64
	key := "contents-reconcile"
	marker := markerFor(key)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gets.Add(1)
		_ = json.NewEncoder(w).Encode([]map[string]any{{"sha": "89abcdef0123456789abcdef0123456789abcdef", "commit": map[string]any{"message": "update\n\n" + marker}}})
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "contents-marker-observation", "historical provider commit list exposed an exact v7 action marker"); err != nil {
		return err
	}
	request := delegation.AdapterRequest{Operation: githubprovider.OpContentsWrite, Resource: "github:repo:Nolane-x/Nolane-sandbox:contents:docs/spec-v7.md@main", Payload: []byte(`{"content_b64":"aGVsbG8=","commit_message":"update"}`), IdempotencyKey: key}
	var result delegation.ReconcileResult
	err = useSyntheticSecret(func(secret delegation.Secret) error { var inner error; result, inner = adapter.Reconcile(ctx, request, secret); return inner })
	if err != nil || result.State != delegation.ReconcileObserved || gets.Load() != 1 || writes.Load() != 0 {
		return errors.New("contents reconciliation was not read-only marker observation")
	}
	return proof(p, "read-only-contents-reconciliation", "contents-marker-observed", "exact commit marker was observed by GET without provider mutation")
}

func actionResourceCollision(ctx context.Context, p *gauntlet.Probe) error {
	var writes atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writes.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":101}`))
	}))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	g1 := baseGrant("collision-a", "github:repo:Nolane-x/Nolane-sandbox:issue:42", githubprovider.OpIssueComment)
	g2 := baseGrant("collision-b", "github:repo:Nolane-x/Nolane-sandbox:issue:43", githubprovider.OpIssueComment)
	plane, _, _, err := makePlane(adapter, []delegation.Grant{g1, g2}, authority.NewMemoryLedger())
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "provider-action-resource-rebind", "same action ID was submitted through a second exact grant targeting another provider resource"); err != nil {
		return err
	}
	if _, err := plane.Execute(ctx, baseIntent(g1, "shared-action", []byte(`{"body":"first"}`))); err != nil {
		return err
	}
	_, err = plane.Execute(ctx, baseIntent(g2, "shared-action", []byte(`{"body":"second"}`)))
	if !errors.Is(err, authority.ErrActionCollision) || writes.Load() != 1 {
		return errors.New("provider action ID was rebound across resources")
	}
	return proof(p, "effect-ledger-provider-binding", "provider-action-collision-denied", "changed provider request digest collided before a second network write")
}

func staleRevokedDenied(ctx context.Context, p *gauntlet.Probe) error {
	var writes atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes.Add(1); w.WriteHeader(http.StatusCreated); _, _ = w.Write([]byte(`{"id":1}`)) }))
	defer server.Close()
	adapter, err := newGitHubAdapter(server)
	if err != nil {
		return err
	}
	if err := record(p, gauntlet.EventAttack, "stale-and-revoked-provider-attempts", "provider writes were attempted once under stale epoch state and once under a revoked grant"); err != nil {
		return err
	}
	g1 := baseGrant("stale-grant", "github:repo:Nolane-x/Nolane-sandbox:issue:42", githubprovider.OpIssueComment)
	plane1, state1, _, err := makePlane(adapter, []delegation.Grant{g1}, authority.NewMemoryLedger())
	if err != nil {
		return err
	}
	state1.AdvanceEpoch()
	if _, err := plane1.Execute(ctx, baseIntent(g1, "stale-action", []byte(`{"body":"x"}`))); !errors.Is(err, world.ErrStaleEpoch) {
		return errors.New("stale provider action was not denied")
	}
	g2 := baseGrant("revoked-grant", "github:repo:Nolane-x/Nolane-sandbox:issue:43", githubprovider.OpIssueComment)
	plane2, _, store2, err := makePlane(adapter, []delegation.Grant{g2}, authority.NewMemoryLedger())
	if err != nil {
		return err
	}
	if err := store2.Revoke(g2.ID); err != nil {
		return err
	}
	if _, err := plane2.Execute(ctx, baseIntent(g2, "revoked-action", []byte(`{"body":"x"}`))); !errors.Is(err, delegation.ErrDelegationRevoked) {
		return errors.New("revoked provider action was not denied")
	}
	if writes.Load() != 0 {
		return errors.New("stale or revoked action reached provider")
	}
	return proof(p, "host-delegation-authority-boundary", "stale-revoked-provider-denied", "epoch and revocation checks stopped both actions before provider entry")
}

func v4EvidenceStable(ctx context.Context, p *gauntlet.Probe) error {
	if err := record(p, gauntlet.EventAttack, "v4-regeneration", "release gauntlet v4 was regenerated under its historical two-second policy"); err != nil {
		return err
	}
	report, err := v4scenarios.RunStandard(ctx, gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 2 * time.Second})
	if err != nil {
		return err
	}
	raw, err := gauntlet.MarshalReport(report)
	if err != nil {
		return err
	}
	if sha256String(raw) != v4EvidenceSHA256 {
		return errors.New("v4 evidence bytes drifted")
	}
	return proof(p, "historical-v4-evidence-boundary", "v4-evidence-unchanged", "v4 canonical report hash matched the pre-v7 contract")
}

func v6EvidenceStable(ctx context.Context, p *gauntlet.Probe) error {
	if err := record(p, gauntlet.EventAttack, "v6-regeneration", "delegated authority v6 was regenerated under its historical three-second policy"); err != nil {
		return err
	}
	report, err := delegationgauntlet.RunStandard(ctx, gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 3 * time.Second})
	if err != nil {
		return err
	}
	raw, err := gauntlet.MarshalReport(report)
	if err != nil {
		return err
	}
	if sha256String(raw) != v6EvidenceSHA256 {
		return errors.New("v6 evidence bytes drifted")
	}
	return proof(p, "historical-v6-evidence-boundary", "v6-evidence-unchanged", "v6 canonical report hash matched the master pre-v7 artifact")
}

func sha256String(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type dummyAdapter struct{ kind delegation.AdapterKind }

func (d dummyAdapter) Kind() delegation.AdapterKind { return d.kind }
func (d dummyAdapter) Execute(context.Context, delegation.AdapterRequest, delegation.Secret) (delegation.Effect, error) {
	return delegation.Effect{}, nil
}
func (d dummyAdapter) Reconcile(context.Context, delegation.AdapterRequest, delegation.Secret) (delegation.ReconcileResult, error) {
	return delegation.ReconcileResult{State: delegation.ReconcileUnknown}, nil
}
