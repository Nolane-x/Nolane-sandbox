from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
realm_source = (ROOT / "NolaneWorld/realm/realization_authority.go").read_text(encoding="utf-8")
cube_source = (ROOT / "NolaneWorld/substrate/cube/realm_resource_authority.go").read_text(encoding="utf-8")
memory_store_source = (ROOT / "NolaneWorld/realm/store.go").read_text(encoding="utf-8")
durable_store_source = (ROOT / "NolaneWorld/realm/durable.go").read_text(encoding="utf-8")

required_realm = (
    "type RealizationAuthority struct",
    "binding RealizationBinding",
    "seal    *realizationAuthoritySeal",
    "type realizationAuthorityStore interface",
    "packageOwnedRealizationAuthorityStore()",
    "func (*MemoryStore) packageOwnedRealizationAuthorityStore()",
    "func (*DurableStore) packageOwnedRealizationAuthorityStore()",
    "func (c *Controller) trustedRealizationAuthorityStore()",
    "func (c *Controller) CurrentRealizationAuthority",
    "func (c *Controller) ValidateRealizationAuthority",
    "store, trusted := c.trustedRealizationAuthorityStore()",
    "PolicyDigest(realmRec.Spec, realmRec.Revision)",
    "store.Realm(realmID)",
    "store.World(realmID, worldID)",
    "WorldObservedReady, WorldLeased, WorldPaused",
)
for needle in required_realm:
    assert needle in realm_source, f"missing Realm authority contract: {needle}"

assert "func NewRealizationAuthority" not in realm_source, "public field-based authority constructor is forbidden"
assert "json:\"" not in realm_source, "opaque authority file must not introduce serialized authority fields"

required_cube = (
    "type RealmResourceAuthority struct",
    "controller.ValidateRealizationAuthority(ctx, realization)",
    "sandboxID := resource.sandboxID",
    "string(binding.SubstrateHandle) != sandboxID",
    "ErrRealmResourceAuthorityMismatch",
)
for needle in required_cube:
    assert needle in cube_source, f"missing Cube realization contract: {needle}"

assert "func NewRealmResourceAuthority" not in cube_source, "public cross-boundary authority constructor is forbidden"
assert "json:\"" not in cube_source, "Cube authority must remain in-process and non-serializable"

handle_transition_guard = (
    'old.Handle != "" && rec.Handle != old.Handle && '
    "rec.RealizationRevision == old.RealizationRevision"
)
assert handle_transition_guard in memory_store_source, (
    "MemoryStore must reject established-handle rebinding without realization advance"
)
assert handle_transition_guard in durable_store_source, (
    "DurableStore and journal replay must reject established-handle rebinding without realization advance"
)
assert "s.validateWorldLocked(r)" in durable_store_source, (
    "durable recovery must replay through the same World transition validator"
)

print("Wave23 Realm/Cube realization authority static contract: PASS")
