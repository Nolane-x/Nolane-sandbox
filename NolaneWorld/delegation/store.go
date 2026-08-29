package delegation

import "sync"

type Resolver interface {
	Lookup(ID) (GrantState, error)
}

type Controller interface {
	Resolver
	Issue(Grant) error
	Revoke(ID) error
}

type MemoryStore struct {
	mu     sync.RWMutex
	states map[ID]GrantState
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{states: make(map[ID]GrantState)}
}

func (s *MemoryStore) Issue(in Grant) error {
	if s == nil {
		return ErrInvalidGrant
	}
	g, err := canonicalGrant(in)
	if err != nil {
		return err
	}
	digest, _ := GrantDigest(g)
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.states[g.ID]; ok {
		priorDigest, err := GrantDigest(prior.Grant)
		if err != nil {
			return ErrInvalidGrant
		}
		if priorDigest != digest {
			return ErrGrantCollision
		}
		return nil
	}
	s.states[g.ID] = GrantState{Grant: cloneGrant(g)}
	return nil
}

func (s *MemoryStore) Revoke(id ID) error {
	if s == nil || !strict(string(id), 256) {
		return ErrInvalidGrant
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[id]
	if !ok {
		return ErrDelegationNotFound
	}
	if state.Revoked {
		return ErrAlreadyRevoked
	}
	state.Revoked = true
	s.states[id] = state
	return nil
}

func (s *MemoryStore) Lookup(id ID) (GrantState, error) {
	if s == nil || !strict(string(id), 256) {
		return GrantState{}, ErrDelegationNotFound
	}
	s.mu.RLock()
	state, ok := s.states[id]
	s.mu.RUnlock()
	if !ok {
		return GrantState{}, ErrDelegationNotFound
	}
	state.Grant = cloneGrant(state.Grant)
	return state, nil
}

func cloneGrant(g Grant) Grant {
	out := g
	out.Operations = append([]Operation(nil), g.Operations...)
	return out
}
