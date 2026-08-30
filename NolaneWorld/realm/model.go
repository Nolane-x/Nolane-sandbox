package realm

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrInvalidSpec       = errors.New("realm: invalid spec")
	ErrRealmExists       = errors.New("realm: already exists")
	ErrRealmNotFound     = errors.New("realm: not found")
	ErrRealmClosed       = errors.New("realm: closed")
	ErrStaleRevision     = errors.New("realm: stale revision")
	ErrIdentityRebind    = errors.New("realm: identity rebind")
	ErrInvalidWorld      = errors.New("realm: invalid world record")
	ErrInvalidService    = errors.New("realm: invalid service record")
	ErrInvalidOperation  = errors.New("realm: invalid operation record")
	ErrInvalidCheckpoint = errors.New("realm: invalid checkpoint record")
	ErrStoreClosed       = errors.New("realm: store closed")
	ErrStoreCorrupt      = errors.New("realm: store corrupt")
	ErrStoreLocked       = errors.New("realm: store locked")
	ErrLockUnsupported   = errors.New("realm: store lock unsupported")
)

type ID string
type ServiceID string
type SessionID string
type CheckpointID string

type NetworkProfile string

const (
	R0InternalOnly NetworkProfile = "R0_INTERNAL_ONLY"
	R1PublicRead   NetworkProfile = "R1_PUBLIC_READ"
	R2SupplyChain  NetworkProfile = "R2_SUPPLY_CHAIN"
)

var opaqueID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}[A-Za-z0-9]$|^[A-Za-z0-9]$`)

func validRealmID(id ID) bool {
	raw := string(id)
	if !strings.HasPrefix(raw, "realm://") {
		return false
	}
	return opaqueID.MatchString(strings.TrimPrefix(raw, "realm://"))
}

func (p NetworkProfile) Valid() bool {
	switch p {
	case R0InternalOnly, R1PublicRead, R2SupplyChain:
		return true
	default:
		return false
	}
}

type ResourceBudget struct {
	CPUUnits  uint64 `json:"cpu_units"`
	MemoryMiB uint64 `json:"memory_mib"`
	DiskMiB   uint64 `json:"disk_mib"`
}

func (b ResourceBudget) Valid() bool {
	return b.CPUUnits > 0 && b.MemoryMiB > 0 && b.DiskMiB > 0
}

type Spec struct {
	ID             ID             `json:"id"`
	MaxWorlds      uint32         `json:"max_worlds"`
	DefaultLease   time.Duration  `json:"default_lease"`
	NetworkProfile NetworkProfile `json:"network_profile"`
	ResourceBudget ResourceBudget `json:"resource_budget"`
}

func (s Spec) Validate() error {
	if !validRealmID(s.ID) || s.MaxWorlds == 0 || s.MaxWorlds > 1_000_000 || s.DefaultLease <= 0 || s.DefaultLease > 30*24*time.Hour || !s.NetworkProfile.Valid() || !s.ResourceBudget.Valid() {
		return ErrInvalidSpec
	}
	return nil
}

type RealmRecord struct {
	Spec     Spec   `json:"spec"`
	Revision uint64 `json:"revision"`
	Closed   bool   `json:"closed"`
}

type WorldPhase string

const (
	WorldRequested     WorldPhase = "requested"
	WorldCreating      WorldPhase = "creating"
	WorldObservedReady WorldPhase = "observed-ready"
	WorldLeased        WorldPhase = "leased"
	WorldPaused        WorldPhase = "paused"
	WorldTerminal      WorldPhase = "terminal"
)

func (p WorldPhase) Valid() bool {
	switch p {
	case WorldRequested, WorldCreating, WorldObservedReady, WorldLeased, WorldPaused, WorldTerminal:
		return true
	default:
		return false
	}
}

type WorldRecord struct {
	RealmID             ID                `json:"realm_id"`
	WorldID             world.ID          `json:"world_id"`
	RealizationRevision uint64            `json:"realization_revision"`
	Phase               WorldPhase        `json:"phase"`
	LeaseGeneration     uint64            `json:"lease_generation"`
	LeaseExpiresUnix    int64             `json:"lease_expires_unix"`
	Handle               substrate.Handle `json:"handle,omitempty"`
	CapabilityDigest    string            `json:"capability_digest,omitempty"`
	AcquireOperationID  string            `json:"acquire_operation_id,omitempty"`
	BaselineID          string            `json:"baseline_id,omitempty"`
}

func (r WorldRecord) Validate() error {
	if !validRealmID(r.RealmID) || r.WorldID == "" || r.RealizationRevision == 0 || !r.Phase.Valid() || r.LeaseGeneration == 0 || r.LeaseExpiresUnix <= 0 {
		return ErrInvalidWorld
	}
	return nil
}

type CheckpointRecord struct {
	ID                  CheckpointID       `json:"id"`
	RealmID             ID                 `json:"realm_id"`
	RealmRevision       uint64             `json:"realm_revision"`
	WorldID             world.ID           `json:"world_id"`
	RealizationRevision uint64             `json:"realization_revision"`
	AuthorityEpoch      world.Epoch        `json:"authority_epoch"`
	Snapshot            substrate.Snapshot `json:"snapshot"`
	CapabilityDigest    string             `json:"capability_digest,omitempty"`
	PolicyDigest        string             `json:"policy_digest,omitempty"`
}

func (r CheckpointRecord) Validate() error {
	if r.ID == "" || !validRealmID(r.RealmID) || r.RealmRevision == 0 || r.WorldID == "" || r.RealizationRevision == 0 || r.AuthorityEpoch == 0 || r.Snapshot == "" {
		return ErrInvalidCheckpoint
	}
	return nil
}

type ServiceProtocol string

const (
	ServiceTCP  ServiceProtocol = "tcp"
	ServiceUDP  ServiceProtocol = "udp"
	ServiceHTTP ServiceProtocol = "http"
)

type ServiceRecord struct {
	ID                  ServiceID       `json:"id"`
	RealmID             ID              `json:"realm_id"`
	WorldID             world.ID        `json:"world_id"`
	RealizationRevision uint64          `json:"realization_revision"`
	Protocol            ServiceProtocol `json:"protocol"`
	Port                uint16          `json:"port"`
	Generation          uint64          `json:"generation"`
	Ready               bool            `json:"ready"`
}

type OperationRecord struct {
	RealmID       ID     `json:"realm_id"`
	OperationID   string `json:"operation_id"`
	RequestDigest string `json:"request_digest"`
	Status        string `json:"status"`
	ReceiptDigest string `json:"receipt_digest,omitempty"`
}
