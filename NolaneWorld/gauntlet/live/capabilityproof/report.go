package capabilityproof

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func proofHash(domain string, fields ...string) string {
	h := sha256.New()
	write := func(value string) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(value))
	}
	write(domain)
	for _, field := range fields {
		write(field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func reportDigest(r Report) string {
	return proofHash(
		"nolane-live-capability-v10/report",
		fmt.Sprint(r.SchemaVersion),
		string(r.Profile),
		string(r.Mode),
		r.Substrate,
		string(r.Status),
		string(r.Reason),
		fmt.Sprint(r.Approved),
		r.EndpointDigest,
		r.TemplateDigest,
		r.SubstrateProof.Digest,
		r.RealmProof.Digest,
		fmt.Sprint(r.Capabilities.GuestExecution),
		fmt.Sprint(r.Capabilities.SnapshotRollback),
		fmt.Sprint(r.Capabilities.PublicIngressDenied),
		fmt.Sprint(r.Capabilities.NetworkIsolation),
		fmt.Sprint(r.Capabilities.InternalMeshVerified),
	)
}

func sealReport(r *Report) error {
	if r == nil {
		return ErrInvalidReport
	}
	r.Digest = reportDigest(*r)
	return VerifyReport(*r)
}

func VerifyReport(r Report) error {
	if r.SchemaVersion != 1 || r.Substrate != "cubesandbox" {
		return ErrInvalidReport
	}
	if r.Profile != realm.R0InternalOnly && r.Profile != realm.R1PublicRead && r.Profile != realm.R2SupplyChain {
		return ErrInvalidReport
	}
	if r.Mode != live.ModeProbe && r.Mode != live.ModeRequireLive {
		return ErrInvalidReport
	}
	if r.Status != live.StatusLivePass && r.Status != live.StatusLiveFail && r.Status != live.StatusUnavailable {
		return ErrInvalidReport
	}
	if err := live.VerifyReport(r.SubstrateProof); err != nil {
		return ErrInvalidReport
	}
	if err := realmproof.VerifyReport(r.RealmProof); err != nil {
		return ErrInvalidReport
	}
	if r.Digest == "" || r.Digest != reportDigest(r) {
		return ErrInvalidReport
	}

	// Both nested proofs are deliberately executed in probe mode. The outer
	// v10 mode alone controls whether an UNAVAILABLE receipt is gate-fatal.
	if r.SubstrateProof.Mode != live.ModeProbe || r.SubstrateProof.Profile != live.ProfileCore {
		return ErrInvalidReport
	}
	if r.RealmProof.Mode != live.ModeProbe || r.RealmProof.Profile != r.Profile {
		return ErrInvalidReport
	}

	if r.Status == live.StatusLivePass {
		return verifyPass(r)
	}
	return verifyNonPass(r)
}

func verifyPass(r Report) error {
	if !r.Approved || r.Reason != ReasonNone || r.EndpointDigest == "" || r.TemplateDigest == "" {
		return ErrInvalidReport
	}
	if r.SubstrateProof.Status != live.StatusLivePass || !r.SubstrateProof.Approved || r.RealmProof.Status != live.StatusLivePass || !r.RealmProof.Approved {
		return ErrInvalidReport
	}
	if r.SubstrateProof.EndpointDigest != r.EndpointDigest || r.SubstrateProof.TemplateDigest != r.TemplateDigest || r.RealmProof.EndpointDigest != r.EndpointDigest {
		return ErrInvalidReport
	}
	expected, ok := deriveCapabilities(r.SubstrateProof, r.RealmProof)
	if !ok || r.Capabilities != expected {
		return ErrInvalidReport
	}
	if !expected.GuestExecution || !expected.SnapshotRollback || !expected.PublicIngressDenied || !expected.NetworkIsolation {
		return ErrInvalidReport
	}
	return nil
}

func verifyNonPass(r Report) error {
	if r.Approved || r.Reason == ReasonNone || r.Capabilities != (Capabilities{}) {
		return ErrInvalidReport
	}

	substrateStatus := r.SubstrateProof.Status
	realmStatus := r.RealmProof.Status
	switch r.Reason {
	case ReasonConfigMissing:
		if r.Status != live.StatusUnavailable || substrateStatus != live.StatusUnavailable || realmStatus != live.StatusUnavailable || r.EndpointDigest != "" || r.TemplateDigest != "" {
			return ErrInvalidReport
		}
	case ReasonFingerprintInvalid:
		if r.Status != live.StatusLiveFail || (r.EndpointDigest != "" && r.TemplateDigest != "") {
			return ErrInvalidReport
		}
	case ReasonEvidenceMismatch:
		if r.Status != live.StatusLiveFail || !fingerprintMismatch(r) {
			return ErrInvalidReport
		}
	case ReasonSubstrateFailed:
		if r.Status != live.StatusLiveFail || substrateStatus != live.StatusLiveFail {
			return ErrInvalidReport
		}
	case ReasonRealmFailed:
		if r.Status != live.StatusLiveFail || substrateStatus == live.StatusLiveFail || realmStatus != live.StatusLiveFail {
			return ErrInvalidReport
		}
	case ReasonSubstrateUnavailable:
		if r.Status != live.StatusUnavailable || substrateStatus != live.StatusUnavailable || realmStatus == live.StatusLiveFail {
			return ErrInvalidReport
		}
	case ReasonRealmUnavailable:
		if r.Status != live.StatusUnavailable || substrateStatus != live.StatusLivePass || realmStatus != live.StatusUnavailable {
			return ErrInvalidReport
		}
	default:
		return ErrInvalidReport
	}
	return nil
}

func fingerprintMismatch(r Report) bool {
	return r.EndpointDigest == "" || r.TemplateDigest == "" ||
		r.SubstrateProof.EndpointDigest != r.EndpointDigest ||
		r.SubstrateProof.TemplateDigest != r.TemplateDigest ||
		r.RealmProof.EndpointDigest != r.EndpointDigest
}

func deriveCapabilities(substrateReport live.Report, realmReport realmproof.Report) (Capabilities, bool) {
	if substrateReport.Status != live.StatusLivePass || !substrateReport.Approved || realmReport.Status != live.StatusLivePass || !realmReport.Approved {
		return Capabilities{}, false
	}
	if !substrateReport.Capabilities.GuestExecution || !substrateReport.Capabilities.SnapshotRollback || !substrateReport.Capabilities.CleanupObserved {
		return Capabilities{}, false
	}
	if !realmReport.Capabilities.GuestExecution || !realmReport.Capabilities.RawPublicDenied || !realmReport.Capabilities.PublicIngressDenied {
		return Capabilities{}, false
	}
	return Capabilities{
		GuestExecution:       true,
		SnapshotRollback:     true,
		PublicIngressDenied:  true,
		NetworkIsolation:     true,
		InternalMeshVerified: realmReport.Capabilities.InternalMeshVerified,
	}, true
}

func MarshalReport(r Report, forbidden ...string) ([]byte, error) {
	if err := VerifyReport(r); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	text := string(encoded)
	for _, secret := range forbidden {
		if secret != "" && strings.Contains(text, secret) {
			return nil, ErrInvalidReport
		}
	}
	return append(encoded, '\n'), nil
}
