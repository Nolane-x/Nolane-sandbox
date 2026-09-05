#!/usr/bin/env python3
# Temporary Wave 21 protocol generation helper. Self-deleted by the workflow commit.

from pathlib import Path

PATHS = (
    Path("agent/libs/protocols/protos/agent.proto"),
    Path("CubeShim/protoc/protos/agent.proto"),
)
RPC_ANCHOR = "\trpc GetOOMEvent(GetOOMEventRequest) returns (OOMEvent);"
RPC_INSERT = RPC_ANCHOR + "\n\trpc GetOOMVictimEvidence(GetOOMVictimEvidenceRequest) returns (GetOOMVictimEvidenceResponse);"
START_ANCHOR = "message StartContainerRequest {\n\tstring container_id = 1;\n}"
START_INSERT = "message StartContainerRequest {\n\tstring container_id = 1;\n\tbytes oom_victim_realization_token = 2;\n}"
WAIT_ANCHOR = "message WaitProcessResponse {\n\tint32 status = 1;\n}"
EVIDENCE = """message GetOOMVictimEvidenceRequest {
\tstring container_id = 1;
\tbytes realization_token = 2;
}

enum OOMVictimScope {
\tOOM_VICTIM_SCOPE_UNSPECIFIED = 0;
\tOOM_VICTIM_SCOPE_MAIN = 1;
\tOOM_VICTIM_SCOPE_MEMBER = 2;
}

message OOMVictimProof {
\tuint32 victim_tid = 1;
\tuint32 victim_tgid = 2;
\tuint64 victim_starttime_ticks = 3;
\tuint64 event_boot_time_ns = 4;
\tuint64 cgroup_v2_id = 5;
\tOOMVictimScope scope = 6;
}

message GetOOMVictimEvidenceResponse {
\tstring guest_boot_id = 1;
\trepeated OOMVictimProof proofs = 2;
}"""

for path in PATHS:
    text = path.read_text(encoding="utf-8")
    for anchor, label in (
        (RPC_ANCHOR, "GetOOMEvent RPC"),
        (START_ANCHOR, "StartContainerRequest"),
        (WAIT_ANCHOR, "WaitProcessResponse"),
    ):
        count = text.count(anchor)
        if count != 1:
            raise SystemExit(f"{path}: expected exactly one {label} anchor, got {count}")
    text = text.replace(RPC_ANCHOR, RPC_INSERT, 1)
    text = text.replace(START_ANCHOR, START_INSERT, 1)
    text = text.replace(WAIT_ANCHOR, WAIT_ANCHOR + "\n\n" + EVIDENCE, 1)
    path.write_text(text, encoding="utf-8")
