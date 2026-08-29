package world

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloseTerminallyRevokesWorld(t *testing.T) {
	s, err := NewState("w1")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Close(); got != 2 {
		t.Fatalf("Close epoch=%d want 2", got)
	}
	if got := s.Close(); got != 2 {
		t.Fatalf("second Close epoch=%d want 2", got)
	}
	if !s.Closed() {
		t.Fatal("world must be closed")
	}
	if err := s.ValidateEpoch(2); !errors.Is(err, ErrClosedWorld) {
		t.Fatalf("ValidateEpoch error=%v want ErrClosedWorld", err)
	}
	var called atomic.Bool
	if err := s.WithEpoch(2, func() error { called.Store(true); return nil }); !errors.Is(err, ErrClosedWorld) {
		t.Fatalf("WithEpoch error=%v want ErrClosedWorld", err)
	}
	if called.Load() {
		t.Fatal("closed world executed authority callback")
	}
	if got := s.AdvanceEpoch(); got != 2 {
		t.Fatalf("AdvanceEpoch after close=%d want stable 2", got)
	}
}

func TestCloseLinearizesAgainstInFlightAuthority(t *testing.T) {
	s, _ := NewState("w1")
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.WithEpoch(1, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()

	select {
	case <-closed:
		t.Fatal("Close returned while old-epoch authority callback was in flight")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not complete")
	}
	if err := s.ValidateEpoch(s.CurrentEpoch()); !errors.Is(err, ErrClosedWorld) {
		t.Fatalf("post-close validation=%v want ErrClosedWorld", err)
	}
}
