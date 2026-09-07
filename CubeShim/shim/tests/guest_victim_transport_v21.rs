// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_victim.rs"]
mod guest_victim;

use guest_victim::{
    EvidenceCache, RealizationToken, TokenBindings, EVIDENCE_METADATA_KEY, EVIDENCE_SOURCE,
};
use std::collections::HashMap;

fn token(byte: u8) -> RealizationToken {
    RealizationToken::from_hex(&format!("{:02x}", byte).repeat(32)).unwrap()
}

fn put_varint(out: &mut Vec<u8>, mut value: u64) {
    loop {
        let mut byte = (value & 0x7f) as u8;
        value >>= 7;
        if value != 0 {
            byte |= 0x80;
        }
        out.push(byte);
        if value == 0 {
            return;
        }
    }
}

fn field_varint(out: &mut Vec<u8>, field: u64, value: u64) {
    put_varint(out, field << 3);
    put_varint(out, value);
}

fn field_bytes(out: &mut Vec<u8>, field: u64, value: &[u8]) {
    put_varint(out, (field << 3) | 2);
    put_varint(out, value.len() as u64);
    out.extend_from_slice(value);
}

fn evidence_payload(container_id: &str, token: RealizationToken, event_boot_ns: u64) -> Vec<u8> {
    let mut record = Vec::new();
    field_varint(&mut record, 1, 1);
    field_bytes(&mut record, 2, container_id.as_bytes());
    field_bytes(&mut record, 3, token.as_bytes());
    field_bytes(
        &mut record,
        4,
        b"11111111-2222-3333-4444-555555555555",
    );
    field_varint(&mut record, 5, 42);
    field_varint(&mut record, 6, 42);
    field_varint(&mut record, 7, 9001);
    field_varint(&mut record, 8, event_boot_ns);
    field_varint(&mut record, 10, 42);
    field_varint(&mut record, 11, 9001);
    field_varint(&mut record, 12, 1);
    field_varint(&mut record, 13, 100);
    field_varint(&mut record, 14, 200);
    field_bytes(&mut record, 15, EVIDENCE_SOURCE.as_bytes());

    let mut payload = Vec::new();
    field_bytes(&mut payload, 1, &record);
    payload
}

#[test]
fn v21_bindings_are_container_scoped_and_main_start_consumes_once() {
    let mut bindings = TokenBindings::default();
    let a = token(0x41);
    let b = token(0x42);

    bindings.bind("sandbox-a", a).unwrap();
    bindings.bind("sandbox-b", b).unwrap();

    assert_eq!(bindings.consume_main("sandbox-a"), Some(a));
    assert_eq!(bindings.consume_main("sandbox-a"), None);
    assert_eq!(bindings.consume_main("sandbox-b"), Some(b));
}

#[test]
fn v21_conflicting_bind_poisons_only_that_container_slot() {
    let mut bindings = TokenBindings::default();
    let a = token(0x51);
    let conflict = token(0x52);
    let other = token(0x53);

    bindings.bind("sandbox-a", a).unwrap();
    bindings.bind("sandbox-b", other).unwrap();
    assert!(bindings.bind("sandbox-a", conflict).is_err());

    assert_eq!(bindings.consume_main("sandbox-a"), None);
    assert_eq!(bindings.consume_main("sandbox-b"), Some(other));
}

#[test]
fn v21_evidence_selector_requires_exact_metadata_token_and_cache_key() {
    let a = token(0x61);
    let b = token(0x62);
    let a_payload = evidence_payload("sandbox-a", a, 150);
    let b_payload = evidence_payload("sandbox-a", b, 151);
    let mut cache = EvidenceCache::default();
    cache
        .insert_finalized("sandbox-a", a, a_payload.clone())
        .unwrap();

    let mut metadata = HashMap::new();
    metadata.insert(
        EVIDENCE_METADATA_KEY.to_string(),
        format!("{:02x}", 0x61).repeat(32),
    );
    let selected = cache
        .select("sandbox-a", &metadata)
        .expect("canonical exact selector");
    assert_eq!(selected, Some(a_payload));

    metadata.insert(
        EVIDENCE_METADATA_KEY.to_string(),
        format!("{:02x}", 0x62).repeat(32),
    );
    assert_eq!(cache.select("sandbox-a", &metadata).unwrap(), None);
    assert_eq!(cache.select("sandbox-b", &metadata).unwrap(), None);

    metadata.insert(EVIDENCE_METADATA_KEY.to_string(), "ABCDEF".to_string());
    assert!(cache.select("sandbox-a", &metadata).is_err());

    metadata.clear();
    assert_eq!(cache.select("sandbox-a", &metadata).unwrap(), None);

    // A cached token for another realization must never be returned merely
    // because the container id matches.
    cache.insert_finalized("sandbox-a", b, b_payload).unwrap();
    assert_eq!(cache.select("sandbox-a", &metadata).unwrap(), None);
}

#[test]
fn v21_finalized_cache_validates_positive_payload_and_is_immutable() {
    let mut cache = EvidenceCache::default();
    let a = token(0x71);
    let payload = evidence_payload("sandbox-a", a, 150);
    let conflict = evidence_payload("sandbox-a", a, 160);

    assert!(cache.insert_finalized("sandbox-a", a, Vec::new()).is_err());
    assert!(cache
        .insert_finalized("sandbox-a", a, vec![0xff, 0xff])
        .is_err());
    assert!(cache
        .insert_finalized("sandbox-b", a, payload.clone())
        .is_err());

    cache
        .insert_finalized("sandbox-a", a, payload.clone())
        .unwrap();
    cache.insert_finalized("sandbox-a", a, payload).unwrap();
    assert!(cache.insert_finalized("sandbox-a", a, conflict).is_err());
}
