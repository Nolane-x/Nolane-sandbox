package world

import (
	"errors"
	"testing"
)

func TestWorldStateStartsAtEpochOneAndOnlyAdvances(t *testing.T) {
	state, err := NewState(ID("world-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := state.CurrentEpoch(); got != 1 {
		t.Fatalf("initial epoch=%d", got)
	}
	if got := state.AdvanceEpoch(); got != 2 {
		t.Fatalf("advanced epoch=%d", got)
	}
	if got := state.AdvanceEpoch(); got != 3 {
		t.Fatalf("advanced epoch=%d", got)
	}
	if err := state.ValidateEpoch(3); err != nil {
		t.Fatalf("current epoch rejected: %v", err)
	}
}

func TestWorldStateRejectsInvalidStaleAndFutureEpochs(t *testing.T) {
	if _, err := NewState(""); !errors.Is(err, ErrInvalidWorld) {
		t.Fatalf("empty ID: %v", err)
	}
	state, _ := NewState(ID("world-1"))
	state.AdvanceEpoch()
	if err := state.ValidateEpoch(0); !errors.Is(err, ErrInvalidEpoch) {
		t.Fatalf("epoch 0: %v", err)
	}
	if err := state.ValidateEpoch(1); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("stale: %v", err)
	}
	if err := state.ValidateEpoch(3); !errors.Is(err, ErrInvalidEpoch) {
		t.Fatalf("future: %v", err)
	}
}
