// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_oom_victim.rs"]
mod guest_oom_victim;

use guest_oom_victim::{
    GuestProcessIdentity, GuestVictimStore, RawVictimEvent, RealizationToken, VictimClass,
};

fn token(byte: u8) -> RealizationToken {
    RealizationToken::from_bytes([byte; 32]).unwrap()
}

#[test]
fn v21_finalized_evidence_carries_exact_guest_realization_authority() {
    let mut store = GuestVictimStore::default();
    let tok = token(0x81);
    let main = GuestProcessIdentity {
        tgid: 41,
        starttime_ticks: 9001,
    };

    store
        .begin(
            tok,
            100,
            "11111111-2222-3333-4444-555555555555",
            main,
            Some(77),
            3,
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

    let evidence = store.finalize(tok, 200, 3).unwrap().unwrap();
    assert!(!evidence.poisoned);
    assert_eq!(evidence.realization_started_boot_ns, 100);
    assert_eq!(evidence.outcome_observed_boot_ns, 200);
    assert_eq!(evidence.main, main);
    assert_eq!(evidence.expected_cgroup_v2_id, Some(77));
    assert_eq!(evidence.victims.len(), 1);
    assert_eq!(evidence.victims[0].class, VictimClass::Main);
}

#[test]
fn v21_loss_poisoned_finalization_preserves_window_authority_but_no_positive_victims() {
    let mut store = GuestVictimStore::default();
    let tok = token(0x82);
    let main = GuestProcessIdentity {
        tgid: 51,
        starttime_ticks: 9101,
    };
    store
        .begin(
            tok,
            300,
            "11111111-2222-3333-4444-555555555555",
            main,
            None,
            5,
        )
        .unwrap();

    let evidence = store.finalize(tok, 400, 6).unwrap().unwrap();
    assert!(evidence.poisoned);
    assert!(evidence.victims.is_empty());
    assert_eq!(evidence.realization_started_boot_ns, 300);
    assert_eq!(evidence.outcome_observed_boot_ns, 400);
    assert_eq!(evidence.main, main);
    assert_eq!(evidence.expected_cgroup_v2_id, None);
}
