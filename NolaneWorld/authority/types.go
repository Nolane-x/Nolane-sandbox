package authority

import (
	"context"
	"errors"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Intent struct {
	WorldID        world.ID
	AuthorityEpoch world.Epoch
	ActionID       string
	Kind           string
	Target         string
	Payload        []byte
}

type Decision uint8

const (
	Deny Decision = iota
	Allow
)

type ActionStatus uint8

const (
	ActionMissing ActionStatus = iota
	ActionPending
	ActionCompleted
)

type Receipt struct {
	WorldID        world.ID
	AuthorityEpoch world.Epoch
	ActionID       string
	RequestDigest  string
	EffectDigest   string
	CompletedAt    time.Time
}

type Policy interface {
	Evaluate(context.Context, Intent) (Decision, error)
}

type Executor interface {
	Execute(context.Context, Intent) ([]byte, error)
}

var (
	ErrInvalidAction         = errors.New("authority: invalid action")
	ErrActionCollision       = errors.New("authority: action id collision")
	ErrActionUncertain       = errors.New("authority: action outcome uncertain")
	ErrLedgerCorrupt         = errors.New("authority: ledger corrupt")
	ErrLedgerClosed          = errors.New("authority: ledger closed")
	ErrLedgerLocked          = errors.New("authority: ledger locked")
	ErrLedgerLockUnsupported = errors.New("authority: ledger locking unsupported")
	ErrDenied                = errors.New("authority: denied")
	ErrPolicyFailure         = errors.New("authority: policy failure")
	ErrExecutionFailure      = errors.New("authority: execution failure")
)
