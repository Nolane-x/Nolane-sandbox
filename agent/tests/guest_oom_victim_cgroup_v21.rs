// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_oom_victim.rs"]
mod guest_oom_victim;

use guest_oom_victim::{
    decode_cgroup_v2_handle, parse_cgroup2_mount, parse_process_cgroup_v2_path,
};

#[test]
fn v21_process_cgroup_parser_accepts_only_exact_unified_membership() {
    assert_eq!(
        parse_process_cgroup_v2_path("0::/cube/pod/runtime\n").unwrap(),
        Some("/cube/pod/runtime".to_string())
    );
    assert_eq!(
        parse_process_cgroup_v2_path("11:memory:/legacy\n10:cpu:/legacy\n").unwrap(),
        None
    );
    assert!(parse_process_cgroup_v2_path("0::relative\n").is_err());
    assert!(parse_process_cgroup_v2_path("0::/cube/../other\n").is_err());
    assert!(parse_process_cgroup_v2_path("0::/one\n0::/two\n").is_err());
}

#[test]
fn v21_cgroup2_mount_parser_is_exact_and_unambiguous() {
    let mountinfo = concat!(
        "31 24 0:27 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw\n",
        "36 24 0:32 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw\n",
    );
    assert_eq!(parse_cgroup2_mount(mountinfo).unwrap(), "/sys/fs/cgroup");

    let ambiguous = format!(
        "{}{}",
        mountinfo,
        "37 24 0:33 / /other-cgroup rw - cgroup2 cgroup rw\n"
    );
    assert!(parse_cgroup2_mount(&ambiguous).is_err());
    assert!(parse_cgroup2_mount("31 24 0:27 / /proc rw - proc proc rw\n").is_err());
}

#[test]
fn v21_cgroup_handle_requires_exact_nonzero_native_u64() {
    let expected = 0x0102_0304_0506_0708u64;
    assert_eq!(decode_cgroup_v2_handle(&expected.to_ne_bytes()).unwrap(), expected);
    assert!(decode_cgroup_v2_handle(&[1u8; 7]).is_err());
    assert!(decode_cgroup_v2_handle(&[0u8; 8]).is_err());
}
