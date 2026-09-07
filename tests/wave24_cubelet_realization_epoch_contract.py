from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
producer = (ROOT / "Cubelet/plugins/cube/internals/sandbox/realization_epoch.go").read_text(encoding="utf-8")
producer_store = (ROOT / "Cubelet/plugins/cube/internals/sandbox/task_outcome_proof.go").read_text(encoding="utf-8")
producer_manager = (ROOT / "Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go").read_text(encoding="utf-8")
producer_export = (ROOT / "Cubelet/plugins/cube/internals/sandbox/realization_epoch_export.go").read_text(encoding="utf-8")
exporter = (ROOT / "Cubelet/plugins/cube/internals/resourcemetrics/realization_epoch_prometheus.go").read_text(encoding="utf-8")
plugin = (ROOT / "Cubelet/plugins/cube/internals/resourcemetrics/plugin.go").read_text(encoding="utf-8")
consumer = (ROOT / "NolaneWorld/substrate/cube/realization_epoch.go").read_text(encoding="utf-8")
consumer_validation = (ROOT / "NolaneWorld/substrate/cube/realization_epoch_validation.go").read_text(encoding="utf-8")
bridge = (ROOT / "NolaneWorld/substrate/cube/realm_resource_epoch_authority.go").read_text(encoding="utf-8")

required_producer = (
    '"crypto/rand"',
    "rand.Read(token[:])",
    "type RealizationEpoch struct",
    "Generation uint64",
    "Token      [32]byte",
    "func (s *taskOutcomeProofStore) BeginRealizationEpoch",
    "func (s *taskOutcomeProofStore) CurrentRealizationEpoch",
    "func (s *taskOutcomeProofStore) IsCurrentRealizationEpoch",
    "beginRealizationEpochWithTokenGenerator",
    "token == ([32]byte{})",
    "s.generations[sandboxID]++",
    "s.realizationEpochs[sandboxID] = epoch",
)
for needle in required_producer:
    assert needle in producer, f"missing Wave24 producer authority contract: {needle}"

for forbidden in ('time.Now', 'sha256', 'md5', 'uuid.New', 'guestOOMVictimTokenGenerator'):
    assert forbidden not in producer, f"forbidden deterministic/reused epoch minting shortcut: {forbidden}"

assert "realizationEpochs map[string]RealizationEpoch" in producer_store
assert "delete(s.realizationEpochs, sandboxID)" in producer_store, "Clear/legacy begin must invalidate old epoch state"
assert "epoch, err := c.beginTaskOutcomeRealizationEpoch(sandboxID)" in producer_manager
assert "generation := epoch.Generation" in producer_manager
assert producer_manager.index("epoch, err := c.beginTaskOutcomeRealizationEpoch(sandboxID)") < producer_manager.index("c.beginKernelVictimWindow"), (
    "epoch authority must be established before downstream realization evidence starts"
)
assert "VisitRealizationEpochs" in producer_export
assert "s.generations[sandboxID] != epoch.Generation" in producer_export

required_exporter = (
    '"cubesandbox_realization_epoch_info"',
    '[]string{"sandbox_id", "generation", "token"}',
    "strconv.FormatUint(generation, 10)",
    "hex.EncodeToString(token[:])",
    "strings.TrimSpace(sandboxID) != sandboxID",
    "token != ([32]byte{})",
    "newProductionTaskEvidenceServiceWithRealizationEpochs",
    "prometheus.NewRegistry()",
    "registry.MustRegister(&realizationEpochPrometheusCollector{epochs: epochs})",
)
for needle in required_exporter:
    assert needle in exporter, f"missing Wave24 exporter contract: {needle}"

assert "realizationEpochs, ok := controllerPlugin.(realizationEpochVisitor)" in plugin
assert "newProductionTaskEvidenceServiceWithRealizationEpochs" in plugin
assert "/v1/metrics/" not in exporter, "Wave24 must not create a parallel HTTP endpoint"

required_consumer = (
    "type RealizationEpochProof struct",
    "seal       *realizationEpochProofSeal",
    "type RealizationEpochObserver struct",
    "hostResourceMetricsPath",
    "parseRealizationEpochMetrics",
    "splitMetricToken(fields[0])",
    'rawValue != "1"',
    "parseCanonicalUint(labels[\"generation\"], 64)",
    "hex.EncodeToString(decoded) != raw",
    "token == ([32]byte{})",
)
for needle in required_consumer:
    assert needle in consumer, f"missing Wave24 consumer contract: {needle}"

assert "func NewRealizationEpochProof" not in consumer, "public field-based epoch proof constructor is forbidden"
assert 'json:"' not in consumer, "epoch proof must remain an in-process capability"

required_freshness = (
    "func (o *RealizationEpochObserver) ValidateCurrent",
    "current, ok, err := o.Observe(ctx, binding)",
    "!sameRealizationEpochProof(current, proof)",
    "ErrStaleRealizationEpochProof",
)
for needle in required_freshness:
    assert needle in consumer_validation, f"missing current-epoch freshness contract: {needle}"

required_bridge = (
    "type RealmResourceEpochAuthority struct",
    "realmResource RealmResourceAuthority",
    "epoch         RealizationEpochProof",
    "seal          *realmResourceEpochAuthoritySeal",
    "ValidateRealmResourceAuthority(ctx, controller, realization, resource)",
    "epochObserver.ValidateCurrent(ctx, resource, epochProof)",
    "epochProof.sandboxID != baseSandboxID",
)
for needle in required_bridge:
    assert needle in bridge, f"missing Wave24 Realm/Cube epoch bridge contract: {needle}"

assert "func NewRealmResourceEpochAuthority" not in bridge, "public bridge authority constructor is forbidden"
assert 'json:"' not in bridge, "Wave24 bridge authority must remain non-serializable"

print("Wave24 Cubelet realization epoch authority static contract: PASS")
