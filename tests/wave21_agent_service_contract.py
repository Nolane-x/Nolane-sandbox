#!/usr/bin/env python3
from pathlib import Path

MAIN = Path("agent/src/main.rs").read_text()
SANDBOX = Path("agent/src/sandbox.rs").read_text()
RPC = Path("agent/src/rpc.rs").read_text()
AUTHORITY = Path("agent/src/guest_oom_victim.rs").read_text()
COLLECTOR = Path("agent/src/oom_victim.rs").read_text()
BPF = Path("agent/src/oom_victim_bpf.rs").read_text()
RUNTIME = Path("agent/src/oom_victim_runtime.rs").read_text()
PROC = Path("agent/src/oom_victim_proc.rs").read_text()

for fragment in [
    "mod guest_oom_victim;",
    "mod oom_victim;",
    "mod oom_victim_bpf;",
    "mod oom_victim_runtime;",
    "mod oom_victim_proc;",
    "oom_victim::start_best_effort(",
]:
    assert fragment in MAIN, f"missing Wave21 Agent collector lifecycle seam: {fragment}"

for fragment in [
    "guest_oom_victim: GuestVictimStore",
    "guest_oom_victim_loss_epoch: u64",
    "begin_guest_oom_victim_realization",
    "record_guest_oom_victim_raw",
    "note_guest_oom_victim_loss",
    "finalize_guest_oom_victim_realization",
    "get_guest_oom_victim_evidence",
]:
    assert fragment in SANDBOX, f"missing Wave21 Sandbox authority seam: {fragment}"

for fragment in [
    "MAX_RAW_VICTIM_EVENTS: usize = 1024",
    "MAX_RAW_AGE_NS",
    "MAX_FINALIZED_REALIZATIONS: usize = 256",
    "MAX_FINALIZED_AGE_NS",
    "start_store_loss_epoch",
    "record_raw_with_disposition",
    "read_process_cgroup_v2_id",
    "read_process_timens_boottime_offset",
    "RecordedWithLoss",
]:
    assert fragment in AUTHORITY, f"missing Wave21 bounded evidence invariant: {fragment}"

for fragment in [
    "Collector::start()",
    "record_guest_oom_victim_raw(event)",
    "note_guest_oom_victim_loss()",
]:
    assert fragment in COLLECTOR, f"missing Wave21 collector runtime seam: {fragment}"

for fragment in [
    "probe_collector_capability()?",
    "parse_kernel_btf_layout",
    "build_raw_tracepoint_program",
    "decode_raw_victim_record",
    "MapCreateAttr::ringbuf",
    "open_raw_tracepoint",
    "lookup_loss_epoch",
    "read_process_timens_boottime_offset",
    "start_boottime_ns_to_starttime_ticks",
]:
    assert fragment in BPF, f"missing Wave21 live collector boundary: {fragment}"

for fragment in [
    "BPF_MAP_TYPE_RINGBUF",
    "BPF_PROG_TYPE_RAW_TRACEPOINT",
    "BPF_RAW_TRACEPOINT_OPEN",
    "RAW_TRACEPOINT_NAME",
    "ring_geometry",
    "create_map",
    "load_raw_tracepoint_program",
    "open_raw_tracepoint",
    "lookup_loss_epoch",
    "RingMapping",
]:
    assert fragment in RUNTIME, f"missing Wave21 object-free BPF runtime seam: {fragment}"

assert "no reviewed object-free raw-tracepoint BPF loader" not in BPF
assert "refusing tracefs/dmesg/exit-code fallback" not in BPF

for fragment in [
    "/sys/kernel/btf/vmlinux",
    "/sys/kernel/tracing/events/oom/mark_victim/id",
    "x86_64",
    "aarch64",
]:
    assert fragment in PROC, f"missing Wave21 collector capability probe: {fragment}"

for fragment in [
    "RealizationToken::from_bytes",
    "oom_victim_realization_token",
    "begin_guest_oom_victim_realization",
    "finalize_guest_oom_victim_realization",
    "async fn get_oom_victim_evidence",
    "get_guest_oom_victim_evidence",
    "OOM_VICTIM_SCOPE_MAIN",
    "OOM_VICTIM_SCOPE_MEMBER",
    "OOMVictimEvidence::new()",
    "proof.version = 1",
    "proof.container_id = req.container_id.clone()",
    "proof.realization_token = req.realization_token.clone()",
    "proof.main_pid = evidence.main.tgid",
    "proof.main_starttime_ticks = evidence.main.starttime_ticks",
    "proof.realization_started_boot_ns = evidence.realization_started_boot_ns",
    "proof.outcome_observed_boot_ns = evidence.outcome_observed_boot_ns",
    "guest.kernel.oom.mark_victim.raw_tracepoint",
]:
    assert fragment in RPC, f"missing Wave21 AgentService production wiring: {fragment}"

# Compatibility/cgroup signals remain separate from exact victim authority.
for source in [AUTHORITY, COLLECTOR, BPF, RUNTIME, PROC]:
    assert "GetOOMEvent" not in source
    assert "memory.events" not in source
    assert "oom_kill" not in source
assert "exit_status == 137" not in RPC
assert "exit_status: 137" not in RPC

print("Wave21 guest agent production wiring contract passed")
