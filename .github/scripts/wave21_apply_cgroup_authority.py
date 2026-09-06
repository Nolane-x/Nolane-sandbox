#!/usr/bin/env python3
from pathlib import Path

HELPER = r'''pub fn parse_process_cgroup_v2_path(raw: &str) -> Result<Option<String>, String> {
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
    let bytes: [u8; 8] = raw
        .try_into()
        .map_err(|_| format!("cgroup-v2 handle must be exactly 8 bytes, got {}", raw.len()))?;
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
    let rc = unsafe {
        name_to_handle_at(
            AT_FDCWD,
            c_path.as_ptr(),
            &mut handle,
            &mut mount_id,
            0,
        )
    };
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
        return Err("guest process cgroup-v2 membership changed during identity capture".to_string());
    }
    Ok(Some(id))
}'''

OLD_BEGIN = '''                (Ok(boot_id), Ok(main)) => {
                    if let Err(e) = s.begin_guest_oom_victim_realization(
                        &cid,
                        token,
                        started_boot_ns,
                        &boot_id,
                        main,
                        None,
                    ) {'''

NEW_BEGIN = '''                (Ok(boot_id), Ok(main)) => {
                    let expected_cgroup_v2_id = match read_process_cgroup_v2_id(init_pid) {
                        Ok(id) => id,
                        Err(e) => {
                            warn!(
                                sl!(),
                                "Wave21 exact cgroup-v2 identity unavailable for {}: {}",
                                cid,
                                e
                            );
                            None
                        }
                    };
                    if let Err(e) = s.begin_guest_oom_victim_realization(
                        &cid,
                        token,
                        started_boot_ns,
                        &boot_id,
                        main,
                        expected_cgroup_v2_id,
                    ) {'''


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if new in text:
        return text
    if old not in text:
        raise SystemExit(f"{label} anchor missing")
    return text.replace(old, new, 1)


authority = Path("agent/src/guest_oom_victim.rs")
text = authority.read_text()
if "pub fn read_process_cgroup_v2_id" not in text:
    marker = "#[repr(C)]\nstruct KernelTimespec {"
    if marker not in text:
        raise SystemExit("guest authority insertion anchor missing")
    authority.write_text(text.replace(marker, HELPER + "\n\n" + marker, 1))

loader = Path("agent/src/oom_victim_loader.rs")
text = loader.read_text()
for old, new in [
    (
        "let actual_encoding = match ty.payload {\n            BtfPayload::Int { encoding } => encoding,",
        "let actual_encoding = match &ty.payload {\n            BtfPayload::Int { encoding } => *encoding,",
    ),
    (
        "matches!(id_type.payload, BtfPayload::Int { encoding: 0 })",
        "matches!(&id_type.payload, BtfPayload::Int { encoding: 0 })",
    ),
    (
        "matches!(inner_type.payload, BtfPayload::Int { encoding: 0 })",
        "matches!(&inner_type.payload, BtfPayload::Int { encoding: 0 })",
    ),
]:
    if old in text:
        text = text.replace(old, new, 1)
loader.write_text(text)

rpc = Path("agent/src/rpc.rs")
text = rpc.read_text()
old_import = (
    "current_boottime_ns, read_guest_boot_id, read_process_identity, "
    "RealizationToken, VictimClass,"
)
new_import = (
    "current_boottime_ns, read_guest_boot_id, read_process_cgroup_v2_id, "
    "read_process_identity, RealizationToken, VictimClass,"
)
text = replace_once(text, old_import, new_import, "rpc Wave21 import")
text = replace_once(text, OLD_BEGIN, NEW_BEGIN, "rpc begin-realization")
rpc.write_text(text)

contract = Path("tests/wave21_agent_service_contract.py")
text = contract.read_text()
marker = '    "begin_guest_oom_victim_realization",\n'
addition = '    "read_process_cgroup_v2_id",\n'
if addition not in text:
    if marker not in text:
        raise SystemExit("agent contract anchor missing")
    contract.write_text(text.replace(marker, marker + addition, 1))

contract_workflow = Path(".github/workflows/cube-guest-kernel-oom-victim-contract.yml")
text = contract_workflow.read_text()
cgroup_test = (
    "          rustc --edition=2021 --test tests/guest_oom_victim_cgroup_v21.rs "
    "-o /tmp/guest_oom_victim_cgroup_v21\n"
    "          /tmp/guest_oom_victim_cgroup_v21 --nocapture\n"
)
if "rustc --edition=2021 --test tests/guest_oom_victim_cgroup_v21.rs" not in text:
    anchor = (
        "          rustc --edition=2021 --test tests/guest_oom_victim_bounds_v21.rs "
        "-o /tmp/guest_oom_victim_bounds_v21\n"
        "          /tmp/guest_oom_victim_bounds_v21 --nocapture\n"
    )
    if anchor not in text:
        raise SystemExit("Wave21 guest contract bounds anchor missing")
    contract_workflow.write_text(text.replace(anchor, anchor + cgroup_test, 1))
