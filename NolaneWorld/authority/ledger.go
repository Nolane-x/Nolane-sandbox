package authority

import (
	"hash/fnv"
	"sync"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Ledger interface {
	ExecuteOnce(world.ID, string, string, func() (Receipt, error)) (Receipt, error)
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

func shardFor(key ledgerKey) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key.worldID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key.actionID))
	return h.Sum32() % 64
}
