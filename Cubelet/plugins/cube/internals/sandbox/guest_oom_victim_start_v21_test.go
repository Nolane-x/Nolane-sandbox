// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"testing"
)

func TestV21ControllerStartCreatesTokenForExactGeneration(t *testing.T) {
	var token [32]byte
	for i := range token {
		token[i] = byte(i + 1)
	}
	controller := &controllerLocal{
		guestOOMVictimTokenGenerator: func() ([32]byte, error) {
			return token, nil
		},
	}

	if _, err := controller.Start(context.Background(), "sandbox-v21"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got, ok := controller.GuestOOMVictimToken("sandbox-v21", 1)
	if !ok {
		t.Fatal("Start did not publish Wave21 token for generation 1")
	}
	if got != token {
		t.Fatalf("published token = %x, want %x", got, token)
	}
}

func TestV21ControllerStartTokenGenerationFailureIsObservationalOnly(t *testing.T) {
	controller := &controllerLocal{
		guestOOMVictimTokenGenerator: func() ([32]byte, error) {
			return [32]byte{}, errors.New("entropy unavailable")
		},
	}

	if _, err := controller.Start(context.Background(), "sandbox-v21"); err != nil {
		t.Fatalf("Start propagated Wave21 evidence failure: %v", err)
	}
	if _, ok := controller.GuestOOMVictimToken("sandbox-v21", 1); ok {
		t.Fatal("Start published a Wave21 token after generator failure")
	}
}
