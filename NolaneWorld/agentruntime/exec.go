package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type ExecRequest struct {
	SessionID       realm.SessionID `json:"session_id"`
	RealmRevision   uint64          `json:"realm_revision"`
	WorldID         world.ID        `json:"world_id"`
	LeaseGeneration uint64          `json:"lease_generation"`
	ActionID        string          `json:"action_id"`
	Command         string          `json:"command"`
	Timeout         time.Duration   `json:"timeout"`
	MaxOutputBytes  int64           `json:"max_output_bytes"`
}

type ExecReceipt struct {
	ReceiptID            string   `json:"receipt_id"`
	WorldID              world.ID `json:"world_id"`
	RealizationRevision  uint64   `json:"realization_revision"`
	ExitCode              int      `json:"exit_code"`
	Stdout                string   `json:"stdout,omitempty"`
	Stderr                string   `json:"stderr,omitempty"`
	StdoutTruncated       bool     `json:"stdout_truncated"`
	StderrTruncated       bool     `json:"stderr_truncated"`
	ObservationDigest     string   `json:"observation_digest"`
}

func (s *Service) Exec(ctx context.Context, req ExecRequest) (ExecReceipt, error) {
	if s == nil || req.WorldID == "" || req.LeaseGeneration == 0 || req.ActionID == "" {
		return ExecReceipt{}, ErrInvalidRequest
	}
	sess, err := s.validateSession(req.SessionID, req.RealmRevision)
	if err != nil {
		return ExecReceipt{}, err
	}
	process := substrate.ProcessRequest{Command: req.Command, Timeout: req.Timeout, MaxOutputBytes: req.MaxOutputBytes}
	if err := process.Validate(64 << 20); err != nil {
		return ExecReceipt{}, ErrInvalidRequest
	}
	digest := execDigest(req)
	opID := "exec:" + req.ActionID
	if prior, ok := s.store.Operation(sess.RealmID, opID); ok {
		if prior.RequestDigest != digest {
			return ExecReceipt{}, ErrOperationCollision
		}
		if prior.Status == "completed" {
			s.mu.RLock()
			receipt, exists := s.exec[execKey(sess.RealmID, req.ActionID)]
			s.mu.RUnlock()
			if exists {
				return receipt, nil
			}
		}
		return ExecReceipt{}, ErrExecUncertain
	}
	h, realization, err := s.fabric.Handle(sess.RealmID, req.WorldID, req.LeaseGeneration)
	if err != nil {
		return ExecReceipt{}, err
	}
	if err := s.store.RecordOperation(realm.OperationRecord{RealmID: sess.RealmID, OperationID: opID, RequestDigest: digest, Status: "pending"}); err != nil {
		return ExecReceipt{}, err
	}
	obs, execErr := s.guest.Exec(ctx, h, process)
	if execErr != nil {
		_ = s.store.RecordOperation(realm.OperationRecord{RealmID: sess.RealmID, OperationID: opID, RequestDigest: digest, Status: "uncertain"})
		return ExecReceipt{}, ErrExecUncertain
	}
	receipt := ExecReceipt{
		WorldID: req.WorldID, RealizationRevision: realization, ExitCode: obs.ExitCode,
		Stdout: string(obs.Stdout), Stderr: string(obs.Stderr),
		StdoutTruncated: obs.StdoutTruncated, StderrTruncated: obs.StderrTruncated,
		ObservationDigest: obs.ObservationDigest,
	}
	receipt.ReceiptID = execReceiptID(sess.RealmID, req.ActionID, digest, receipt)
	s.mu.Lock()
	s.exec[execKey(sess.RealmID, req.ActionID)] = receipt
	s.mu.Unlock()
	if err := s.store.RecordOperation(realm.OperationRecord{RealmID: sess.RealmID, OperationID: opID, RequestDigest: digest, Status: "completed", ReceiptDigest: receipt.ReceiptID}); err != nil {
		// Provider already executed. Failure to durably mark completion must not
		// make the command replayable.
		return ExecReceipt{}, ErrExecUncertain
	}
	return receipt, nil
}

func execKey(realmID realm.ID, action string) string { return string(realmID) + "\x00" + action }

func execDigest(req ExecRequest) string {
	type canonical struct {
		RealmRevision   uint64        `json:"realm_revision"`
		WorldID         world.ID      `json:"world_id"`
		LeaseGeneration uint64        `json:"lease_generation"`
		ActionID        string        `json:"action_id"`
		Command         string        `json:"command"`
		Timeout         time.Duration `json:"timeout"`
		MaxOutputBytes  int64         `json:"max_output_bytes"`
	}
	raw, _ := json.Marshal(canonical{req.RealmRevision, req.WorldID, req.LeaseGeneration, req.ActionID, req.Command, req.Timeout, req.MaxOutputBytes})
	h := sha256.Sum256(append([]byte("nolane.agentruntime.exec.v1\x00"), raw...))
	return hex.EncodeToString(h[:])
}

func execReceiptID(realmID realm.ID, action, requestDigest string, receipt ExecReceipt) string {
	receipt.ReceiptID = ""
	raw, _ := json.Marshal(receipt)
	h := sha256.New()
	_, _ = h.Write([]byte("nolane.agentruntime.exec-receipt.v1\x00"))
	_, _ = h.Write([]byte(realmID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(action))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(requestDigest))
	_, _ = h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
