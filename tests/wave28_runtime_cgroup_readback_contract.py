from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "NolaneWorld/substrate/cube/runtime_cgroup_readback.go"


def test_wave28_runtime_cgroup_readback_contract() -> None:
    text = SOURCE.read_text()

    required = (
        "type RuntimeCgroupReadbackObserver struct",
        "func NewRuntimeCgroupReadbackObserver() *RuntimeCgroupReadbackObserver",
        '"/sys/fs/cgroup"',
        "type RuntimeCgroupReadbackAuthority struct",
        "func ValidateRuntimeCgroupReadbackAuthority(",
        "ValidateRealmResourceRuntimeAuthority(",
        '"cgroup.controllers"',
        '"cgroup.procs"',
        '"cpu.max"',
        '"cpu.stat"',
        '"memory.max"',
        '"memory.events"',
        "nolane.runtime-cgroup-readback.v28\\x00",
        "runtime-cgroup-readback-v28:",
        "HostPID",
        "RuntimeDigest",
        "ReadbackDigest",
    )
    for needle in required:
        assert needle in text, f"missing Wave28 production contract: {needle}"

    assert text.count("ValidateRealmResourceRuntimeAuthority(") >= 2, (
        "Wave28 must freshly reconstruct Wave27 before and after cgroup readback"
    )

    forbidden = (
        "TrustedReport",
        "buildTrustedReport",
        "LIVE_PASS",
        "CapabilityEvidence",
        "CgroupRoot",
        "HostFileSource",
        "HostPressureRunner",
    )
    for needle in forbidden:
        assert needle not in text, f"Wave28 production exposes forbidden trusted shortcut: {needle}"

    constructor_start = text.index("func NewRuntimeCgroupReadbackObserver()")
    constructor_end = text.index("{", constructor_start)
    constructor_signature = text[constructor_start:constructor_end]
    assert "root" not in constructor_signature.lower()
    assert "read" not in constructor_signature.lower()

    validator_start = text.index("func ValidateRuntimeCgroupReadbackAuthority(")
    validator_end = text.index(") (RuntimeCgroupReadbackAuthority, error)", validator_start)
    validator_signature = text[validator_start:validator_end]
    for forbidden_param in ("string", "Binding", "HostFileSource", "HostPressureRunner"):
        assert forbidden_param not in validator_signature, (
            f"Wave28 validator accepts caller-controlled locator/evidence: {forbidden_param}"
        )
