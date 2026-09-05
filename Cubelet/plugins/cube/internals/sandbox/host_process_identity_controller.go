// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

// ensureHostProcessInspector returns a trusted host inspector backed by procfs.
// The inspector itself is stateless; realization/lifetime authority remains in
// taskOutcomeProofStore under the shared proof lock.
func (c *controllerLocal) ensureHostProcessInspector() *hostProcessInspector {
	return newHostProcessInspector(nil, nil)
}
