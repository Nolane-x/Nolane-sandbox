from pathlib import Path

path = Path("agent/src/oom_victim_loader.rs")
text = path.read_text()

if "pub fn decode_raw_victim_record(" in text:
    raise SystemExit(0)

anchor = "pub const RAW_VICTIM_RECORD_SIZE: i32 = 40;\n"
if anchor not in text:
    raise SystemExit("Wave21 raw decoder insertion anchor missing")

block = r'''

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
'''

path.write_text(text.replace(anchor, anchor + block, 1))
