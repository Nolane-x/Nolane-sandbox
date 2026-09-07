// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/oom_victim_loader.rs"]
mod loader;

use loader::{
    classify_ring_record_header, decode_raw_victim_record, RingRecordHeaderDisposition,
    EVENT_VERSION_V1, RAW_VICTIM_RECORD_SIZE, RINGBUF_BUSY_BIT, RINGBUF_DISCARD_BIT,
};

fn raw_record(
    version: u32,
    flags: u32,
    tid: u32,
    tgid: u32,
    start_boottime_ns: u64,
    event_boottime_ns: u64,
    cgroup_v2_id: u64,
) -> Vec<u8> {
    let mut raw = Vec::with_capacity(RAW_VICTIM_RECORD_SIZE as usize);
    raw.extend_from_slice(&version.to_ne_bytes());
    raw.extend_from_slice(&flags.to_ne_bytes());
    raw.extend_from_slice(&tid.to_ne_bytes());
    raw.extend_from_slice(&tgid.to_ne_bytes());
    raw.extend_from_slice(&start_boottime_ns.to_ne_bytes());
    raw.extend_from_slice(&event_boottime_ns.to_ne_bytes());
    raw.extend_from_slice(&cgroup_v2_id.to_ne_bytes());
    raw
}

#[test]
fn v21_decodes_exact_40_byte_kernel_record_without_pid_only_fallback() {
    let raw = raw_record(EVENT_VERSION_V1, 0, 17, 11, 40_000_000, 50_000_000, 99);
    let event = decode_raw_victim_record(&raw).unwrap();
    assert_eq!(event.tid, 17);
    assert_eq!(event.tgid, 11);
    assert_eq!(event.start_boottime_ns, 40_000_000);
    assert_eq!(event.event_boot_ns, 50_000_000);
    assert_eq!(event.cgroup_v2_id, Some(99));

    let unknown_cgroup = decode_raw_victim_record(&raw_record(
        EVENT_VERSION_V1,
        0,
        17,
        11,
        40_000_000,
        50_000_000,
        0,
    ))
    .unwrap();
    assert_eq!(unknown_cgroup.cgroup_v2_id, None);
}

#[test]
fn v21_rejects_framing_reserved_flags_and_impossible_kernel_identity() {
    let valid = raw_record(EVENT_VERSION_V1, 0, 17, 11, 40_000_000, 50_000_000, 99);
    assert!(decode_raw_victim_record(&valid[..valid.len() - 1]).is_err());
    assert!(decode_raw_victim_record(&[0u8; 41]).is_err());
    assert!(decode_raw_victim_record(&raw_record(2, 0, 17, 11, 40, 50, 99)).is_err());
    assert!(
        decode_raw_victim_record(&raw_record(EVENT_VERSION_V1, 1, 17, 11, 40, 50, 99)).is_err()
    );
    assert!(decode_raw_victim_record(&raw_record(EVENT_VERSION_V1, 0, 0, 11, 40, 50, 99)).is_err());
    assert!(decode_raw_victim_record(&raw_record(EVENT_VERSION_V1, 0, 17, 0, 40, 50, 99)).is_err());
    assert!(decode_raw_victim_record(&raw_record(EVENT_VERSION_V1, 0, 17, 11, 0, 50, 99)).is_err());
    assert!(
        decode_raw_victim_record(&raw_record(EVENT_VERSION_V1, 0, 17, 11, 60, 50, 99)).is_err()
    );
}

#[test]
fn v21_ring_header_uses_kernel_alignment_and_never_consumes_busy_records() {
    assert_eq!(
        classify_ring_record_header(RAW_VICTIM_RECORD_SIZE as u32).unwrap(),
        RingRecordHeaderDisposition::Ready {
            payload_len: RAW_VICTIM_RECORD_SIZE as usize,
            span: 48,
        }
    );
    assert_eq!(
        classify_ring_record_header(RINGBUF_BUSY_BIT | RAW_VICTIM_RECORD_SIZE as u32).unwrap(),
        RingRecordHeaderDisposition::Busy
    );
    assert_eq!(
        classify_ring_record_header(RINGBUF_DISCARD_BIT | RAW_VICTIM_RECORD_SIZE as u32).unwrap(),
        RingRecordHeaderDisposition::Discarded { span: 48 }
    );
}

#[test]
fn v21_ring_header_rejects_impossible_payload_spans() {
    // A v1 producer can only emit the exact 40-byte victim payload. A zero or
    // other framing value is not allowed to be silently consumed as evidence.
    assert!(classify_ring_record_header(0).is_err());
    assert!(classify_ring_record_header(39).is_err());
    assert!(classify_ring_record_header(41).is_err());
}
