from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
MODEL = ROOT / "NolaneWorld" / "realm" / "model.go"
AUTH = ROOT / "NolaneWorld" / "realm" / "runtime_cpu_policy_authority.go"


def test_wave31_model_field_is_optional_and_explicit():
    model = MODEL.read_text()
    assert re.search(
        r'RuntimeCPULimitMilliCPU\s+uint64\s+`json:"runtime_cpu_limit_millicpu,omitempty"`',
        model,
    )
    assert re.search(r'CPUUnits\s+uint64\s+`json:"cpu_units"`', model)


def test_wave31_authority_surface_is_sealed_and_domain_separated():
    auth = AUTH.read_text()
    assert 'realm-runtime-cpu-policy-v31:' in auth
    assert 'nolane.realm-runtime-cpu-policy.v31\\x00' in auth
    assert 'type RuntimeCPUPolicyAuthority struct {' in auth
    assert 'type RuntimeCPUPolicyBinding struct {' in auth
    assert 'CurrentRuntimeCPUPolicyAuthority' in auth
    assert 'ValidateRuntimeCPUPolicyAuthority' in auth

    authority_body = re.search(
        r'type\s+RuntimeCPUPolicyAuthority\s+struct\s*\{(?P<body>.*?)\n\}',
        auth,
        re.S,
    )
    assert authority_body, "missing RuntimeCPUPolicyAuthority struct"
    exported_field = re.search(r'^\s*[A-Z][A-Za-z0-9_]*\s+', authority_body.group('body'), re.M)
    assert not exported_field, "authority must not expose exported fields"


def test_wave31_public_api_has_no_caller_authority_inputs():
    auth = AUTH.read_text()

    mint = re.search(
        r'func\s+\(c\s+\*Controller\)\s+CurrentRuntimeCPUPolicyAuthority\s*\(\s*ctx\s+context\.Context\s*,\s*realmID\s+ID\s*\)\s*\(RuntimeCPUPolicyAuthority,\s*error\)',
        auth,
        re.S,
    )
    assert mint, "minting signature must accept only context plus Realm ID"

    validate = re.search(
        r'func\s+\(c\s+\*Controller\)\s+ValidateRuntimeCPUPolicyAuthority\s*\(\s*ctx\s+context\.Context\s*,\s*authority\s+RuntimeCPUPolicyAuthority\s*\)\s*\(RuntimeCPUPolicyBinding,\s*error\)',
        auth,
        re.S,
    )
    assert validate, "validation signature must accept only context plus opaque authority"

    assert not re.search(r'func\s+NewRuntimeCPUPolicyAuthority\s*\(', auth)


def test_wave31_does_not_launder_accounting_into_kernel_policy():
    auth = AUTH.read_text()
    forbidden = [
        'CPUUnits *',
        'CPUUnits/',
        'CPUUnits /',
        'ResourceBudget.CPUUnits',
        'HostPressureRunner',
        'TrustedReport',
        'LIVE_PASS',
        'cpu.max',
        'cgroup.procs',
        'RuntimeCPUThrottleInterventionAuthority',
        'RuntimeCgroupReadbackAuthority',
        'RuntimeCgroupReadbackObserver',
        'RealmResourceRuntimeAuthority',
    ]
    for token in forbidden:
        assert token not in auth, f"Wave31 authority source contains forbidden shortcut/integration: {token}"


def test_wave31_accepts_only_exact_package_owned_stores():
    auth = AUTH.read_text()
    assert 'case *MemoryStore:' in auth
    assert 'case *DurableStore:' in auth
    assert 'default:' in auth
    assert 'trustedRuntimeCPUPolicyStore' in auth


def test_wave31_scope_does_not_modify_resourceproof_or_cube_contracts():
    # This static test intentionally inspects only Wave31 production source. Whole-diff
    # scope is additionally checked through the GitHub compare audit before closure.
    auth = AUTH.read_text()
    assert 'substrate/cube' not in auth
    assert 'gauntlet/live/resourceproof' not in auth
