package world

import (
	"errors"
	"testing"
)

func TestStateImplementsHostAuthorityControl(t *testing.T) {
	var _ AuthorityControl = (*State)(nil)
	s, err := NewState("w1")
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.AdvanceAuthority()
	if err != nil || e != 2 {
		t.Fatalf("advance=(%d,%v)", e, err)
	}
	e, err = s.CloseAuthority()
	if err != nil || e != 3 {
		t.Fatalf("close=(%d,%v)", e, err)
	}
	if _, err := s.AdvanceAuthority(); !errors.Is(err, ErrClosedWorld) {
		t.Fatalf("advance closed=%v", err)
	}
	if err := s.Release(); err != nil {
		t.Fatal(err)
	}
}
