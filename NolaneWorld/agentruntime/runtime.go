package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/fabric"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrInvalidRuntime     = errors.New("agentruntime: invalid runtime")
	ErrInvalidRequest     = errors.New("agentruntime: invalid request")
	ErrSessionNotFound    = errors.New("agentruntime: session not found")
	ErrStaleSession       = errors.New("agentruntime: stale session")
	ErrOperationCollision = errors.New("agentruntime: operation collision")
	ErrExecUncertain      = errors.New("agentruntime: execution outcome uncertain")
)

type Runtime interface {
	Enter(context.Context, EnterRequest) (Session, error)
	Acquire(context.Context, AcquireRequest) (WorldLease, error)
	Exec(context.Context, ExecRequest) (ExecReceipt, error)
	Spawn(context.Context, SpawnRequest) (WorldLease, error)
	Checkpoint(context.Context, CheckpointRequest) (CheckpointReceipt, error)
	Resume(context.Context, ResumeRequest) (WorldLease, error)
	RegisterService(context.Context, ServiceRequest) (ServiceReceipt, error)
	Capabilities(context.Context, CapabilityRequest) (CapabilityReport, error)
	Release(context.Context, ReleaseRequest) error
}

type Fabric interface {
	Acquire(context.Context, fabric.AcquireRequest) (fabric.Lease, error)
	Spawn(context.Context, fabric.SpawnRequest) (fabric.Lease, error)
	Release(context.Context, realm.ID, world.ID, uint64) error
	Handle(realm.ID, world.ID, uint64) (substrate.Handle, uint64, error)
	Checkpoint(context.Context, realm.ID, world.ID, uint64) (realm.CheckpointRecord, error)
	Resume(context.Context, realm.CheckpointID, uint64) (fabric.Lease, error)
}

type Session struct {
	ID            realm.SessionID `json:"id"`
	RealmID       realm.ID        `json:"realm_id"`
	RealmRevision uint64          `json:"realm_revision"`
	PolicyDigest  string          `json:"policy_digest"`
}

type WorldLease struct {
	WorldID             world.ID `json:"world_id"`
	Generation          uint64   `json:"generation"`
	RealizationRevision uint64   `json:"realization_revision"`
	ExpiresUnix         int64    `json:"expires_unix"`
}

type EnterRequest struct {
	RealmID          realm.ID
	ExpectedRevision uint64
	PolicyDigest     string
}

type AcquireRequest struct {
	SessionID       realm.SessionID
	RealmRevision   uint64
	WorldID         world.ID
	OperationID     string
	Units           realm.ResourceBudget
	ExpiresUnix     int64
}

type SpawnRequest = AcquireRequest

type ReleaseRequest struct {
	SessionID       realm.SessionID
	RealmRevision   uint64
	WorldID         world.ID
	LeaseGeneration uint64
}

type Service struct {
	store    realm.Store
	fabric   Fabric
	guest    substrate.GuestRuntime
	mu       sync.RWMutex
	sessions map[realm.SessionID]Session
	exec     map[string]ExecReceipt
	seq      atomic.Uint64
}

func New(store realm.Store, fabricRuntime Fabric, guest substrate.GuestRuntime) (*Service, error) {
	if store == nil || fabricRuntime == nil || guest == nil {
		return nil, ErrInvalidRuntime
	}
	return &Service{store: store, fabric: fabricRuntime, guest: guest, sessions: make(map[realm.SessionID]Session), exec: make(map[string]ExecReceipt)}, nil
}

func (s *Service) Enter(ctx context.Context, req EnterRequest) (Session, error) {
	if s == nil || s.store == nil || req.RealmID == "" || req.ExpectedRevision == 0 || req.PolicyDigest == "" {
		return Session{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	rec, ok := s.store.Realm(req.RealmID)
	if !ok {
		return Session{}, realm.ErrRealmNotFound
	}
	if rec.Closed {
		return Session{}, realm.ErrRealmClosed
	}
	if rec.Revision != req.ExpectedRevision {
		return Session{}, ErrStaleSession
	}
	n := s.seq.Add(1)
	raw := fmt.Sprintf("nolane.session.v1\x00%s\x00%d\x00%s\x00%d", req.RealmID, rec.Revision, req.PolicyDigest, n)
	h := sha256.Sum256([]byte(raw))
	session := Session{ID: realm.SessionID("session://" + hex.EncodeToString(h[:])), RealmID: req.RealmID, RealmRevision: rec.Revision, PolicyDigest: req.PolicyDigest}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return session, nil
}

func (s *Service) Acquire(ctx context.Context, req AcquireRequest) (WorldLease, error) {
	sess, err := s.validateSession(req.SessionID, req.RealmRevision)
	if err != nil {
		return WorldLease{}, err
	}
	lease, err := s.fabric.Acquire(ctx, fabric.AcquireRequest{RealmID: sess.RealmID, RealmRevision: sess.RealmRevision, WorldID: req.WorldID, OperationID: req.OperationID, Units: req.Units, ExpiresUnix: req.ExpiresUnix})
	if err != nil {
		return WorldLease{}, err
	}
	return projectLease(lease), nil
}

func (s *Service) Spawn(ctx context.Context, req SpawnRequest) (WorldLease, error) {
	sess, err := s.validateSession(req.SessionID, req.RealmRevision)
	if err != nil {
		return WorldLease{}, err
	}
	lease, err := s.fabric.Spawn(ctx, fabric.SpawnRequest{RealmID: sess.RealmID, RealmRevision: sess.RealmRevision, WorldID: req.WorldID, OperationID: req.OperationID, Units: req.Units, ExpiresUnix: req.ExpiresUnix})
	if err != nil {
		return WorldLease{}, err
	}
	return projectLease(lease), nil
}

func (s *Service) Release(ctx context.Context, req ReleaseRequest) error {
	sess, err := s.validateSession(req.SessionID, req.RealmRevision)
	if err != nil {
		return err
	}
	if req.WorldID == "" || req.LeaseGeneration == 0 {
		return ErrInvalidRequest
	}
	return s.fabric.Release(ctx, sess.RealmID, req.WorldID, req.LeaseGeneration)
}

func (s *Service) validateSession(id realm.SessionID, revision uint64) (Session, error) {
	if s == nil || id == "" || revision == 0 {
		return Session{}, ErrInvalidRequest
	}
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if sess.RealmRevision != revision {
		return Session{}, ErrStaleSession
	}
	rec, ok := s.store.Realm(sess.RealmID)
	if !ok || rec.Closed {
		return Session{}, ErrStaleSession
	}
	if rec.Revision != sess.RealmRevision {
		return Session{}, ErrStaleSession
	}
	return sess, nil
}

func projectLease(l fabric.Lease) WorldLease {
	return WorldLease{WorldID: l.WorldID, Generation: l.Generation, RealizationRevision: l.RealizationRevision, ExpiresUnix: l.ExpiresUnix}
}
