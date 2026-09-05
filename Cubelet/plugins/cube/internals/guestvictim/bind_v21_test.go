// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package guestvictim

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestBindBeforeStartOrdersBindBeforeStart(t *testing.T) {
	var calls []string
	bind := func(context.Context, string, [32]byte) error {
		calls = append(calls, "bind")
		return nil
	}
	start := func(context.Context) error {
		calls = append(calls, "start")
		return nil
	}
	token := [32]byte{1}

	result := BindBeforeStart(context.Background(), "sandbox-a", token, bind, start)
	if result.BindErr != nil || result.StartErr != nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if want := []string{"bind", "start"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("call order = %v, want %v", calls, want)
	}
}

func TestBindBeforeStartBindFailureDoesNotBlockStart(t *testing.T) {
	bindErr := errors.New("bind unavailable")
	started := false
	bind := func(context.Context, string, [32]byte) error { return bindErr }
	start := func(context.Context) error {
		started = true
		return nil
	}

	result := BindBeforeStart(context.Background(), "sandbox-a", [32]byte{2}, bind, start)
	if !errors.Is(result.BindErr, bindErr) {
		t.Fatalf("BindErr = %v, want %v", result.BindErr, bindErr)
	}
	if result.StartErr != nil {
		t.Fatalf("StartErr = %v, want nil", result.StartErr)
	}
	if !started {
		t.Fatal("start was not called after evidence bind failure")
	}
}

func TestBindBeforeStartStartFailureIsReturnedSeparately(t *testing.T) {
	startErr := errors.New("workload start failed")
	bind := func(context.Context, string, [32]byte) error { return nil }
	start := func(context.Context) error { return startErr }

	result := BindBeforeStart(context.Background(), "sandbox-a", [32]byte{3}, bind, start)
	if result.BindErr != nil {
		t.Fatalf("BindErr = %v, want nil", result.BindErr)
	}
	if !errors.Is(result.StartErr, startErr) {
		t.Fatalf("StartErr = %v, want %v", result.StartErr, startErr)
	}
}

func TestBindBeforeStartSkipsInvalidAuthorityWithoutBlockingStart(t *testing.T) {
	bindCalled := false
	startCalled := false
	bind := func(context.Context, string, [32]byte) error {
		bindCalled = true
		return nil
	}
	start := func(context.Context) error {
		startCalled = true
		return nil
	}

	result := BindBeforeStart(context.Background(), "", [32]byte{}, bind, start)
	if result.BindErr == nil {
		t.Fatal("expected invalid evidence authority to be reported")
	}
	if bindCalled {
		t.Fatal("bind must not run for invalid authority")
	}
	if !startCalled {
		t.Fatal("workload start must still run")
	}
}
