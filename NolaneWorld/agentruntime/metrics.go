package agentruntime

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidTimingRuntime = errors.New("agentruntime: invalid timing runtime")

type TimingOperation string

const (
	TimingAcquire    TimingOperation = "acquire"
	TimingExec       TimingOperation = "exec"
	TimingSpawn      TimingOperation = "spawn"
	TimingCheckpoint TimingOperation = "checkpoint"
	TimingResume     TimingOperation = "resume"
)

type TimingSample struct {
	Operation TimingOperation
	Duration  time.Duration
}

type TimingSink interface {
	ObserveTiming(TimingSample)
}

type TimedRuntime struct {
	inner Runtime
	sink  TimingSink
}

func NewTimedRuntime(inner Runtime, sink TimingSink) (*TimedRuntime, error) {
	if inner == nil || sink == nil {
		return nil, ErrInvalidTimingRuntime
	}
	return &TimedRuntime{inner: inner, sink: sink}, nil
}

func (r *TimedRuntime) Enter(ctx context.Context, req EnterRequest) (Session, error) {
	return r.inner.Enter(ctx, req)
}

func (r *TimedRuntime) Acquire(ctx context.Context, req AcquireRequest) (lease WorldLease, err error) {
	started := time.Now()
	defer r.observe(TimingAcquire, started)
	return r.inner.Acquire(ctx, req)
}

func (r *TimedRuntime) Exec(ctx context.Context, req ExecRequest) (receipt ExecReceipt, err error) {
	started := time.Now()
	defer r.observe(TimingExec, started)
	return r.inner.Exec(ctx, req)
}

func (r *TimedRuntime) Spawn(ctx context.Context, req SpawnRequest) (lease WorldLease, err error) {
	started := time.Now()
	defer r.observe(TimingSpawn, started)
	return r.inner.Spawn(ctx, req)
}

func (r *TimedRuntime) Checkpoint(ctx context.Context, req CheckpointRequest) (receipt CheckpointReceipt, err error) {
	started := time.Now()
	defer r.observe(TimingCheckpoint, started)
	return r.inner.Checkpoint(ctx, req)
}

func (r *TimedRuntime) Resume(ctx context.Context, req ResumeRequest) (lease WorldLease, err error) {
	started := time.Now()
	defer r.observe(TimingResume, started)
	return r.inner.Resume(ctx, req)
}

func (r *TimedRuntime) RegisterService(ctx context.Context, req ServiceRequest) (ServiceReceipt, error) {
	return r.inner.RegisterService(ctx, req)
}

func (r *TimedRuntime) Capabilities(ctx context.Context, req CapabilityRequest) (CapabilityReport, error) {
	return r.inner.Capabilities(ctx, req)
}

func (r *TimedRuntime) Release(ctx context.Context, req ReleaseRequest) error {
	return r.inner.Release(ctx, req)
}

func (r *TimedRuntime) observe(operation TimingOperation, started time.Time) {
	duration := time.Since(started)
	if duration < 0 {
		duration = 0
	}
	r.sink.ObserveTiming(TimingSample{Operation: operation, Duration: duration})
}

var _ Runtime = (*TimedRuntime)(nil)
