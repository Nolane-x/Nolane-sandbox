from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
INTERVENTION = ROOT / "NolaneWorld/substrate/cube/runtime_cpu_intervention.go"
PROTOCOL = ROOT / "NolaneWorld/substrate/cube/runtime_cgroup_helper_protocol.go"
MAIN = ROOT / "NolaneWorld/cmd/nolane-gauntlet-live/main.go"


def test_wave30_exact_cpu_throttle_intervention_contract() -> None:
    intervention = INTERVENTION.read_text()
    protocol = PROTOCOL.read_text()
    main = MAIN.read_text()
    combined = intervention + "\n" + protocol

    required = (
        "type RuntimeCPUInterventionExecutor struct",
        "func NewRuntimeCPUInterventionExecutor() (*RuntimeCPUInterventionExecutor, error)",
        "type RuntimeCPUThrottleInterventionAuthority struct",
        "func ValidateRuntimeCPUThrottleInterventionAuthority(",
        '"/sys/fs/cgroup"',
        '"cpu-throttle-v30"',
        '"NOLANE_INTERNAL_CGROUP_HELPER_BURN_MICROS"',
        'helperProtocolRecord("READY", nonce)',
        'helperProtocolRecord("START", nonce)',
        'helperProtocolRecord("BURN_DONE", nonce)',
        'helperProtocolRecord("EXIT", nonce)',
        'helperProtocolRecord("DONE", nonce)',
        '"/proc/" + strconv.Itoa(pid) + "/schedstat"',
        'filepath.Join(target, "cgroup.procs")',
        "ValidateRuntimeCgroupReadbackAuthority(",
        '"runtime-cpu-throttle-intervention-v30:"',
        '"nolane.runtime-cpu-throttle-intervention.v30\\x00"',
        "runtime.LockOSThread()",
        "syscall.Gettid() != os.Getpid()",
        "ErrRuntimeCPUInterventionUnavailable",
        "quota >= period",
        "period * 2",
        "period * 4",
        "pressureAfterSnapshot.NrThrottled <= controlAfterSnapshot.NrThrottled",
        "pressureAfterSnapshot.ThrottledUsec <= controlAfterSnapshot.ThrottledUsec",
        "schedAfter-schedBefore < params.minimumCPUNS",
        "waitRuntimeCPUInterventionChild(",
    )
    for needle in required:
        assert needle in combined, f"missing Wave30 trust contract: {needle}"

    # Fresh Wave28 authority is reconstructed for the basis, control-before,
    # control-after, pressure-after and final states through the package-owned
    # observer closure rather than accepting caller-carried counter documents.
    assert intervention.count("observe(ctx)") >= 5, (
        "Wave30 must freshly observe the Wave28 chain across all intervention phases"
    )

    # The per-helper CPU baseline is sampled after the quiet control wait but
    # before the final control-after Wave28 readback. That makes the Wave28
    # control-after counters the freshest cgroup baseline immediately before
    # START while excluding parked helper CPU from the intervention delta.
    control_sleep = intervention.index("e.sleep(ctx, params.controlDuration)")
    sched_before = intervention.index("schedBefore, err := e.readSchedstat(pid)")
    control_after = intervention.index("controlAfterAt :=")
    start = intervention.index("child.Start(nonce)")
    assert control_sleep < sched_before < control_after < start, (
        "Wave30 attribution order must be quiet-control -> schedstat -> "
        "control-after -> START"
    )

    # The control phase is fail-closed on any pre-intervention throttling.
    assert (
        "controlAfterSnapshot.NrThrottled != controlBeforeSnapshot.NrThrottled"
        in intervention
    )
    assert (
        "controlAfterSnapshot.ThrottledUsec != controlBeforeSnapshot.ThrottledUsec"
        in intervention
    )

    # Exact helper identity is rechecked at multiple phase boundaries.
    assert intervention.count("e.requireHelperIdentity(") >= 4
    assert "readHelperExecutableDigest" in intervention
    assert "readStartTime" in intervention
    assert "containsExactCgroupPID" in intervention

    # Public production construction remains zero-argument.
    constructor_start = intervention.index("func NewRuntimeCPUInterventionExecutor()")
    constructor_open = intervention.index("{", constructor_start)
    constructor_sig = intervention[constructor_start:constructor_open]
    assert "()" in constructor_sig

    # Public validator accepts only opaque authorities/observers plus the sealed
    # executor. It must not accept caller-controlled locators or observation data.
    validator_start = intervention.index(
        "func ValidateRuntimeCPUThrottleInterventionAuthority("
    )
    validator_end = intervention.index(
        ") (RuntimeCPUThrottleInterventionAuthority, error)", validator_start
    )
    validator_sig = intervention[validator_start:validator_end]
    for forbidden_param in (
        "string",
        "ResourceBinding",
        "HostFileSource",
        "HostPressureRunner",
        "[]byte",
        "time.Duration",
        "uint32",
        "uint64",
        "exec.Cmd",
        "func(",
    ):
        assert forbidden_param not in validator_sig, (
            f"Wave30 validator accepts caller-controlled evidence: {forbidden_param}"
        )

    # No resourceproof authority laundering is allowed in the Wave30 producer.
    for forbidden in (
        "TrustedReport",
        "buildTrustedReport",
        "CapabilityEvidenceSource",
        "LIVE_PASS",
        "HostPressureRunner",
        "HostFileSource",
    ):
        assert forbidden not in combined, (
            f"Wave30 production exposes forbidden shortcut: {forbidden}"
        )

    # The single command-facing helper dispatcher still runs before the ordinary
    # gauntlet path, preserving both Wave29 and Wave30 internal modes.
    helper_index = main.index("MaybeRunInternalCgroupHelper()")
    normal_index = main.index("run(os.Args[1:]")
    assert helper_index < normal_index
