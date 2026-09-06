#[path = "../src/oom_victim_loader.rs"]
mod loader;

use loader::{build_raw_tracepoint_program, Layout};

const BPF_FUNC_MAP_LOOKUP_ELEM: i32 = 1;
const BPF_FUNC_PROBE_READ_KERNEL: i32 = 113;

#[test]
fn required_identity_read_failures_poison_loss_epoch() {
    let program = build_raw_tracepoint_program(
        Layout {
            pid_offset: 12,
            tgid_offset: 28,
            start_boottime_offset: 104,
            cgroup_v2: None,
        },
        7,
        8,
    )
    .unwrap();

    let loss_lookup = program
        .iter()
        .position(|insn| insn.code == 0x85 && insn.imm == BPF_FUNC_MAP_LOOKUP_ELEM)
        .expect("loss epoch map lookup must exist");

    let required_reads: Vec<usize> = program
        .iter()
        .enumerate()
        .filter_map(|(index, insn)| {
            (insn.code == 0x85 && insn.imm == BPF_FUNC_PROBE_READ_KERNEL).then_some(index)
        })
        .take(3)
        .collect();
    assert_eq!(required_reads.len(), 3, "pid/tgid/start_boottime must all be read");

    for call_index in required_reads {
        let branch_index = call_index + 1;
        let branch = &program[branch_index];
        assert_eq!(branch.code, 0x55, "required read must branch on helper failure");
        let target = (branch_index as isize + 1 + branch.off as isize) as usize;
        assert!(
            target < loss_lookup,
            "required identity read failure silently bypasses loss accounting: branch {} -> {}, loss lookup {}",
            branch_index,
            target,
            loss_lookup
        );
    }
}
