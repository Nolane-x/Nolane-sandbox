package substrate

import (
	"context"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Handle string
type Snapshot string

type SandboxSubstrate interface {
	Create(context.Context, world.ID) (Handle, error)
	Destroy(context.Context, Handle) error
	Pause(context.Context, Handle) error
	Resume(context.Context, Handle) error
	Snapshot(context.Context, Handle) (Snapshot, error)
	Rollback(context.Context, Handle, Snapshot) error
	Clone(context.Context, Handle, Snapshot, world.ID) (Handle, error)
}
