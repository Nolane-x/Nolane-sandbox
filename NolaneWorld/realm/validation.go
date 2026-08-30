package realm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

func (r ServiceRecord) Validate() error {
	if r.ID == "" || !validRealmID(r.RealmID) || r.WorldID == "" || r.RealizationRevision == 0 || !validServiceProtocol(r.Protocol) || r.Port == 0 || r.Generation == 0 || !validServiceIDForRealm(r.ID, r.RealmID) {
		return ErrInvalidService
	}
	return nil
}

func (r OperationRecord) Validate() error {
	if !validRealmID(r.RealmID) || r.OperationID == "" || len(r.OperationID) > 4096 || strings.IndexByte(r.OperationID, 0) >= 0 || r.RequestDigest == "" || !validOperationStatus(r.Status) {
		return ErrInvalidOperation
	}
	return nil
}

func validOperationStatus(status string) bool {
	switch status {
	case "pending", "uncertain", "completed":
		return true
	default:
		return false
	}
}

func validServiceIDForRealm(id ServiceID, realmID ID) bool {
	if !validRealmID(realmID) {
		return false
	}
	prefix := "service://" + strings.TrimPrefix(string(realmID), "realm://") + "/"
	raw := string(id)
	if !strings.HasPrefix(raw, prefix) {
		return false
	}
	return validServiceName(strings.TrimPrefix(raw, prefix))
}

func (r ServiceRecord) MarshalJSON() ([]byte, error) {
	if r.Validate() != nil {
		return nil, ErrInvalidService
	}
	type alias ServiceRecord
	return json.Marshal(alias(r))
}

func (r *ServiceRecord) UnmarshalJSON(raw []byte) error {
	if r == nil {
		return ErrInvalidService
	}
	type alias ServiceRecord
	var decoded alias
	if err := strictDecodeRecord(raw, &decoded); err != nil {
		return err
	}
	value := ServiceRecord(decoded)
	if value.Validate() != nil {
		return ErrInvalidService
	}
	*r = value
	return nil
}

func (r OperationRecord) MarshalJSON() ([]byte, error) {
	if r.Validate() != nil {
		return nil, ErrInvalidOperation
	}
	type alias OperationRecord
	return json.Marshal(alias(r))
}

func (r *OperationRecord) UnmarshalJSON(raw []byte) error {
	if r == nil {
		return ErrInvalidOperation
	}
	type alias OperationRecord
	var decoded alias
	if err := strictDecodeRecord(raw, &decoded); err != nil {
		return err
	}
	value := OperationRecord(decoded)
	if value.Validate() != nil {
		return ErrInvalidOperation
	}
	*r = value
	return nil
}

func strictDecodeRecord(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrStoreCorrupt
		}
		return err
	}
	return nil
}
