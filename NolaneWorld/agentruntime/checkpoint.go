package agentruntime

import (
	"context"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type CheckpointRequest struct {
	SessionID       realm.SessionID `json:"session_id"`
	RealmRevision   uint64          `json:"realm_revision"`
	WorldID         world.ID        `json:"world_id"`
	LeaseGeneration uint64          `json:"lease_generation"`
}

type CheckpointReceipt struct {
	ID                  realm.CheckpointID `json:"id"`
	WorldID             world.ID           `json:"world_id"`
	RealizationRevision uint64             `json:"realization_revision"`
}

type ResumeRequest struct {
	SessionID       realm.SessionID    `json:"session_id"`
	RealmRevision   uint64             `json:"realm_revision"`
	CheckpointID    realm.CheckpointID `json:"checkpoint_id"`
}

func (s *Service) Checkpoint(ctx context.Context, req CheckpointRequest) (CheckpointReceipt, error) {
	if s == nil || req.WorldID == "" || req.LeaseGeneration == 0 {
		return CheckpointReceipt{}, ErrInvalidRequest
	}
	sess, err := s.validateSession(req.SessionID, req.RealmRevision)
	if err != nil {
		return CheckpointReceipt{}, err
	}
	cp, err := s.fabric.Checkpoint(ctx, sess.RealmID, req.WorldID, req.LeaseGeneration)
	if err != nil {
		return CheckpointReceipt{}, err
	}
	return CheckpointReceipt{ID: cp.ID, WorldID: cp.WorldID, RealizationRevision: cp.RealizationRevision}, nil
}

func (s *Service) Resume(ctx context.Context, req ResumeRequest) (WorldLease, error) {
	if s == nil || req.CheckpointID == "" {
		return WorldLease{}, ErrInvalidRequest
	}
	sess, err := s.validateSession(req.SessionID, req.RealmRevision)
	if err != nil {
		return WorldLease{}, err
	}
	lease, err := s.fabric.Resume(ctx, req.CheckpointID, sess.RealmRevision)
	if err != nil {
		return WorldLease{}, err
	}
	return projectLease(lease), nil
}
