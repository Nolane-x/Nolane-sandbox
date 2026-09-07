# Wave 21 Guest Kernel OOM Victim Provenance — Closure Record

Date: 2026-09-06
Updated: 2026-09-07

## Trust closure

Wave 21 extends the Wave 17 generation authority through an exact 32-byte realization token into CubeShim, the guest agent, Cubelet metrics, and NolaneWorld without introducing an OOM-causality claim.

The implementation keeps these boundaries fail-closed:

- Exit 137, SIGKILL, `GetOOMEvent`, TaskOOM, `memory.events`, host-side victim evidence, dmesg, and kmsg are not substitutes for guest-kernel victim provenance.
- Guest evidence is selected only by the exact realization token and is finalized only at the exact main `WaitProcess` boundary.
- Guest victim correlation requires the exact main lifetime identity or the exact non-zero cgroup-v2 identity for members.
- Raw evidence is bounded to 1024 records with a ten-minute retention window. Capacity, age, malformed-record, and collector loss advance a loss epoch and therefore poison overlapping realizations rather than permit partial positive proof.
- Finalized evidence is bounded to 256 realizations with deterministic oldest-first eviction and a ten-minute retention window.
- More than 64 victims for one realization makes the proof unavailable rather than truncating it into a misleading positive set.

## Collector capability boundary

The production Agent owns a best-effort exact collector lifecycle. Wave 21 now includes the reviewed object-free Rust raw-tracepoint path needed for live positive collection: it parses the running kernel BTF for the required task identity layout, assembles the raw-tracepoint BPF program at runtime, creates a private ring buffer plus loss-epoch map, attaches only after the fallible map/program/mmap preparation succeeds, and consumes the versioned victim record through the exact ingestion seam.

The collector remains capability-gated and fail-closed. If the running guest cannot establish the required BTF shape, raw-tracepoint capability, BPF maps/program, attachment, ring mapping, clock bridge, or exact decoder semantics, Agent startup continues but Wave 21 positive evidence stays unavailable. Any detected runtime loss, malformed framing, decoder failure, discarded ring record, or loss-epoch advance marks collector coverage lost and poisons overlapping realizations rather than permitting a partial positive proof.

No tracefs-text, dmesg/kmsg, cgroup-counter, signal, exit-code, TaskOOM, or `GetOOMEvent` fallback is introduced.

## Implementation closure evidence

Before the final-head CI rerun, the implementation sequence had already demonstrated the dedicated Wave 21 realization, lookup, metadata, bounded-store, BPF wire, loader, runtime, cgroup, time-bridge, protocol, Agent production-wiring, CubeShim transport, Cubelet authority, and NolaneWorld contracts on preceding heads. The final cleanup also removed superseded internal seams rather than weakening the active store loss-epoch authority.

Those preceding results are development evidence only. This record does not self-certify merge readiness. The branch is considered verified only when fresh CI on the final candidate HEAD confirms Build, Format, Unit Test, DCO, Docs, NolaneWorld, Live Substrate Gauntlet, the Wave 17–20 trust contracts, and the dedicated Wave 21 guest-kernel OOM victim contract without a source-level failure.

Autonomously-by: ChatGPT:GPT-5.6-Sol
