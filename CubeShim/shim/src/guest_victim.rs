// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use std::collections::HashMap;

pub const UPDATE_ACTION_ANNOTATION: &str = "cube.shimapi.update.action";
pub const REALIZATION_TOKEN_ANNOTATION: &str = "cube.shimapi.update.oom_victim_realization_token";
pub const BIND_ACTION: &str = "BindOOMVictimRealization";
pub const EVIDENCE_METADATA_KEY: &str = "cube-wave21-guest-oom-evidence";
pub const EVIDENCE_TYPE_URL: &str = "io.cubesandbox.v1.GuestOOMVictimEvidenceSet";
pub const EVIDENCE_SOURCE: &str = "guest.kernel.oom.mark_victim.raw_tracepoint";
pub const MAX_EVIDENCE_RECORDS: usize = 64;

#[derive(Clone, Copy, Debug, Eq, Hash, PartialEq)]
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

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum EvidenceScope {
    Main,
    Member,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct EvidenceRecord {
    pub version: u32,
    pub container_id: String,
    pub realization_token: RealizationToken,
    pub guest_boot_id: String,
    pub victim_tid: u32,
    pub victim_tgid: u32,
    pub victim_starttime_ticks: u64,
    pub event_boot_time_ns: u64,
    pub cgroup_v2_id: Option<u64>,
    pub main_pid: u32,
    pub main_starttime_ticks: u64,
    pub scope: EvidenceScope,
    pub realization_started_boot_ns: u64,
    pub outcome_observed_boot_ns: u64,
    pub source: String,
}

fn canonical_lower_uuid(value: &str) -> bool {
    if value.len() != 36 {
        return false;
    }
    value.bytes().enumerate().all(|(index, byte)| match index {
        8 | 13 | 18 | 23 => byte == b'-',
        _ => byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte),
    })
}

fn same_evidence_authority(a: &EvidenceRecord, b: &EvidenceRecord) -> bool {
    a.container_id == b.container_id
        && a.realization_token == b.realization_token
        && a.guest_boot_id == b.guest_boot_id
        && a.main_pid == b.main_pid
        && a.main_starttime_ticks == b.main_starttime_ticks
        && a.realization_started_boot_ns == b.realization_started_boot_ns
        && a.outcome_observed_boot_ns == b.outcome_observed_boot_ns
        && a.source == b.source
}

fn validate_evidence_record(
    expected_container_id: &str,
    expected_token: RealizationToken,
    record: &EvidenceRecord,
) -> Result<(), &'static str> {
    if record.version != 1 {
        return Err("Wave21 evidence version must be exactly one");
    }
    if record.container_id.is_empty()
        || record.container_id.trim() != record.container_id
        || record.container_id != expected_container_id
    {
        return Err("Wave21 evidence container identity does not match exact request");
    }
    if record.realization_token != expected_token {
        return Err("Wave21 evidence realization token does not match exact request");
    }
    if !canonical_lower_uuid(&record.guest_boot_id) {
        return Err("Wave21 evidence guest boot ID is not canonical");
    }
    if record.victim_tid == 0 || record.victim_tgid == 0 || record.victim_starttime_ticks == 0 {
        return Err("Wave21 evidence victim process identity is incomplete");
    }
    if record.main_pid == 0 || record.main_starttime_ticks == 0 {
        return Err("Wave21 evidence main-process authority is incomplete");
    }
    if record.realization_started_boot_ns == 0
        || record.outcome_observed_boot_ns == 0
        || record.event_boot_time_ns == 0
        || record.outcome_observed_boot_ns < record.realization_started_boot_ns
        || record.event_boot_time_ns < record.realization_started_boot_ns
        || record.event_boot_time_ns > record.outcome_observed_boot_ns
    {
        return Err("Wave21 evidence event is outside the exact guest realization window");
    }
    if record.source != EVIDENCE_SOURCE {
        return Err("Wave21 evidence source is not authoritative");
    }
    if matches!(record.cgroup_v2_id, Some(0)) {
        return Err("Wave21 evidence cgroup-v2 identity cannot be zero when present");
    }

    match record.scope {
        EvidenceScope::Main => {
            if record.victim_tgid != record.main_pid
                || record.victim_starttime_ticks != record.main_starttime_ticks
            {
                return Err("Wave21 MAIN evidence does not match exact main lifetime");
            }
        }
        EvidenceScope::Member => {
            if record.cgroup_v2_id.is_none() {
                return Err("Wave21 MEMBER evidence requires exact cgroup-v2 identity");
            }
        }
    }
    Ok(())
}

// validate_evidence_set is the CubeShim trust gate between the guest RPC and
// the immutable exact-token cache. It accepts positive evidence only and never
// reconstructs missing authority from containerd state, exit status, TaskOOM,
// cgroup counters, or any other ambient signal.
pub fn validate_evidence_set(
    container_id: &str,
    token: RealizationToken,
    records: &[EvidenceRecord],
) -> Result<Vec<EvidenceRecord>, &'static str> {
    if container_id.is_empty() || container_id.trim() != container_id {
        return Err("Wave21 evidence request container identity is invalid");
    }
    if records.is_empty() {
        return Err("Wave21 finalized evidence payload must contain positive records");
    }
    if records.len() > MAX_EVIDENCE_RECORDS {
        return Err("Wave21 finalized evidence payload exceeds 64 records");
    }

    let mut validated = records.to_vec();
    for record in &validated {
        validate_evidence_record(container_id, token, record)?;
        if !same_evidence_authority(&validated[0], record) {
            return Err("Wave21 evidence set mixes realization authority");
        }
    }

    validated.sort_by(|a, b| {
        let a_scope = match a.scope {
            EvidenceScope::Main => 0u8,
            EvidenceScope::Member => 1u8,
        };
        let b_scope = match b.scope {
            EvidenceScope::Main => 0u8,
            EvidenceScope::Member => 1u8,
        };
        a.event_boot_time_ns
            .cmp(&b.event_boot_time_ns)
            .then_with(|| a.victim_tgid.cmp(&b.victim_tgid))
            .then_with(|| a.victim_tid.cmp(&b.victim_tid))
            .then_with(|| a.victim_starttime_ticks.cmp(&b.victim_starttime_ticks))
            .then_with(|| a_scope.cmp(&b_scope))
            .then_with(|| a.cgroup_v2_id.cmp(&b.cgroup_v2_id))
    });

    for pair in validated.windows(2) {
        if pair[0] == pair[1] {
            return Err("Wave21 evidence set contains a duplicate record");
        }
    }
    Ok(validated)
}

// parse_bind_annotations recognizes only the reviewed Wave 21 Update action.
// Unrelated Update requests remain on the existing extension path. Once the
// exact bind action is selected, a missing or malformed token fails closed.
pub fn parse_bind_annotations(
    annotations: &HashMap<String, String>,
) -> Result<Option<RealizationToken>, &'static str> {
    match annotations
        .get(UPDATE_ACTION_ANNOTATION)
        .map(String::as_str)
    {
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

#[derive(Clone, Debug, Default)]
pub struct TokenBindings {
    slots: HashMap<String, TokenSlot>,
}

impl TokenBindings {
    pub fn bind(
        &mut self,
        container_id: &str,
        token: RealizationToken,
    ) -> Result<(), &'static str> {
        if container_id.is_empty() {
            return Err("container id is required");
        }
        self.slots
            .entry(container_id.to_string())
            .or_default()
            .bind(token)
    }

    pub fn consume_main(&mut self, container_id: &str) -> Option<RealizationToken> {
        let token = self.slots.get_mut(container_id)?.consume_main();
        self.slots.remove(container_id);
        token
    }

    pub fn clear(&mut self, container_id: &str) {
        self.slots.remove(container_id);
    }

    pub fn clear_all(&mut self) {
        self.slots.clear();
    }
}

fn consume_varint(input: &mut &[u8]) -> Result<u64, &'static str> {
    let mut value = 0u64;
    for shift in (0..70).step_by(7) {
        let (&byte, rest) = input
            .split_first()
            .ok_or("Wave21 protobuf varint is truncated")?;
        *input = rest;
        if shift == 63 && byte > 1 {
            return Err("Wave21 protobuf varint overflows u64");
        }
        value |= u64::from(byte & 0x7f) << shift;
        if byte & 0x80 == 0 {
            return Ok(value);
        }
    }
    Err("Wave21 protobuf varint is overlong")
}

fn consume_tag(input: &mut &[u8]) -> Result<(u32, u8), &'static str> {
    let raw = consume_varint(input)?;
    let field = raw >> 3;
    if field == 0 || field > u64::from(u32::MAX) {
        return Err("Wave21 protobuf field number is invalid");
    }
    Ok((field as u32, (raw & 0x07) as u8))
}

fn consume_bytes<'a>(input: &mut &'a [u8]) -> Result<&'a [u8], &'static str> {
    let len = consume_varint(input)?;
    let len = usize::try_from(len).map_err(|_| "Wave21 protobuf length overflows usize")?;
    if input.len() < len {
        return Err("Wave21 protobuf bytes field is truncated");
    }
    let (value, rest) = input.split_at(len);
    *input = rest;
    Ok(value)
}

fn consume_string(input: &mut &[u8]) -> Result<String, &'static str> {
    let bytes = consume_bytes(input)?;
    std::str::from_utf8(bytes)
        .map(str::to_string)
        .map_err(|_| "Wave21 protobuf string is not UTF-8")
}

fn u32_value(value: u64) -> Result<u32, &'static str> {
    u32::try_from(value).map_err(|_| "Wave21 protobuf uint32 field overflows")
}

fn decode_evidence_record(
    expected_container_id: &str,
    expected_token: RealizationToken,
    mut input: &[u8],
) -> Result<EvidenceRecord, &'static str> {
    let mut seen = [false; 16];
    let mut version = 0u32;
    let mut container_id = String::new();
    let mut realization_token = None;
    let mut guest_boot_id = String::new();
    let mut victim_tid = 0u32;
    let mut victim_tgid = 0u32;
    let mut victim_starttime_ticks = 0u64;
    let mut event_boot_time_ns = 0u64;
    let mut cgroup_v2_id = None;
    let mut main_pid = 0u32;
    let mut main_starttime_ticks = 0u64;
    let mut scope = None;
    let mut realization_started_boot_ns = 0u64;
    let mut outcome_observed_boot_ns = 0u64;
    let mut source = String::new();

    while !input.is_empty() {
        let (field, wire) = consume_tag(&mut input)?;
        if field > 15 || seen[field as usize] {
            return Err("Wave21 evidence record contains unknown or duplicate field");
        }
        seen[field as usize] = true;
        match field {
            1 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 => {
                if wire != 0 {
                    return Err("Wave21 evidence record has wrong protobuf wire type");
                }
                let value = consume_varint(&mut input)?;
                match field {
                    1 => version = u32_value(value)?,
                    5 => victim_tid = u32_value(value)?,
                    6 => victim_tgid = u32_value(value)?,
                    7 => victim_starttime_ticks = value,
                    8 => event_boot_time_ns = value,
                    9 => cgroup_v2_id = Some(value),
                    10 => main_pid = u32_value(value)?,
                    11 => main_starttime_ticks = value,
                    12 => {
                        scope = Some(match value {
                            1 => EvidenceScope::Main,
                            2 => EvidenceScope::Member,
                            _ => return Err("Wave21 evidence scope is not authoritative"),
                        })
                    }
                    13 => realization_started_boot_ns = value,
                    14 => outcome_observed_boot_ns = value,
                    _ => unreachable!(),
                }
            }
            2 | 3 | 4 | 15 => {
                if wire != 2 {
                    return Err("Wave21 evidence record has wrong protobuf wire type");
                }
                match field {
                    2 => container_id = consume_string(&mut input)?,
                    3 => {
                        let raw = consume_bytes(&mut input)?;
                        if raw.len() != 32 {
                            return Err(
                                "Wave21 evidence realization token must be exactly 32 bytes",
                            );
                        }
                        let mut bytes = [0u8; 32];
                        bytes.copy_from_slice(raw);
                        realization_token = Some(RealizationToken::from_bytes(bytes)?);
                    }
                    4 => guest_boot_id = consume_string(&mut input)?,
                    15 => source = consume_string(&mut input)?,
                    _ => unreachable!(),
                }
            }
            _ => return Err("Wave21 evidence record contains unknown field"),
        }
    }

    let record = EvidenceRecord {
        version,
        container_id,
        realization_token: realization_token
            .ok_or("Wave21 evidence realization token is missing")?,
        guest_boot_id,
        victim_tid,
        victim_tgid,
        victim_starttime_ticks,
        event_boot_time_ns,
        cgroup_v2_id,
        main_pid,
        main_starttime_ticks,
        scope: scope.ok_or("Wave21 evidence scope is missing")?,
        realization_started_boot_ns,
        outcome_observed_boot_ns,
        source,
    };
    validate_evidence_record(expected_container_id, expected_token, &record)?;
    Ok(record)
}

pub fn decode_and_validate_evidence_payload(
    container_id: &str,
    token: RealizationToken,
    mut payload: &[u8],
) -> Result<Vec<EvidenceRecord>, &'static str> {
    let mut records = Vec::new();
    while !payload.is_empty() {
        let (field, wire) = consume_tag(&mut payload)?;
        if field != 1 || wire != 2 {
            return Err("Wave21 evidence set contains unknown protobuf field");
        }
        let encoded = consume_bytes(&mut payload)?;
        records.push(decode_evidence_record(container_id, token, encoded)?);
        if records.len() > MAX_EVIDENCE_RECORDS {
            return Err("Wave21 finalized evidence payload exceeds 64 records");
        }
    }
    validate_evidence_set(container_id, token, &records)
}

#[derive(Clone, Debug, Default)]
pub struct EvidenceCache {
    entries: HashMap<(String, RealizationToken), Vec<u8>>,
}

impl EvidenceCache {
    pub fn insert_finalized(
        &mut self,
        container_id: &str,
        token: RealizationToken,
        payload: Vec<u8>,
    ) -> Result<(), &'static str> {
        if container_id.is_empty() {
            return Err("container id is required");
        }
        if payload.is_empty() {
            return Err("finalized evidence payload must be positive");
        }
        decode_and_validate_evidence_payload(container_id, token, &payload)?;

        let key = (container_id.to_string(), token);
        match self.entries.get(&key) {
            None => {
                self.entries.insert(key, payload);
                Ok(())
            }
            Some(existing) if *existing == payload => Ok(()),
            Some(_) => Err("conflicting finalized evidence for exact realization token"),
        }
    }

    pub fn select(
        &self,
        container_id: &str,
        metadata: &HashMap<String, String>,
    ) -> Result<Option<Vec<u8>>, &'static str> {
        let raw = match metadata.get(EVIDENCE_METADATA_KEY) {
            None => return Ok(None),
            Some(raw) => raw,
        };
        let token = RealizationToken::from_hex(raw)?;
        Ok(self
            .entries
            .get(&(container_id.to_string(), token))
            .cloned())
    }

    pub fn clear(&mut self, container_id: &str) {
        self.entries.retain(|(id, _), _| id != container_id);
    }

    pub fn clear_all(&mut self) {
        self.entries.clear();
    }
}
