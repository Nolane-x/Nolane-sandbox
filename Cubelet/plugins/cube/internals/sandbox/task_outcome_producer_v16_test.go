// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/errdefs"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeTaskOutcomeRuntimeService struct {
	waitFn  func(context.Context, *task.WaitRequest) (*task.WaitResponse, error)
	stateFn func(context.Context, *task.StateRequest) (*task.StateResponse, error)
}

func (f *fakeTaskOutcomeRuntimeService) Wait(ctx context.Context, req *task.WaitRequest) (*task.WaitResponse, error) {
	if f.waitFn == nil {
		return nil, errors.New("unexpected Wait call")
	}
	return f.waitFn(ctx, req)
}

func (f *fakeTaskOutcomeRuntimeService) State(ctx context.Context, req *task.StateRequest) (*task.StateResponse, error) {
	if f.stateFn == nil {
		return nil, errors.New("unexpected State call")
	}
	return f.stateFn(ctx, req)
}

func (f *fakeTaskOutcomeRuntimeService) Stats(context.Context, *task.StatsRequest) (*task.StatsResponse, error) {
	return &task.StatsResponse{}, nil
}

func taskOutcomeControllerWithService(service taskRuntimeService) *controllerLocal {
	return &controllerLocal{
		taskServiceResolver: func(context.Context, string) (taskRuntimeService, error) {
			return service, nil
		},
		sandboxEndpointResolver: func(context.Context, string) (string, uint32, error) {
			return "test-endpoint", 2, nil
		},
	}
}

func TestTaskOutcomeWaitNotFoundFailsClosed(t *testing.T) {
	controller := &controllerLocal{
		taskServiceResolver: func(context.Context, string) (taskRuntimeService, error) {
			return nil, errdefs.ErrNotFound
		},
	}

	outcome, err := controller.Wait(context.Background(), "sandbox-a")
	if err == nil {
		t.Fatal("Wait NotFound fabricated a successful terminal outcome")
	}
	if outcome.ExitStatus != 0 || !outcome.ExitedAt.IsZero() {
		t.Fatalf("Wait NotFound returned non-zero proof-like outcome: %+v", outcome)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("Wait NotFound populated task-outcome proof")
	}
}

func TestTaskOutcomeWaitErrorDiscardsPartialResponse(t *testing.T) {
	exitedAt := time.Unix(1_725_000_013, 707).UTC()
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		waitFn: func(context.Context, *task.WaitRequest) (*task.WaitResponse, error) {
			return &task.WaitResponse{ExitStatus: 137, ExitedAt: timestamppb.New(exitedAt)}, errors.New("runtime wait failed")
		},
	})
	controller.Start(context.Background(), "sandbox-a")

	outcome, err := controller.Wait(context.Background(), "sandbox-a")
	if err == nil {
		t.Fatal("failed Wait response became a successful terminal outcome")
	}
	if outcome.ExitStatus != 0 || !outcome.ExitedAt.IsZero() {
		t.Fatalf("failed Wait leaked partial proof-like outcome: %+v", outcome)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("failed Wait populated task-outcome proof")
	}
}

func TestTaskOutcomeWaitRequiresRuntimeExitTimestamp(t *testing.T) {
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		waitFn: func(context.Context, *task.WaitRequest) (*task.WaitResponse, error) {
			return &task.WaitResponse{ExitStatus: 7}, nil
		},
	})
	controller.Start(context.Background(), "sandbox-a")

	outcome, err := controller.Wait(context.Background(), "sandbox-a")
	if err == nil {
		t.Fatal("Wait without runtime exit timestamp became exact outcome")
	}
	if outcome.ExitStatus != 0 || !outcome.ExitedAt.IsZero() {
		t.Fatalf("invalid Wait returned proof-like outcome: %+v", outcome)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("invalid Wait populated task-outcome proof")
	}
}

func TestTaskOutcomeWaitSuccessRecordsExact137WithoutReinterpretation(t *testing.T) {
	exitedAt := time.Unix(1_725_000_014, 808_909).UTC()
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		waitFn: func(context.Context, *task.WaitRequest) (*task.WaitResponse, error) {
			return &task.WaitResponse{ExitStatus: 137, ExitedAt: timestamppb.New(exitedAt)}, nil
		},
	})
	controller.Start(context.Background(), "sandbox-a")

	outcome, err := controller.Wait(context.Background(), "sandbox-a")
	if err != nil {
		t.Fatalf("exact Wait: %v", err)
	}
	if outcome.ExitStatus != 137 || !outcome.ExitedAt.Equal(exitedAt) {
		t.Fatalf("Wait lost exact outcome: %+v", outcome)
	}
	proof, ok := controller.TaskOutcomeProof("sandbox-a")
	if !ok {
		t.Fatal("exact Wait did not populate proof")
	}
	if proof.ExitCode != 137 || !proof.ExitedAt.Equal(exitedAt) || proof.Source != TaskOutcomeProofSourceWait {
		t.Fatalf("Wait proof mismatch: %+v", proof)
	}
}

func TestTaskOutcomeStatusNotFoundStaysOperationalOnly(t *testing.T) {
	controller := &controllerLocal{
		taskServiceResolver: func(context.Context, string) (taskRuntimeService, error) {
			return nil, errdefs.ErrNotFound
		},
	}

	status, err := controller.Status(context.Background(), "sandbox-a", false)
	if err != nil {
		t.Fatalf("operational NotFound status: %v", err)
	}
	if status.ExitedAt.IsZero() {
		t.Fatal("operational NotFound status no longer reconciles with a synthetic exit time")
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("synthetic NotFound status populated proof")
	}
}

func TestTaskOutcomeStatusRunningDoesNotCreateProof(t *testing.T) {
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		stateFn: func(context.Context, *task.StateRequest) (*task.StateResponse, error) {
			return &task.StateResponse{Status: tasktypes.Status_RUNNING, Pid: 123}, nil
		},
	})

	status, err := controller.Status(context.Background(), "sandbox-a", false)
	if err != nil {
		t.Fatalf("running status: %v", err)
	}
	if status.State != tasktypes.Status_RUNNING.String() {
		t.Fatalf("running state = %q", status.State)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("running state populated terminal proof")
	}
}

func TestTaskOutcomeStatusStoppedReconstructsExactProof(t *testing.T) {
	exitedAt := time.Unix(1_725_000_015, 909_010).UTC()
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		stateFn: func(context.Context, *task.StateRequest) (*task.StateResponse, error) {
			return &task.StateResponse{
				Status:     tasktypes.Status_STOPPED,
				ExitStatus: 31,
				ExitedAt:   timestamppb.New(exitedAt),
			}, nil
		},
	})

	if _, err := controller.Status(context.Background(), "sandbox-a", false); err != nil {
		t.Fatalf("stopped status: %v", err)
	}
	proof, ok := controller.TaskOutcomeProof("sandbox-a")
	if !ok {
		t.Fatal("exact STOPPED state did not reconstruct proof")
	}
	if proof.ExitCode != 31 || !proof.ExitedAt.Equal(exitedAt) || proof.Source != TaskOutcomeProofSourceState {
		t.Fatalf("STOPPED state proof mismatch: %+v", proof)
	}
}

func TestTaskOutcomeStatusStoppedWithoutTimestampRemainsUnproven(t *testing.T) {
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		stateFn: func(context.Context, *task.StateRequest) (*task.StateResponse, error) {
			return &task.StateResponse{Status: tasktypes.Status_STOPPED, ExitStatus: 5}, nil
		},
	})

	status, err := controller.Status(context.Background(), "sandbox-a", false)
	if err != nil {
		t.Fatalf("operational stopped status should survive missing proof timestamp: %v", err)
	}
	if status.State != tasktypes.Status_STOPPED.String() {
		t.Fatalf("stopped operational state = %q", status.State)
	}
	if _, ok := controller.TaskOutcomeProof("sandbox-a"); ok {
		t.Fatal("STOPPED state without runtime timestamp populated proof")
	}
}

func TestTaskOutcomeStatusConflictingExactOutcomeFailsClosed(t *testing.T) {
	firstExitedAt := time.Unix(1_725_000_016, 111).UTC()
	secondExitedAt := firstExitedAt.Add(time.Second)
	controller := taskOutcomeControllerWithService(&fakeTaskOutcomeRuntimeService{
		stateFn: func(context.Context, *task.StateRequest) (*task.StateResponse, error) {
			return &task.StateResponse{
				Status:     tasktypes.Status_STOPPED,
				ExitStatus: 44,
				ExitedAt:   timestamppb.New(secondExitedAt),
			}, nil
		},
	})
	controller.Start(context.Background(), "sandbox-a")
	if _, err := controller.recordTaskOutcomeCandidate(taskOutcomeCandidate{
		SandboxID: "sandbox-a",
		ExitCode:  43,
		ExitedAt:  firstExitedAt,
		Source:    TaskOutcomeProofSourceWait,
	}); err != nil {
		t.Fatalf("seed exact proof: %v", err)
	}

	if _, err := controller.Status(context.Background(), "sandbox-a", false); err == nil {
		t.Fatal("conflicting exact STOPPED outcome did not fail closed")
	}
}
