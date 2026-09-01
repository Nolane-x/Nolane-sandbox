// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"testing"
)

func TestTaskOutcomeWaitResolverErrorFailsClosed(t *testing.T) {
	controller := &controllerLocal{
		taskServiceResolver: func(context.Context, string) (taskRuntimeService, error) {
			return nil, errors.New("runtime resolver failed")
		},
	}

	outcome, err := controller.Wait(context.Background(), "sandbox-a")
	if err == nil {
		t.Fatal("runtime resolver error became a successful terminal outcome")
	}
	if outcome.ExitStatus != 0 || !outcome.ExitedAt.IsZero() {
		t.Fatalf("runtime resolver error leaked proof-like outcome: %+v", outcome)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("runtime resolver error populated task-outcome proof")
	}
}
