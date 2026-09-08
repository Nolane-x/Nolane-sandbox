from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

RUNTIME = ROOT / "NolaneWorld/substrate/cube/runtime_realization.go"
BRIDGE = ROOT / "NolaneWorld/substrate/cube/realm_resource_runtime_authority.go"
BINDING = ROOT / "NolaneWorld/gauntlet/live/resourceproof/runtime_binding_v27.go"


def read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def test_wave27_runtime_realization_symbols_exist():
    text = read(RUNTIME)
    for symbol in (
        "type RuntimeRealizationObserver struct",
        "type RuntimeRealizationProof struct",
        "func NewRuntimeRealizationObserver(",
        "func (o *RuntimeRealizationObserver) Observe(",
        "func (o *RuntimeRealizationObserver) ValidateCurrent(",
        "ErrRuntimeRealizationUnavailable",
        "ErrInvalidRuntimeRealizationProof",
        "ErrStaleRuntimeRealizationProof",
    ):
        assert symbol in text


def test_wave27_runtime_observer_reuses_exact_existing_parsers():
    text = read(RUNTIME)
    assert "exactRealizationEpochFromSample" in text
    assert "exactHostSandboxProcessIdentityFromSample" in text
    assert "hostResourceMetricsPath" in text


def test_wave27_bridge_is_sealed_and_domain_separated():
    text = read(BRIDGE)
    assert "type RealmResourceRuntimeAuthority struct" in text
    assert "realmResourceRuntimeAuthoritySeal" in text
    assert "ValidateRealmResourceRuntimeAuthority" in text
    assert "runtime-realization-v27:" in text
    assert "nolane.runtime-realization.v27\\x00" in text


def test_wave27_binding_projection_exists_without_trusted_report_constructor():
    text = read(BINDING)
    assert "BindingFromRuntimeAuthority" in text
    combined = read(RUNTIME) + read(BRIDGE) + text
    assert "func NewTrustedReport" not in combined
    assert "func TrustedReportFrom" not in combined


def test_wave27_forbidden_shortcuts_absent():
    combined = "\n".join(read(path) for path in (RUNTIME, BRIDGE, BINDING))
    forbidden = (
        "RuntimeDigest string `json:",
        "NewRealmResourceRuntimeAuthorityFromFields",
        "RuntimeAuthorityFromJSON",
        "sha256.Sum256([]byte(resource.SandboxID()))",
        "EndpointSPKIHex())",
        "time.Now().UnixNano()",
    )
    for marker in forbidden:
        assert marker not in combined
