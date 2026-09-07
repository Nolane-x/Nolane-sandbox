// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use crate::oom_victim_bpf::Collector;
use crate::sandbox::Sandbox;
use anyhow::Result;
use slog::{info, warn, Logger};
use std::sync::Arc;
use tokio::sync::{watch::Receiver, Mutex};
use tokio::task::JoinHandle;

/// Start Wave21 guest-kernel victim collection without making Agent startup
/// depend on BPF/BTF availability. A missing or rejected exact collector keeps
/// Wave21 positive evidence disabled; it never enables an inference fallback.
pub fn start_best_effort(
    logger: &Logger,
    sandbox: Arc<Mutex<Sandbox>>,
    mut shutdown: Receiver<bool>,
) -> Option<JoinHandle<Result<()>>> {
    let mut collector = match Collector::start() {
        Ok(collector) => collector,
        Err(err) => {
            warn!(logger, "Wave21 guest OOM collector disabled: {}", err);
            return None;
        }
    };

    let logger = logger.clone();
    Some(tokio::spawn(async move {
        sandbox.lock().await.mark_guest_oom_victim_collector_live();
        info!(logger, "Wave21 exact guest OOM collector started");
        loop {
            tokio::select! {
                changed = shutdown.changed() => {
                    if changed.is_err() || *shutdown.borrow() {
                        sandbox.lock().await.mark_guest_oom_victim_collector_lost();
                        return Ok(());
                    }
                }
                next = collector.next_event() => {
                    match next {
                        Ok(event) => {
                            if let Err(err) = sandbox.lock().await.record_guest_oom_victim_raw(event) {
                                // record_raw treats malformed input as loss before returning.
                                warn!(logger, "Wave21 guest OOM event rejected and loss-poisoned: {}", err);
                            }
                        }
                        Err(err) => {
                            sandbox.lock().await.mark_guest_oom_victim_collector_lost();
                            warn!(logger, "Wave21 guest OOM collector stopped after evidence loss: {}", err);
                            return Ok(());
                        }
                    }
                }
            }
        }
    }))
}
