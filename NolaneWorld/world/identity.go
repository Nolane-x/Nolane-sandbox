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
	current := s.epoch
	s.mu.RUnlock()
	if epoch < current {
		return ErrStaleEpoch
	}
	if epoch > current {
		return ErrInvalidEpoch
	}
	return nil
}
