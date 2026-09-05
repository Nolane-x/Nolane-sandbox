// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use std::collections::HashMap;

pub const MAX_VICTIMS_PER_REALIZATION: usize = 64;

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
        if value.len() != 64 || !value.bytes().all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte)) {
            return Err("realization token must be 64 lowercase hexadecimal characters");
        }

        let mut bytes = [0u8; 32];
        let raw = value.as_bytes();
        for (index, out) in bytes.iter_mut().enumerate() {
            let high = decode_hex(raw[index * 2])?;
            let low = decode_hex(raw[index * 2 + 1])?;
            *out = (high << 4) | low;
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
pub struct GuestProcessIdentity {
    pub tgid: u32,
    pub starttime_ticks: u64,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RawVictimEvent {
    pub tid: u32,
    pub tgid: u32,
    pub starttime_ticks: u64,
    pub event_boot_ns: u64,
    pub cgroup_v2_id: Option<u64>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum VictimClass {
    Main,
    Member,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct VictimProof {
    pub tid: u32,
    pub tgid: u32,
    pub starttime_ticks: u64,
    pub event_boot_ns: u64,
    pub cgroup_v2_id: Option<u64>,
    pub class: VictimClass,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct FinalizedEvidence {
    pub guest_boot_id: String,
    pub poisoned: bool,
    pub victims: Vec<VictimProof>,
}

#[derive(Clone, Debug)]
struct OpenRealization {
    started_boot_ns: u64,
    guest_boot_id: String,
    main: GuestProcessIdentity,
    expected_cgroup_v2_id: Option<u64>,
    start_loss_epoch: u64,
}

#[derive(Default)]
pub struct GuestVictimStore {
    open: HashMap<RealizationToken, OpenRealization>,
    finalized: HashMap<RealizationToken, FinalizedEvidence>,
    raw: Vec<RawVictimEvent>,
}

impl GuestVictimStore {
    pub fn begin(
        &mut self,
        token: RealizationToken,
        started_boot_ns: u64,
        guest_boot_id: &str,
        main: GuestProcessIdentity,
        expected_cgroup_v2_id: Option<u64>,
        loss_epoch: u64,
    ) -> Result<(), &'static str> {
        if started_boot_ns == 0 {
            return Err("realization start time must be non-zero");
        }
        if main.tgid == 0 || main.starttime_ticks == 0 {
            return Err("main process identity must be exact and non-zero");
        }
        if !canonical_boot_id(guest_boot_id) {
            return Err("guest boot id must be canonical");
        }
        if matches!(expected_cgroup_v2_id, Some(0)) {
            return Err("zero cgroup id means unknown and must use None");
        }
        if self.finalized.contains_key(&token) {
            return Err("finalized realization is immutable");
        }

        self.open.insert(
            token,
            OpenRealization {
                started_boot_ns,
                guest_boot_id: guest_boot_id.to_owned(),
                main,
                expected_cgroup_v2_id,
                start_loss_epoch: loss_epoch,
            },
        );
        Ok(())
    }

    pub fn record_raw(&mut self, event: RawVictimEvent) -> Result<(), &'static str> {
        if event.tid == 0
            || event.tgid == 0
            || event.starttime_ticks == 0
            || event.event_boot_ns == 0
            || matches!(event.cgroup_v2_id, Some(0))
        {
            return Err("raw victim event identity must be exact and non-zero");
        }
        if !self.raw.contains(&event) {
            self.raw.push(event);
        }
        Ok(())
    }

    pub fn finalize(
        &mut self,
        token: RealizationToken,
        outcome_observed_boot_ns: u64,
        loss_epoch: u64,
    ) -> Result<Option<FinalizedEvidence>, &'static str> {
        if let Some(evidence) = self.finalized.get(&token) {
            return Ok(Some(evidence.clone()));
        }

        let Some(realization) = self.open.remove(&token) else {
            return Ok(None);
        };
        if outcome_observed_boot_ns < realization.started_boot_ns {
            return Err("outcome observation precedes realization start");
        }

        if loss_epoch != realization.start_loss_epoch {
            let evidence = FinalizedEvidence {
                guest_boot_id: realization.guest_boot_id,
                poisoned: true,
                victims: Vec::new(),
            };
            self.finalized.insert(token, evidence.clone());
            return Ok(Some(evidence));
        }

        let mut victims = Vec::new();
        for event in &self.raw {
            if event.event_boot_ns < realization.started_boot_ns
                || event.event_boot_ns > outcome_observed_boot_ns
            {
                continue;
            }

            let class = if event.tgid == realization.main.tgid
                && event.starttime_ticks == realization.main.starttime_ticks
            {
                Some(VictimClass::Main)
            } else if event.cgroup_v2_id.is_some()
                && event.cgroup_v2_id == realization.expected_cgroup_v2_id
            {
                Some(VictimClass::Member)
            } else {
                None
            };

            let Some(class) = class else {
                continue;
            };
            let proof = VictimProof {
                tid: event.tid,
                tgid: event.tgid,
                starttime_ticks: event.starttime_ticks,
                event_boot_ns: event.event_boot_ns,
                cgroup_v2_id: event.cgroup_v2_id,
                class,
            };
            if !victims.contains(&proof) {
                victims.push(proof);
            }
        }

        victims.sort_by_key(|proof| {
            (
                proof.event_boot_ns,
                proof.tgid,
                proof.tid,
                proof.starttime_ticks,
            )
        });

        let poisoned = victims.len() > MAX_VICTIMS_PER_REALIZATION;
        if poisoned {
            victims.clear();
        }
        let evidence = FinalizedEvidence {
            guest_boot_id: realization.guest_boot_id,
            poisoned,
            victims,
        };
        self.finalized.insert(token, evidence.clone());
        Ok(Some(evidence))
    }
}

fn canonical_boot_id(value: &str) -> bool {
    if value.len() != 36 {
        return false;
    }
    for (index, byte) in value.bytes().enumerate() {
        if matches!(index, 8 | 13 | 18 | 23) {
            if byte != b'-' {
                return false;
            }
        } else if !(byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte)) {
            return false;
        }
    }
    true
}
