// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_oom_victim.rs"]
mod guest_oom_victim;

use guest_oom_victim::{GuestProcessIdentity, GuestVictimStore, RealizationToken};

#[test]
fn v21_guest_lookup_is_exact_token_and_finalized_only() {
    let token = RealizationToken::from_bytes([0x31; 32]).unwrap();
    let other = RealizationToken::from_bytes([0x32; 32]).unwrap();
    let mut store = GuestVictimStore::default();
    store
        .begin(
            token,
            100,
            "01234567-89ab-cdef-0123-456789abcdef",
            GuestProcessIdentity {
                tgid: 41,
                starttime_ticks: 9001,
            },
            Some(77),
            0,
        )
        .unwrap();

    assert!(store.finalized(&token).is_none());
    assert!(store.finalized(&other).is_none());

    let finalized = store.finalize(token, 200, 0).unwrap().unwrap();
    assert_eq!(store.finalized(&token), Some(finalized));
    assert!(store.finalized(&other).is_none());
}
