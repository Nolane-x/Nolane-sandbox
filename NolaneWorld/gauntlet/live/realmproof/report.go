package realmproof

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
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

func scenarioDigest(s ScenarioEvidence) string {
	fields := []string{s.ID, string(s.Outcome), string(s.Reason)}
	fields = append(fields, s.Markers...)
	return proofHash("nolane-live-realm-v9/scenario", fields...)
}

func reportDigest(r Report) string {
	fields := []string{
		fmt.Sprint(r.SchemaVersion),
		string(r.Profile),
		string(r.Mode),
		r.Substrate,
		string(r.Status),
		string(r.Reason),
		fmt.Sprint(r.Approved),
		r.EndpointDigest,
		r.TargetDigest,
		fmt.Sprint(r.Capabilities.GuestExecution),
		fmt.Sprint(r.Capabilities.RawPublicDenied),
		fmt.Sprint(r.Capabilities.PublicIngressDenied),
		fmt.Sprint(r.Capabilities.InternalMeshVerified),
	}
	for _, scenario := range r.Scenarios {
		fields = append(fields, scenario.Digest)
	}
	return proofHash("nolane-live-realm-v9/report", fields...)
}

func SealReport(r *Report) error {
	if r == nil {
		return ErrInvalidReport
	}
	for i := range r.Scenarios {
		r.Scenarios[i].Markers = append([]string(nil), r.Scenarios[i].Markers...)
		r.Scenarios[i].Digest = scenarioDigest(r.Scenarios[i])
	}
	sort.Slice(r.Scenarios, func(i, j int) bool { return r.Scenarios[i].ID < r.Scenarios[j].ID })
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

	seen := make(map[string]ScenarioEvidence, len(r.Scenarios))
	lastID := ""
	for _, scenario := range r.Scenarios {
		if scenario.ID == "" || (lastID != "" && scenario.ID <= lastID) {
			return ErrInvalidReport
		}
		if _, exists := seen[scenario.ID]; exists {
			return ErrInvalidReport
		}
		if scenario.Outcome != live.OutcomePass && scenario.Outcome != live.OutcomeFail && scenario.Outcome != live.OutcomeUnavailable {
			return ErrInvalidReport
		}
		if scenario.Outcome == live.OutcomePass && scenario.Reason != ReasonNone {
			return ErrInvalidReport
		}
		if scenario.Outcome != live.OutcomePass && scenario.Reason == ReasonNone {
			return ErrInvalidReport
		}
		if len(scenario.Markers) == 0 || scenario.Digest != scenarioDigest(scenario) {
			return ErrInvalidReport
		}
		for _, marker := range scenario.Markers {
			if strings.TrimSpace(marker) == "" {
				return ErrInvalidReport
			}
		}
		if scenario.Outcome == live.OutcomePass && !hasRequiredMarkers(scenario) {
			return ErrInvalidReport
		}
		seen[scenario.ID] = scenario
		lastID = scenario.ID
	}

	if r.Digest == "" || r.Digest != reportDigest(r) {
		return ErrInvalidReport
	}

	mesh, meshPresent := seen[ScenarioInternalMesh]
	if r.Capabilities.InternalMeshVerified {
		if !meshPresent || mesh.Outcome != live.OutcomePass {
			return ErrInvalidReport
		}
	}

	if r.Status == live.StatusLivePass {
		if !r.Approved || r.Reason != ReasonNone || r.EndpointDigest == "" || r.TargetDigest == "" {
			return ErrInvalidReport
		}
		required := []string{
			ScenarioProfileApply,
			ScenarioGuestAfterProfile,
			ScenarioRawPublicDenied,
			ScenarioPublicIngressDenied,
			ScenarioCleanup,
		}
		for _, id := range required {
			scenario, ok := seen[id]
			if !ok || scenario.Outcome != live.OutcomePass {
				return ErrInvalidReport
			}
		}
		if !r.Capabilities.GuestExecution || !r.Capabilities.RawPublicDenied || !r.Capabilities.PublicIngressDenied {
			return ErrInvalidReport
		}
		return nil
	}

	if r.Approved || r.Reason == ReasonNone {
		return ErrInvalidReport
	}
	if r.Status != live.StatusUnavailable && r.Status != live.StatusLiveFail {
		return ErrInvalidReport
	}
	return nil
}

func MarshalReport(r Report, forbidden ...string) ([]byte, error) {
	if err := VerifyReport(r); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	for _, secret := range forbidden {
		if secret != "" && strings.Contains(string(encoded), secret) {
			return nil, ErrInvalidReport
		}
	}
	return append(encoded, '\n'), nil
}

func hasRequiredMarkers(s ScenarioEvidence) bool {
	required := map[string][]string{
		ScenarioProfileApply:        {"profile-applied"},
		ScenarioGuestAfterProfile:   {"guest-after-profile"},
		ScenarioRawPublicDenied:     {"raw-public-denied"},
		ScenarioPublicIngressDenied: {"unauthenticated-ingress-denied"},
		ScenarioInternalMesh:        {"private-mesh-observed"},
		ScenarioCleanup:             {"cleanup-observed"},
	}
	want, known := required[s.ID]
	if !known {
		return false
	}
	markers := make(map[string]bool, len(s.Markers))
	for _, marker := range s.Markers {
		markers[marker] = true
	}
	for _, marker := range want {
		if !markers[marker] {
			return false
		}
	}
	return true
}
