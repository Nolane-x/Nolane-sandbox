package control

import (
	"context"
	"errors"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrInvalidManager = errors.New("control: invalid manager")
	ErrWorldExists    = errors.New("control: world already exists")
	ErrWorldNotFound  = errors.New("control: world not found")
	ErrWorldNotReady  = errors.New("control: world not ready")
	ErrWorldClosed    = errors.New("control: world closed")
)

type entry struct {
	mu        sync.Mutex
	state     *world.State
	handle    substrate.Handle
	ready     bool
	terminal  bool
	destroyed bool
}

type Manager struct {
	substrate substrate.SandboxSubstrate
	mu        sync.RWMutex
	worlds    map[world.ID]*entry
}

func NewManager(s substrate.SandboxSubstrate) (*Manager, error) {
	if s == nil {
		return nil, ErrInvalidManager
	}
	return &Manager{substrate: s, worlds: make(map[world.ID]*entry)}, nil
}

func (m *Manager) Create(ctx context.Context, id world.ID) (substrate.Handle, error) {
	if m == nil || m.substrate == nil || id == "" {
		return "", ErrInvalidManager
	}
	st, err := world.NewState(id)
	if err != nil {
		return "", err
	}
	e := &entry{state: st}

	m.mu.Lock()
	if _, exists := m.worlds[id]; exists {
		m.mu.Unlock()
		return "", ErrWorldExists
	}
	m.worlds[id] = e
	m.mu.Unlock()

	h, err := m.substrate.Create(ctx, id)
	if err != nil {
		m.mu.Lock()
		if m.worlds[id] == e {
			delete(m.worlds, id)
		}
		m.mu.Unlock()
		return "", err
	}
	if h == "" {
		m.mu.Lock()
		if m.worlds[id] == e {
			delete(m.worlds, id)
		}
		m.mu.Unlock()
		return "", ErrWorldNotReady
	}
	e.mu.Lock()
	e.handle = h
	e.ready = true
	e.mu.Unlock()
	return h, nil
}

func (m *Manager) AuthorityState(id world.ID) (*world.State, bool) {
	e, ok := m.lookup(id)
	if !ok {
		return nil, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return nil, false
	}
	return e.state, true
}

func (m *Manager) Pause(ctx context.Context, id world.ID) error {
	e, err := m.activeEntry(id)
	if err != nil {
		return err
	}
	defer e.mu.Unlock()
	return m.substrate.Pause(ctx, e.handle)
}

func (m *Manager) Resume(ctx context.Context, id world.ID) error {
	e, err := m.activeEntry(id)
	if err != nil {
		return err
	}
	defer e.mu.Unlock()
	return m.substrate.Resume(ctx, e.handle)
}

func (m *Manager) Snapshot(ctx context.Context, id world.ID) (substrate.Snapshot, error) {
	e, err := m.activeEntry(id)
	if err != nil {
		return "", err
	}
	defer e.mu.Unlock()
	return m.substrate.Snapshot(ctx, e.handle)
}

func (m *Manager) Rollback(ctx context.Context, id world.ID, snap substrate.Snapshot) error {
	if snap == "" {
		return ErrWorldNotReady
	}
	e, err := m.activeEntry(id)
	if err != nil {
		return err
	}
	defer e.mu.Unlock()

	e.state.AdvanceEpoch()
	return m.substrate.Rollback(ctx, e.handle, snap)
}

func (m *Manager) Destroy(ctx context.Context, id world.ID) error {
	e, ok := m.lookup(id)
	if !ok {
		return ErrWorldNotFound
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return ErrWorldNotReady
	}
	if !e.terminal {
		e.state.Close()
		e.terminal = true
	}
	if e.destroyed {
		return nil
	}
	if err := m.substrate.Destroy(ctx, e.handle); err != nil {
		return err
	}
	e.destroyed = true
	return nil
}

func (m *Manager) Clone(ctx context.Context, source world.ID, snap substrate.Snapshot, child world.ID) (substrate.Handle, error) {
	if child == "" || snap == "" {
		return "", ErrInvalidManager
	}
	src, err := m.activeEntry(source)
	if err != nil {
		return "", err
	}
	defer src.mu.Unlock()

	childState, err := world.NewState(child)
	if err != nil {
		return "", err
	}
	childEntry := &entry{state: childState}

	m.mu.Lock()
	if _, exists := m.worlds[child]; exists {
		m.mu.Unlock()
		return "", ErrWorldExists
	}
	m.worlds[child] = childEntry
	m.mu.Unlock()

	h, err := m.substrate.Clone(ctx, src.handle, snap, child)
	if err != nil || h == "" {
		m.mu.Lock()
		if m.worlds[child] == childEntry {
			delete(m.worlds, child)
		}
		m.mu.Unlock()
		if err != nil {
			return "", err
		}
		return "", ErrWorldNotReady
	}
	childEntry.mu.Lock()
	childEntry.handle = h
	childEntry.ready = true
	childEntry.mu.Unlock()
	return h, nil
}

func (m *Manager) activeEntry(id world.ID) (*entry, error) {
	e, ok := m.lookup(id)
	if !ok {
		return nil, ErrWorldNotFound
	}
	e.mu.Lock()
	if !e.ready {
		e.mu.Unlock()
		return nil, ErrWorldNotReady
	}
	if e.terminal || e.state.Closed() {
		e.mu.Unlock()
		return nil, ErrWorldClosed
	}
	return e, nil
}

func (m *Manager) lookup(id world.ID) (*entry, bool) {
	if m == nil || id == "" {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.worlds[id]
	return e, ok
}
