// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use std::collections::HashMap;
use std::fs;

pub const MAX_VICTIMS_PER_REALIZATION: usize = 64;
pub const MAX_RAW_VICTIM_EVENTS: usize = 1024;
pub const MAX_RAW_AGE_NS: u64 = 10 * 60 * 1_000_000_000;
pub const MAX_FINALIZED_REALIZATIONS: usize = 256;
pub const MAX_FINALIZED_AGE_NS: u64 = 10 * 60 * 1_000_000_000;

#[derive(Clone, Copy, Debug, Eq, Hash, PartialEq)]
pub struct RealizationToken([u8; 32]);

impl RealizationToken {
    pub fn from_bytes(bytes: [u8; 32]) -> Result<Self, &'static str> {
        if bytes.iter().all(|byte| *byte == 0) {
            return Err("realization token must be non-zero");
        }
        Ok(Self(bytes))
    }

    #[cfg(test)]
    pub fn from_hex(value: &str) -> Result<Self, &'static str> {
        if value.len() != 64
            || !value
                .bytes()
                .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
        {
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

    #[cfg(test)]
    pub fn as_bytes(&self) -> &[u8; 32] {
        &self.0
    }
}

#[cfg(test)]
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
pub enum RawRecordDisposition {
    Recorded,
    Duplicate,
    RecordedWithLoss,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct GuestVictimCollectorCoverage {
    live: bool,
    loss_epoch: u64,
}

impl GuestVictimCollectorCoverage {
    pub fn mark_live(&mut self) {
        self.live = true;
    }

    pub fn mark_lost(&mut self) {
        self.live = false;
        self.loss_epoch = self.loss_epoch.saturating_add(1);
    }

    pub fn begin_epoch(&mut self) -> u64 {
        let start_epoch = self.loss_epoch;
        if !self.live {
            self.loss_epoch = self.loss_epoch.saturating_add(1);
        }
        start_epoch
    }

    pub fn current_epoch(&self) -> u64 {
        self.loss_epoch
    }
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
    pub realization_started_boot_ns: u64,
    pub outcome_observed_boot_ns: u64,
    pub main: GuestProcessIdentity,
    pub expected_cgroup_v2_id: Option<u64>,
    pub poisoned: bool,
    pub victims: Vec<VictimProof>,
}

#[derive(Clone, Debug)]
struct OpenRealization {
    started_boot_ns: u64,
    guest_boot_id: String,
    main: GuestProcessIdentity,
    expected_cgroup_v2_id: Option<u64>,
    start_external_loss_epoch: u64,
    start_store_loss_epoch: u128,
}

#[derive(Debug, Default)]
pub struct GuestVictimStore {
    open: HashMap<RealizationToken, OpenRealization>,
    finalized: HashMap<RealizationToken, FinalizedEvidence>,
    raw: Vec<RawVictimEvent>,
    raw_watermark_boot_ns: u64,
    finalized_watermark_boot_ns: u64,
    store_loss_epoch: u128,
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
                start_external_loss_epoch: loss_epoch,
                start_store_loss_epoch: self.store_loss_epoch,
            },
        );
        Ok(())
    }

    pub fn record_raw(&mut self, event: RawVictimEvent) -> Result<(), &'static str> {
        let disposition = self.record_raw_with_disposition(event)?;
        if disposition == RawRecordDisposition::RecordedWithLoss {
            self.note_loss();
        }
        Ok(())
    }

    pub fn record_raw_with_disposition(
        &mut self,
        event: RawVictimEvent,
    ) -> Result<RawRecordDisposition, &'static str> {
        if event.tid == 0
            || event.tgid == 0
            || event.starttime_ticks == 0
            || event.event_boot_ns == 0
            || matches!(event.cgroup_v2_id, Some(0))
        {
            self.note_loss();
            return Err("raw victim event identity must be exact and non-zero");
        }
        if self.raw.contains(&event) {
            return Ok(RawRecordDisposition::Duplicate);
        }

        self.raw_watermark_boot_ns = self.raw_watermark_boot_ns.max(event.event_boot_ns);
        let cutoff = self.raw_watermark_boot_ns.saturating_sub(MAX_RAW_AGE_NS);
        let before = self.raw.len();
        self.raw.retain(|existing| existing.event_boot_ns >= cutoff);
        let mut lost = self.raw.len() != before;

        if event.event_boot_ns < cutoff {
            return Ok(RawRecordDisposition::RecordedWithLoss);
        }

        if self.raw.len() >= MAX_RAW_VICTIM_EVENTS {
            if let Some((oldest, _)) = self.raw.iter().enumerate().min_by_key(|(_, existing)| {
                (
                    existing.event_boot_ns,
                    existing.tgid,
                    existing.tid,
                    existing.starttime_ticks,
                )
            }) {
                self.raw.remove(oldest);
                lost = true;
            }
        }
        self.raw.push(event);

        Ok(if lost {
            RawRecordDisposition::RecordedWithLoss
        } else {
            RawRecordDisposition::Recorded
        })
    }

    pub fn note_loss(&mut self) {
        self.store_loss_epoch = self.store_loss_epoch.saturating_add(1);
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

        if loss_epoch != realization.start_external_loss_epoch
            || self.store_loss_epoch != realization.start_store_loss_epoch
        {
            let evidence = FinalizedEvidence {
                guest_boot_id: realization.guest_boot_id,
                realization_started_boot_ns: realization.started_boot_ns,
                outcome_observed_boot_ns,
                main: realization.main,
                expected_cgroup_v2_id: realization.expected_cgroup_v2_id,
                poisoned: true,
                victims: Vec::new(),
            };
            self.insert_finalized(token, evidence.clone());
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
            realization_started_boot_ns: realization.started_boot_ns,
            outcome_observed_boot_ns,
            main: realization.main,
            expected_cgroup_v2_id: realization.expected_cgroup_v2_id,
            poisoned,
            victims,
        };
        self.insert_finalized(token, evidence.clone());
        Ok(Some(evidence))
    }

    fn insert_finalized(&mut self, token: RealizationToken, evidence: FinalizedEvidence) {
        self.finalized_watermark_boot_ns = self
            .finalized_watermark_boot_ns
            .max(evidence.outcome_observed_boot_ns);
        let cutoff = self
            .finalized_watermark_boot_ns
            .saturating_sub(MAX_FINALIZED_AGE_NS);
        self.finalized
            .retain(|_, existing| existing.outcome_observed_boot_ns >= cutoff);

        if !self.finalized.contains_key(&token)
            && self.finalized.len() >= MAX_FINALIZED_REALIZATIONS
        {
            let oldest = self
                .finalized
                .iter()
                .min_by_key(|(existing_token, existing)| {
                    (existing.outcome_observed_boot_ns, existing_token.0)
                })
                .map(|(existing_token, _)| *existing_token);
            if let Some(oldest) = oldest {
                self.finalized.remove(&oldest);
            }
        }
        self.finalized.insert(token, evidence);
    }

    pub fn finalized(&self, token: &RealizationToken) -> Option<FinalizedEvidence> {
        self.finalized.get(token).cloned()
    }

    #[cfg(test)]
    pub fn raw_len(&self) -> usize {
        self.raw.len()
    }

    #[cfg(test)]
    pub fn finalized_len(&self) -> usize {
        self.finalized.len()
    }
}

pub fn parse_process_cgroup_v2_path(raw: &str) -> Result<Option<String>, String> {
    let mut seen_unified = false;
    let mut result = None;
    for line in raw.lines() {
        if line.is_empty() {
            continue;
        }
        let mut parts = line.splitn(3, ':');
        let hierarchy = parts.next().unwrap_or_default();
        let controllers = parts
            .next()
            .ok_or_else(|| "guest process cgroup record is malformed".to_string())?;
        let path = parts
            .next()
            .ok_or_else(|| "guest process cgroup record is malformed".to_string())?;
        if hierarchy == "0" && controllers.is_empty() {
            if seen_unified {
                return Err("guest process has ambiguous cgroup-v2 membership".to_string());
            }
            seen_unified = true;
            if path == "/" {
                result = None;
            } else {
                validate_cgroup_path(path)?;
                result = Some(path.to_string());
            }
        }
    }
    Ok(result)
}

fn validate_cgroup_path(path: &str) -> Result<(), String> {
    if !path.starts_with('/') || path == "/" || path.contains('\0') {
        return Err("guest cgroup-v2 path is not a non-root absolute path".to_string());
    }
    if path.ends_with('/') || path.contains("//") {
        return Err("guest cgroup-v2 path is not canonical".to_string());
    }
    for component in path.split('/').skip(1) {
        if component.is_empty() || component == "." || component == ".." {
            return Err("guest cgroup-v2 path is not canonical".to_string());
        }
    }
    Ok(())
}

pub fn parse_cgroup2_mount(raw: &str) -> Result<String, String> {
    let mut mount = None;
    for line in raw.lines() {
        let Some((pre, post)) = line.split_once(" - ") else {
            continue;
        };
        let post_fields: Vec<&str> = post.split_whitespace().collect();
        if post_fields.first().copied() != Some("cgroup2") {
            continue;
        }
        let pre_fields: Vec<&str> = pre.split_whitespace().collect();
        if pre_fields.len() < 5 {
            return Err("malformed cgroup2 mountinfo record".to_string());
        }
        let candidate = unescape_mountinfo_path(pre_fields[4])?;
        if !candidate.starts_with('/') || candidate.contains("//") {
            return Err("cgroup2 mount point is not canonical".to_string());
        }
        match &mount {
            Some(existing) if existing != &candidate => {
                return Err("multiple cgroup2 mounts are ambiguous".to_string())
            }
            Some(_) => {}
            None => mount = Some(candidate),
        }
    }
    mount.ok_or_else(|| "cgroup2 mount is unavailable".to_string())
}

fn unescape_mountinfo_path(raw: &str) -> Result<String, String> {
    let value = raw
        .replace("\\040", " ")
        .replace("\\011", "\t")
        .replace("\\012", "\n")
        .replace("\\134", "\\");
    if value.contains('\0') || value.contains('\n') || value.contains('\t') {
        return Err("cgroup2 mount point contains unsafe characters".to_string());
    }
    Ok(value)
}

pub fn decode_cgroup_v2_handle(raw: &[u8]) -> Result<u64, String> {
    let bytes: [u8; 8] = raw.try_into().map_err(|_| {
        format!(
            "cgroup-v2 handle must be exactly 8 bytes, got {}",
            raw.len()
        )
    })?;
    let id = u64::from_ne_bytes(bytes);
    if id == 0 {
        return Err("cgroup-v2 handle resolved zero identity".to_string());
    }
    Ok(id)
}

#[repr(C)]
struct ExactFileHandle {
    handle_bytes: u32,
    handle_type: core::ffi::c_int,
    bytes: [u8; 8],
}

unsafe extern "C" {
    fn name_to_handle_at(
        dirfd: core::ffi::c_int,
        pathname: *const core::ffi::c_char,
        handle: *mut ExactFileHandle,
        mount_id: *mut core::ffi::c_int,
        flags: core::ffi::c_int,
    ) -> core::ffi::c_int;
}

const AT_FDCWD: core::ffi::c_int = -100;

fn resolve_cgroup_v2_id(path: &str) -> Result<u64, String> {
    validate_cgroup_path(path)?;
    let mountinfo = fs::read_to_string("/proc/self/mountinfo")
        .map_err(|e| format!("read guest mountinfo failed: {}", e))?;
    let mount = parse_cgroup2_mount(&mountinfo)?;
    let full = format!(
        "{}/{}",
        mount.trim_end_matches('/'),
        path.trim_start_matches('/')
    );
    let c_path = std::ffi::CString::new(full.as_bytes())
        .map_err(|_| "guest cgroup-v2 filesystem path contains NUL".to_string())?;
    let mut handle = ExactFileHandle {
        handle_bytes: 8,
        handle_type: 0,
        bytes: [0u8; 8],
    };
    let mut mount_id = 0;
    let rc = unsafe { name_to_handle_at(AT_FDCWD, c_path.as_ptr(), &mut handle, &mut mount_id, 0) };
    if rc != 0 {
        return Err(format!(
            "name_to_handle_at for guest cgroup-v2 identity failed: {}",
            std::io::Error::last_os_error()
        ));
    }
    if handle.handle_bytes != 8 {
        return Err(format!(
            "guest cgroup-v2 handle payload must be exactly 8 bytes, got {}",
            handle.handle_bytes
        ));
    }
    decode_cgroup_v2_handle(&handle.bytes)
}

pub fn read_process_cgroup_v2_id(pid: i32) -> Result<Option<u64>, String> {
    if pid <= 0 {
        return Err("guest process pid must be positive".to_string());
    }
    let proc_path = format!("/proc/{}/cgroup", pid);
    let first_raw = fs::read_to_string(&proc_path)
        .map_err(|e| format!("read guest process cgroup failed: {}", e))?;
    let first = parse_process_cgroup_v2_path(&first_raw)?;
    let Some(first_path) = first else {
        return Ok(None);
    };
    let id = resolve_cgroup_v2_id(&first_path)?;
    let second_raw = fs::read_to_string(&proc_path)
        .map_err(|e| format!("re-read guest process cgroup failed: {}", e))?;
    let second = parse_process_cgroup_v2_path(&second_raw)?;
    if second.as_deref() != Some(first_path.as_str()) {
        return Err(
            "guest process cgroup-v2 membership changed during identity capture".to_string(),
        );
    }
    Ok(Some(id))
}

#[repr(C)]
struct KernelTimespec {
    tv_sec: core::ffi::c_long,
    tv_nsec: core::ffi::c_long,
}

unsafe extern "C" {
    fn clock_gettime(clock_id: core::ffi::c_int, ts: *mut KernelTimespec) -> core::ffi::c_int;
}

const CLOCK_BOOTTIME: core::ffi::c_int = 7;
const NANOSECONDS_PER_SECOND: i128 = 1_000_000_000;
const USER_HZ_100_TICK_NS: u64 = 10_000_000;

pub fn current_boottime_ns() -> Result<u64, String> {
    let mut ts = KernelTimespec {
        tv_sec: 0,
        tv_nsec: 0,
    };
    let rc = unsafe { clock_gettime(CLOCK_BOOTTIME, &mut ts) };
    if rc != 0 {
        return Err(format!(
            "clock_gettime(CLOCK_BOOTTIME) failed: {}",
            std::io::Error::last_os_error()
        ));
    }
    if ts.tv_sec < 0 || ts.tv_nsec < 0 {
        return Err("CLOCK_BOOTTIME returned a negative value".to_string());
    }
    let sec =
        u64::try_from(ts.tv_sec).map_err(|_| "CLOCK_BOOTTIME seconds overflow".to_string())?;
    let nsec =
        u64::try_from(ts.tv_nsec).map_err(|_| "CLOCK_BOOTTIME nanoseconds overflow".to_string())?;
    sec.checked_mul(1_000_000_000)
        .and_then(|v| v.checked_add(nsec))
        .ok_or_else(|| "CLOCK_BOOTTIME nanoseconds overflow".to_string())
}

pub fn parse_time_namespace_inode(value: &str) -> Result<u64, String> {
    let digits = value
        .strip_prefix("time:[")
        .and_then(|rest| rest.strip_suffix(']'))
        .ok_or_else(|| "time namespace handle is not canonical".to_string())?;
    if digits.is_empty() || !digits.bytes().all(|byte| byte.is_ascii_digit()) {
        return Err("time namespace inode is invalid".to_string());
    }
    let inode = digits
        .parse::<u64>()
        .map_err(|_| "time namespace inode overflow".to_string())?;
    if inode == 0 || format!("time:[{}]", inode) != value {
        return Err("time namespace handle is not canonical".to_string());
    }
    Ok(inode)
}

pub fn parse_timens_boottime_offset_ns(value: &str) -> Result<i128, String> {
    let mut boottime_offset = None;

    for line in value.lines() {
        if line.trim().is_empty() {
            return Err("time namespace offsets contain an empty record".to_string());
        }
        let mut fields = line.split_ascii_whitespace();
        let clock = fields
            .next()
            .ok_or_else(|| "time namespace offset record is missing clock id".to_string())?;
        let seconds_raw = fields
            .next()
            .ok_or_else(|| "time namespace offset record is missing seconds".to_string())?;
        let nanoseconds_raw = fields
            .next()
            .ok_or_else(|| "time namespace offset record is missing nanoseconds".to_string())?;
        if fields.next().is_some() {
            return Err("time namespace offset record has extra fields".to_string());
        }
        if clock != "boottime" {
            continue;
        }
        if boottime_offset.is_some() {
            return Err("time namespace offsets contain duplicate boottime records".to_string());
        }

        let seconds = seconds_raw
            .parse::<i128>()
            .map_err(|_| "time namespace boottime seconds are invalid".to_string())?;
        let nanoseconds = nanoseconds_raw
            .parse::<i128>()
            .map_err(|_| "time namespace boottime nanoseconds are invalid".to_string())?;
        if !(0..NANOSECONDS_PER_SECOND).contains(&nanoseconds) {
            return Err("time namespace boottime nanoseconds are out of range".to_string());
        }

        let offset = seconds
            .checked_mul(NANOSECONDS_PER_SECOND)
            .and_then(|base| base.checked_add(nanoseconds))
            .ok_or_else(|| "time namespace boottime offset overflow".to_string())?;
        boottime_offset = Some(offset);
    }

    boottime_offset.ok_or_else(|| "time namespace offsets are missing boottime".to_string())
}

pub fn authoritative_timens_boottime_offset(
    namespace_before: Option<&str>,
    namespace_after: Option<&str>,
    offsets: Option<&str>,
) -> Result<i128, String> {
    match (namespace_before, namespace_after, offsets) {
        (None, None, None) => Ok(0),
        (Some(before), Some(after), Some(offsets)) => {
            let before_inode = parse_time_namespace_inode(before)?;
            let after_inode = parse_time_namespace_inode(after)?;
            if before_inode != after_inode {
                return Err("time namespace changed during boottime authority capture".to_string());
            }
            parse_timens_boottime_offset_ns(offsets)
        }
        (None, None, Some(_)) => {
            Err("time namespace offsets are exposed without namespace authority".to_string())
        }
        (Some(_), Some(_), None) => {
            Err("time namespace is exposed without boottime offsets".to_string())
        }
        _ => Err("time namespace authority capture is incomplete".to_string()),
    }
}

pub fn read_current_timens_boottime_offset() -> Result<i128, String> {
    fn read_namespace() -> Result<Option<String>, String> {
        match fs::read_link("/proc/self/ns/time") {
            Ok(target) => target
                .into_os_string()
                .into_string()
                .map(Some)
                .map_err(|_| "current time namespace handle is not valid UTF-8".to_string()),
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(err) => Err(format!("read current time namespace failed: {}", err)),
        }
    }

    fn read_offsets() -> Result<Option<String>, String> {
        match fs::read_to_string("/proc/self/timens_offsets") {
            Ok(value) => Ok(Some(value)),
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(err) => Err(format!(
                "read current time namespace offsets failed: {}",
                err
            )),
        }
    }

    let namespace_before = read_namespace()?;
    let offsets = read_offsets()?;
    let namespace_after = read_namespace()?;
    authoritative_timens_boottime_offset(
        namespace_before.as_deref(),
        namespace_after.as_deref(),
        offsets.as_deref(),
    )
}

pub fn boottime_ns_to_visible_boot_ns(
    boottime_ns: u64,
    boottime_offset_ns: i128,
) -> Result<u64, String> {
    let visible_ns = i128::from(boottime_ns)
        .checked_add(boottime_offset_ns)
        .ok_or_else(|| "visible boottime overflow".to_string())?;
    if visible_ns < 0 || visible_ns > i128::from(u64::MAX) {
        return Err("visible boottime is outside the supported range".to_string());
    }
    u64::try_from(visible_ns).map_err(|_| "visible boottime conversion failed".to_string())
}

pub fn start_boottime_ns_to_starttime_ticks(
    start_boottime_ns: u64,
    boottime_offset_ns: i128,
) -> Result<u64, String> {
    Ok(
        boottime_ns_to_visible_boot_ns(start_boottime_ns, boottime_offset_ns)?
            / USER_HZ_100_TICK_NS,
    )
}

pub fn read_guest_boot_id() -> Result<String, String> {
    let value = fs::read_to_string("/proc/sys/kernel/random/boot_id")
        .map_err(|e| format!("read guest boot id failed: {}", e))?;
    let value = value.trim().to_string();
    if !canonical_boot_id(&value) {
        return Err("guest boot id is not canonical".to_string());
    }
    Ok(value)
}

pub fn read_process_identity(pid: i32) -> Result<GuestProcessIdentity, String> {
    if pid <= 0 {
        return Err("guest process pid must be positive".to_string());
    }
    let stat = fs::read_to_string(format!("/proc/{}/stat", pid))
        .map_err(|e| format!("read guest process stat failed: {}", e))?;
    let close = stat
        .rfind(')')
        .ok_or_else(|| "guest process stat is missing comm terminator".to_string())?;
    let tail = stat
        .get(close + 1..)
        .ok_or_else(|| "guest process stat tail is missing".to_string())?;
    let fields: Vec<&str> = tail.split_whitespace().collect();
    if fields.len() <= 19 {
        return Err("guest process stat is missing field 22".to_string());
    }
    let starttime_ticks = fields[19]
        .parse::<u64>()
        .map_err(|_| "guest process starttime is invalid".to_string())?;
    if starttime_ticks == 0 {
        return Err("guest process starttime must be non-zero".to_string());
    }
    Ok(GuestProcessIdentity {
        tgid: u32::try_from(pid).map_err(|_| "guest process pid overflow".to_string())?,
        starttime_ticks,
    })
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
