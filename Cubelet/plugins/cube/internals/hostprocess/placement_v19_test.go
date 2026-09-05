package hostprocess

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePlacementRecorder struct {
	calls     int
	sandboxID string
	group     string
	pid       uint32
	placedAt  time.Time
	err       error
}

func (f *fakePlacementRecorder) RecordHostProcessPlacement(_ context.Context, sandboxID, group string, pid uint32, placedAt time.Time) error {
	f.calls++
	f.sandboxID = sandboxID
	f.group = group
	f.pid = pid
	f.placedAt = placedAt
	return f.err
}

func TestAddProcAndRecordRecordsOnlyAfterSuccessfulPlacement(t *testing.T) {
	now := time.Date(2026, 9, 5, 6, 0, 0, 123456789, time.UTC)
	recorder := &fakePlacementRecorder{}
	addCalls := 0
	err := AddProcAndRecord(
		context.Background(), "sandbox-a", "/cube/42", 1234,
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
		t.Fatalf("AddProcAndRecord: %v", err)
	}
	if addCalls != 1 || recorder.calls != 1 {
		t.Fatalf("add calls=%d recorder calls=%d", addCalls, recorder.calls)
	}
	if recorder.sandboxID != "sandbox-a" || recorder.group != "/cube/42" || recorder.pid != 1234 || !recorder.placedAt.Equal(now) {
		t.Fatalf("unexpected recorder call: %+v", recorder)
	}
}

func TestAddProcAndRecordNeverRecordsFailedPlacement(t *testing.T) {
	recorder := &fakePlacementRecorder{}
	want := errors.New("add proc failed")
	err := AddProcAndRecord(
		context.Background(), "sandbox-a", "/cube/42", 1234,
		func(string, uint64) error { return want }, recorder, time.Now,
	)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
	if recorder.calls != 0 {
		t.Fatalf("recorder called %d times after failed AddProc", recorder.calls)
	}
}

func TestAddProcAndRecordTreatsEvidenceFailureAsObservational(t *testing.T) {
	recorder := &fakePlacementRecorder{err: errors.New("proc evidence unavailable")}
	err := AddProcAndRecord(
		context.Background(), "sandbox-a", "/cube/42", 1234,
		func(string, uint64) error { return nil }, recorder, time.Now,
	)
	if err != nil {
		t.Fatalf("observational identity error leaked into execution path: %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("recorder calls=%d", recorder.calls)
	}
}

func TestAddProcAndRecordRejectsInvalidAuthorityInputsBeforeAddProc(t *testing.T) {
	cases := []struct {
		name      string
		sandboxID string
		group     string
		pid       uint32
	}{
		{"blank sandbox", "", "/cube/42", 1234},
		{"whitespace sandbox", " sandbox-a", "/cube/42", 1234},
		{"blank group", "sandbox-a", "", 1234},
		{"relative group", "sandbox-a", "cube/42", 1234},
		{"unclean group", "sandbox-a", "/cube/../42", 1234},
		{"zero pid", "sandbox-a", "/cube/42", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addCalls := 0
			err := AddProcAndRecord(context.Background(), tc.sandboxID, tc.group, tc.pid,
				func(string, uint64) error { addCalls++; return nil }, &fakePlacementRecorder{}, time.Now)
			if err == nil {
				t.Fatal("invalid authority input must fail closed")
			}
			if addCalls != 0 {
				t.Fatalf("AddProc called %d times for invalid input", addCalls)
			}
		})
	}
}
