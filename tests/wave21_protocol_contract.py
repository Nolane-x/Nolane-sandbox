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
    "string container_id = 1;",
    "bytes realization_token = 2;",
    "enum OOMVictimScope",
    "OOM_VICTIM_SCOPE_MAIN = 1;",
    "OOM_VICTIM_SCOPE_MEMBER = 2;",
    "message OOMVictimEvidence",
    "uint32 version = 1;",
    "string container_id = 2;",
    "bytes realization_token = 3;",
    "string guest_boot_id = 4;",
    "uint32 victim_tid = 5;",
    "uint32 victim_tgid = 6;",
    "uint64 victim_starttime_ticks = 7;",
    "uint64 event_boot_time_ns = 8;",
    "uint64 cgroup_v2_id = 9;",
    "uint32 main_pid = 10;",
    "uint64 main_starttime_ticks = 11;",
    "OOMVictimScope scope = 12;",
    "uint64 realization_started_boot_ns = 13;",
    "uint64 outcome_observed_boot_ns = 14;",
    "string source = 15;",
    "message GetOOMVictimEvidenceResponse",
    "repeated OOMVictimEvidence evidence = 1;",
)

FORBIDDEN_PARTIAL = (
    "message OOMVictimProof",
    "repeated OOMVictimProof proofs = 2;",
)


def main() -> None:
    contents = []
    for path in PROTO_PATHS:
        text = path.read_text(encoding="utf-8")
        contents.append(text)
        missing = [snippet for snippet in REQUIRED if snippet not in text]
        assert not missing, f"{path}: missing Wave21 full-authority protocol elements: {missing}"
        leaked = [snippet for snippet in FORBIDDEN_PARTIAL if snippet in text]
        assert not leaked, f"{path}: retained partial Wave21 proof schema: {leaked}"

    # Historical unrelated differences may remain, but every Wave21 authority
    # declaration must use identical spelling and field numbers in both copies.
    for snippet in REQUIRED:
        assert contents[0].count(snippet) == contents[1].count(snippet), (
            f"Wave21 proto parity mismatch for {snippet!r}: "
            f"agent={contents[0].count(snippet)} shim={contents[1].count(snippet)}"
        )

    # The proof record itself is singular in each protocol copy. Request and
    # StartContainer also contain container/token fields, so global count==1 is
    # intentionally not required for those shared field spellings.
    for snippet in (
        "message OOMVictimEvidence",
        "uint32 version = 1;",
        "string guest_boot_id = 4;",
        "uint32 main_pid = 10;",
        "uint64 main_starttime_ticks = 11;",
        "OOMVictimScope scope = 12;",
        "uint64 realization_started_boot_ns = 13;",
        "uint64 outcome_observed_boot_ns = 14;",
        "string source = 15;",
        "repeated OOMVictimEvidence evidence = 1;",
    ):
        assert contents[0].count(snippet) == 1, f"agent proto must contain exactly one {snippet!r}"
        assert contents[1].count(snippet) == 1, f"shim proto must contain exactly one {snippet!r}"


if __name__ == "__main__":
    main()
