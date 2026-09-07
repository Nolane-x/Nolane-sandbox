#!/usr/bin/env python3
from pathlib import Path

TASK = Path("CubeShim/shim/src/service/task_srv.rs").read_text()
SB = Path("CubeShim/shim/src/sandbox/sb.rs").read_text()
CONTAINER = Path("CubeShim/shim/src/container/mod.rs").read_text()
GUEST_VICTIM = Path("CubeShim/shim/src/guest_victim.rs").read_text()

required_task_fragments = [
    "use crate::guest_victim::parse_bind_annotations;",
    "EvidenceCache, RealizationToken, TokenBindings",
    "victim_bindings: Arc<Mutex<TokenBindings>>",
    "victim_active: Arc<Mutex<HashMap<String, RealizationToken>>>",
    "victim_evidence: Arc<Mutex<EvidenceCache>>",
    "parse_bind_annotations(&req.annotations)",
    "consume_main(&req.id)",
    "start_container_with_oom_victim(&req.id, victim_token)",
    "get_oom_victim_evidence(&req.id, token.as_bytes())",
    "insert_finalized(&req.id, token, payload)",
    "EVIDENCE_METADATA_KEY",
    "EVIDENCE_TYPE_URL",
    ".select(&req.id, &metadata)",
]

for fragment in required_task_fragments:
    assert fragment in TASK, f"missing Wave21 TaskService wiring: {fragment}"

assert "pub async fn start_container_with_oom_victim" in SB, "sandbox must accept exact optional Wave21 token"
assert "pub async fn get_oom_victim_evidence" in SB, "sandbox must expose finalized guest evidence RPC"
assert "oom_victim_realization_token" in CONTAINER, "guest StartContainer request must carry Wave21 token"

# Cache admission itself is the trust boundary: every caller must pass strict
# protobuf decoding plus the exact evidence-set validator before bytes become immutable.
insert_start = GUEST_VICTIM.index("pub fn insert_finalized(")
select_start = GUEST_VICTIM.index("pub fn select(", insert_start)
insert_block = GUEST_VICTIM[insert_start:select_start]
assert "decode_and_validate_evidence_payload(container_id, token, &payload)?;" in insert_block
assert "validate_evidence_set(container_id, token, &records)" in GUEST_VICTIM

# Existing operational Stats path must remain present; the Wave21 selector is additive.
assert "sb.stats_container(&req.id)" in TASK
assert "encode_guest_stats(&guest_stats)" in TASK

# Old TaskOOM/GetOOMEvent compatibility signal is not a victim-proof source here.
assert "GetOOMEvent" not in TASK
assert "exit_status == 137" not in TASK
assert "exit_status: 137" not in TASK

print("Wave21 CubeShim production wiring contract passed")
