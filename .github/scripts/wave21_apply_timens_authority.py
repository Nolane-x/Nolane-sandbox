#!/usr/bin/env python3
from pathlib import Path

path = Path("agent/src/guest_oom_victim.rs")
text = path.read_text()

if "pub fn authoritative_timens_boottime_offset" not in text:
    anchor = '''pub fn start_boottime_ns_to_starttime_ticks(
    start_boottime_ns: u64,
    boottime_offset_ns: i128,
) -> Result<u64, String> {
'''
    helper = r'''pub fn authoritative_timens_boottime_offset(
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

'''
    if anchor not in text:
        raise SystemExit("time namespace authority insertion anchor missing")
    text = text.replace(anchor, helper + anchor, 1)
    path.write_text(text)
