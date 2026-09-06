// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

#[path = "../src/guest_victim.rs"]
mod guest_victim;

use guest_victim::{
    parse_bind_annotations, BIND_ACTION, REALIZATION_TOKEN_ANNOTATION, UPDATE_ACTION_ANNOTATION,
};
use std::collections::HashMap;

#[test]
fn v21_shim_bind_parser_accepts_only_exact_action_and_token() {
    let mut annotations = HashMap::new();
    annotations.insert(UPDATE_ACTION_ANNOTATION.to_string(), BIND_ACTION.to_string());
    annotations.insert(
        REALIZATION_TOKEN_ANNOTATION.to_string(),
        "2121212121212121212121212121212121212121212121212121212121212121".to_string(),
    );

    let token = parse_bind_annotations(&annotations)
        .expect("valid bind annotations")
        .expect("Wave21 action");
    assert_eq!(token.as_bytes(), &[0x21; 32]);
}

#[test]
fn v21_shim_bind_parser_ignores_unrelated_updates() {
    let mut annotations = HashMap::new();
    annotations.insert(
        UPDATE_ACTION_ANNOTATION.to_string(),
        "PauseToSnapshot".to_string(),
    );
    assert!(parse_bind_annotations(&annotations).unwrap().is_none());
}

#[test]
fn v21_shim_bind_parser_rejects_malformed_token() {
    let mut annotations = HashMap::new();
    annotations.insert(UPDATE_ACTION_ANNOTATION.to_string(), BIND_ACTION.to_string());
    annotations.insert(
        REALIZATION_TOKEN_ANNOTATION.to_string(),
        "ABCDEF".to_string(),
    );
    assert!(parse_bind_annotations(&annotations).is_err());
}
