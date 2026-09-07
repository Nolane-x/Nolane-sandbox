// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use crate::guest_oom_victim::{
    boottime_ns_to_visible_boot_ns, read_current_timens_boottime_offset,
    start_boottime_ns_to_starttime_ticks, RawVictimEvent,
};
use crate::oom_victim_loader::{
    build_raw_tracepoint_program, classify_ring_record_header, decode_raw_victim_record,
    parse_kernel_btf_layout, RingRecordHeaderDisposition,
};
use crate::oom_victim_proc::probe_collector_capability;
use crate::oom_victim_runtime::{
    create_map, load_raw_tracepoint_program, lookup_loss_epoch, open_raw_tracepoint, BpfFd,
    MapCreateAttr, RingMapping,
};
use std::fs;
use std::os::fd::AsRawFd;
use std::time::Duration;

const RINGBUF_CAPACITY: u32 = 64 * 1024;
const IDLE_POLL_INTERVAL: Duration = Duration::from_millis(1);

/// Exact Wave21 guest-kernel OOM victim collector.
///
/// The collector accepts only the kernel `oom:mark_victim` raw tracepoint,
/// derives task layout from the running kernel BTF, and transports a fixed
/// versioned record through a private BPF ring buffer. Any map loss, malformed
/// framing, decoder failure, or clock-domain ambiguity terminates collection
/// with an error so the Sandbox authority can poison evidence fail-closed.
pub struct Collector {
    _events: BpfFd,
    loss: BpfFd,
    _program: BpfFd,
    _attachment: BpfFd,
    ring: RingMapping,
    observed_loss_epoch: u64,
}

impl Collector {
    pub fn start() -> Result<Self, String> {
        let capability = probe_collector_capability()?;
        let btf = fs::read(&capability.btf_path).map_err(|err| {
            format!(
                "Wave21 read kernel BTF {} failed: {}",
                capability.btf_path.display(),
                err
            )
        })?;
        let layout = parse_kernel_btf_layout(&btf)?;

        let events = create_map(MapCreateAttr::ringbuf(RINGBUF_CAPACITY)?)?;
        let loss = create_map(MapCreateAttr::loss_epoch())?;
        let insns = build_raw_tracepoint_program(layout, events.as_raw_fd(), loss.as_raw_fd())?;
        let program = load_raw_tracepoint_program(&insns)?;
        let ring = RingMapping::map(&events, RINGBUF_CAPACITY as usize)?;

        let observed_loss_epoch = lookup_loss_epoch(&loss)?;
        if observed_loss_epoch != 0 {
            return Err("Wave21 new loss map did not start at epoch zero".to_string());
        }

        // Attach only after all fallible map/program/mmap preparation has
        // succeeded so there is no unobserved producer interval during setup.
        let attachment = open_raw_tracepoint(&program)?;
        let after_attach = lookup_loss_epoch(&loss)?;
        if after_attach != observed_loss_epoch {
            return Err("Wave21 loss epoch advanced while attaching raw tracepoint".to_string());
        }

        Ok(Self {
            _events: events,
            loss,
            _program: program,
            _attachment: attachment,
            ring,
            observed_loss_epoch,
        })
    }

    pub async fn next_event(&mut self) -> Result<RawVictimEvent, String> {
        loop {
            let before = lookup_loss_epoch(&self.loss)?;
            if before != self.observed_loss_epoch {
                self.observed_loss_epoch = before;
                return Err("Wave21 kernel loss epoch advanced before ring consumption".to_string());
            }

            let Some(raw_header) = self.ring.peek_header()? else {
                tokio::time::sleep(IDLE_POLL_INTERVAL).await;
                continue;
            };

            match classify_ring_record_header(raw_header)? {
                RingRecordHeaderDisposition::Busy => {
                    tokio::time::sleep(IDLE_POLL_INTERVAL).await;
                    continue;
                }
                RingRecordHeaderDisposition::Discarded { span } => {
                    self.ring.consume(span)?;
                    self.observed_loss_epoch = lookup_loss_epoch(&self.loss)?;
                    return Err("Wave21 kernel ring record was discarded".to_string());
                }
                RingRecordHeaderDisposition::Ready { payload_len, span } => {
                    let raw = self.ring.copy_payload(payload_len)?;
                    self.ring.consume(span)?;

                    let after = lookup_loss_epoch(&self.loss)?;
                    if after != before {
                        self.observed_loss_epoch = after;
                        return Err(
                            "Wave21 kernel loss epoch advanced while consuming a ring record"
                                .to_string(),
                        );
                    }

                    let decoded = decode_raw_victim_record(&raw)?;
                    let boottime_offset_ns = read_current_timens_boottime_offset()?;
                    let starttime_ticks = start_boottime_ns_to_starttime_ticks(
                        decoded.start_boottime_ns,
                        boottime_offset_ns,
                    )?;
                    let event_boot_ns =
                        boottime_ns_to_visible_boot_ns(decoded.event_boot_ns, boottime_offset_ns)?;

                    return Ok(RawVictimEvent {
                        tid: decoded.tid,
                        tgid: decoded.tgid,
                        starttime_ticks,
                        event_boot_ns,
                        cgroup_v2_id: decoded.cgroup_v2_id,
                    });
                }
            }
        }
    }
}
