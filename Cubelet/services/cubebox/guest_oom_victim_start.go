// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package cubebox

import (
	"context"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/tencentcloud/CubeSandbox/Cubelet/pkg/log"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictim"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictimbridge"
)

type guestOOMVictimStartTask interface {
	Update(context.Context, ...containerd.UpdateTaskOpts) error
	Start(context.Context) error
}

// bindGuestOOMVictimBeforeStart performs the single observational Wave 21
// bind attempt for the just-created pod task, then always delegates workload
// authority to Start. Bind construction/RPC/reporting failures never replace
// or mask the workload Start result.
func bindGuestOOMVictimBeforeStart(ctx context.Context, task guestOOMVictimStartTask, sandboxID string) error {
	binding, ok := guestvictimbridge.ClaimStartBinding(sandboxID)
	if ok {
		annotations, bindErr := guestvictim.TaskUpdateAnnotations(binding.Token)
		if bindErr == nil {
			bindErr = task.Update(ctx, containerd.WithAnnotations(annotations))
		}
		if bindErr != nil {
			guestvictimbridge.MarkUnavailable(binding)
			log.G(ctx).Warnf("Wave21 guest OOM victim bind unavailable for sandbox %s generation %d: %v", sandboxID, binding.Generation, bindErr)
		} else {
			guestvictimbridge.MarkBound(binding)
		}
	}
	return task.Start(ctx)
}
