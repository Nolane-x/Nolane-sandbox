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

type noEffectError struct{ err error }

func (e noEffectError) Error() string { return e.err.Error() }
func (e noEffectError) Unwrap() error { return e.err }
func (e noEffectError) definitelyNoEffect() bool { return true }

// MarkNoEffect is a host-only proof annotation for failures known to occur
// before entry into an external side effect. Ledgers may remove the pending
// transition only when this explicit marker is present; ordinary sentinels do
// not imply no-effect by themselves.
func MarkNoEffect(err error) error {
	if err == nil {
		return nil
	}
	var marked interface{ definitelyNoEffect() bool }
	if errors.As(err, &marked) && marked.definitelyNoEffect() {
		return err
	}
	return noEffectError{err: err}
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
