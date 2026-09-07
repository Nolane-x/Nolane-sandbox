// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"testing"

	coresandbox "github.com/containerd/containerd/v2/core/sandbox"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictimbridge"
)

func TestV21ControllerPublishesAndRotatesStartBinding(t *testing.T) {
	const sandboxID = "sandbox-v21-controller-bridge"
	guestvictimbridge.Clear(sandboxID)
	t.Cleanup(func() { guestvictimbridge.Clear(sandboxID) })

	var next byte = 1
	controller := &controllerLocal{
		guestOOMVictimTokenGenerator: func() ([32]byte, error) {
			var token [32]byte
			for i := range token {
				token[i] = next + byte(i)
			}
			next++
			return token, nil
		},
	}

	if _, err := controller.Start(context.Background(), sandboxID); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	first, ok := guestvictimbridge.CurrentStartBinding(sandboxID)
	if !ok || first.Generation != 1 {
		t.Fatalf("first bridge binding = (%+v,%v), want generation 1", first, ok)
	}

	if _, err := controller.Start(context.Background(), sandboxID); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	second, ok := guestvictimbridge.CurrentStartBinding(sandboxID)
	if !ok || second.Generation != 2 || second.Token == first.Token {
		t.Fatalf("second bridge binding = (%+v,%v), want fresh generation/token", second, ok)
	}

	if err := controller.Create(context.Background(), coresandbox.Sandbox{ID: sandboxID}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := guestvictimbridge.CurrentStartBinding(sandboxID); ok {
		t.Fatal("Create fence left stale Wave21 bridge binding available")
	}
}
