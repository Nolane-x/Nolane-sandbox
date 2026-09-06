# Wave 21 Guest Kernel OOM Victim Provenance — Closure Record

Date: 2026-09-06

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

The production Agent owns a best-effort collector lifecycle and exact ingestion/loss seams. The current repository toolchain does not contain a reviewed object-free Rust raw-tracepoint loader equivalent to the Wave 20 Go/BTF/eBPF implementation. The implementation therefore follows the Wave 21 dependency rule: raw `oom:mark_victim` collection remains capability-disabled when the exact loader cannot be established, and Agent startup continues without guest-positive evidence.

No tracefs-text, dmesg/kmsg, cgroup-counter, signal, or exit-code fallback is introduced. Enabling live positive collection later requires an exact raw-tracepoint/BTF loader that preserves the same event schema and loss accounting.

## Verification evidence before final-head rerun

The closure sequence has already demonstrated:

- standalone guest realization, lookup, metadata, and bounded-store contracts pass;
- the Wave 21 Agent production-wiring contract passes;
- CubeShim protobuf ownership is resolved at the protocol crate boundary rather than by mixing protobuf major-version traits;
- CubeShim amd64/arm64 production builds and Docker smoke tests passed on the immediately preceding verified head;
- Unit Test, Format, DCO, Docs, NolaneWorld, Live Substrate Gauntlet, and Wave 17–20 trust contracts passed on the immediately preceding verified head;
- the remaining Agent build warning was traced to the not-yet-linked collector lifecycle and the lifecycle is now production-linked.

This record intentionally does not declare the branch merge-ready by itself. Merge readiness is established only by fresh CI on the commit containing this record and the collector lifecycle closure.

Autonomously-by: ChatGPT:GPT-5.6-Sol
