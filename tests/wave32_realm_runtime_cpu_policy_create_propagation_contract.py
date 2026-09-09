from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
FABRIC = ROOT / "NolaneWorld" / "fabric" / "fabric.go"
CUBE = ROOT / "NolaneWorld" / "substrate" / "cube" / "client.go"
SUBSTRATE = ROOT / "NolaneWorld" / "substrate" / "runtime_cpu_policy_create_propagation.go"


def _text(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def test_acquire_request_does_not_accept_cpu_policy_authority_material():
    source = _text(FABRIC)
    match = re.search(r"type\s+AcquireRequest\s+struct\s*\{(?P<body>.*?)\n\}", source, re.S)
    assert match, "AcquireRequest struct not found"
    body = match.group("body")
    for forbidden in (
        "RuntimeCPULimitMilliCPU",
        "RuntimeCPUPolicyAuthority",
        "PolicyDigest",
        "LimitMilliCPU",
        "AuthorityDigest",
    ):
        assert forbidden not in body, f"caller-controlled AcquireRequest contains {forbidden}"


def test_fabric_positive_policy_path_requires_sealed_wave31_authority():
    source = _text(FABRIC)
    assert "type RuntimeCPUPolicyWorldManager interface" in source
    assert re.search(
        r"CreateWithRuntimeCPUPolicy\s*\(\s*context\.Context\s*,\s*world\.ID\s*,\s*realm\.RuntimeCPUPolicyAuthority\s*\)",
        source,
        re.S,
    )
    assert "CurrentRuntimeCPUPolicyAuthority(" in source
    assert source.count("ValidateRuntimeCPUPolicyAuthority(") >= 2
    assert "RuntimeCPULimitMilliCPU > 0" in source or "RuntimeCPULimitMilliCPU != 0" in source
    assert "ErrRuntimeCPUPolicyPropagationUnavailable" in source


def test_cube_provider_boundary_derives_only_from_authority_binding():
    source = _text(CUBE)
    assert "CreateWithRuntimeCPUPolicy(" in source
    assert "authority.Binding()" in source
    for key in (
        "nolane.realm.id",
        "nolane.realm.revision",
        "nolane.realm.policy_digest",
        "nolane.realm.runtime_cpu_policy_digest",
        "nolane.realm.runtime_cpu_limit_millicpu",
    ):
        assert key in source
    assert "nolane.runtime-cpu-policy-create-propagation.v32\\x00" in source
    assert "RuntimeCPUPolicyCreatePropagationDigestPrefix" in source


def test_receipt_is_descriptive_and_canonical():
    source = _text(SUBSTRATE)
    assert 'RuntimeCPUPolicyCreatePropagationDigestPrefix = "runtime-cpu-policy-create-propagation-v32:"' in source
    for field in (
        "RealmID",
        "RealmRevision",
        "PolicyDigest",
        "LimitMilliCPU",
        "AuthorityDigest",
        "WorldID",
        "RequestDigest",
    ):
        assert field in source
    assert "func (p RuntimeCPUPolicyCreatePropagation) Valid() bool" in source


def test_wave32_does_not_launder_propagation_into_enforcement_claims():
    source = _text(FABRIC) + "\n" + _text(CUBE) + "\n" + _text(SUBSTRATE)
    for forbidden in ("cpu.max", "TrustedReport", "LIVE_PASS"):
        assert forbidden not in source
