// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use std::collections::HashMap;

pub const UPDATE_ACTION_ANNOTATION: &str = "cube.shimapi.update.action";
pub const REALIZATION_TOKEN_ANNOTATION: &str = "cube.shimapi.update.oom_victim_realization_token";
pub const BIND_ACTION: &str = "BindOOMVictimRealization";

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RealizationToken([u8; 32]);

impl RealizationToken {
    pub fn from_bytes(bytes: [u8; 32]) -> Result<Self, &'static str> {
        if bytes.iter().all(|byte| *byte == 0) {
            return Err("realization token must be non-zero");
        }
        Ok(Self(bytes))
    }

    pub fn from_hex(value: &str) -> Result<Self, &'static str> {
        if value.len() != 64
            || !value
                .bytes()
                .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
        {
            return Err("realization token must be 64 lowercase hexadecimal characters");
        }

        let raw = value.as_bytes();
        let mut bytes = [0u8; 32];
        for (index, out) in bytes.iter_mut().enumerate() {
            *out = (decode_hex(raw[index * 2])? << 4) | decode_hex(raw[index * 2 + 1])?;
        }
        Self::from_bytes(bytes)
    }

    pub fn as_bytes(&self) -> &[u8; 32] {
        &self.0
    }
}

fn decode_hex(value: u8) -> Result<u8, &'static str> {
    match value {
        b'0'..=b'9' => Ok(value - b'0'),
        b'a'..=b'f' => Ok(value - b'a' + 10),
        _ => Err("invalid hexadecimal digit"),
    }
}

// parse_bind_annotations recognizes only the reviewed Wave 21 Update action.
// Unrelated Update requests remain on the existing extension path. Once the
// exact bind action is selected, a missing or malformed token fails closed.
pub fn parse_bind_annotations(
    annotations: &HashMap<String, String>,
) -> Result<Option<RealizationToken>, &'static str> {
    match annotations.get(UPDATE_ACTION_ANNOTATION).map(String::as_str) {
        None => Ok(None),
        Some(action) if action != BIND_ACTION => Ok(None),
        Some(_) => {
            let raw = annotations
                .get(REALIZATION_TOKEN_ANNOTATION)
                .ok_or("Wave21 realization-token annotation is required")?;
            RealizationToken::from_hex(raw).map(Some)
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TokenSlotState {
    Empty,
    Bound,
    Poisoned,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum TokenSlotInner {
    Empty,
    Bound(RealizationToken),
    Poisoned,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct TokenSlot {
    inner: TokenSlotInner,
}

impl Default for TokenSlot {
    fn default() -> Self {
        Self {
            inner: TokenSlotInner::Empty,
        }
    }
}

impl TokenSlot {
    pub fn state(&self) -> TokenSlotState {
        match self.inner {
            TokenSlotInner::Empty => TokenSlotState::Empty,
            TokenSlotInner::Bound(_) => TokenSlotState::Bound,
            TokenSlotInner::Poisoned => TokenSlotState::Poisoned,
        }
    }

    pub fn bind(&mut self, token: RealizationToken) -> Result<(), &'static str> {
        match self.inner {
            TokenSlotInner::Empty => {
                self.inner = TokenSlotInner::Bound(token);
                Ok(())
            }
            TokenSlotInner::Bound(existing) if existing == token => Ok(()),
            TokenSlotInner::Bound(_) => {
                self.inner = TokenSlotInner::Poisoned;
                Err("conflicting realization token poisoned pending main-start slot")
            }
            TokenSlotInner::Poisoned => Err("pending main-start slot is poisoned"),
        }
    }

    pub fn consume_main(&mut self) -> Option<RealizationToken> {
        let current = self.inner;
        self.inner = TokenSlotInner::Empty;
        match current {
            TokenSlotInner::Bound(token) => Some(token),
            TokenSlotInner::Empty | TokenSlotInner::Poisoned => None,
        }
    }

    pub fn peek_for_exec(&self) -> Option<RealizationToken> {
        None
    }

    pub fn clear(&mut self) {
        self.inner = TokenSlotInner::Empty;
    }
}
