package realm

import (
	"context"
	"errors"
)

var ErrInvalidController = errors.New("realm: invalid controller")

type Controller struct {
	store Store
}

func NewController(store Store) (*Controller, error) {
	if store == nil {
		return nil, ErrInvalidController
	}
	return &Controller{store: store}, nil
}

func (c *Controller) Create(ctx context.Context, spec Spec) (RealmRecord, error) {
	if c == nil || c.store == nil {
		return RealmRecord{}, ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return RealmRecord{}, err
	}
	return c.store.CreateRealm(spec)
}

func (c *Controller) Update(ctx context.Context, id ID, expected uint64, spec Spec) (RealmRecord, error) {
	if c == nil || c.store == nil {
		return RealmRecord{}, ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return RealmRecord{}, err
	}
	return c.store.UpdateRealm(id, expected, spec)
}

func (c *Controller) Close(ctx context.Context, id ID, expected uint64) error {
	if c == nil || c.store == nil {
		return ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.store.CloseRealm(id, expected)
}
