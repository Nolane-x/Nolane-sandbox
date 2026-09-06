// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_victim.rs"]
mod guest_victim;

use guest_victim::{EvidenceCache, RealizationToken, TokenBindings, EVIDENCE_METADATA_KEY};
use std::collections::HashMap;

fn token(byte: u8) -> RealizationToken {
    RealizationToken::from_hex(&format!("{:02x}", byte).repeat(32)).unwrap()
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
    let mut cache = EvidenceCache::default();
    cache
        .insert_finalized("sandbox-a", a, vec![1, 2, 3])
        .unwrap();

    let mut metadata = HashMap::new();
    metadata.insert(
        EVIDENCE_METADATA_KEY.to_string(),
        format!("{:02x}", 0x61).repeat(32),
    );
    let selected = cache
        .select("sandbox-a", &metadata)
        .expect("canonical exact selector");
    assert_eq!(selected, Some(vec![1, 2, 3]));

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
    cache.insert_finalized("sandbox-a", b, vec![9]).unwrap();
    assert_eq!(cache.select("sandbox-a", &metadata).unwrap(), None);
}

#[test]
fn v21_finalized_cache_is_positive_only_and_immutable() {
    let mut cache = EvidenceCache::default();
    let a = token(0x71);

    assert!(cache.insert_finalized("sandbox-a", a, Vec::new()).is_err());
    cache.insert_finalized("sandbox-a", a, vec![7, 8]).unwrap();
    cache.insert_finalized("sandbox-a", a, vec![7, 8]).unwrap();
    assert!(cache.insert_finalized("sandbox-a", a, vec![9]).is_err());
}
