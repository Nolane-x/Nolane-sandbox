package cubebox

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeHostProcessPlacementRecorder struct {
	calls     int
	sandboxID string
	group     string
	pid       uint32
	placedAt  time.Time
	err       error
}

func (f *fakeHostProcessPlacementRecorder) RecordHostProcessPlacement(_ context.Context, sandboxID, group string, pid uint32, placedAt time.Time) error {
	f.calls++
	f.sandboxID = sandboxID
	f.group = group
	f.pid = pid
	f.placedAt = placedAt
	return f.err
}

func TestHostProcessPlacementRecorderRunsOnlyAfterAddProcSuccess(t *testing.T) {
	now := time.Date(2026, 9, 5, 6, 0, 0, 123456789, time.UTC)
	recorder := &fakeHostProcessPlacementRecorder{}
	addCalls := 0
	err := setCgroupWithPlacementRecorder(
		context.Background(), "sandbox-a", 1234, "/cube/42",
		func(group string, pid uint64) error {
			addCalls++
			if group != "/cube/42" || pid != 1234 {
				t.Fatalf("unexpected AddProc arguments: %q %d", group, pid)
			}
			return nil
		},
		recorder,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("set cgroup: %v", err)
	}
	if addCalls != 1 || recorder.calls != 1 {
		t.Fatalf("add calls=%d recorder calls=%d", addCalls, recorder.calls)
	}
	if recorder.sandboxID != "sandbox-a" || recorder.group != "/cube/42" || recorder.pid != 1234 || !recorder.placedAt.Equal(now) {
		t.Fatalf("unexpected recorder call: %+v", recorder)
	}
}

func TestHostProcessPlacementRecorderNeverRunsAfterAddProcFailure(t *testing.T) {
	recorder := &fakeHostProcessPlacementRecorder{}
	want := errors.New("add proc failed")
	err := setCgroupWithPlacementRecorder(
		context.Background(), "sandbox-a", 1234, "/cube/42",
		func(string, uint64) error { return want },
		recorder,
		time.Now,
	)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
	if recorder.calls != 0 {
		t.Fatalf("recorder called %d times after failed AddProc", recorder.calls)
	}
}

func TestHostProcessEvidenceFailureDoesNotTurnSuccessfulAddProcIntoExecutionFailure(t *testing.T) {
	recorder := &fakeHostProcessPlacementRecorder{err: errors.New("proc evidence unavailable")}
	err := setCgroupWithPlacementRecorder(
		context.Background(), "sandbox-a", 1234, "/cube/42",
		func(string, uint64) error { return nil },
		recorder,
		time.Now,
	)
	if err != nil {
		t.Fatalf("observational identity error leaked into execution path: %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("recorder calls=%d", recorder.calls)
	}
}
