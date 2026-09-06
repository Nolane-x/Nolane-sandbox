// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_oom_victim.rs"]
mod guest_oom_victim;

use guest_oom_victim::{
    GuestProcessIdentity, GuestVictimStore, RawRecordDisposition, RawVictimEvent, RealizationToken,
    MAX_FINALIZED_AGE_NS, MAX_FINALIZED_REALIZATIONS, MAX_RAW_AGE_NS, MAX_RAW_VICTIM_EVENTS,
};

fn token(index: u16) -> RealizationToken {
    let mut raw = [0u8; 32];
    raw[30..].copy_from_slice(&index.to_be_bytes());
    RealizationToken::from_bytes(raw).unwrap()
}

fn event(index: u32, boot_ns: u64) -> RawVictimEvent {
    RawVictimEvent {
        tid: index + 1,
        tgid: index + 1,
        starttime_ticks: u64::from(index) + 1,
        event_boot_ns: boot_ns,
        cgroup_v2_id: Some(77),
    }
}

#[test]
fn v21_raw_store_is_bounded_and_reports_capacity_loss() {
    let mut store = GuestVictimStore::default();
    for index in 0..MAX_RAW_VICTIM_EVENTS {
        assert_eq!(
            store.record_raw(event(index as u32, index as u64 + 1)).unwrap(),
            RawRecordDisposition::Recorded
        );
    }

    assert_eq!(
        store
            .record_raw(event(MAX_RAW_VICTIM_EVENTS as u32, MAX_RAW_VICTIM_EVENTS as u64 + 1))
            .unwrap(),
        RawRecordDisposition::RecordedWithLoss
    );
    assert_eq!(store.raw_len(), MAX_RAW_VICTIM_EVENTS);
}

#[test]
fn v21_raw_store_age_eviction_reports_loss() {
    let mut store = GuestVictimStore::default();
    assert_eq!(
        store.record_raw(event(1, 1)).unwrap(),
        RawRecordDisposition::Recorded
    );
    assert_eq!(
        store.record_raw(event(2, MAX_RAW_AGE_NS + 2)).unwrap(),
        RawRecordDisposition::RecordedWithLoss
    );
    assert_eq!(store.raw_len(), 1);
}

#[test]
fn v21_finalized_cache_evicts_oldest_deterministically_at_capacity() {
    let mut store = GuestVictimStore::default();
    let main = GuestProcessIdentity {
        tgid: 41,
        starttime_ticks: 9001,
    };

    for index in 1..=(MAX_FINALIZED_REALIZATIONS as u16 + 1) {
        let tok = token(index);
        let observed = u64::from(index) + 100;
        store
            .begin(
                tok,
                100,
                "01234567-89ab-cdef-0123-456789abcdef",
                main,
                None,
                0,
            )
            .unwrap();
        store.finalize(tok, observed, 0).unwrap().unwrap();
    }

    assert!(store.finalized(&token(1)).is_none());
    assert!(store.finalized(&token(2)).is_some());
    assert!(store
        .finalized(&token(MAX_FINALIZED_REALIZATIONS as u16 + 1))
        .is_some());
    assert_eq!(store.finalized_len(), MAX_FINALIZED_REALIZATIONS);
}

#[test]
fn v21_finalized_cache_expires_entries_older_than_retention_window() {
    let mut store = GuestVictimStore::default();
    let main = GuestProcessIdentity {
        tgid: 51,
        starttime_ticks: 9101,
    };
    let old = token(1);
    let fresh = token(2);

    store
        .begin(
            old,
            1,
            "01234567-89ab-cdef-0123-456789abcdef",
            main,
            None,
            0,
        )
        .unwrap();
    store.finalize(old, 2, 0).unwrap().unwrap();

    let fresh_started = MAX_FINALIZED_AGE_NS + 3;
    store
        .begin(
            fresh,
            fresh_started,
            "01234567-89ab-cdef-0123-456789abcdef",
            main,
            None,
            0,
        )
        .unwrap();
    store
        .finalize(fresh, fresh_started + 1, 0)
        .unwrap()
        .unwrap();

    assert!(store.finalized(&old).is_none());
    assert!(store.finalized(&fresh).is_some());
    assert_eq!(store.finalized_len(), 1);
}
