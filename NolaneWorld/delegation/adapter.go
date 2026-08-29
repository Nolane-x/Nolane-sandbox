package delegation

import (
	"context"
	"strings"
)

type AdapterRequest struct {
	WorldID        string
	ActionID       string
	Operation      Operation
	Resource       string
	Payload        []byte
	IdempotencyKey string
}

type Effect struct {
	Evidence []byte
}

type ReconcileResult struct {
	State    ReconcileState
	Evidence []byte
}

type Adapter interface {
	Kind() AdapterKind
	Execute(context.Context, AdapterRequest, Secret) (Effect, error)
	Reconcile(context.Context, AdapterRequest, Secret) (ReconcileResult, error)
}

type Registry struct {
	adapters map[AdapterKind]Adapter
}

func NewRegistry(adapters ...Adapter) (*Registry, error) {
	r := &Registry{adapters: make(map[AdapterKind]Adapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil {
			return nil, ErrAdapterNotFound
		}
		kind := adapter.Kind()
		if err := validateAdapterKind(kind); err != nil {
			return nil, err
		}
		if _, exists := r.adapters[kind]; exists {
			return nil, ErrAdapterCollision
		}
		r.adapters[kind] = adapter
	}
	return r, nil
}

func (r *Registry) Lookup(kind AdapterKind) (Adapter, error) {
	if r == nil {
		return nil, ErrAdapterNotFound
	}
	adapter, ok := r.adapters[kind]
	if !ok {
		return nil, ErrAdapterNotFound
	}
	return adapter, nil
}

func validateAdapterKind(kind AdapterKind) error {
	if !strict(string(kind), 128) {
		return ErrAdapterNotFound
	}
	normalized := strings.ToLower(string(kind))
	switch normalized {
	case "http", "https", "raw-http", "raw_http", "generic-http", "generic_http", "authenticated-http", "authenticated_http":
		return ErrGenericAdapter
	}
	return nil
}
