// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

//! Dependency-free Wave 21 raw-tracepoint loader primitives.
//!
//! This module deliberately contains no ELF/object dependency. It parses the
//! running kernel BTF shape needed by oom:mark_victim and assembles the exact
//! BPF instruction stream at runtime. The syscall/FD lifecycle is owned by
//! `oom_victim_bpf`; keeping this layer pure makes verifier inputs testable by
//! standalone `rustc` contracts.

use std::collections::HashMap;

pub const EVENT_VERSION_V1: u32 = 1;
pub const RAW_VICTIM_RECORD_SIZE: i32 = 40;
pub const RINGBUF_BUSY_BIT: u32 = 1 << 31;
pub const RINGBUF_DISCARD_BIT: u32 = 1 << 30;
const RINGBUF_LENGTH_MASK: u32 = !(RINGBUF_BUSY_BIT | RINGBUF_DISCARD_BIT);
const RINGBUF_HEADER_SIZE: usize = 8;
const RINGBUF_ALIGNMENT: usize = 8;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum RingRecordHeaderDisposition {
    Busy,
    Discarded { span: usize },
    Ready { payload_len: usize, span: usize },
}

pub fn classify_ring_record_header(raw_len: u32) -> Result<RingRecordHeaderDisposition, String> {
    let payload_len = (raw_len & RINGBUF_LENGTH_MASK) as usize;
    if payload_len != RAW_VICTIM_RECORD_SIZE as usize {
        return Err(format!(
            "Wave21 ring record payload has invalid framing: got {} bytes, want {}",
            payload_len, RAW_VICTIM_RECORD_SIZE
        ));
    }

    let unaligned = RINGBUF_HEADER_SIZE
        .checked_add(payload_len)
        .ok_or_else(|| "Wave21 ring record span overflow".to_string())?;
    let span = unaligned
        .checked_add(RINGBUF_ALIGNMENT - 1)
        .map(|value| value & !(RINGBUF_ALIGNMENT - 1))
        .ok_or_else(|| "Wave21 ring record alignment overflow".to_string())?;

    if raw_len & RINGBUF_BUSY_BIT != 0 {
        return Ok(RingRecordHeaderDisposition::Busy);
    }
    if raw_len & RINGBUF_DISCARD_BIT != 0 {
        return Ok(RingRecordHeaderDisposition::Discarded { span });
    }
    Ok(RingRecordHeaderDisposition::Ready { payload_len, span })
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct DecodedRawVictimRecord {
    pub tid: u32,
    pub tgid: u32,
    pub start_boottime_ns: u64,
    pub event_boot_ns: u64,
    pub cgroup_v2_id: Option<u64>,
}

pub fn decode_raw_victim_record(raw: &[u8]) -> Result<DecodedRawVictimRecord, String> {
    if raw.len() != RAW_VICTIM_RECORD_SIZE as usize {
        return Err(format!(
            "Wave21 raw victim record has invalid framing: got {} bytes, want {}",
            raw.len(),
            RAW_VICTIM_RECORD_SIZE
        ));
    }

    let version = u32::from_ne_bytes(
        raw[0..4]
            .try_into()
            .map_err(|_| "Wave21 raw victim version framing failed".to_string())?,
    );
    let flags = u32::from_ne_bytes(
        raw[4..8]
            .try_into()
            .map_err(|_| "Wave21 raw victim flags framing failed".to_string())?,
    );
    let tid = u32::from_ne_bytes(
        raw[8..12]
            .try_into()
            .map_err(|_| "Wave21 raw victim tid framing failed".to_string())?,
    );
    let tgid = u32::from_ne_bytes(
        raw[12..16]
            .try_into()
            .map_err(|_| "Wave21 raw victim tgid framing failed".to_string())?,
    );
    let start_boottime_ns = u64::from_ne_bytes(
        raw[16..24]
            .try_into()
            .map_err(|_| "Wave21 raw victim start time framing failed".to_string())?,
    );
    let event_boot_ns = u64::from_ne_bytes(
        raw[24..32]
            .try_into()
            .map_err(|_| "Wave21 raw victim event time framing failed".to_string())?,
    );
    let cgroup_v2_id = u64::from_ne_bytes(
        raw[32..40]
            .try_into()
            .map_err(|_| "Wave21 raw victim cgroup framing failed".to_string())?,
    );

    if version != EVENT_VERSION_V1 {
        return Err(format!(
            "Wave21 raw victim record version {} is unsupported",
            version
        ));
    }
    if flags != 0 {
        return Err("Wave21 raw victim record uses reserved flags".to_string());
    }
    if tid == 0 || tgid == 0 {
        return Err("Wave21 raw victim record has zero task identity".to_string());
    }
    if start_boottime_ns == 0 {
        return Err("Wave21 raw victim record has zero process lifetime".to_string());
    }
    if start_boottime_ns > event_boot_ns {
        return Err("Wave21 raw victim record predates its process lifetime".to_string());
    }

    Ok(DecodedRawVictimRecord {
        tid,
        tgid,
        start_boottime_ns,
        event_boot_ns,
        cgroup_v2_id: (cgroup_v2_id != 0).then_some(cgroup_v2_id),
    })
}

const BTF_MAGIC: u16 = 0xeb9f;
const BTF_HEADER_LEN: usize = 24;
const BTF_KIND_INT: u32 = 1;
const BTF_KIND_PTR: u32 = 2;
const BTF_KIND_ARRAY: u32 = 3;
const BTF_KIND_STRUCT: u32 = 4;
const BTF_KIND_UNION: u32 = 5;
const BTF_KIND_ENUM: u32 = 6;
const BTF_KIND_FWD: u32 = 7;
const BTF_KIND_TYPEDEF: u32 = 8;
const BTF_KIND_VOLATILE: u32 = 9;
const BTF_KIND_CONST: u32 = 10;
const BTF_KIND_RESTRICT: u32 = 11;
const BTF_KIND_FUNC: u32 = 12;
const BTF_KIND_FUNC_PROTO: u32 = 13;
const BTF_KIND_VAR: u32 = 14;
const BTF_KIND_DATASEC: u32 = 15;
const BTF_KIND_FLOAT: u32 = 16;
const BTF_KIND_DECL_TAG: u32 = 17;
const BTF_KIND_TYPE_TAG: u32 = 18;
const BTF_KIND_ENUM64: u32 = 19;

const BTF_INT_SIGNED: u32 = 1;

const BPF_FUNC_MAP_LOOKUP_ELEM: i32 = 1;
const BPF_FUNC_PROBE_READ_KERNEL: i32 = 113;
const BPF_FUNC_KTIME_GET_BOOT_NS: i32 = 125;
const BPF_FUNC_RINGBUF_OUTPUT: i32 = 130;

const BPF_PSEUDO_MAP_FD: u8 = 1;
const BPF_REG_FP: u8 = 10;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CgroupV2Layout {
    pub task_cgroups_offset: i16,
    pub default_cgroup_offset: i16,
    pub kernfs_node_offset: i16,
    pub kernfs_id_offset: i16,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct Layout {
    pub pid_offset: i16,
    pub tgid_offset: i16,
    pub start_boottime_offset: i16,
    pub cgroup_v2: Option<CgroupV2Layout>,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct BpfInsn {
    pub code: u8,
    pub dst_src: u8,
    pub off: i16,
    pub imm: i32,
}

impl BpfInsn {
    fn new(code: u8, dst: u8, src: u8, off: i16, imm: i32) -> Self {
        Self {
            code,
            dst_src: (dst & 0x0f) | ((src & 0x0f) << 4),
            off,
            imm,
        }
    }
}

#[derive(Clone, Debug)]
struct BtfMember {
    name: String,
    type_id: u32,
    bit_offset: u32,
    bitfield_size: u8,
}

#[derive(Clone, Debug)]
enum BtfPayload {
    Int { encoding: u32 },
    Members(Vec<BtfMember>),
    None,
}

#[derive(Clone, Debug)]
struct BtfType {
    kind: u32,
    size_or_type: u32,
    payload: BtfPayload,
}

struct BtfDb {
    types: Vec<Option<BtfType>>,
    names: HashMap<String, u32>,
}

pub fn parse_kernel_btf_layout(raw: &[u8]) -> Result<Layout, String> {
    let db = parse_btf(raw)?;
    let task_id = *db
        .names
        .get("task_struct")
        .ok_or_else(|| "kernel BTF is missing task_struct".to_string())?;
    let task = db.expect_kind(task_id, BTF_KIND_STRUCT, "task_struct")?;

    let pid = db.required_integer_member(task, "pid", 4, Some(BTF_INT_SIGNED))?;
    let tgid = db.required_integer_member(task, "tgid", 4, Some(BTF_INT_SIGNED))?;
    let start = db.required_integer_member(task, "start_boottime", 8, Some(0))?;

    Ok(Layout {
        pid_offset: pid,
        tgid_offset: tgid,
        start_boottime_offset: start,
        cgroup_v2: db.resolve_optional_cgroup(task),
    })
}

fn parse_btf(raw: &[u8]) -> Result<BtfDb, String> {
    if raw.len() < BTF_HEADER_LEN {
        return Err("kernel BTF header is truncated".to_string());
    }
    if le_u16(raw, 0)? != BTF_MAGIC {
        return Err("kernel BTF magic is invalid or unsupported-endian".to_string());
    }
    if raw[2] != 1 {
        return Err(format!("unsupported BTF version {}", raw[2]));
    }

    let hdr_len = le_u32(raw, 4)? as usize;
    let type_off = le_u32(raw, 8)? as usize;
    let type_len = le_u32(raw, 12)? as usize;
    let str_off = le_u32(raw, 16)? as usize;
    let str_len = le_u32(raw, 20)? as usize;
    if hdr_len < BTF_HEADER_LEN || hdr_len > raw.len() {
        return Err("kernel BTF header length is invalid".to_string());
    }

    let type_start = checked_add(hdr_len, type_off, "BTF type offset")?;
    let type_end = checked_add(type_start, type_len, "BTF type length")?;
    let str_start = checked_add(hdr_len, str_off, "BTF string offset")?;
    let str_end = checked_add(str_start, str_len, "BTF string length")?;
    if type_end > raw.len() || str_end > raw.len() || type_start > type_end || str_start > str_end {
        return Err("kernel BTF sections escape input".to_string());
    }
    let strings = &raw[str_start..str_end];
    if strings.first().copied() != Some(0) {
        return Err("kernel BTF string table is not canonical".to_string());
    }

    let mut types: Vec<Option<BtfType>> = vec![None];
    let mut names = HashMap::new();
    let mut cursor = type_start;
    while cursor < type_end {
        if type_end - cursor < 12 {
            return Err("kernel BTF type record is truncated".to_string());
        }
        let name_off = le_u32(raw, cursor)?;
        let info = le_u32(raw, cursor + 4)?;
        let size_or_type = le_u32(raw, cursor + 8)?;
        cursor += 12;

        let kind = (info >> 24) & 0x1f;
        let kind_flag = (info >> 31) != 0;
        let vlen = (info & 0xffff) as usize;
        let name = btf_string(strings, name_off)?;

        let payload = match kind {
            BTF_KIND_INT => {
                require_bytes(cursor, 4, type_end, "BTF int")?;
                let data = le_u32(raw, cursor)?;
                cursor += 4;
                BtfPayload::Int {
                    encoding: (data >> 24) & 0x0f,
                }
            }
            BTF_KIND_STRUCT | BTF_KIND_UNION => {
                let bytes = vlen
                    .checked_mul(12)
                    .ok_or_else(|| "BTF member count overflow".to_string())?;
                require_bytes(cursor, bytes, type_end, "BTF members")?;
                let mut members = Vec::with_capacity(vlen);
                for _ in 0..vlen {
                    let member_name = btf_string(strings, le_u32(raw, cursor)?)?;
                    let type_id = le_u32(raw, cursor + 4)?;
                    let raw_offset = le_u32(raw, cursor + 8)?;
                    cursor += 12;
                    let (bitfield_size, bit_offset) = if kind_flag {
                        ((raw_offset >> 24) as u8, raw_offset & 0x00ff_ffff)
                    } else {
                        (0, raw_offset)
                    };
                    members.push(BtfMember {
                        name: member_name,
                        type_id,
                        bit_offset,
                        bitfield_size,
                    });
                }
                BtfPayload::Members(members)
            }
            BTF_KIND_ARRAY => {
                require_bytes(cursor, 12, type_end, "BTF array")?;
                cursor += 12;
                BtfPayload::None
            }
            BTF_KIND_ENUM => {
                let bytes = vlen
                    .checked_mul(8)
                    .ok_or_else(|| "BTF enum count overflow".to_string())?;
                require_bytes(cursor, bytes, type_end, "BTF enum")?;
                cursor += bytes;
                BtfPayload::None
            }
            BTF_KIND_FUNC_PROTO => {
                let bytes = vlen
                    .checked_mul(8)
                    .ok_or_else(|| "BTF func-proto count overflow".to_string())?;
                require_bytes(cursor, bytes, type_end, "BTF func proto")?;
                cursor += bytes;
                BtfPayload::None
            }
            BTF_KIND_VAR => {
                require_bytes(cursor, 4, type_end, "BTF var")?;
                cursor += 4;
                BtfPayload::None
            }
            BTF_KIND_DATASEC => {
                let bytes = vlen
                    .checked_mul(12)
                    .ok_or_else(|| "BTF datasec count overflow".to_string())?;
                require_bytes(cursor, bytes, type_end, "BTF datasec")?;
                cursor += bytes;
                BtfPayload::None
            }
            BTF_KIND_DECL_TAG => {
                require_bytes(cursor, 4, type_end, "BTF decl tag")?;
                cursor += 4;
                BtfPayload::None
            }
            BTF_KIND_ENUM64 => {
                let bytes = vlen
                    .checked_mul(12)
                    .ok_or_else(|| "BTF enum64 count overflow".to_string())?;
                require_bytes(cursor, bytes, type_end, "BTF enum64")?;
                cursor += bytes;
                BtfPayload::None
            }
            BTF_KIND_PTR | BTF_KIND_FWD | BTF_KIND_TYPEDEF | BTF_KIND_VOLATILE | BTF_KIND_CONST
            | BTF_KIND_RESTRICT | BTF_KIND_FUNC | BTF_KIND_FLOAT | BTF_KIND_TYPE_TAG => {
                BtfPayload::None
            }
            _ => return Err(format!("unsupported kernel BTF kind {}", kind)),
        };

        let id = u32::try_from(types.len()).map_err(|_| "BTF type id overflow".to_string())?;
        if !name.is_empty() && matches!(kind, BTF_KIND_STRUCT | BTF_KIND_UNION) {
            names.entry(name).or_insert(id);
        }
        types.push(Some(BtfType {
            kind,
            size_or_type,
            payload,
        }));
    }
    if cursor != type_end {
        return Err("kernel BTF type section is not exact".to_string());
    }
    Ok(BtfDb { types, names })
}

impl BtfDb {
    fn get(&self, id: u32) -> Result<&BtfType, String> {
        self.types
            .get(id as usize)
            .and_then(|entry| entry.as_ref())
            .ok_or_else(|| format!("kernel BTF references unknown type {}", id))
    }

    fn expect_kind(&self, id: u32, kind: u32, label: &str) -> Result<&BtfType, String> {
        let ty = self.get(id)?;
        if ty.kind != kind {
            return Err(format!(
                "kernel BTF {} has incompatible kind {}",
                label, ty.kind
            ));
        }
        Ok(ty)
    }

    fn underlying_id(&self, mut id: u32) -> Result<u32, String> {
        for _ in 0..64 {
            let ty = self.get(id)?;
            match ty.kind {
                BTF_KIND_TYPEDEF | BTF_KIND_VOLATILE | BTF_KIND_CONST | BTF_KIND_RESTRICT
                | BTF_KIND_TYPE_TAG => {
                    id = ty.size_or_type;
                }
                _ => return Ok(id),
            }
        }
        Err("kernel BTF modifier chain is cyclic or too deep".to_string())
    }

    fn pointer_target_struct(&self, id: u32) -> Result<&BtfType, String> {
        let ptr_id = self.underlying_id(id)?;
        let ptr = self.expect_kind(ptr_id, BTF_KIND_PTR, "pointer")?;
        let target_id = self.underlying_id(ptr.size_or_type)?;
        self.expect_kind(target_id, BTF_KIND_STRUCT, "pointer target")
    }

    fn named_member<'a>(&self, ty: &'a BtfType, name: &str) -> Option<&'a BtfMember> {
        match &ty.payload {
            BtfPayload::Members(members) => members.iter().find(|member| member.name == name),
            _ => None,
        }
    }

    fn member_offset(&self, member: &BtfMember) -> Result<i16, String> {
        if member.bitfield_size != 0 || member.bit_offset % 8 != 0 {
            return Err(format!(
                "kernel BTF member {} is not a byte-aligned scalar",
                member.name
            ));
        }
        let bytes = member.bit_offset / 8;
        i16::try_from(bytes)
            .map_err(|_| format!("kernel BTF member {} offset is too large", member.name))
    }

    fn required_integer_member(
        &self,
        owner: &BtfType,
        name: &str,
        size: u32,
        encoding: Option<u32>,
    ) -> Result<i16, String> {
        let member = self
            .named_member(owner, name)
            .ok_or_else(|| format!("kernel BTF is missing required member {}", name))?;
        let id = self.underlying_id(member.type_id)?;
        let ty = self.expect_kind(id, BTF_KIND_INT, name)?;
        if ty.size_or_type != size {
            return Err(format!(
                "kernel BTF member {} has size {}, expected {}",
                name, ty.size_or_type, size
            ));
        }
        let actual_encoding = match &ty.payload {
            BtfPayload::Int { encoding } => *encoding,
            _ => {
                return Err(format!(
                    "kernel BTF member {} has malformed integer metadata",
                    name
                ))
            }
        };
        if let Some(expected) = encoding {
            if actual_encoding != expected {
                return Err(format!(
                    "kernel BTF member {} has incompatible integer encoding",
                    name
                ));
            }
        }
        self.member_offset(member)
    }

    fn resolve_optional_cgroup(&self, task: &BtfType) -> Option<CgroupV2Layout> {
        (|| {
            let cgroups = self.named_member(task, "cgroups")?;
            let task_cgroups_offset = self.member_offset(cgroups).ok()?;
            let css = self.pointer_target_struct(cgroups.type_id).ok()?;

            let dfl = self.named_member(css, "dfl_cgrp")?;
            let default_cgroup_offset = self.member_offset(dfl).ok()?;
            let cgroup = self.pointer_target_struct(dfl.type_id).ok()?;

            let kn = self.named_member(cgroup, "kn")?;
            let kernfs_node_offset = self.member_offset(kn).ok()?;
            let kernfs = self.pointer_target_struct(kn.type_id).ok()?;

            let id_member = self.named_member(kernfs, "id")?;
            let mut kernfs_id_offset = i32::from(self.member_offset(id_member).ok()?);
            let id_type = self.get(self.underlying_id(id_member.type_id).ok()?).ok()?;
            match id_type.kind {
                BTF_KIND_INT => {
                    if id_type.size_or_type != 8 {
                        return None;
                    }
                    if !matches!(&id_type.payload, BtfPayload::Int { encoding: 0 }) {
                        return None;
                    }
                }
                BTF_KIND_UNION => {
                    let inner = self.named_member(id_type, "id")?;
                    kernfs_id_offset += i32::from(self.member_offset(inner).ok()?);
                    let inner_type = self.get(self.underlying_id(inner.type_id).ok()?).ok()?;
                    if inner_type.kind != BTF_KIND_INT
                        || inner_type.size_or_type != 8
                        || !matches!(&inner_type.payload, BtfPayload::Int { encoding: 0 })
                    {
                        return None;
                    }
                }
                _ => return None,
            }
            Some(CgroupV2Layout {
                task_cgroups_offset,
                default_cgroup_offset,
                kernfs_node_offset,
                kernfs_id_offset: i16::try_from(kernfs_id_offset).ok()?,
            })
        })()
    }
}

pub fn build_raw_tracepoint_program(
    layout: Layout,
    event_map_fd: i32,
    loss_map_fd: i32,
) -> Result<Vec<BpfInsn>, String> {
    if event_map_fd < 0 || loss_map_fd < 0 {
        return Err("Wave21 BPF map descriptors must be non-negative".to_string());
    }
    for (name, offset) in [
        ("pid", layout.pid_offset),
        ("tgid", layout.tgid_offset),
        ("start_boottime", layout.start_boottime_offset),
    ] {
        if offset < 0 {
            return Err(format!("Wave21 BPF {} offset must be non-negative", name));
        }
    }

    let mut p = Program::default();
    // R6 = raw tracepoint ctx args[0] (victim task_struct *).
    p.emit(ldx_dw(6, 1, 0));
    p.jump_imm(0x15, 6, 0, "exit");

    // Fixed record starts at FP[-64]: version, flags, tid, tgid,
    // start_boottime, event_boottime, cgroup-v2 id.
    p.emit(st_imm_w(BPF_REG_FP, -64, EVENT_VERSION_V1 as i32));
    p.emit(st_imm_w(BPF_REG_FP, -60, 0));
    p.emit(st_imm_dw(BPF_REG_FP, -32, 0));

    emit_probe_read(&mut p, -56, 4, 6, layout.pid_offset, "loss");
    emit_probe_read(&mut p, -52, 4, 6, layout.tgid_offset, "loss");
    emit_probe_read(&mut p, -48, 8, 6, layout.start_boottime_offset, "loss");

    if let Some(cg) = layout.cgroup_v2 {
        // task_struct.cgroups -> temporary FP[-72]
        emit_probe_read(&mut p, -72, 8, 6, cg.task_cgroups_offset, "event_time");
        p.emit(ldx_dw(7, BPF_REG_FP, -72));
        p.jump_imm(0x15, 7, 0, "event_time");

        // css_set.dfl_cgrp
        emit_probe_read(&mut p, -72, 8, 7, cg.default_cgroup_offset, "event_time");
        p.emit(ldx_dw(7, BPF_REG_FP, -72));
        p.jump_imm(0x15, 7, 0, "event_time");

        // cgroup.kn
        emit_probe_read(&mut p, -72, 8, 7, cg.kernfs_node_offset, "event_time");
        p.emit(ldx_dw(7, BPF_REG_FP, -72));
        p.jump_imm(0x15, 7, 0, "event_time");

        // kernfs_node.id exact u64. Slot remains zero if the optional chain
        // cannot be read, which means member correlation is unavailable.
        emit_probe_read(&mut p, -32, 8, 7, cg.kernfs_id_offset, "cgroup_unknown");
        p.jump("event_time");
        p.label("cgroup_unknown");
        p.emit(st_imm_dw(BPF_REG_FP, -32, 0));
    }

    p.label("event_time");
    p.call(BPF_FUNC_KTIME_GET_BOOT_NS);
    p.emit(stx_dw(BPF_REG_FP, 0, -40));

    // bpf_ringbuf_output(events, &record, 40, 0)
    p.ld_map_fd(1, event_map_fd);
    p.emit(mov64_reg(2, BPF_REG_FP));
    p.emit(add64_imm(2, -64));
    p.emit(mov64_imm(3, RAW_VICTIM_RECORD_SIZE));
    p.emit(mov64_imm(4, 0));
    p.call(BPF_FUNC_RINGBUF_OUTPUT);
    p.jump_imm(0x15, 0, 0, "exit");

    // A ring-buffer output failure is evidence loss. Increment a one-entry
    // array-map epoch with atomic XADD; failure to resolve the map entry is
    // itself fail-closed because userspace also treats reader errors as loss.
    p.label("loss");
    p.emit(st_imm_w(BPF_REG_FP, -76, 0));
    p.ld_map_fd(1, loss_map_fd);
    p.emit(mov64_reg(2, BPF_REG_FP));
    p.emit(add64_imm(2, -76));
    p.call(BPF_FUNC_MAP_LOOKUP_ELEM);
    p.jump_imm(0x15, 0, 0, "exit");
    p.emit(mov64_imm(1, 1));
    p.emit(BpfInsn::new(0xdb, 0, 1, 0, 0)); // lock *(u64 *)R0 += R1

    p.label("exit");
    p.emit(mov64_imm(0, 0));
    p.emit(BpfInsn::new(0x95, 0, 0, 0, 0));
    p.finish()
}

fn emit_probe_read(
    p: &mut Program,
    dst_stack: i16,
    size: i32,
    base_reg: u8,
    member_offset: i16,
    failure: &'static str,
) {
    p.emit(mov64_reg(1, BPF_REG_FP));
    p.emit(add64_imm(1, i32::from(dst_stack)));
    p.emit(mov64_imm(2, size));
    p.emit(mov64_reg(3, base_reg));
    p.emit(add64_imm(3, i32::from(member_offset)));
    p.call(BPF_FUNC_PROBE_READ_KERNEL);
    p.jump_imm(0x55, 0, 0, failure); // if R0 != 0
}

#[derive(Default)]
struct Program {
    insns: Vec<BpfInsn>,
    labels: HashMap<&'static str, usize>,
    fixups: Vec<(usize, &'static str)>,
}

impl Program {
    fn emit(&mut self, insn: BpfInsn) {
        self.insns.push(insn);
    }

    fn label(&mut self, name: &'static str) {
        self.labels.insert(name, self.insns.len());
    }

    fn call(&mut self, helper: i32) {
        self.emit(BpfInsn::new(0x85, 0, 0, 0, helper));
    }

    fn jump_imm(&mut self, code: u8, dst: u8, imm: i32, target: &'static str) {
        let index = self.insns.len();
        self.emit(BpfInsn::new(code, dst, 0, 0, imm));
        self.fixups.push((index, target));
    }

    fn jump(&mut self, target: &'static str) {
        let index = self.insns.len();
        self.emit(BpfInsn::new(0x05, 0, 0, 0, 0));
        self.fixups.push((index, target));
    }

    fn ld_map_fd(&mut self, dst: u8, fd: i32) {
        self.emit(BpfInsn::new(0x18, dst, BPF_PSEUDO_MAP_FD, 0, fd));
        self.emit(BpfInsn::default());
    }

    fn finish(mut self) -> Result<Vec<BpfInsn>, String> {
        for (index, target) in self.fixups {
            let target_index = *self
                .labels
                .get(target)
                .ok_or_else(|| format!("Wave21 BPF label {} is unresolved", target))?;
            let delta = target_index as isize - index as isize - 1;
            self.insns[index].off = i16::try_from(delta)
                .map_err(|_| format!("Wave21 BPF jump to {} is out of range", target))?;
        }
        Ok(self.insns)
    }
}

fn mov64_imm(dst: u8, imm: i32) -> BpfInsn {
    BpfInsn::new(0xb7, dst, 0, 0, imm)
}
fn mov64_reg(dst: u8, src: u8) -> BpfInsn {
    BpfInsn::new(0xbf, dst, src, 0, 0)
}
fn add64_imm(dst: u8, imm: i32) -> BpfInsn {
    BpfInsn::new(0x07, dst, 0, 0, imm)
}
fn ldx_dw(dst: u8, src: u8, off: i16) -> BpfInsn {
    BpfInsn::new(0x79, dst, src, off, 0)
}
fn stx_dw(dst: u8, src: u8, off: i16) -> BpfInsn {
    BpfInsn::new(0x7b, dst, src, off, 0)
}
fn st_imm_w(dst: u8, off: i16, imm: i32) -> BpfInsn {
    BpfInsn::new(0x62, dst, 0, off, imm)
}
fn st_imm_dw(dst: u8, off: i16, imm: i32) -> BpfInsn {
    BpfInsn::new(0x7a, dst, 0, off, imm)
}

fn checked_add(a: usize, b: usize, label: &str) -> Result<usize, String> {
    a.checked_add(b)
        .ok_or_else(|| format!("{} overflows", label))
}

fn require_bytes(cursor: usize, bytes: usize, end: usize, label: &str) -> Result<(), String> {
    if cursor
        .checked_add(bytes)
        .filter(|value| *value <= end)
        .is_none()
    {
        return Err(format!("{} is truncated", label));
    }
    Ok(())
}

fn btf_string(strings: &[u8], offset: u32) -> Result<String, String> {
    let start = usize::try_from(offset).map_err(|_| "BTF string offset overflow".to_string())?;
    if start >= strings.len() {
        return Err("BTF string offset is out of range".to_string());
    }
    let tail = &strings[start..];
    let end = tail
        .iter()
        .position(|byte| *byte == 0)
        .ok_or_else(|| "BTF string is unterminated".to_string())?;
    std::str::from_utf8(&tail[..end])
        .map(str::to_owned)
        .map_err(|_| "BTF string is not UTF-8".to_string())
}

fn le_u16(raw: &[u8], offset: usize) -> Result<u16, String> {
    let bytes = raw
        .get(offset..offset + 2)
        .ok_or_else(|| "input is truncated".to_string())?;
    Ok(u16::from_le_bytes([bytes[0], bytes[1]]))
}

fn le_u32(raw: &[u8], offset: usize) -> Result<u32, String> {
    let bytes = raw
        .get(offset..offset + 4)
        .ok_or_else(|| "input is truncated".to_string())?;
    Ok(u32::from_le_bytes([bytes[0], bytes[1], bytes[2], bytes[3]]))
}
