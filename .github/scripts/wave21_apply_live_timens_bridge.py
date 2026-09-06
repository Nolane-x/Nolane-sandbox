#!/usr/bin/env python3
from pathlib import Path

path = Path("agent/src/guest_oom_victim.rs")
text = path.read_text()

old = '''pub fn start_boottime_ns_to_starttime_ticks(
    start_boottime_ns: u64,
    boottime_offset_ns: i128,
) -> Result<u64, String> {
    let visible_start_ns = i128::from(start_boottime_ns)
        .checked_add(boottime_offset_ns)
        .ok_or_else(|| "visible process start time overflow".to_string())?;
    if visible_start_ns < 0 || visible_start_ns > i128::from(u64::MAX) {
        return Err("visible process start time is outside the supported range".to_string());
    }
    let visible_start_ns = u64::try_from(visible_start_ns)
        .map_err(|_| "visible process start time conversion failed".to_string())?;
    Ok(visible_start_ns / USER_HZ_100_TICK_NS)
}
'''

new = '''pub fn read_current_timens_boottime_offset() -> Result<i128, String> {
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
            Err(err) => Err(format!("read current time namespace offsets failed: {}", err)),
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
'''

if "pub fn read_current_timens_boottime_offset" in text:
    raise SystemExit("live timens bridge already present; refusing duplicate patch")
if old not in text:
    raise SystemExit("live timens bridge insertion anchor missing")
path.write_text(text.replace(old, new, 1))
