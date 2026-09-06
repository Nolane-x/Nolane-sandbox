#!/usr/bin/env python3
from pathlib import Path

path = Path("agent/src/oom_victim_loader.rs")
text = path.read_text()

anchor = "pub const EVENT_VERSION_V1: u32 = 1;\npub const RAW_VICTIM_RECORD_SIZE: i32 = 40;\n"
addition = r'''pub const EVENT_VERSION_V1: u32 = 1;
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
'''

if "pub fn classify_ring_record_header" not in text:
    if anchor not in text:
        raise SystemExit("ring framing insertion anchor missing")
    text = text.replace(anchor, addition, 1)
    path.write_text(text)
