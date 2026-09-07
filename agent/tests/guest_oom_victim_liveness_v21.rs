// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_oom_victim.rs"]
mod guest_oom_victim;

use guest_oom_victim::GuestVictimCollectorCoverage;

#[test]
fn v21_unavailable_collector_poison_epoch_at_begin() {
    let mut coverage = GuestVictimCollectorCoverage::default();
    let start_epoch = coverage.begin_epoch();
    assert_ne!(
        start_epoch,
        coverage.current_epoch(),
        "a realization that starts without collector coverage must be poisoned"
    );
}

#[test]
fn v21_live_collector_keeps_epoch_stable_until_loss() {
    let mut coverage = GuestVictimCollectorCoverage::default();
    coverage.mark_live();

    let start_epoch = coverage.begin_epoch();
    assert_eq!(start_epoch, coverage.current_epoch());

    coverage.mark_lost();
    assert_ne!(
        start_epoch,
        coverage.current_epoch(),
        "collector death must poison realizations that overlapped the loss"
    );
}

#[test]
fn v21_restart_never_heals_prior_coverage_loss() {
    let mut coverage = GuestVictimCollectorCoverage::default();
    coverage.mark_live();
    let before_loss = coverage.begin_epoch();

    coverage.mark_lost();
    coverage.mark_live();
    assert_ne!(before_loss, coverage.current_epoch());

    let after_restart = coverage.begin_epoch();
    assert_eq!(
        after_restart,
        coverage.current_epoch(),
        "a fresh realization may be complete only after live coverage is restored"
    );
}
