#!/usr/bin/env python3
from pathlib import Path

MAIN = Path("agent/src/main.rs").read_text()
SANDBOX = Path("agent/src/sandbox.rs").read_text()
RPC = Path("agent/src/rpc.rs").read_text()

assert "mod guest_oom_victim;" in MAIN, "guest victim authority module must be production-linked"

for fragment in [
    "guest_oom_victim: GuestVictimStore",
    "guest_oom_victim_loss_epoch: u64",
    "begin_guest_oom_victim_realization",
    "finalize_guest_oom_victim_realization",
    "get_guest_oom_victim_evidence",
]:
    assert fragment in SANDBOX, f"missing Wave21 Sandbox authority seam: {fragment}"

for fragment in [
    "RealizationToken::from_bytes",
    "oom_victim_realization_token",
    "begin_guest_oom_victim_realization",
    "finalize_guest_oom_victim_realization",
    "async fn get_oom_victim_evidence",
    "get_guest_oom_victim_evidence",
    "OOM_VICTIM_SCOPE_MAIN",
    "OOM_VICTIM_SCOPE_MEMBER",
]:
    assert fragment in RPC, f"missing Wave21 AgentService production wiring: {fragment}"

# Compatibility signals remain separate from victim authority.
assert "GetOOMEvent" not in Path("agent/src/guest_oom_victim.rs").read_text()
assert "exit_status == 137" not in RPC
assert "exit_status: 137" not in RPC

print("Wave21 guest agent production wiring contract passed")
