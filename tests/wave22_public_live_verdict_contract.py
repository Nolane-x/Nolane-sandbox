#!/usr/bin/env python3
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
VERDICT = ROOT / "NolaneWorld/gauntlet/live/resourceproof/verdict_v22.go"
SOURCE = ROOT / "NolaneWorld/gauntlet/live/resourceproof/evidence_source.go"

verdict = VERDICT.read_text(encoding="utf-8")
source = SOURCE.read_text(encoding="utf-8")

required = [
    "func (t TrustedReport) ProjectVerdict",
    "VerifyTrustedReport(t)",
    "ResourceDisk",
    "DimensionUnavailable",
    "func VerifyVerdict(",
    "func MarshalVerdict(",
]
for needle in required:
    assert needle in verdict, f"missing Wave22 trust contract: {needle}"

for forbidden in [
    "func (r Report) ProjectVerdict",
    "json.Unmarshal",
    "BuildReport(",
    "TrustedReport{report:",
]:
    assert forbidden not in verdict, f"forbidden authority path in Wave22 verdict: {forbidden}"

# Disk is deliberately never a verified dimension in Wave 22.
disk_case = verdict.index("case ResourceDisk:")
disk_tail = verdict[disk_case : disk_case + 240]
assert "unavailableDimension(dimension)" in disk_tail, "disk no longer fails closed"
assert "DimensionVerified" not in disk_tail, "disk gained unsupported verified projection"

assert "func NewCapabilityEvidenceSource(trusted TrustedReport" in source, (
    "capability authority no longer requires opaque TrustedReport"
)

print("Wave22 public live verdict trust contract: PASS")
