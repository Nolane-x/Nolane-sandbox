package authority

import (
	"hash/fnv"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Ledger interface {
	ExecuteOnce(world.ID, string, string, func() (Receipt, error)) (Receipt, error)
}

// InspectableLedger adds read-only action-state inspection without widening
// Ledger execution authority. Callers use it for reconciliation, never to
// mutate or replay an action.
type InspectableLedger interface {
	Ledger
	Status(world.ID, string, string) (ActionStatus, Receipt, error)
}

// ResolvingLedger may close an already-pending uncertain action after an
// independent reconciliation proves that the external effect occurred.
type ResolvingLedger interface {
	InspectableLedger
	Resolve(world.ID, string, string, Receipt) error
}

type ledgerKey struct {
	worldID  world.ID
	actionID string
}

type memoryEntry struct {
	requestDigest string
	pending       bool
	receipt       Receipt
}

type MemoryLedger struct {
	mu      sync.RWMutex
	entries map[ledgerKey]memoryEntry
	locks   [64]sync.Mutex
}

func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{entries: make(map[ledgerKey]memoryEntry)}
}

func (l *MemoryLedger) ExecuteOnce(worldID world.ID, actionID, requestDigest string, fn func() (Receipt, error)) (Receipt, error) {
	if l == nil || worldID == "" || actionID == "" || requestDigest == "" || fn == nil {
		return Receipt{}, ErrInvalidAction
	}
	key := ledgerKey{worldID: worldID, actionID: actionID}
	lock := &l.locks[shardFor(key)]
	lock.Lock()
	defer lock.Unlock()

	l.mu.RLock()
	prior, ok := l.entries[key]
	l.mu.RUnlock()
	if ok {
		if prior.requestDigest != requestDigest {
			return Receipt{}, ErrActionCollision
		}
		if prior.pending {
			return Receipt{}, ErrActionUncertain
		}
		return prior.receipt, nil
	}

	// Mark the action pending before entering the effect callback. If the
	// callback returns an error that does not prove "no effect", the pending
	// row remains and a retry is quarantined instead of re-executed.
	l.mu.Lock()
	l.entries[key] = memoryEntry{requestDigest: requestDigest, pending: true}
	l.mu.Unlock()

	receipt, err := fn()
	if err != nil {
		if definitelyNoEffect(err) {
			l.mu.Lock()
			delete(l.entries, key)
			l.mu.Unlock()
		}
		return Receipt{}, err
	}
	if receipt.WorldID != worldID || receipt.ActionID != actionID || receipt.RequestDigest != requestDigest {
		// Invalid success metadata cannot prove that the real-world effect did
		// not happen. Keep the action pending and fail closed.
		return Receipt{}, ErrExecutionFailure
	}

	l.mu.Lock()
	l.entries[key] = memoryEntry{requestDigest: requestDigest, receipt: receipt}
	l.mu.Unlock()
	return receipt, nil
}

func (l *MemoryLedger) Status(worldID world.ID, actionID, requestDigest string) (ActionStatus, Receipt, error) {
	if l == nil || worldID == "" || actionID == "" || requestDigest == "" {
		return ActionMissing, Receipt{}, ErrInvalidAction
	}
	key := ledgerKey{worldID: worldID, actionID: actionID}
	lock := &l.locks[shardFor(key)]
	lock.Lock()
	defer lock.Unlock()
	l.mu.RLock()
	prior, ok := l.entries[key]
	l.mu.RUnlock()
	if !ok {
		return ActionMissing, Receipt{}, nil
	}
	if prior.requestDigest != requestDigest {
		return ActionMissing, Receipt{}, ErrActionCollision
	}
	if prior.pending {
		return ActionPending, Receipt{}, nil
	}
	return ActionCompleted, prior.receipt, nil
}

func shardFor(key ledgerKey) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key.worldID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key.actionID))
	return h.Sum32() % 64
}
