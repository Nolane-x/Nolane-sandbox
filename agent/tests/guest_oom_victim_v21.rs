// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_oom_victim.rs"]
mod guest_oom_victim;

use guest_oom_victim::{
    GuestProcessIdentity, GuestVictimStore, RawVictimEvent, RealizationToken, VictimClass,
};

fn token(seed: u8) -> RealizationToken {
    RealizationToken::from_bytes([seed; 32]).expect("non-zero token")
}

#[test]
fn v21_guest_store_classifies_exact_main_lifetime() {
    let mut store = GuestVictimStore::default();
    let tok = token(0x21);
    store
        .begin(
            tok,
            100,
            "01234567-89ab-cdef-0123-456789abcdef",
            GuestProcessIdentity {
                tgid: 41,
                starttime_ticks: 9001,
            },
            Some(77),
            5,
        )
        .unwrap();
    store
        .record_raw(RawVictimEvent {
            tid: 42,
            tgid: 41,
            starttime_ticks: 9001,
            event_boot_ns: 150,
            cgroup_v2_id: Some(77),
        })
        .unwrap();

    let proof = store.finalize(tok, 200, 5).unwrap().unwrap();
    assert!(!proof.poisoned);
    assert_eq!(proof.victims.len(), 1);
    assert_eq!(proof.victims[0].class, VictimClass::Main);
    assert_eq!(proof.victims[0].tid, 42);
    assert_eq!(proof.victims[0].tgid, 41);
}

#[test]
fn v21_guest_store_rejects_pid_reuse_as_main() {
    let mut store = GuestVictimStore::default();
    let tok = token(0x22);
    store
        .begin(
            tok,
            100,
            "01234567-89ab-cdef-0123-456789abcdef",
            GuestProcessIdentity {
                tgid: 41,
                starttime_ticks: 9001,
            },
            Some(77),
            1,
        )
        .unwrap();
    store
        .record_raw(RawVictimEvent {
            tid: 41,
            tgid: 41,
            starttime_ticks: 9002,
            event_boot_ns: 150,
            cgroup_v2_id: Some(77),
        })
        .unwrap();

    let proof = store.finalize(tok, 200, 1).unwrap().unwrap();
    assert_eq!(proof.victims.len(), 1);
    assert_eq!(proof.victims[0].class, VictimClass::Member);
}

#[test]
fn v21_guest_store_poisoned_on_loss_or_overflow() {
    let mut store = GuestVictimStore::default();
    let tok = token(0x23);
    store
        .begin(
            tok,
            100,
            "01234567-89ab-cdef-0123-456789abcdef",
            GuestProcessIdentity {
                tgid: 1,
                starttime_ticks: 1,
            },
            Some(88),
            10,
        )
        .unwrap();
    let proof = store.finalize(tok, 200, 11).unwrap().unwrap();
    assert!(proof.poisoned, "collector loss must poison realization evidence");

    let mut store = GuestVictimStore::default();
    let tok = token(0x24);
    store
        .begin(
            tok,
            100,
            "01234567-89ab-cdef-0123-456789abcdef",
            GuestProcessIdentity {
                tgid: 1,
                starttime_ticks: 1,
            },
            Some(99),
            0,
        )
        .unwrap();
    for i in 0..65u32 {
        store
            .record_raw(RawVictimEvent {
                tid: i + 100,
                tgid: i + 100,
                starttime_ticks: i as u64 + 1,
                event_boot_ns: 120 + i as u64,
                cgroup_v2_id: Some(99),
            })
            .unwrap();
    }
    let proof = store.finalize(tok, 500, 0).unwrap().unwrap();
    assert!(proof.poisoned, "65 victims must poison rather than truncate");
    assert!(proof.victims.is_empty(), "poisoned proof must not expose a partial victim set");
}

#[test]
fn v21_guest_store_is_exact_token_and_finalization_is_immutable() {
    let mut store = GuestVictimStore::default();
    let tok = token(0x25);
    let other = token(0x26);
    store
        .begin(
            tok,
            100,
            "01234567-89ab-cdef-0123-456789abcdef",
            GuestProcessIdentity {
                tgid: 9,
                starttime_ticks: 99,
            },
            Some(7),
            0,
        )
        .unwrap();
    assert!(store.finalize(other, 200, 0).unwrap().is_none());
    let first = store.finalize(tok, 200, 0).unwrap().unwrap();
    let second = store.finalize(tok, 999, 7).unwrap().unwrap();
    assert_eq!(first, second, "finalized evidence must be immutable");
}
