#[path = "../src/oom_victim_loader.rs"]
mod loader;

use loader::{build_raw_tracepoint_program, parse_kernel_btf_layout, Layout};

const BPF_FUNC_MAP_LOOKUP_ELEM: i32 = 1;
const BPF_FUNC_PROBE_READ_KERNEL: i32 = 113;
const BPF_FUNC_KTIME_GET_BOOT_NS: i32 = 125;
const BPF_FUNC_RINGBUF_OUTPUT: i32 = 130;

fn push_u16(out: &mut Vec<u8>, value: u16) {
    out.extend_from_slice(&value.to_le_bytes());
}

fn push_u32(out: &mut Vec<u8>, value: u32) {
    out.extend_from_slice(&value.to_le_bytes());
}

fn btf_info(kind: u32, vlen: u32) -> u32 {
    (kind << 24) | (vlen & 0xffff)
}

fn synthetic_kernel_btf() -> Vec<u8> {
    // Strings are addressed from the beginning of the BTF string section.
    let mut strings = vec![0u8];
    let mut string_offset = |s: &str| {
        let offset = strings.len() as u32;
        strings.extend_from_slice(s.as_bytes());
        strings.push(0);
        offset
    };

    let int_name = string_offset("int");
    let u64_name = string_offset("u64");
    let task_name = string_offset("task_struct");
    let pid_name = string_offset("pid");
    let tgid_name = string_offset("tgid");
    let start_name = string_offset("start_boottime");
    let cgroups_name = string_offset("cgroups");
    let css_name = string_offset("css_set");
    let dfl_name = string_offset("dfl_cgrp");
    let cgroup_name = string_offset("cgroup");
    let kn_name = string_offset("kn");
    let kernfs_name = string_offset("kernfs_node");
    let id_name = string_offset("id");

    let mut types = Vec::new();

    // type 1: signed 32-bit int
    push_u32(&mut types, int_name);
    push_u32(&mut types, btf_info(1, 0));
    push_u32(&mut types, 4);
    push_u32(&mut types, (1 << 24) | 32);

    // type 2: unsigned 64-bit integer
    push_u32(&mut types, u64_name);
    push_u32(&mut types, btf_info(1, 0));
    push_u32(&mut types, 8);
    push_u32(&mut types, 64);

    // type 3: kernfs_node { u64 id @ 16 }
    push_u32(&mut types, kernfs_name);
    push_u32(&mut types, btf_info(4, 1));
    push_u32(&mut types, 32);
    push_u32(&mut types, id_name);
    push_u32(&mut types, 2);
    push_u32(&mut types, 16 * 8);

    // type 4: *kernfs_node
    push_u32(&mut types, 0);
    push_u32(&mut types, btf_info(2, 0));
    push_u32(&mut types, 3);

    // type 5: cgroup { *kernfs_node kn @ 8 }
    push_u32(&mut types, cgroup_name);
    push_u32(&mut types, btf_info(4, 1));
    push_u32(&mut types, 24);
    push_u32(&mut types, kn_name);
    push_u32(&mut types, 4);
    push_u32(&mut types, 8 * 8);

    // type 6: *cgroup
    push_u32(&mut types, 0);
    push_u32(&mut types, btf_info(2, 0));
    push_u32(&mut types, 5);

    // type 7: css_set { *cgroup dfl_cgrp @ 16 }
    push_u32(&mut types, css_name);
    push_u32(&mut types, btf_info(4, 1));
    push_u32(&mut types, 32);
    push_u32(&mut types, dfl_name);
    push_u32(&mut types, 6);
    push_u32(&mut types, 16 * 8);

    // type 8: *css_set
    push_u32(&mut types, 0);
    push_u32(&mut types, btf_info(2, 0));
    push_u32(&mut types, 7);

    // type 9: task_struct with required identity plus optional cgroup chain.
    push_u32(&mut types, task_name);
    push_u32(&mut types, btf_info(4, 4));
    push_u32(&mut types, 160);
    for (name, ty, byte_offset) in [
        (pid_name, 1, 12u32),
        (tgid_name, 1, 28u32),
        (start_name, 2, 104u32),
        (cgroups_name, 8, 88u32),
    ] {
        push_u32(&mut types, name);
        push_u32(&mut types, ty);
        push_u32(&mut types, byte_offset * 8);
    }

    let mut out = Vec::new();
    push_u16(&mut out, 0xeb9f);
    out.push(1); // version
    out.push(0); // flags
    push_u32(&mut out, 24); // hdr_len
    push_u32(&mut out, 0); // type_off
    push_u32(&mut out, types.len() as u32);
    push_u32(&mut out, types.len() as u32); // str_off
    push_u32(&mut out, strings.len() as u32);
    out.extend_from_slice(&types);
    out.extend_from_slice(&strings);
    out
}

#[test]
fn resolves_required_and_optional_identity_offsets_from_kernel_btf() {
    let layout = parse_kernel_btf_layout(&synthetic_kernel_btf()).unwrap();
    assert_eq!(layout.pid_offset, 12);
    assert_eq!(layout.tgid_offset, 28);
    assert_eq!(layout.start_boottime_offset, 104);
    let cgroup = layout.cgroup_v2.expect("synthetic BTF has exact cgroup-v2 chain");
    assert_eq!(cgroup.task_cgroups_offset, 88);
    assert_eq!(cgroup.default_cgroup_offset, 16);
    assert_eq!(cgroup.kernfs_node_offset, 8);
    assert_eq!(cgroup.kernfs_id_offset, 16);
}

#[test]
fn rejects_truncated_or_non_btf_input_fail_closed() {
    for bytes in [Vec::new(), vec![0u8; 23], b"not-btf".to_vec()] {
        assert!(parse_kernel_btf_layout(&bytes).is_err());
    }
}

#[test]
fn builds_raw_tracepoint_program_with_event_time_identity_and_loss_accounting() {
    let layout = Layout {
        pid_offset: 12,
        tgid_offset: 28,
        start_boottime_offset: 104,
        cgroup_v2: None,
    };
    let program = build_raw_tracepoint_program(layout, 7, 8).unwrap();
    assert!(!program.is_empty());

    let helper_calls: Vec<i32> = program
        .iter()
        .filter(|insn| insn.code == 0x85)
        .map(|insn| insn.imm)
        .collect();
    assert!(helper_calls.contains(&BPF_FUNC_PROBE_READ_KERNEL));
    assert!(helper_calls.contains(&BPF_FUNC_KTIME_GET_BOOT_NS));
    assert!(helper_calls.contains(&BPF_FUNC_RINGBUF_OUTPUT));
    assert!(helper_calls.contains(&BPF_FUNC_MAP_LOOKUP_ELEM));

    // LDDW map-pointer loads must reference both the event ring and the loss map.
    assert!(program.iter().any(|insn| insn.code == 0x18 && insn.imm == 7));
    assert!(program.iter().any(|insn| insn.code == 0x18 && insn.imm == 8));

    // Atomic XADD on the loss-map value is the fail-closed signal for a ring-buffer drop.
    assert!(program.iter().any(|insn| insn.code == 0xdb));
}

#[test]
fn refuses_invalid_map_descriptors() {
    let layout = Layout {
        pid_offset: 12,
        tgid_offset: 28,
        start_boottime_offset: 104,
        cgroup_v2: None,
    };
    assert!(build_raw_tracepoint_program(layout, -1, 8).is_err());
    assert!(build_raw_tracepoint_program(layout, 7, -1).is_err());
}
