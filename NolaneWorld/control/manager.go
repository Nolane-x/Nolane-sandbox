package control

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

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

type RecoveryIssueKind string

const RecoveryIssuePossibleOrphan RecoveryIssueKind = "possible-orphan"

type RecoveryIssue struct {
	Kind    RecoveryIssueKind
	WorldID world.ID
	Detail  string
}

type AuthorityFactory interface {
	Create(world.ID) (world.AuthorityControl, error)
	Open(world.ID) (world.AuthorityControl, error)
}

type memoryAuthorityFactory struct{}

func (memoryAuthorityFactory) Create(id world.ID) (world.AuthorityControl, error) {
	return world.NewState(id)
}
func (memoryAuthorityFactory) Open(world.ID) (world.AuthorityControl, error) {
	return nil, world.ErrStateNotFound
}

type entry struct {
	mu        sync.Mutex
	state     world.AuthorityControl
	handle    substrate.Handle
	ready     bool
	terminal  bool
	destroyed bool
	fence     atomic.Bool
}

type authorityView struct {
	manager *Manager
	state   world.AuthorityState
	fence   *atomic.Bool
}

func (v *authorityView) managerClosed() bool {
	if v == nil || v.manager == nil {
		return true
	}
	v.manager.mu.RLock()
	defer v.manager.mu.RUnlock()
	return v.manager.closed
}

func (v *authorityView) ID() world.ID {
	if v == nil || v.manager == nil || v.state == nil || v.fence == nil {
		return ""
	}
	v.manager.opMu.RLock()
	defer v.manager.opMu.RUnlock()
	if v.managerClosed() {
		return ""
	}
	return v.state.ID()
}

func (v *authorityView) CurrentEpoch() world.Epoch {
	if v == nil || v.manager == nil || v.state == nil || v.fence == nil {
		return 0
	}
	v.manager.opMu.RLock()
	defer v.manager.opMu.RUnlock()
	if v.managerClosed() {
		return 0
	}
	return v.state.CurrentEpoch()
}

func (v *authorityView) ValidateEpoch(epoch world.Epoch) error {
	if v == nil || v.manager == nil || v.state == nil || v.fence == nil {
		return world.ErrInvalidWorld
	}
	v.manager.opMu.RLock()
	defer v.manager.opMu.RUnlock()
	if v.managerClosed() || v.fence.Load() {
		return world.ErrClosedWorld
	}
	return v.state.ValidateEpoch(epoch)
}

func (v *authorityView) WithEpoch(epoch world.Epoch, fn func() error) error {
	if v == nil || v.manager == nil || v.state == nil || v.fence == nil {
		return world.ErrInvalidWorld
	}
	v.manager.opMu.RLock()
	defer v.manager.opMu.RUnlock()
	if v.managerClosed() || v.fence.Load() {
		return world.ErrClosedWorld
	}
	return v.state.WithEpoch(epoch, fn)
}

type Manager struct {
	substrate substrate.SandboxSubstrate
	factory   AuthorityFactory
	catalog   LifecycleCatalog
	opMu      sync.RWMutex
	mu        sync.RWMutex
	worlds    map[world.ID]*entry
	closed    bool
}

func NewManager(s substrate.SandboxSubstrate) (*Manager, error) {
	if s == nil {
		return nil, ErrInvalidManager
	}
	return &Manager{substrate: s, factory: memoryAuthorityFactory{}, worlds: make(map[world.ID]*entry)}, nil
}

func NewPersistentManager(s substrate.SandboxSubstrate, factory AuthorityFactory, catalog LifecycleCatalog) (*Manager, []RecoveryIssue, error) {
	if s == nil || factory == nil || catalog == nil {
		return nil, nil, ErrInvalidManager
	}
	m := &Manager{substrate: s, factory: factory, catalog: catalog, worlds: make(map[world.ID]*entry)}
	issues, err := m.recover()
	if err != nil {
		_ = m.Close()
		return nil, nil, err
	}
	return m, issues, nil
}

func (m *Manager) recover() ([]RecoveryIssue, error) {
	entries := m.catalog.Entries()
	issues := make([]RecoveryIssue, 0)
	for id, rec := range entries {
		state, err := m.factory.Open(id)
		if err != nil {
			return nil, err
		}
		e := &entry{state: state, handle: rec.Handle, ready: true}
		switch rec.Phase {
		case PhaseCreating:
			if _, err := state.CloseAuthority(); err != nil {
				_ = state.Release()
				return nil, err
			}
			if err := m.catalog.Terminal(id); err != nil {
				_ = state.Release()
				return nil, err
			}
			e.terminal = true
			e.fence.Store(true)
			issues = append(issues, RecoveryIssue{Kind: RecoveryIssuePossibleOrphan, WorldID: id, Detail: "creation outcome unknown; authority terminally revoked"})
		case PhaseReady:
			if state.Closed() {
				if err := m.catalog.Terminal(id); err != nil {
					_ = state.Release()
					return nil, err
				}
				e.terminal = true
				e.fence.Store(true)
			}
		case PhaseTerminal:
			if !state.Closed() {
				if _, err := state.CloseAuthority(); err != nil {
					_ = state.Release()
					return nil, err
				}
			}
			e.terminal = true
			e.fence.Store(true)
		case PhaseDestroyed:
			if !state.Closed() {
				if _, err := state.CloseAuthority(); err != nil {
					_ = state.Release()
					return nil, err
				}
			}
			e.terminal = true
			e.fence.Store(true)
			e.destroyed = true
		default:
			_ = state.Release()
			return nil, ErrCatalogCorrupt
		}
		m.worlds[id] = e
	}
	return issues, nil
}

func (m *Manager) Create(ctx context.Context, id world.ID) (substrate.Handle, error) {
	if m == nil {
		return "", ErrInvalidManager
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	if m == nil || m.substrate == nil || m.factory == nil || id == "" {
		return "", ErrInvalidManager
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", ErrInvalidManager
	}
	if _, exists := m.worlds[id]; exists {
		m.mu.Unlock()
		return "", ErrWorldExists
	}
	placeholder := &entry{}
	m.worlds[id] = placeholder
	m.mu.Unlock()

	state, err := m.factory.Create(id)
	if err != nil {
		m.removeIf(id, placeholder)
		return "", err
	}
	placeholder.mu.Lock()
	placeholder.state = state
	placeholder.mu.Unlock()

	if m.catalog != nil {
		if err := m.catalog.BeginCreate(id); err != nil {
			_, closeErr := state.CloseAuthority()
			_ = state.Release()
			m.removeIf(id, placeholder)
			return "", errors.Join(err, closeErr)
		}
	}

	h, err := m.substrate.Create(ctx, id)
	if err != nil || h == "" {
		if m.catalog == nil {
			_ = state.Release()
			m.removeIf(id, placeholder)
			if err != nil {
				return "", err
			}
			return "", ErrWorldNotReady
		}
		_, closeErr := state.CloseAuthority()
		terminalErr := m.catalog.Terminal(id)
		placeholder.mu.Lock()
		placeholder.ready = true
		placeholder.terminal = true
		placeholder.fence.Store(true)
		placeholder.mu.Unlock()
		if err == nil {
			err = ErrWorldNotReady
		}
		return "", errors.Join(err, closeErr, terminalErr)
	}

	if m.catalog != nil {
		if err := m.catalog.Ready(id, h); err != nil {
			_, closeErr := state.CloseAuthority()
			terminalErr := m.catalog.Quarantine(id, h)
			var destroyErr error
			destroyed := false
			if terminalErr == nil {
				destroyErr = m.substrate.Destroy(ctx, h)
				if destroyErr == nil {
					destroyErr = m.catalog.Destroyed(id)
					destroyed = destroyErr == nil
				}
			}
			placeholder.mu.Lock()
			placeholder.handle = h
			placeholder.ready = true
			placeholder.terminal = true
			placeholder.fence.Store(true)
			placeholder.destroyed = destroyed
			placeholder.mu.Unlock()
			return "", errors.Join(err, closeErr, terminalErr, destroyErr)
		}
	}
	placeholder.mu.Lock()
	placeholder.handle = h
	placeholder.ready = true
	placeholder.mu.Unlock()
	return h, nil
}

func (m *Manager) AuthorityState(id world.ID) (world.AuthorityState, bool) {
	if m == nil {
		return nil, false
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	e, ok := m.lookup(id)
	if !ok {
		return nil, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready || e.state == nil {
		return nil, false
	}
	return &authorityView{manager: m, state: e.state, fence: &e.fence}, true
}

func (m *Manager) Pause(ctx context.Context, id world.ID) error {
	if m == nil {
		return ErrInvalidManager
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	e, err := m.activeEntry(id)
	if err != nil {
		return err
	}
	defer e.mu.Unlock()
	return m.substrate.Pause(ctx, e.handle)
}

func (m *Manager) Resume(ctx context.Context, id world.ID) error {
	if m == nil {
		return ErrInvalidManager
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	e, err := m.activeEntry(id)
	if err != nil {
		return err
	}
	defer e.mu.Unlock()
	return m.substrate.Resume(ctx, e.handle)
}

func (m *Manager) Snapshot(ctx context.Context, id world.ID) (substrate.Snapshot, error) {
	if m == nil {
		return "", ErrInvalidManager
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	e, err := m.activeEntry(id)
	if err != nil {
		return "", err
	}
	defer e.mu.Unlock()
	return m.substrate.Snapshot(ctx, e.handle)
}

func (m *Manager) Rollback(ctx context.Context, id world.ID, snap substrate.Snapshot) error {
	if m == nil {
		return ErrInvalidManager
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	if snap == "" {
		return ErrWorldNotReady
	}
	e, err := m.activeEntry(id)
	if err != nil {
		return err
	}
	defer e.mu.Unlock()

	if _, err := e.state.AdvanceAuthority(); err != nil {
		return err
	}
	return m.substrate.Rollback(ctx, e.handle, snap)
}

func (m *Manager) Destroy(ctx context.Context, id world.ID) error {
	if m == nil {
		return ErrInvalidManager
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	e, ok := m.lookup(id)
	if !ok {
		return ErrWorldNotFound
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready || e.state == nil {
		return ErrWorldNotReady
	}

	// For persistent managers, terminal lifecycle truth is the first durable
	// fence. If the authority journal then fails to close, broker-facing
	// authority views still deny immediately and recovery will retry the close
	// before the world can ever become active again.
	if !e.terminal {
		if m.catalog != nil {
			if err := m.catalog.Terminal(id); err != nil {
				return err
			}
			e.terminal = true
			e.fence.Store(true)
			if _, err := e.state.CloseAuthority(); err != nil {
				return err
			}
		} else {
			if _, err := e.state.CloseAuthority(); err != nil {
				return err
			}
			e.terminal = true
			e.fence.Store(true)
		}
	} else if !e.state.Closed() {
		// A previous persistent terminal transition may have succeeded while
		// the authority journal write failed. Retry the authority close before
		// touching the execution substrate.
		if _, err := e.state.CloseAuthority(); err != nil {
			return err
		}
	}
	if e.destroyed {
		return nil
	}
	if e.handle == "" {
		return ErrWorldNotReady
	}
	if err := m.substrate.Destroy(ctx, e.handle); err != nil {
		return err
	}
	if m.catalog != nil {
		if err := m.catalog.Destroyed(id); err != nil {
			return err
		}
	}
	e.destroyed = true
	return nil
}

func (m *Manager) Clone(ctx context.Context, source world.ID, snap substrate.Snapshot, child world.ID) (substrate.Handle, error) {
	if m == nil {
		return "", ErrInvalidManager
	}
	m.opMu.RLock()
	defer m.opMu.RUnlock()
	if child == "" || snap == "" {
		return "", ErrInvalidManager
	}
	src, err := m.activeEntry(source)
	if err != nil {
		return "", err
	}
	defer src.mu.Unlock()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", ErrInvalidManager
	}
	if _, exists := m.worlds[child]; exists {
		m.mu.Unlock()
		return "", ErrWorldExists
	}
	childEntry := &entry{}
	m.worlds[child] = childEntry
	m.mu.Unlock()

	childState, err := m.factory.Create(child)
	if err != nil {
		m.removeIf(child, childEntry)
		return "", err
	}
	childEntry.mu.Lock()
	childEntry.state = childState
	childEntry.mu.Unlock()
	if m.catalog != nil {
		if err := m.catalog.BeginCreate(child); err != nil {
			_, closeErr := childState.CloseAuthority()
			_ = childState.Release()
			m.removeIf(child, childEntry)
			return "", errors.Join(err, closeErr)
		}
	}

	h, cloneErr := m.substrate.Clone(ctx, src.handle, snap, child)
	if cloneErr != nil || h == "" {
		if m.catalog == nil {
			_ = childState.Release()
			m.removeIf(child, childEntry)
			if cloneErr != nil {
				return "", cloneErr
			}
			return "", ErrWorldNotReady
		}
		_, closeErr := childState.CloseAuthority()
		terminalErr := m.catalog.Terminal(child)
		childEntry.mu.Lock()
		childEntry.ready = true
		childEntry.terminal = true
		childEntry.fence.Store(true)
		childEntry.mu.Unlock()
		if cloneErr == nil {
			cloneErr = ErrWorldNotReady
		}
		return "", errors.Join(cloneErr, closeErr, terminalErr)
	}
	if m.catalog != nil {
		if err := m.catalog.Ready(child, h); err != nil {
			_, closeErr := childState.CloseAuthority()
			terminalErr := m.catalog.Quarantine(child, h)
			var destroyErr error
			destroyed := false
			if terminalErr == nil {
				destroyErr = m.substrate.Destroy(ctx, h)
				if destroyErr == nil {
					destroyErr = m.catalog.Destroyed(child)
					destroyed = destroyErr == nil
				}
			}
			childEntry.mu.Lock()
			childEntry.handle = h
			childEntry.ready = true
			childEntry.terminal = true
			childEntry.fence.Store(true)
			childEntry.destroyed = destroyed
			childEntry.mu.Unlock()
			return "", errors.Join(err, closeErr, terminalErr, destroyErr)
		}
	}
	childEntry.mu.Lock()
	childEntry.handle = h
	childEntry.ready = true
	childEntry.mu.Unlock()
	return h, nil
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	entries := make([]*entry, 0, len(m.worlds))
	for _, e := range m.worlds {
		entries = append(entries, e)
	}
	m.mu.Unlock()

	var errs []error
	for _, e := range entries {
		e.mu.Lock()
		if e.state != nil {
			errs = append(errs, e.state.Release())
		}
		e.mu.Unlock()
	}
	if m.catalog != nil {
		errs = append(errs, m.catalog.Close())
	}
	return errors.Join(errs...)
}

func (m *Manager) activeEntry(id world.ID) (*entry, error) {
	e, ok := m.lookup(id)
	if !ok {
		return nil, ErrWorldNotFound
	}
	e.mu.Lock()
	if !e.ready || e.state == nil {
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
	if m.closed {
		return nil, false
	}
	e, ok := m.worlds[id]
	return e, ok
}

func (m *Manager) removeIf(id world.ID, target *entry) {
	m.mu.Lock()
	if m.worlds[id] == target {
		delete(m.worlds, id)
	}
	m.mu.Unlock()
}
