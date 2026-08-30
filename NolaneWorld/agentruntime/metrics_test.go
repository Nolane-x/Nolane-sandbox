package agentruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type timingSinkRecorder struct {
	samples []TimingSample
}

func (r *timingSinkRecorder) ObserveTiming(sample TimingSample) {
	r.samples = append(r.samples, sample)
}

type timingRuntimeStub struct {
	execErr error
}

func (s *timingRuntimeStub) Enter(context.Context, EnterRequest) (Session, error) {
	return Session{}, nil
}
func (s *timingRuntimeStub) Acquire(context.Context, AcquireRequest) (WorldLease, error) {
	return WorldLease{}, nil
}
func (s *timingRuntimeStub) Exec(context.Context, ExecRequest) (ExecReceipt, error) {
	return ExecReceipt{}, s.execErr
}
func (s *timingRuntimeStub) Spawn(context.Context, SpawnRequest) (WorldLease, error) {
	return WorldLease{}, nil
}
func (s *timingRuntimeStub) Checkpoint(context.Context, CheckpointRequest) (CheckpointReceipt, error) {
	return CheckpointReceipt{}, nil
}
func (s *timingRuntimeStub) Resume(context.Context, ResumeRequest) (WorldLease, error) {
	return WorldLease{}, nil
}
func (s *timingRuntimeStub) RegisterService(context.Context, ServiceRequest) (ServiceReceipt, error) {
	return ServiceReceipt{}, nil
}
func (s *timingRuntimeStub) Capabilities(context.Context, CapabilityRequest) (CapabilityReport, error) {
	return CapabilityReport{}, nil
}
func (s *timingRuntimeStub) Release(context.Context, ReleaseRequest) error {
	return nil
}

func TestTimedRuntimeEmitsOnlySemanticOperationAndDuration(t *testing.T) {
	sink := &timingSinkRecorder{}
	runtime, err := NewTimedRuntime(&timingRuntimeStub{}, sink)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = runtime.Acquire(ctx, AcquireRequest{})
	_, _ = runtime.Exec(ctx, ExecRequest{})
	_, _ = runtime.Spawn(ctx, SpawnRequest{})
	_, _ = runtime.Checkpoint(ctx, CheckpointRequest{})
	_, _ = runtime.Resume(ctx, ResumeRequest{})

	want := []TimingOperation{TimingAcquire, TimingExec, TimingSpawn, TimingCheckpoint, TimingResume}
	if len(sink.samples) != len(want) {
		t.Fatalf("samples=%d want=%d", len(sink.samples), len(want))
	}
	for i, sample := range sink.samples {
		if sample.Operation != want[i] {
			t.Fatalf("sample[%d].operation=%q want=%q", i, sample.Operation, want[i])
		}
		if sample.Duration < 0 || sample.Duration > time.Minute {
			t.Fatalf("sample[%d].duration=%s", i, sample.Duration)
		}
	}

	typ := reflect.TypeOf(TimingSample{})
	if typ.NumField() != 2 || typ.Field(0).Name != "Operation" || typ.Field(1).Name != "Duration" {
		t.Fatalf("timing sample surface expanded: %v", typ)
	}
}

func TestTimedRuntimeDoesNotObserveUntargetedSemanticOperations(t *testing.T) {
	sink := &timingSinkRecorder{}
	runtime, err := NewTimedRuntime(&timingRuntimeStub{}, sink)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = runtime.Enter(ctx, EnterRequest{})
	_ = runtime.Release(ctx, ReleaseRequest{})
	_, _ = runtime.RegisterService(ctx, ServiceRequest{})
	_, _ = runtime.Capabilities(ctx, CapabilityRequest{})
	if len(sink.samples) != 0 {
		t.Fatalf("untargeted operations emitted timing samples: %+v", sink.samples)
	}
}

func TestTimedRuntimeRecordsFailedTimedOperationWithoutErrorPayload(t *testing.T) {
	wantErr := errors.New("provider diagnostics must not enter metrics")
	sink := &timingSinkRecorder{}
	runtime, err := NewTimedRuntime(&timingRuntimeStub{execErr: wantErr}, sink)
	if err != nil {
		t.Fatal(err)
	}
	_, gotErr := runtime.Exec(context.Background(), ExecRequest{})
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("exec error=%v", gotErr)
	}
	if len(sink.samples) != 1 || sink.samples[0].Operation != TimingExec {
		t.Fatalf("samples=%+v", sink.samples)
	}
}

func TestNewTimedRuntimeFailsClosedOnMissingDependency(t *testing.T) {
	if _, err := NewTimedRuntime(nil, &timingSinkRecorder{}); !errors.Is(err, ErrInvalidTimingRuntime) {
		t.Fatalf("nil runtime err=%v", err)
	}
	if _, err := NewTimedRuntime(&timingRuntimeStub{}, nil); !errors.Is(err, ErrInvalidTimingRuntime) {
		t.Fatalf("nil sink err=%v", err)
	}
}
