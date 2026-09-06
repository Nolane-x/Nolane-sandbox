// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use crate::guest_oom_victim::RawVictimEvent;
use crate::oom_victim_proc::probe_collector_capability;

/// Exact Wave21 collector handle.
///
/// The current Agent dependency graph contains no object-free raw-tracepoint
/// loader equivalent to the reviewed Wave20 cilium/ebpf path. Tracefs text is
/// deliberately not used as a fallback because it cannot carry the exact
/// task lifetime and cgroup identity required by Wave21.
pub struct Collector {
    _private: (),
}

impl Collector {
    pub fn start() -> Result<Self, String> {
        let capability = probe_collector_capability()?;
        Err(format!(
            "Wave21 guest OOM collection disabled: exact oom:mark_victim capability is present (BTF {}, event {}), but this Agent build has no reviewed object-free raw-tracepoint BPF loader; refusing tracefs/dmesg/exit-code fallback",
            capability.btf_path.display(),
            capability.mark_victim_id_path.display(),
        ))
    }

    pub async fn next_event(&mut self) -> Result<RawVictimEvent, String> {
        std::future::pending::<Result<RawVictimEvent, String>>().await
    }
}
