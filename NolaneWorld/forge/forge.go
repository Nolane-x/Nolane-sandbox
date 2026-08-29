package forge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/artifact"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/capability"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrInvalidForge      = errors.New("forge: invalid configuration")
	ErrValidationFailed  = errors.New("forge: validation failed")
	ErrInvalidEvidence   = errors.New("forge: invalid evidence")
	ErrTeardownFailed    = errors.New("forge: validator teardown failed")
	ErrIdentityCollision = errors.New("forge: validator identity collision")
)

type Evidence struct {
	Report []byte
}

type Validator interface {
	Validate(context.Context, substrate.Handle, capability.Candidate, []byte, []byte) (Evidence, error)
}

type Forge struct {
	worlds     substrate.SandboxSubstrate
	validator  Validator
	registry   *capability.Registry
	gate       artifact.Gate
	verifierID string
	now        func() time.Time
	newID      func(string) (string, error)
}

func New(worlds substrate.SandboxSubstrate, validator Validator, registry *capability.Registry, gate artifact.Gate, verifierID string) (*Forge, error) {
	if worlds == nil || validator == nil || registry == nil || gate.MaxBytes <= 0 || verifierID == "" {
		return nil, ErrInvalidForge
	}
	return &Forge{
		worlds: worlds, validator: validator, registry: registry, gate: gate, verifierID: verifierID,
		now: func() time.Time { return time.Now().UTC() }, newID: randomID,
	}, nil
}

func (f *Forge) Promote(ctx context.Context, origin world.ID, name, version string, content, manifest []byte) (capability.PromotionReceipt, error) {
	if f == nil || origin == "" || name == "" || version == "" || f.verifierID == "" || f.verifierID == string(origin) {
		return capability.PromotionReceipt{}, ErrInvalidForge
	}
	contentCopy := append([]byte(nil), content...)
	manifestCopy := append([]byte(nil), manifest...)

	contentReceipt, err := f.gate.Accept(origin, "candidate/content.bin", "application/octet-stream", contentCopy)
	if err != nil {
		return capability.PromotionReceipt{}, err
	}
	manifestReceipt, err := f.gate.Accept(origin, "candidate/manifest.bin", "application/octet-stream", manifestCopy)
	if err != nil {
		return capability.PromotionReceipt{}, err
	}

	candidateID, err := f.newID("cand-")
	if err != nil || candidateID == "" {
		return capability.PromotionReceipt{}, errors.Join(ErrInvalidForge, err)
	}
	validatorIDRaw, err := f.newID("val-")
	if err != nil || validatorIDRaw == "" {
		return capability.PromotionReceipt{}, errors.Join(ErrInvalidForge, err)
	}
	validatorID := world.ID(validatorIDRaw)
	if validatorID == origin {
		return capability.PromotionReceipt{}, ErrIdentityCollision
	}

	candidate := capability.Candidate{
		CandidateID: candidateID, OriginWorldID: origin, Name: name, Version: version,
		ContentDigest: contentReceipt.ContentDigest, ManifestDigest: manifestReceipt.ContentDigest,
		CreatedAt: f.now(),
	}

	handle, err := f.worlds.Create(ctx, validatorID)
	if err != nil {
		return capability.PromotionReceipt{}, errors.Join(ErrValidationFailed, err)
	}
	if handle == "" {
		return capability.PromotionReceipt{}, ErrValidationFailed
	}

	evidence, validationErr := safeValidate(f.validator, ctx, handle, candidate,
		append([]byte(nil), contentCopy...), append([]byte(nil), manifestCopy...))
	teardownErr := f.worlds.Destroy(ctx, handle)

	if validationErr != nil {
		if teardownErr != nil {
			return capability.PromotionReceipt{}, errors.Join(ErrValidationFailed, validationErr, ErrTeardownFailed, teardownErr)
		}
		return capability.PromotionReceipt{}, errors.Join(ErrValidationFailed, validationErr)
	}
	if len(evidence.Report) == 0 {
		if teardownErr != nil {
			return capability.PromotionReceipt{}, errors.Join(ErrInvalidEvidence, ErrTeardownFailed, teardownErr)
		}
		return capability.PromotionReceipt{}, ErrInvalidEvidence
	}
	if teardownErr != nil {
		return capability.PromotionReceipt{}, errors.Join(ErrTeardownFailed, teardownErr)
	}

	return f.registry.Promote(capability.PromotionRequest{
		Candidate: candidate, Content: contentCopy, Manifest: manifestCopy,
		VerifierID: f.verifierID, VerificationDigest: capability.Digest(evidence.Report),
	})
}

func randomID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

func safeValidate(v Validator, ctx context.Context, handle substrate.Handle, candidate capability.Candidate, content, manifest []byte) (evidence Evidence, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: validator panic: %v", ErrValidationFailed, recovered)
		}
	}()
	return v.Validate(ctx, handle, candidate, content, manifest)
}
