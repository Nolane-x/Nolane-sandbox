from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
client = (ROOT / "NolaneWorld/substrate/cube/client.go").read_text(encoding="utf-8")
transport = (ROOT / "NolaneWorld/substrate/cube/provider_endpoint_spki_transport.go").read_text(encoding="utf-8")
proof = (ROOT / "NolaneWorld/substrate/cube/provider_endpoint_spki.go").read_text(encoding="utf-8")
bridge = (ROOT / "NolaneWorld/substrate/cube/realm_resource_provider_endpoint_authority.go").read_text(encoding="utf-8")
wave26_authority = "\n".join((transport, proof, bridge))

# Configuration is an explicit canonical SHA-256 SPKI allow-set. No TOFU,
# caller-normalized aliases, zero pins or duplicate entries are authority.
required_config = (
    "EndpointSPKIPins []string",
    "endpointSPKIPins, err := parseEndpointSPKIPins(cfg.EndpointSPKIPins)",
    'if len(endpointSPKIPins) > 0 && u.Scheme != "https"',
    "hardenedPinnedHTTPClient(cfg.HTTPClient, 30*time.Second, endpointSPKIPins)",
    "endpointSPKIPins: endpointSPKIPins",
)
for needle in required_config:
    assert needle in client, f"missing Wave26 client pin contract: {needle}"

required_parser = (
    "len(raw) != 64",
    "raw != strings.ToLower(raw)",
    "hex.DecodeString(raw)",
    "len(decoded) != sha256.Size",
    "if digest == zero",
    "if _, exists := pins[digest]; exists",
)
for needle in required_parser:
    assert needle in transport, f"missing Wave26 canonical pin parser rule: {needle}"

# Pinning augments the standard TLS handshake; it must not replace WebPKI or
# permit a custom TLS dial path / arbitrary RoundTripper that skips it.
required_transport = (
    "switch candidate := hc.Transport.(type)",
    "case *http.Transport:",
    "transport = candidate.Clone()",
    "transport.DialTLS != nil || transport.DialTLSContext != nil",
    "if tlsConfig.InsecureSkipVerify",
    "originalVerifyConnection := tlsConfig.VerifyConnection",
    "tlsConfig.VerifyConnection = func(state tls.ConnectionState) error",
    "originalVerifyConnection(state)",
    "len(state.PeerCertificates) == 0",
    "sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)",
    "trustedPins[digest]",
    "ErrEndpointSPKIMismatch",
    "transport.TLSClientConfig = tlsConfig",
)
for needle in required_transport:
    assert needle in transport, f"missing Wave26 TLS trust rule: {needle}"

# The type switch must contain a fail-closed default branch. Keep this less
# formatting-sensitive than an exact multi-line source assertion.
transport_switch = transport.split("switch candidate := hc.Transport.(type)", 1)[1].split("// These hooks", 1)[0]
assert "default:" in transport_switch and "ErrInvalidEndpointSPKIConfig" in transport_switch, (
    "pinned non-*http.Transport RoundTripper must fail closed"
)

for forbidden in (
    "InsecureSkipVerify: true",
    "InsecureSkipVerify = true",
    "VerifyPeerCertificate = nil",
    "TrustOnFirstUse",
    "trustOnFirstUse",
    "TOFU",
):
    assert forbidden not in transport, f"forbidden Wave26 TLS shortcut: {forbidden}"

# Endpoint proof is an opaque, exact-client capability minted only after a
# fresh request traverses the pinned transport. HTTP health content is not an
# identity source; authority comes from resp.TLS peer state.
required_proof = (
    "type ProviderEndpointSPKIProof struct",
    "spkiSHA256 [32]byte",
    "client     *Client",
    "seal       *providerEndpointSPKIProofSeal",
    "func (c *Client) ObserveProviderEndpointSPKI",
    'c.apiURL+"/health"',
    "resp, err := c.http.Do(req)",
    "resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0",
    "sha256.Sum256(resp.TLS.PeerCertificates[0].RawSubjectPublicKeyInfo)",
    "c.endpointSPKIPins[digest]",
    "func (c *Client) ValidateProviderEndpointSPKI",
    "proof.client != c",
    "current, err := c.ObserveProviderEndpointSPKI(ctx)",
    "!sameProviderEndpointSPKIProof(current, proof)",
    "ErrStaleProviderEndpointSPKIProof",
)
for needle in required_proof:
    assert needle in proof, f"missing Wave26 endpoint proof contract: {needle}"

assert "func NewProviderEndpointSPKIProof" not in proof, "public endpoint-proof constructor is forbidden"
proof_struct = proof.split("type ProviderEndpointSPKIProof struct {", 1)[1].split("\n}", 1)[0]
assert 'json:"' not in proof_struct, "endpoint proof authority fields must remain non-serializable"
assert "json.Unmarshal" not in proof, "health payload must not become endpoint identity authority"
assert "json.NewDecoder" not in proof, "health payload must not become endpoint identity authority"

# Wave26 bridge is additive. It must reconstruct Wave25 freshness first, prove
# the supplied Wave25 capability is exact, then freshly validate endpoint SPKI.
required_bridge = (
    "type RealmResourceProviderEndpointAuthority struct",
    "provider RealmResourceProviderIncarnationAuthority",
    "endpoint ProviderEndpointSPKIProof",
    "seal     *realmResourceProviderEndpointAuthoritySeal",
    "func ValidateRealmResourceProviderEndpointAuthority(",
    "ValidateRealmResourceProviderIncarnationAuthority(",
    "sameRealmResourceProviderIncarnationAuthority(freshProviderAuthority, providerAuthority)",
    "endpoint.client != client",
    "client.ValidateProviderEndpointSPKI(ctx, endpoint)",
    "ErrInvalidRealmResourceProviderEndpointAuthority",
)
for needle in required_bridge:
    assert needle in bridge, f"missing Wave26 bridge contract: {needle}"

assert bridge.index("ValidateRealmResourceProviderIncarnationAuthority(") < bridge.index(
    "client.ValidateProviderEndpointSPKI(ctx, endpoint)"
), "Wave26 bridge must freshly revalidate Wave25 authority before endpoint freshness"
assert "func NewRealmResourceProviderEndpointAuthority" not in bridge, "public Wave26 bridge constructor is forbidden"
bridge_struct = bridge.split("type RealmResourceProviderEndpointAuthority struct {", 1)[1].split("\n}", 1)[0]
assert 'json:"' not in bridge_struct, "Wave26 bridge authority must remain an opaque in-process capability"

# No authority laundering from self-reported HTTP identity, URL/certificate
# metadata, API credentials, timestamps, request/client IDs, or persistence.
# Scope these bans to Wave26 authority production files, not test fixtures.
for forbidden in (
    "Header.Get(",
    '"Server"',
    '"X-Provider-ID"',
    '"X-SPKI"',
    "time.Now(",
    "requestID",
    "requestId",
    "clientID",
    "clientId",
    "os.WriteFile(",
    "os.Create(",
    "ioutil.WriteFile(",
    "certificate.SerialNumber",
    "PeerCertificates[0].Raw)",
):
    assert forbidden not in wave26_authority, f"forbidden Wave26 endpoint-identity shortcut: {forbidden}"

# Sandbox/resource identity may be checked for equality in the bridge, but it
# must never be hashed into endpoint cryptographic authority.
for forbidden in (
    "sha256.Sum256([]byte(sandboxID",
    "sha256.Sum256([]byte(resource",
    "sha256.Sum256([]byte(c.apiURL",
    "sha256.Sum256([]byte(c.apiKey",
):
    assert forbidden not in wave26_authority, f"forbidden Wave26 derived endpoint identity: {forbidden}"

# Scope guard: endpoint cryptographic identity must not silently become a claim
# of task outcome, OOM causality, resource enforcement, hardware attestation or
# LIVE_PASS. Those words may appear only in explicit non-claim comments; no
# authority fields may encode them. "resource" itself is intentionally allowed
# because the older RealmResource authority is a legitimate Wave26 input.
for struct_body in (proof_struct, bridge_struct):
    for forbidden in ("oom", "task", "enforcement", "hardware", "live", "attestation"):
        assert forbidden not in struct_body.lower(), f"Wave26 authority field launders out-of-scope claim: {forbidden}"

print("Wave26 provider endpoint SPKI authority static contract: PASS")
