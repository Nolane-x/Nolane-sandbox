// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

//! Minimal object-free Linux BPF runtime for the Wave21 guest OOM collector.
//!
//! Keep this module dependency-free so its ABI geometry can be verified by
//! standalone `rustc` contracts. BTF parsing, instruction construction and
//! record decoding remain in `oom_victim_loader`; this module owns only the
//! Linux syscall/FD/mmap boundary.

use core::ffi::{c_int, c_long, c_uint, c_void};
use std::io;
use std::mem::{size_of, MaybeUninit};
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd, RawFd};
use std::ptr;
use std::sync::atomic::{AtomicU32, AtomicU64, Ordering};

pub const BPF_MAP_CREATE: u32 = 0;
pub const BPF_MAP_LOOKUP_ELEM: u32 = 1;
pub const BPF_PROG_LOAD: u32 = 5;
pub const BPF_RAW_TRACEPOINT_OPEN: u32 = 17;

pub const BPF_MAP_TYPE_ARRAY: u32 = 2;
pub const BPF_MAP_TYPE_RINGBUF: u32 = 27;
pub const BPF_PROG_TYPE_RAW_TRACEPOINT: u32 = 17;
pub const RAW_TRACEPOINT_NAME: &[u8] = b"mark_victim\0";

const PROT_READ: c_int = 0x1;
const PROT_WRITE: c_int = 0x2;
const MAP_SHARED: c_int = 0x01;
const BPF_RINGBUF_HEADER_SIZE: usize = 8;
const VERIFIER_LOG_SIZE: usize = 64 * 1024;

#[cfg(target_arch = "x86_64")]
const SYS_BPF: c_long = 321;
#[cfg(target_arch = "aarch64")]
const SYS_BPF: c_long = 280;

unsafe extern "C" {
    fn syscall(number: c_long, ...) -> c_long;
    fn getpagesize() -> c_int;
    fn mmap(
        addr: *mut c_void,
        length: usize,
        prot: c_int,
        flags: c_int,
        fd: c_int,
        offset: i64,
    ) -> *mut c_void;
    fn munmap(addr: *mut c_void, length: usize) -> c_int;
}

#[derive(Debug)]
pub struct BpfFd(OwnedFd);

impl AsRawFd for BpfFd {
    fn as_raw_fd(&self) -> RawFd {
        self.0.as_raw_fd()
    }
}

impl BpfFd {
    fn from_syscall_result(result: c_long, operation: &str) -> Result<Self, String> {
        if result < 0 {
            return Err(format!(
                "{} failed: {}",
                operation,
                io::Error::last_os_error()
            ));
        }
        let fd =
            i32::try_from(result).map_err(|_| format!("{} returned an invalid fd", operation))?;
        // SAFETY: a successful BPF syscall returns a newly owned file descriptor.
        Ok(Self(unsafe { OwnedFd::from_raw_fd(fd) }))
    }
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MapCreateAttr {
    pub map_type: u32,
    pub key_size: u32,
    pub value_size: u32,
    pub max_entries: u32,
    pub map_flags: u32,
}

impl MapCreateAttr {
    pub fn ringbuf(capacity: u32) -> Result<Self, String> {
        if capacity < 4096 || !capacity.is_power_of_two() {
            return Err(
                "Wave21 ringbuf capacity must be a page-sized-or-larger power of two".to_string(),
            );
        }
        Ok(Self {
            map_type: BPF_MAP_TYPE_RINGBUF,
            key_size: 0,
            value_size: 0,
            max_entries: capacity,
            map_flags: 0,
        })
    }

    pub fn loss_epoch() -> Self {
        Self {
            map_type: BPF_MAP_TYPE_ARRAY,
            key_size: 4,
            value_size: 8,
            max_entries: 1,
            map_flags: 0,
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RingGeometry {
    pub page_size: usize,
    pub capacity: usize,
    pub mask: usize,
    pub consumer_map_len: usize,
    pub producer_map_len: usize,
    pub data_offset: usize,
}

pub fn ring_geometry(page_size: usize, capacity: usize) -> Result<RingGeometry, String> {
    if page_size == 0 || !page_size.is_power_of_two() {
        return Err("Wave21 system page size must be a non-zero power of two".to_string());
    }
    if capacity < page_size || !capacity.is_power_of_two() {
        return Err("Wave21 ring capacity must be a page-sized-or-larger power of two".to_string());
    }
    let doubled = capacity
        .checked_mul(2)
        .ok_or_else(|| "Wave21 ring double mapping length overflow".to_string())?;
    let producer_map_len = page_size
        .checked_add(doubled)
        .ok_or_else(|| "Wave21 producer mapping length overflow".to_string())?;
    Ok(RingGeometry {
        page_size,
        capacity,
        mask: capacity - 1,
        consumer_map_len: page_size,
        producer_map_len,
        data_offset: page_size,
    })
}

pub fn system_page_size() -> Result<usize, String> {
    // SAFETY: getpagesize takes no arguments and has no memory safety preconditions.
    let page = unsafe { getpagesize() };
    if page <= 0 {
        return Err("Wave21 getpagesize returned an invalid value".to_string());
    }
    let page = usize::try_from(page).map_err(|_| "Wave21 page size overflow".to_string())?;
    if !page.is_power_of_two() {
        return Err("Wave21 system page size is not a power of two".to_string());
    }
    Ok(page)
}

pub fn create_map(attr: MapCreateAttr) -> Result<BpfFd, String> {
    sys_bpf_fd(BPF_MAP_CREATE, &attr, "BPF_MAP_CREATE")
}

#[repr(C)]
struct ProgLoadAttr {
    prog_type: u32,
    insn_cnt: u32,
    insns: u64,
    license: u64,
    log_level: u32,
    log_size: u32,
    log_buf: u64,
    kern_version: u32,
    prog_flags: u32,
}

pub fn load_raw_tracepoint_program<T>(insns: &[T]) -> Result<BpfFd, String> {
    if insns.is_empty() {
        return Err("Wave21 raw-tracepoint program must not be empty".to_string());
    }
    if size_of::<T>() != 8 {
        return Err("Wave21 BPF instruction ABI must be exactly 8 bytes".to_string());
    }
    let insn_cnt = u32::try_from(insns.len())
        .map_err(|_| "Wave21 raw-tracepoint program is too large".to_string())?;
    static LICENSE: &[u8] = b"GPL\0";
    let mut verifier_log = vec![0u8; VERIFIER_LOG_SIZE];
    let attr = ProgLoadAttr {
        prog_type: BPF_PROG_TYPE_RAW_TRACEPOINT,
        insn_cnt,
        insns: insns.as_ptr() as usize as u64,
        license: LICENSE.as_ptr() as usize as u64,
        log_level: 1,
        log_size: u32::try_from(verifier_log.len()).unwrap_or(u32::MAX),
        log_buf: verifier_log.as_mut_ptr() as usize as u64,
        kern_version: 0,
        prog_flags: 0,
    };

    match sys_bpf_fd(BPF_PROG_LOAD, &attr, "BPF_PROG_LOAD") {
        Ok(fd) => Ok(fd),
        Err(err) => {
            let end = verifier_log
                .iter()
                .position(|byte| *byte == 0)
                .unwrap_or(verifier_log.len());
            let log = String::from_utf8_lossy(&verifier_log[..end]);
            if log.trim().is_empty() {
                Err(err)
            } else {
                Err(format!("{}; verifier: {}", err, log.trim()))
            }
        }
    }
}

#[repr(C)]
struct RawTracepointOpenAttr {
    name: u64,
    prog_fd: u32,
    _pad: u32,
}

pub fn open_raw_tracepoint(program: &BpfFd) -> Result<BpfFd, String> {
    let attr = RawTracepointOpenAttr {
        name: RAW_TRACEPOINT_NAME.as_ptr() as usize as u64,
        prog_fd: u32::try_from(program.as_raw_fd())
            .map_err(|_| "Wave21 raw-tracepoint program fd is invalid".to_string())?,
        _pad: 0,
    };
    sys_bpf_fd(
        BPF_RAW_TRACEPOINT_OPEN,
        &attr,
        "BPF_RAW_TRACEPOINT_OPEN(mark_victim)",
    )
}

#[repr(C)]
struct MapLookupAttr {
    map_fd: u32,
    _pad: u32,
    key: u64,
    value: u64,
    flags: u64,
}

pub fn lookup_loss_epoch(map: &BpfFd) -> Result<u64, String> {
    let key: u32 = 0;
    let mut value = MaybeUninit::<u64>::uninit();
    let attr = MapLookupAttr {
        map_fd: u32::try_from(map.as_raw_fd())
            .map_err(|_| "Wave21 loss map fd is invalid".to_string())?,
        _pad: 0,
        key: (&key as *const u32) as usize as u64,
        value: value.as_mut_ptr() as usize as u64,
        flags: 0,
    };
    sys_bpf_unit(
        BPF_MAP_LOOKUP_ELEM,
        &attr,
        "BPF_MAP_LOOKUP_ELEM(loss_epoch)",
    )?;
    // SAFETY: a successful BPF_MAP_LOOKUP_ELEM initialized the complete u64 value.
    Ok(unsafe { value.assume_init() })
}

pub struct RingMapping {
    consumer: *mut c_void,
    producer: *mut c_void,
    data: *const u8,
    geometry: RingGeometry,
}

// SAFETY: RingMapping owns two mmaps and is consumed by one collector task. It
// is movable between executor threads but deliberately not Sync/shared.
unsafe impl Send for RingMapping {}

impl RingMapping {
    pub fn map(fd: &BpfFd, capacity: usize) -> Result<Self, String> {
        let geometry = ring_geometry(system_page_size()?, capacity)?;
        let raw_fd = fd.as_raw_fd();
        // SAFETY: arguments follow the BPF ring-buffer mmap UAPI. The returned
        // mappings are checked against MAP_FAILED and are released in Drop.
        let consumer = unsafe {
            mmap(
                ptr::null_mut(),
                geometry.consumer_map_len,
                PROT_READ | PROT_WRITE,
                MAP_SHARED,
                raw_fd,
                0,
            )
        };
        if is_map_failed(consumer) {
            return Err(format!(
                "Wave21 mmap ring consumer page failed: {}",
                io::Error::last_os_error()
            ));
        }

        // SAFETY: producer offset is exactly one page as required by the BPF
        // ring-buffer mmap ABI; the data region is mapped twice for wraparound.
        let producer = unsafe {
            mmap(
                ptr::null_mut(),
                geometry.producer_map_len,
                PROT_READ,
                MAP_SHARED,
                raw_fd,
                i64::try_from(geometry.page_size)
                    .map_err(|_| "Wave21 producer mmap offset overflow".to_string())?,
            )
        };
        if is_map_failed(producer) {
            // SAFETY: consumer is a live mapping created just above.
            unsafe {
                munmap(consumer, geometry.consumer_map_len);
            }
            return Err(format!(
                "Wave21 mmap ring producer/data pages failed: {}",
                io::Error::last_os_error()
            ));
        }

        // SAFETY: producer points to a mapping of producer_map_len bytes and
        // data_offset is the first byte after its producer-position page.
        let data = unsafe { (producer as *const u8).add(geometry.data_offset) };
        Ok(Self {
            consumer,
            producer,
            data,
            geometry,
        })
    }

    pub fn peek_header(&self) -> Result<Option<u32>, String> {
        let consumer = self.consumer_position().load(Ordering::Acquire);
        let producer = self.producer_position().load(Ordering::Acquire);
        let available = producer
            .checked_sub(consumer)
            .ok_or_else(|| "Wave21 ring producer position moved behind consumer".to_string())?;
        if available == 0 {
            return Ok(None);
        }
        if available > self.geometry.capacity as u64 {
            return Err("Wave21 ring producer outran bounded consumer capacity".to_string());
        }
        let offset = (consumer as usize) & self.geometry.mask;
        // SAFETY: consumer positions are 8-byte aligned by the kernel record
        // ABI and the double-mapped data region covers a complete wrapped record.
        let header = unsafe { &*(self.data.add(offset) as *const AtomicU32) };
        Ok(Some(header.load(Ordering::Acquire)))
    }

    pub fn copy_payload(&self, payload_len: usize) -> Result<Vec<u8>, String> {
        if payload_len == 0
            || payload_len
                .checked_add(BPF_RINGBUF_HEADER_SIZE)
                .map(|span| span > self.geometry.capacity)
                .unwrap_or(true)
        {
            return Err("Wave21 ring payload length escapes bounded mapping".to_string());
        }
        let consumer = self.consumer_position().load(Ordering::Acquire);
        let offset = (consumer as usize) & self.geometry.mask;
        let mut payload = vec![0u8; payload_len];
        // SAFETY: the producer mapping exposes two copies of the data pages,
        // so a payload crossing the physical ring end is still contiguous.
        unsafe {
            ptr::copy_nonoverlapping(
                self.data.add(offset + BPF_RINGBUF_HEADER_SIZE),
                payload.as_mut_ptr(),
                payload_len,
            );
        }
        Ok(payload)
    }

    pub fn consume(&self, span: usize) -> Result<(), String> {
        if span == 0 || span > self.geometry.capacity || span % 8 != 0 {
            return Err("Wave21 ring consume span is invalid".to_string());
        }
        let consumer_pos = self.consumer_position();
        let consumer = consumer_pos.load(Ordering::Acquire);
        let producer = self.producer_position().load(Ordering::Acquire);
        let available = producer
            .checked_sub(consumer)
            .ok_or_else(|| "Wave21 ring producer position moved behind consumer".to_string())?;
        if available < span as u64 {
            return Err("Wave21 ring record span exceeds committed producer data".to_string());
        }
        let next = consumer
            .checked_add(span as u64)
            .ok_or_else(|| "Wave21 ring consumer position overflow".to_string())?;
        consumer_pos.store(next, Ordering::Release);
        Ok(())
    }

    fn consumer_position(&self) -> &AtomicU64 {
        // SAFETY: the consumer mmap begins with the naturally aligned u64
        // consumer position defined by the BPF ring-buffer UAPI.
        unsafe { &*(self.consumer as *const AtomicU64) }
    }

    fn producer_position(&self) -> &AtomicU64 {
        // SAFETY: the producer mmap begins with the naturally aligned u64
        // producer position defined by the BPF ring-buffer UAPI.
        unsafe { &*(self.producer as *const AtomicU64) }
    }
}

impl Drop for RingMapping {
    fn drop(&mut self) {
        // SAFETY: both pointers are successful mmap results owned by self.
        unsafe {
            munmap(self.producer, self.geometry.producer_map_len);
            munmap(self.consumer, self.geometry.consumer_map_len);
        }
    }
}

fn is_map_failed(value: *mut c_void) -> bool {
    value as isize == -1
}

fn sys_bpf_fd<T>(command: u32, attr: &T, operation: &str) -> Result<BpfFd, String> {
    let result = sys_bpf(command, attr)?;
    BpfFd::from_syscall_result(result, operation)
}

fn sys_bpf_unit<T>(command: u32, attr: &T, operation: &str) -> Result<(), String> {
    let result = sys_bpf(command, attr)?;
    if result != 0 {
        return Err(format!(
            "{} returned unexpected result {}",
            operation, result
        ));
    }
    Ok(())
}

fn sys_bpf<T>(command: u32, attr: &T) -> Result<c_long, String> {
    #[cfg(any(target_arch = "x86_64", target_arch = "aarch64"))]
    {
        let size = size_of::<T>();
        // SAFETY: attr points to a repr(C) syscall attribute that remains live
        // for the duration of the syscall; command and size are value arguments.
        let result = unsafe {
            syscall(
                SYS_BPF,
                command as c_uint,
                attr as *const T as *const c_void,
                size,
            )
        };
        if result < 0 {
            return Err(io::Error::last_os_error().to_string());
        }
        Ok(result)
    }
    #[cfg(not(any(target_arch = "x86_64", target_arch = "aarch64")))]
    {
        let _ = (command, attr);
        Err("Wave21 BPF runtime supports only x86_64 and aarch64".to_string())
    }
}
