// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/oom_victim_runtime.rs"]
mod oom_victim_runtime;

use oom_victim_runtime::{
    ring_geometry, MapCreateAttr, BPF_MAP_TYPE_ARRAY, BPF_MAP_TYPE_RINGBUF,
    BPF_PROG_TYPE_RAW_TRACEPOINT, BPF_RAW_TRACEPOINT_OPEN, RAW_TRACEPOINT_NAME,
};

#[test]
fn v21_runtime_uses_exact_kernel_bpf_abi_constants() {
    assert_eq!(BPF_MAP_TYPE_ARRAY, 2);
    assert_eq!(BPF_MAP_TYPE_RINGBUF, 27);
    assert_eq!(BPF_PROG_TYPE_RAW_TRACEPOINT, 17);
    assert_eq!(BPF_RAW_TRACEPOINT_OPEN, 17);
    assert_eq!(RAW_TRACEPOINT_NAME, b"mark_victim\0");
}

#[test]
fn v21_ring_geometry_matches_kernel_producer_mmap_contract() {
    let g = ring_geometry(4096, 1 << 16).unwrap();
    assert_eq!(g.page_size, 4096);
    assert_eq!(g.capacity, 65536);
    assert_eq!(g.mask, 65535);
    assert_eq!(g.consumer_map_len, 4096);
    assert_eq!(g.producer_map_len, 4096 + 2 * 65536);
    assert_eq!(g.data_offset, 4096);

    assert!(ring_geometry(0, 65536).is_err());
    assert!(ring_geometry(4096, 0).is_err());
    assert!(ring_geometry(4096, 32768 + 4096).is_err());
    assert!(ring_geometry(4096, 2048).is_err());
}

#[test]
fn v21_ringbuf_and_loss_maps_are_exact_and_bounded() {
    let ring = MapCreateAttr::ringbuf(65536).unwrap();
    assert_eq!(ring.map_type, BPF_MAP_TYPE_RINGBUF);
    assert_eq!(ring.key_size, 0);
    assert_eq!(ring.value_size, 0);
    assert_eq!(ring.max_entries, 65536);
    assert_eq!(ring.map_flags, 0);

    let loss = MapCreateAttr::loss_epoch();
    assert_eq!(loss.map_type, BPF_MAP_TYPE_ARRAY);
    assert_eq!(loss.key_size, 4);
    assert_eq!(loss.value_size, 8);
    assert_eq!(loss.max_entries, 1);
    assert_eq!(loss.map_flags, 0);

    assert!(MapCreateAttr::ringbuf(0).is_err());
    assert!(MapCreateAttr::ringbuf(65535).is_err());
}
