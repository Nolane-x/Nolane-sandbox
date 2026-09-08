from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
producer_identity = (ROOT / "CubeAPI/src/services/provider_incarnation.rs").read_text(encoding="utf-8")
producer_service = (ROOT / "CubeAPI/src/services/sandboxes.rs").read_text(encoding="utf-8")
models = (ROOT / "CubeAPI/src/models/mod.rs").read_text(encoding="utf-8")
consumer = (ROOT / "NolaneWorld/substrate/cube/provider_incarnation.go").read_text(encoding="utf-8")
bridge = (ROOT / "NolaneWorld/substrate/cube/realm_resource_provider_incarnation_authority.go").read_text(encoding="utf-8")

required_identity = (
    'PROVIDER_INCARNATION_ANNOTATION: &str = "nolane.provider.incarnation.v1"',
    "Uuid::new_v4().hyphenated().to_string()",
    "Uuid::parse_str(raw).ok()?",
    "parsed.get_version_num() != 4",
    "parsed.get_variant() != Variant::RFC4122",
    "parsed.hyphenated().to_string() != *raw",
)
for needle in required_identity:
    assert needle in producer_identity, f"missing Wave25 provider identity contract: {needle}"

# The authority mint module must remain independent from operational IDs, time,
# local realization state and caller metadata. Scope this ban to the dedicated
# identity module rather than unrelated CubeAPI operations.
for forbidden in (
    "chrono::Utc::now",
    "started_at",
    "create_at",
    "request_id",
    "client_id",
    "host_id",
    "sandbox_id",
    "sha256",
    "md5",
    "realization",
    "epoch",
    "Realm",
    "World",
    "metadata",
):
    assert forbidden not in producer_identity, f"forbidden provider incarnation derivation shortcut: {forbidden}"

mint = "let provider_incarnation_id = new_provider_incarnation_id();"
insert = "PROVIDER_INCARNATION_ANNOTATION.to_string(),"
create_req = "let req = CreateSandboxRequest {"
assert mint in producer_service
assert insert in producer_service
assert create_req in producer_service
assert producer_service.index(mint) < producer_service.index(insert) < producer_service.index(create_req), (
    "provider incarnation must be minted and inserted before CreateSandboxRequest is sent"
)

mint_block = producer_service[producer_service.index(mint) : producer_service.index(create_req)]
for forbidden in ("request_id", "host_id", "client_id", "chrono::", "started_at", "sandbox_id", "metadata"):
    assert forbidden not in mint_block, f"create-boundary incarnation mint depends on forbidden input: {forbidden}"

assert producer_service.count("provider_incarnation_from_annotations(&d.annotations)") >= 3, (
    "get/connect/resume must project only the persisted provider annotation"
)
assert "incarnation_id: provider_incarnation_id" in producer_service
assert "Some(provider_incarnation_id)" in producer_service

new_sandbox = models.split("pub struct NewSandbox {", 1)[1].split("\n}", 1)[0]
assert "incarnation" not in new_sandbox, "public NewSandbox must not let callers nominate provider incarnation"
assert "annotations" not in new_sandbox, "public NewSandbox must not expose internal annotations"
assert models.count('serde(rename = "incarnationID", skip_serializing_if = "Option::is_none")') >= 2, (
    "Sandbox and SandboxDetail must expose additive read-only incarnationID projection"
)

required_consumer = (
    "type ProviderIncarnationProof struct",
    "sandboxID     string",
    "incarnationID string",
    "client        *Client",
    "seal          *providerIncarnationProofSeal",
    "func (c *Client) ObserveProviderIncarnation",
    "c.doJSON(",
    '"/sandboxes/"+url.PathEscape(sandboxID)',
    "isCanonicalProviderIncarnationID(out.IncarnationID)",
    "proof.client != c",
    "current, err := c.ObserveProviderIncarnation(ctx, resource)",
    "!sameProviderIncarnationProof(current, proof)",
    "ErrStaleProviderIncarnationProof",
)
for needle in required_consumer:
    assert needle in consumer, f"missing Wave25 NolaneWorld provider proof contract: {needle}"

assert "func NewProviderIncarnationProof" not in consumer, "public provider-proof constructor is forbidden"
proof_struct = consumer.split("type ProviderIncarnationProof struct {", 1)[1].split("\n}", 1)[0]
assert 'json:"' not in proof_struct, "provider proof authority fields must remain non-serializable"
assert "strings.ReplaceAll(raw, \"-\", \"\")" in consumer
assert "raw[14] != '4'" in consumer
assert "case '8', '9', 'a', 'b':" in consumer

required_bridge = (
    "type RealmResourceProviderIncarnationAuthority struct",
    "local    RealmResourceEpochAuthority",
    "provider ProviderIncarnationProof",
    "seal     *realmResourceProviderIncarnationAuthoritySeal",
    "func ValidateRealmResourceProviderIncarnationAuthority(",
    "ValidateRealmResourceEpochAuthority(",
    "sameRealmResourceEpochAuthority(freshLocal, local)",
    "provider.client != client",
    "client.ValidateProviderIncarnation(ctx, resource, provider)",
    "ErrInvalidRealmResourceProviderIncarnationAuthority",
)
for needle in required_bridge:
    assert needle in bridge, f"missing Wave25 provider-bound Realm/Cube bridge contract: {needle}"

assert bridge.index("ValidateRealmResourceEpochAuthority(") < bridge.index(
    "client.ValidateProviderIncarnation(ctx, resource, provider)"
), "Wave25 bridge must freshly revalidate local Wave23/24 authority before provider freshness"
assert "func NewRealmResourceProviderIncarnationAuthority" not in bridge, "public Wave25 bridge constructor is forbidden"
assert 'json:"' not in bridge, "Wave25 bridge authority must remain an opaque in-process capability"

print("Wave25 provider-persisted incarnation authority static contract: PASS")
