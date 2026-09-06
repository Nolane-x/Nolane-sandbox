// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_oom_victim.rs"]
mod guest_oom_victim;

use guest_oom_victim::{
    parse_time_namespace_inode, parse_timens_boottime_offset_ns,
    start_boottime_ns_to_starttime_ticks,
};

#[test]
fn v21_timens_parser_requires_one_exact_boottime_record() {
    let raw = "monotonic 0 0\nboottime -2 500000000\n";
    assert_eq!(parse_timens_boottime_offset_ns(raw).unwrap(), -1_500_000_000);

    assert_eq!(
        parse_timens_boottime_offset_ns("monotonic 1 2\nboottime 7 9\n").unwrap(),
        7_000_000_009
    );

    assert!(parse_timens_boottime_offset_ns("monotonic 0 0\n").is_err());
    assert!(parse_timens_boottime_offset_ns("boottime 0 0\nboottime 1 0\n").is_err());
    assert!(parse_timens_boottime_offset_ns("boottime 0 1000000000\n").is_err());
    assert!(parse_timens_boottime_offset_ns("boottime nope 0\n").is_err());
    assert!(parse_timens_boottime_offset_ns("boottime 0 -1\n").is_err());
}

#[test]
fn v21_time_namespace_handle_parser_is_canonical() {
    assert_eq!(
        parse_time_namespace_inode("time:[4026531834]").unwrap(),
        4_026_531_834
    );
    assert!(parse_time_namespace_inode("time:[0]").is_err());
    assert!(parse_time_namespace_inode("time:[004026531834]").is_err());
    assert!(parse_time_namespace_inode("mnt:[4026531834]").is_err());
    assert!(parse_time_namespace_inode("time:4026531834").is_err());
    assert!(parse_time_namespace_inode(" time:[4026531834]").is_err());
}

#[test]
fn v21_start_boottime_bridge_uses_exact_user_hz_100_integer_math() {
    // USER_HZ=100 => one tick is exactly 10,000,000ns on supported amd64/arm64.
    assert_eq!(
        start_boottime_ns_to_starttime_ticks(25_000_000, 5_000_000).unwrap(),
        3
    );
    assert_eq!(
        start_boottime_ns_to_starttime_ticks(25_000_000, -5_000_000).unwrap(),
        2
    );
    assert_eq!(
        start_boottime_ns_to_starttime_ticks(19_999_999, 0).unwrap(),
        1
    );
}

#[test]
fn v21_start_boottime_bridge_fails_closed_on_underflow_or_overflow() {
    assert!(start_boottime_ns_to_starttime_ticks(1, -2).is_err());
    assert!(start_boottime_ns_to_starttime_ticks(u64::MAX, i128::from(u64::MAX)).is_err());
}
