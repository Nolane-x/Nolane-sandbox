package world

import (
	"errors"
	"sync"
)

type ID string
type Epoch uint64

var (
	ErrInvalidWorld = errors.New("world: invalid world")
	ErrInvalidEpoch = errors.New("world: invalid epoch")
	ErrStaleEpoch   = errors.New("world: stale epoch")
)

type State struct {
	id    ID
	mu    sync.RWMutex
	epoch Epoch
}

func NewState(id ID) (*State, error) {
	if id == "" {
		return nil, ErrInvalidWorld
	}
	return &State{id: id, epoch: 1}, nil
}

func (s *State) ID() ID {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *State) CurrentEpoch() Epoch {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epoch
}

func (s *State) AdvanceEpoch() Epoch {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch++
	return s.epoch
}

func (s *State) ValidateEpoch(epoch Epoch) error {
	if s == nil || s.id == "" {
		return ErrInvalidWorld
	}
	if epoch == 0 {
		return ErrInvalidEpoch
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return validateEpoch(epoch, s.epoch)
}

// WithEpoch linearizes an authority-sensitive operation against epoch
// advancement. AdvanceEpoch cannot return while fn is executing under the
// previously-current epoch, and a caller cannot begin fn after revocation has
// advanced the epoch. fn must not call AdvanceEpoch on this State.
func (s *State) WithEpoch(epoch Epoch, fn func() error) error {
	if s == nil || s.id == "" {
		return ErrInvalidWorld
	}
	if epoch == 0 || fn == nil {
		return ErrInvalidEpoch
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := validateEpoch(epoch, s.epoch); err != nil {
		return err
	}
	return fn()
}

func validateEpoch(epoch, current Epoch) error {
	if epoch < current {
		return ErrStaleEpoch
	}
	if epoch > current {
		return ErrInvalidEpoch
	}
	return nil
}
