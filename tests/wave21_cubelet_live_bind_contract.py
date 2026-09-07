#!/usr/bin/env python3
"""Wave 21 live Cubelet/CubeBox realization-binding contract.

This contract deliberately inspects the two production authority seams rather
than helper-only tests. Wave 21 is not live unless the controller publishes
exact-token start authority only after realization admission and CubeBox
consumes that authority on the pod task before workload Start.

Wave 24 strengthens the realization-begin seam by minting a Cubelet epoch and
then projecting its generation. This contract therefore checks the semantic
ordering rather than pinning Wave 21 to the pre-Wave24 helper spelling.
"""

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MANAGER = ROOT / "Cubelet/plugins/cube/internals/sandbox/cube_sandbox_manager.go"
CUBEBOX = ROOT / "Cubelet/services/cubebox/cube_container_create.go"


def require_once(text: str, needle: str, label: str) -> int:
    count = text.count(needle)
    if count != 1:
        raise AssertionError(f"{label}: expected exactly one occurrence, found {count}")
    return text.index(needle)


def method_slice(text: str, signature: str, next_signature: str) -> str:
    start = text.index(signature)
    end = text.index(next_signature, start)
    return text[start:end]


def verify_controller_seam() -> None:
    text = MANAGER.read_text()

    create = method_slice(
        text,
        "func (c *controllerLocal) Create(",
        "func (c *controllerLocal) Start(",
    )
    clear_create = require_once(
        create,
        "c.clearGuestOOMVictimStartBinding(info.ID)",
        "Create stale-binding fence",
    )
    store_clear = require_once(create, "store.Clear(info.ID)", "Create proof-store clear")
    if clear_create > store_clear:
        raise AssertionError("Create must fence stale start binding before clearing proof state")

    start = method_slice(
        text,
        "func (c *controllerLocal) Start(",
        "func (c *controllerLocal) Platform(",
    )
    clear_start = require_once(
        start,
        "c.clearGuestOOMVictimStartBinding(sandboxID)",
        "Start stale-binding fence",
    )

    legacy_begin = "generation := c.beginTaskOutcomeRealization(sandboxID)"
    wave24_begin = "epoch, err := c.beginTaskOutcomeRealizationEpoch(sandboxID)"
    if legacy_begin in start:
        begin_realization = require_once(
            start,
            legacy_begin,
            "Start realization generation",
        )
        generation_ready = begin_realization
    elif wave24_begin in start:
        begin_realization = require_once(
            start,
            wave24_begin,
            "Start realization epoch",
        )
        fail_closed = require_once(
            start,
            "if err != nil {",
            "Start realization epoch fail-closed check",
        )
        generation_ready = require_once(
            start,
            "generation := epoch.Generation",
            "Start realization generation projection",
        )
        if not (begin_realization < fail_closed < generation_ready):
            raise AssertionError(
                "Wave24 Start must mint epoch -> fail closed -> project generation"
            )
    else:
        raise AssertionError(
            "Start realization authority seam is missing both Wave21 and Wave24 forms"
        )

    admit = require_once(
        start,
        "if err := store.BeginGuestOOMVictimRealization(sandboxID, generation, token); err == nil {",
        "guest-victim realization admission",
    )
    publish = require_once(
        start,
        "_ = c.publishGuestOOMVictimStartBinding(sandboxID, generation, token)",
        "exact-token start-binding publish",
    )
    if not (clear_start < begin_realization <= generation_ready < admit < publish):
        raise AssertionError(
            "Start authority order must be clear -> realization authority -> generation -> admission -> publish"
        )


def verify_cubebox_seam() -> None:
    text = CUBEBOX.read_text()
    anchor = require_once(text, "\tci.ExitCh = exitCh\n", "CubeBox task-start anchor")
    bind = require_once(
        text,
        "bindGuestOOMVictimBeforeStart(ctx, task, cubebox.ID)",
        "live pod-task Wave21 binding",
    )

    if bind <= anchor:
        raise AssertionError("Wave21 binding must occur after the created task has its exit channel")

    block = text[anchor : bind + 600]
    required = (
        "var startErr error",
        "if ci.IsPod {",
        "startErr = bindGuestOOMVictimBeforeStart(ctx, task, cubebox.ID)",
        "} else {",
        "startErr = task.Start(ctx)",
        "if startErr != nil {",
    )
    positions = []
    for item in required:
        if item not in block:
            raise AssertionError(f"CubeBox live start seam is missing {item!r}")
        positions.append(block.index(item))
    if positions != sorted(positions):
        raise AssertionError("CubeBox live start seam ordering is not canonical")


def main() -> None:
    verify_controller_seam()
    verify_cubebox_seam()
    print("Wave21 Cubelet live bind contract: PASS")


if __name__ == "__main__":
    main()
