from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PLACEMENT = ROOT / "NolaneWorld/substrate/cube/runtime_cgroup_helper_placement.go"
PROTOCOL = ROOT / "NolaneWorld/substrate/cube/runtime_cgroup_helper_protocol.go"
MAIN = ROOT / "NolaneWorld/cmd/nolane-gauntlet-live/main.go"


def test_wave29_runtime_cgroup_helper_placement_contract() -> None:
    placement = PLACEMENT.read_text()
    protocol = PROTOCOL.read_text()
    main = MAIN.read_text()
    combined = placement + "\n" + protocol

    required = (
        "type RuntimeCgroupHelperExecutor struct",
        "func NewRuntimeCgroupHelperExecutor() (*RuntimeCgroupHelperExecutor, error)",
        "type RuntimeCgroupHelperPlacementAuthority struct",
        "func ValidateRuntimeCgroupHelperPlacementAuthority(",
        '"/sys/fs/cgroup"',
        "os.Executable()",
        "sha256",
        '"/proc/"',
        '"/exe"',
        "readHelperExecutableDigest",
        "measureRuntimeCgroupHelperProcessExecutable",
        "waitRuntimeCgroupHelperChild(",
        '"cgroup.procs"',
        "ValidateRuntimeCgroupReadbackAuthority(",
        '"runtime-cgroup-helper-placement-v29:"',
        '"nolane.runtime-cgroup-helper-placement.v29\\x00"',
        '"NOLANE_INTERNAL_CGROUP_HELPER"',
        '"NOLANE_INTERNAL_CGROUP_HELPER_NONCE"',
        '"park-exit"',
        '"READY "',
        '"GO "',
        '"DONE "',
        "MaybeRunInternalCgroupHelper()",
    )
    for needle in required:
        assert needle in combined or needle in main, f"missing Wave29 contract: {needle}"

    assert combined.count("ValidateRuntimeCgroupReadbackAuthority(") >= 2, (
        "Wave29 must mint fresh Wave28 authority both before and after helper lifecycle"
    )
    assert 'sha256File("/proc/" + strconv.Itoa(pid) + "/exe")' in placement, (
        "Wave29 must hash the exact started helper executable via /proc/<pid>/exe"
    )
    assert "_ = child.Kill()" in placement and "got := <-ch" in placement, (
        "context-aware helper wait must kill and still reap the exact child"
    )

    constructor_start = placement.index("func NewRuntimeCgroupHelperExecutor()")
    constructor_open = placement.index("{", constructor_start)
    constructor_sig = placement[constructor_start:constructor_open]
    assert "(" in constructor_sig and "()" in constructor_sig

    validator_start = placement.index("func ValidateRuntimeCgroupHelperPlacementAuthority(")
    validator_end = placement.index(") (RuntimeCgroupHelperPlacementAuthority, error)", validator_start)
    validator_sig = placement[validator_start:validator_end]
    for forbidden_param in (
        "string",
        "ResourceBinding",
        "HostFileSource",
        "HostPressureRunner",
        "exec.Cmd",
        "func(",
        "uint32",
        "uint64",
        "[]byte",
    ):
        assert forbidden_param not in validator_sig, (
            f"Wave29 validator accepts caller-controlled evidence: {forbidden_param}"
        )

    forbidden = (
        "TrustedReport",
        "buildTrustedReport",
        "LIVE_PASS",
        "HostPressureRunner",
        "HostFileSource",
    )
    for needle in forbidden:
        assert needle not in combined, f"Wave29 production exposes forbidden shortcut: {needle}"

    assert "const runtimeCgroupHelperReleaseFD = 3" in protocol
    assert "const runtimeCgroupHelperAckFD = 4" in protocol

    helper_index = main.index("MaybeRunInternalCgroupHelper()")
    normal_index = main.index("run(os.Args[1:]")
    assert helper_index < normal_index, "internal helper dispatch must precede normal gauntlet execution"
