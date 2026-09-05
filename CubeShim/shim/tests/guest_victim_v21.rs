// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use containerd_shim_cube_rs::guest_victim::{RealizationToken, TokenSlot, TokenSlotState};

fn hex_token(byte: u8) -> String {
    format!("{:02x}", byte).repeat(32)
}

#[test]
fn v21_token_parser_requires_canonical_256_bit_lower_hex() {
    assert!(RealizationToken::from_hex(&hex_token(0x21)).is_ok());
    assert!(RealizationToken::from_hex("ab").is_err());
    assert!(RealizationToken::from_hex(&"A1".repeat(32)).is_err());
    assert!(RealizationToken::from_hex(&"gg".repeat(32)).is_err());
    assert!(RealizationToken::from_hex(&"00".repeat(32)).is_err());
}

#[test]
fn v21_token_slot_is_idempotent_then_poisoned_on_conflict() {
    let a = RealizationToken::from_hex(&hex_token(0x21)).unwrap();
    let b = RealizationToken::from_hex(&hex_token(0x22)).unwrap();
    let mut slot = TokenSlot::default();

    slot.bind(a).unwrap();
    slot.bind(a).unwrap();
    assert_eq!(slot.state(), TokenSlotState::Bound);

    assert!(slot.bind(b).is_err());
    assert_eq!(slot.state(), TokenSlotState::Poisoned);
    assert!(slot.consume_main().is_none());
}

#[test]
fn v21_main_start_consumes_once_but_exec_start_does_not() {
    let token = RealizationToken::from_hex(&hex_token(0x33)).unwrap();
    let mut slot = TokenSlot::default();
    slot.bind(token).unwrap();

    assert!(slot.peek_for_exec().is_none(), "exec Start may not consume or receive main token");
    assert_eq!(slot.state(), TokenSlotState::Bound);
    assert_eq!(slot.consume_main(), Some(token));
    assert_eq!(slot.state(), TokenSlotState::Empty);
    assert!(slot.consume_main().is_none());
}
