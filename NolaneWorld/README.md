# Nolane Sandbox Trust Kernel

`NolaneWorld` is the trust/evolution boundary for **Nolane Sandbox**.

The execution substrate remains CubeSandbox. This module deliberately does not fork the hypervisor, KVM/RustVMM integration, CubeEgress, CubeNet, or CubeCoW. Instead it defines the host-owned rules that let an untrusted AI build arbitrary software capabilities inside a disposable world without being able to mint external authority or trusted persistence.

## Current milestone

Trust Kernel v0 implements:

- a compiled 12-law constitution;
- world identity and host-owned monotonic authority epochs;
- typed authority intents;
- exact-once in-process effect receipts for duplicate actions, including cross-broker serialization;
- fail-closed policy evaluation;
- capability candidate → independent promotion → trusted registry flow;
- exact content + manifest digest binding;
- artifact export validation and provenance receipt digests;
- explicit network authority classes `N0` through `N5`;
- a narrow `SandboxSubstrate` interface for future CubeAPI wiring.

The in-memory authority ledger and registry are **not production durability claims**. A later milestone will add crash-safe external persistence and Cube-backed integration tests before the project can claim rollback-safe production behavior across process/node failure.

## Security model

Assume the model, agent, guest root, downloaded packages, webpages, generated code, snapshots, and exported files are hostile.

The Trust Kernel owns authority state outside the guest. A guest snapshot is execution state only; it is never a source of truth for authority epochs, external-effect history, promotion receipts, or trusted capability records.

The central invariant is:

> **Unbounded capability creation; bounded authority, promotion, truth, and persistence.**

See `../docs/superpowers/specs/2026-08-29-nolane-sandbox-world-design.md` for the normative design.

## Verify

```bash
cd NolaneWorld
go test ./...
go test -race ./...
go vet ./...
```
