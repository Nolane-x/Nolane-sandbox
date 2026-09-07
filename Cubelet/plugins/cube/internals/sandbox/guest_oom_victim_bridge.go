// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import "github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictimbridge"

func (c *controllerLocal) clearGuestOOMVictimStartBinding(sandboxID string) {
	guestvictimbridge.Clear(sandboxID)
}

func (c *controllerLocal) publishGuestOOMVictimStartBinding(sandboxID string, generation uint64, token [32]byte) error {
	return guestvictimbridge.PublishStartBinding(guestvictimbridge.StartBinding{
		SandboxID:  sandboxID,
		Generation: generation,
		Token:      token,
	})
}
