#!/usr/bin/env python3
# Copyright (c) 2024 Tencent Inc.
# SPDX-License-Identifier: Apache-2.0

from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PROTO_PATHS = (
    ROOT / "agent/libs/protocols/protos/agent.proto",
    ROOT / "CubeShim/protoc/protos/agent.proto",
)

REQUIRED = (
    "rpc GetOOMVictimEvidence(GetOOMVictimEvidenceRequest) returns (GetOOMVictimEvidenceResponse);",
    "bytes oom_victim_realization_token = 2;",
    "message GetOOMVictimEvidenceRequest",
    "bytes realization_token = 2;",
    "enum OOMVictimScope",
    "OOM_VICTIM_SCOPE_MAIN = 1;",
    "OOM_VICTIM_SCOPE_MEMBER = 2;",
    "message OOMVictimProof",
    "uint32 victim_tid = 1;",
    "uint32 victim_tgid = 2;",
    "uint64 victim_starttime_ticks = 3;",
    "uint64 event_boot_time_ns = 4;",
    "uint64 cgroup_v2_id = 5;",
    "OOMVictimScope scope = 6;",
    "message GetOOMVictimEvidenceResponse",
    "string guest_boot_id = 1;",
    "repeated OOMVictimProof proofs = 2;",
)


def main() -> None:
    contents = []
    for path in PROTO_PATHS:
        text = path.read_text(encoding="utf-8")
        contents.append(text)
        missing = [snippet for snippet in REQUIRED if snippet not in text]
        assert not missing, f"{path}: missing Wave21 protocol elements: {missing}"

    # The repositories may retain unrelated historical differences, but every
    # Wave21 declaration must use the exact same spelling and field numbers.
    for snippet in REQUIRED:
        assert contents[0].count(snippet) == 1, f"agent proto must contain exactly one {snippet!r}"
        assert contents[1].count(snippet) == 1, f"shim proto must contain exactly one {snippet!r}"


if __name__ == "__main__":
    main()
