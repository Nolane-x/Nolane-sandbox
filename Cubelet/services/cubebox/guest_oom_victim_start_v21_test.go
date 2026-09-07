// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package cubebox

import (
	"context"
	"errors"
	"reflect"
	"testing"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictim"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/guestvictimbridge"
)

type v21FakeTask struct {
	calls       []string
	annotations map[string]string
	updateErr   error
	startErr    error
}

func (f *v21FakeTask) Update(ctx context.Context, opts ...containerd.UpdateTaskOpts) error {
	f.calls = append(f.calls, "update")
	var info containerd.UpdateTaskInfo
	for _, opt := range opts {
		if err := opt(ctx, nil, &info); err != nil {
			return err
		}
	}
	f.annotations = info.Annotations
	return f.updateErr
}

func (f *v21FakeTask) Start(context.Context) error {
	f.calls = append(f.calls, "start")
	return f.startErr
}

func v21CubeBoxToken(seed byte) [32]byte {
	var token [32]byte
	for i := range token {
		token[i] = seed + byte(i)
	}
	return token
}

func TestV21CubeBoxBindsExactTokenBeforeStart(t *testing.T) {
	const sandboxID = "sandbox-v21-cubebox"
	guestvictimbridge.Clear(sandboxID)
	t.Cleanup(func() { guestvictimbridge.Clear(sandboxID) })
	token := v21CubeBoxToken(1)
	binding := guestvictimbridge.StartBinding{SandboxID: sandboxID, Generation: 5, Token: token}
	if err := guestvictimbridge.PublishStartBinding(binding); err != nil {
		t.Fatalf("PublishStartBinding: %v", err)
	}

	fake := &v21FakeTask{}
	if err := bindGuestOOMVictimBeforeStart(context.Background(), fake, sandboxID); err != nil {
		t.Fatalf("bindGuestOOMVictimBeforeStart: %v", err)
	}
	if !reflect.DeepEqual(fake.calls, []string{"update", "start"}) {
		t.Fatalf("call order = %v, want [update start]", fake.calls)
	}
	want, err := guestvictim.TaskUpdateAnnotations(token)
	if err != nil {
		t.Fatalf("TaskUpdateAnnotations: %v", err)
	}
	if !reflect.DeepEqual(fake.annotations, want) {
		t.Fatalf("annotations = %#v, want %#v", fake.annotations, want)
	}
	if _, ok := guestvictimbridge.CurrentStartBinding(sandboxID); ok {
		t.Fatal("successful bind remained available for reuse")
	}
}

func TestV21CubeBoxBindFailureIsObservationalOnly(t *testing.T) {
	const sandboxID = "sandbox-v21-bind-failure"
	guestvictimbridge.Clear(sandboxID)
	t.Cleanup(func() { guestvictimbridge.Clear(sandboxID) })
	binding := guestvictimbridge.StartBinding{SandboxID: sandboxID, Generation: 8, Token: v21CubeBoxToken(2)}
	if err := guestvictimbridge.PublishStartBinding(binding); err != nil {
		t.Fatalf("PublishStartBinding: %v", err)
	}

	bindErr := errors.New("bind unavailable")
	startErr := errors.New("workload start failed")
	fake := &v21FakeTask{updateErr: bindErr, startErr: startErr}
	got := bindGuestOOMVictimBeforeStart(context.Background(), fake, sandboxID)
	if !errors.Is(got, startErr) {
		t.Fatalf("returned error = %v, want workload Start error %v", got, startErr)
	}
	if !reflect.DeepEqual(fake.calls, []string{"update", "start"}) {
		t.Fatalf("call order = %v, want [update start]", fake.calls)
	}
	if _, ok := guestvictimbridge.CurrentStartBinding(sandboxID); ok {
		t.Fatal("failed bind remained available for repair/retry")
	}
}

func TestV21CubeBoxNoBindingStartsWithoutUpdate(t *testing.T) {
	const sandboxID = "sandbox-v21-no-binding"
	guestvictimbridge.Clear(sandboxID)
	fake := &v21FakeTask{}
	if err := bindGuestOOMVictimBeforeStart(context.Background(), fake, sandboxID); err != nil {
		t.Fatalf("bindGuestOOMVictimBeforeStart: %v", err)
	}
	if !reflect.DeepEqual(fake.calls, []string{"start"}) {
		t.Fatalf("call order = %v, want [start]", fake.calls)
	}
}
