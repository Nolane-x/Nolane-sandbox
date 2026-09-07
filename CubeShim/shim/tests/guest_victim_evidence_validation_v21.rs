// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_victim.rs"]
mod guest_victim;

use guest_victim::{validate_evidence_set, EvidenceRecord, EvidenceScope, RealizationToken};

const SOURCE: &str = "guest.kernel.oom.mark_victim.raw_tracepoint";

fn token(byte: u8) -> RealizationToken {
    RealizationToken::from_bytes([byte; 32]).unwrap()
}

fn main_record(tok: RealizationToken) -> EvidenceRecord {
    EvidenceRecord {
        version: 1,
        container_id: "sandbox-a".to_string(),
        realization_token: tok,
        guest_boot_id: "11111111-2222-3333-4444-555555555555".to_string(),
        victim_tid: 42,
        victim_tgid: 41,
        victim_starttime_ticks: 9001,
        event_boot_time_ns: 150,
        cgroup_v2_id: None,
        main_pid: 41,
        main_starttime_ticks: 9001,
        scope: EvidenceScope::Main,
        realization_started_boot_ns: 100,
        outcome_observed_boot_ns: 200,
        source: SOURCE.to_string(),
    }
}

#[test]
fn v21_strict_evidence_accepts_exact_main_and_member_records() {
    let tok = token(0x21);
    let main = main_record(tok);
    let mut member = main.clone();
    member.victim_tid = 52;
    member.victim_tgid = 51;
    member.victim_starttime_ticks = 9101;
    member.cgroup_v2_id = Some(77);
    member.scope = EvidenceScope::Member;
    member.event_boot_time_ns = 160;

    let validated = validate_evidence_set("sandbox-a", tok, &[main, member]).unwrap();
    assert_eq!(validated.len(), 2);
}

#[test]
fn v21_strict_evidence_rejects_empty_wrong_selector_and_partial_authority() {
    let tok = token(0x31);
    assert!(validate_evidence_set("sandbox-a", tok, &[]).is_err());

    let valid = main_record(tok);
    assert!(validate_evidence_set("sandbox-b", tok, &[valid.clone()]).is_err());
    assert!(validate_evidence_set("sandbox-a", token(0x32), &[valid.clone()]).is_err());

    let mut broken = valid.clone();
    broken.version = 0;
    assert!(validate_evidence_set("sandbox-a", tok, &[broken]).is_err());

    let mut broken = valid.clone();
    broken.guest_boot_id = "NOT-A-CANONICAL-UUID".to_string();
    assert!(validate_evidence_set("sandbox-a", tok, &[broken]).is_err());

    let mut broken = valid.clone();
    broken.main_pid = 0;
    assert!(validate_evidence_set("sandbox-a", tok, &[broken]).is_err());

    let mut broken = valid.clone();
    broken.source = "compat.GetOOMEvent".to_string();
    assert!(validate_evidence_set("sandbox-a", tok, &[broken]).is_err());
}

#[test]
fn v21_strict_evidence_enforces_window_and_scope_semantics() {
    let tok = token(0x41);
    let mut main = main_record(tok);
    main.victim_tgid = 99;
    assert!(validate_evidence_set("sandbox-a", tok, &[main]).is_err());

    let mut main = main_record(tok);
    main.victim_starttime_ticks = 9002;
    assert!(validate_evidence_set("sandbox-a", tok, &[main]).is_err());

    let mut member = main_record(tok);
    member.scope = EvidenceScope::Member;
    member.victim_tgid = 51;
    member.victim_starttime_ticks = 9101;
    member.cgroup_v2_id = None;
    assert!(validate_evidence_set("sandbox-a", tok, &[member]).is_err());

    let mut outside = main_record(tok);
    outside.event_boot_time_ns = 201;
    assert!(validate_evidence_set("sandbox-a", tok, &[outside]).is_err());

    let mut inverted = main_record(tok);
    inverted.realization_started_boot_ns = 201;
    inverted.outcome_observed_boot_ns = 200;
    assert!(validate_evidence_set("sandbox-a", tok, &[inverted]).is_err());
}

#[test]
fn v21_strict_evidence_rejects_duplicate_conflict_and_mixed_authority() {
    let tok = token(0x51);
    let record = main_record(tok);
    assert!(validate_evidence_set("sandbox-a", tok, &[record.clone(), record.clone()]).is_err());

    let mut mixed_boot = record.clone();
    mixed_boot.guest_boot_id = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee".to_string();
    assert!(validate_evidence_set("sandbox-a", tok, &[record.clone(), mixed_boot]).is_err());

    let mut mixed_main = record.clone();
    mixed_main.main_pid = 43;
    mixed_main.victim_tgid = 43;
    assert!(validate_evidence_set("sandbox-a", tok, &[record.clone(), mixed_main]).is_err());

    let mut mixed_window = record.clone();
    mixed_window.outcome_observed_boot_ns = 250;
    assert!(validate_evidence_set("sandbox-a", tok, &[record, mixed_window]).is_err());
}

#[test]
fn v21_strict_evidence_rejects_more_than_64_records() {
    let tok = token(0x61);
    let base = main_record(tok);
    let mut records = Vec::new();
    for index in 0..65u32 {
        let mut record = base.clone();
        record.victim_tid = index + 1;
        record.event_boot_time_ns = 100 + u64::from(index);
        records.push(record);
    }
    assert!(validate_evidence_set("sandbox-a", tok, &records).is_err());
}
