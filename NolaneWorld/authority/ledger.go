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

type MemoryLedger struct {
	mu       sync.RWMutex
	receipts map[ledgerKey]Receipt
	locks    [64]sync.Mutex
}

func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{receipts: make(map[ledgerKey]Receipt)}
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
	prior, ok := l.receipts[key]
	l.mu.RUnlock()
	if ok {
		if prior.RequestDigest != requestDigest {
			return Receipt{}, ErrActionCollision
		}
		return prior, nil
	}

	receipt, err := fn()
	if err != nil {
		return Receipt{}, err
	}
	if receipt.WorldID != worldID || receipt.ActionID != actionID || receipt.RequestDigest != requestDigest {
		return Receipt{}, ErrExecutionFailure
	}

	l.mu.Lock()
	l.receipts[key] = receipt
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
	prior, ok := l.receipts[key]
	l.mu.RUnlock()
	if !ok {
		return ActionMissing, Receipt{}, nil
	}
	if prior.RequestDigest != requestDigest {
		return ActionMissing, Receipt{}, ErrActionCollision
	}
	return ActionCompleted, prior, nil
}

func shardFor(key ledgerKey) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key.worldID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key.actionID))
	return h.Sum32() % 64
}
