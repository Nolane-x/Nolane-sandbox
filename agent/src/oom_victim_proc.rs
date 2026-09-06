// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use std::path::{Path, PathBuf};

const KERNEL_BTF: &str = "/sys/kernel/btf/vmlinux";
const TRACEFS_MARK_VICTIM: &str = "/sys/kernel/tracing/events/oom/mark_victim/id";
const DEBUGFS_MARK_VICTIM: &str = "/sys/kernel/debug/tracing/events/oom/mark_victim/id";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CollectorCapability {
    pub btf_path: PathBuf,
    pub mark_victim_id_path: PathBuf,
}

pub fn probe_collector_capability() -> Result<CollectorCapability, String> {
    if !matches!(std::env::consts::ARCH, "x86_64" | "aarch64") {
        return Err(format!(
            "Wave21 guest raw-tracepoint collector is unsupported on architecture {}",
            std::env::consts::ARCH
        ));
    }

    let btf_path = PathBuf::from(KERNEL_BTF);
    if !is_nonempty_regular_file(&btf_path) {
        return Err(format!(
            "Wave21 guest collector requires readable kernel BTF at {}",
            btf_path.display()
        ));
    }

    let mark_victim_id_path = [TRACEFS_MARK_VICTIM, DEBUGFS_MARK_VICTIM]
        .iter()
        .map(PathBuf::from)
        .find(|path| is_nonempty_regular_file(path))
        .ok_or_else(|| {
            "Wave21 guest collector requires the oom:mark_victim tracepoint; no tracefs/debugfs event id is readable"
                .to_string()
        })?;

    let id = std::fs::read_to_string(&mark_victim_id_path)
        .map_err(|e| format!("read {}: {}", mark_victim_id_path.display(), e))?;
    let id = id
        .trim()
        .parse::<u64>()
        .map_err(|_| format!("{} contains an invalid tracepoint id", mark_victim_id_path.display()))?;
    if id == 0 {
        return Err(format!(
            "{} contains a zero tracepoint id",
            mark_victim_id_path.display()
        ));
    }

    Ok(CollectorCapability {
        btf_path,
        mark_victim_id_path,
    })
}

fn is_nonempty_regular_file(path: &Path) -> bool {
    std::fs::metadata(path)
        .map(|metadata| metadata.is_file() && metadata.len() > 0)
        .unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn supported_arch_contract_matches_release_targets() {
        assert!(matches!("x86_64", "x86_64" | "aarch64"));
        assert!(matches!("aarch64", "x86_64" | "aarch64"));
        assert!(!matches!("riscv64", "x86_64" | "aarch64"));
    }
}
