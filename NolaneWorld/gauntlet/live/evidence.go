package live

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func digestString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func liveHash(domain string, fields ...string) string {
	h := sha256.New()
	write := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(s))
	}
	write(domain)
	for _, f := range fields {
		write(f)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func scenarioDigest(s ScenarioEvidence) string {
	fields := []string{s.ID, string(s.Outcome), string(s.Reason), s.RuntimeDigest}
	fields = append(fields, s.Markers...)
	return liveHash("nolane-live-v5/scenario", fields...)
}

func reportDigest(r Report) string {
	fields := []string{
		fmt.Sprint(r.SchemaVersion), string(r.Profile), string(r.Mode), r.Substrate,
		string(r.Status), string(r.Reason), fmt.Sprint(r.Approved), r.EndpointDigest, r.TemplateDigest,
		fmt.Sprint(r.Capabilities.ControlPlane), fmt.Sprint(r.Capabilities.GuestExecution),
		fmt.Sprint(r.Capabilities.SnapshotRollback), fmt.Sprint(r.Capabilities.CleanupObserved),
		fmt.Sprint(r.Capabilities.EgressHTTP), fmt.Sprint(r.Capabilities.EgressTCP),
		fmt.Sprint(r.Capabilities.EgressUDP), fmt.Sprint(r.Capabilities.EgressDNS),
	}
	for _, s := range r.Scenarios {
		fields = append(fields, s.Digest)
	}
	return liveHash("nolane-live-v5/report", fields...)
}

func sealScenario(s *ScenarioEvidence) {
	s.Markers = append([]string(nil), s.Markers...)
	s.Digest = scenarioDigest(*s)
}

func sealReport(r *Report) {
	for i := range r.Scenarios {
		sealScenario(&r.Scenarios[i])
	}
	sort.Slice(r.Scenarios, func(i, j int) bool { return r.Scenarios[i].ID < r.Scenarios[j].ID })
	r.Digest = reportDigest(*r)
}

func newUnavailableReport(profile Profile, mode Mode, reason ReasonCode, endpointDigest, templateDigest string) Report {
	r := Report{SchemaVersion: 1, Profile: profile, Mode: mode, Substrate: "cubesandbox", Status: StatusUnavailable, Reason: reason, Approved: false, EndpointDigest: endpointDigest, TemplateDigest: templateDigest, Scenarios: []ScenarioEvidence{}}
	sealReport(&r)
	return r
}

func VerifyReport(r Report) error {
	if r.SchemaVersion != 1 || r.Substrate != "cubesandbox" {
		return ErrInvalidLiveReport
	}
	if r.Mode != ModeProbe && r.Mode != ModeRequireLive {
		return ErrInvalidLiveReport
	}
	if r.Profile != ProfileCore && r.Profile != ProfileFullEgress {
		return ErrInvalidLiveReport
	}
	seen := map[string]bool{}
	last := ""
	for _, s := range r.Scenarios {
		if s.ID == "" || seen[s.ID] || (last != "" && s.ID <= last) {
			return ErrInvalidLiveReport
		}
		seen[s.ID] = true
		last = s.ID
		if s.Outcome != OutcomePass && s.Outcome != OutcomeFail && s.Outcome != OutcomeUnavailable {
			return ErrInvalidLiveReport
		}
		if s.Outcome == OutcomePass && s.Reason != ReasonNone {
			return ErrInvalidLiveReport
		}
		if len(s.Markers) == 0 || s.Digest != scenarioDigest(s) {
			return ErrInvalidLiveReport
		}
		for _, m := range s.Markers {
			if strings.TrimSpace(m) == "" {
				return ErrInvalidLiveReport
			}
		}
		if s.Outcome == OutcomePass && !hasRequiredProofMarkers(s) {
			return ErrInvalidLiveReport
		}
	}
	if r.Digest != reportDigest(r) {
		return ErrInvalidLiveReport
	}
	if r.Status == StatusLivePass {
		if !r.Approved || r.Reason != ReasonNone || r.EndpointDigest == "" || r.TemplateDigest == "" {
			return ErrInvalidLiveReport
		}
		if !r.Capabilities.ControlPlane || !r.Capabilities.GuestExecution || !r.Capabilities.SnapshotRollback || !r.Capabilities.CleanupObserved {
			return ErrInvalidLiveReport
		}
		required := []string{ScenarioGuestExecution, ScenarioSnapshotAuthority}
		if r.Profile == ProfileFullEgress {
			if !r.Capabilities.EgressHTTP || !r.Capabilities.EgressTCP || !r.Capabilities.EgressUDP || !r.Capabilities.EgressDNS {
				return ErrInvalidLiveReport
			}
			required = append(required, ScenarioEgressHTTP, ScenarioEgressTCP, ScenarioEgressUDP, ScenarioEgressDNS)
		}
		for _, id := range required {
			if !seen[id] {
				return ErrInvalidLiveReport
			}
			for _, s := range r.Scenarios {
				if s.ID == id && s.Outcome != OutcomePass {
					return ErrInvalidLiveReport
				}
			}
		}
		for _, s := range r.Scenarios {
			if s.Outcome != OutcomePass {
				return ErrInvalidLiveReport
			}
		}
		return nil
	}
	if r.Approved {
		return ErrInvalidLiveReport
	}
	if r.Status != StatusUnavailable && r.Status != StatusLiveFail {
		return ErrInvalidLiveReport
	}
	if r.Reason == ReasonNone {
		return ErrInvalidLiveReport
	}
	return nil
}

func MarshalReport(r Report, forbidden ...string) ([]byte, error) {
	if err := VerifyReport(r); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	for _, secret := range forbidden {
		if secret != "" && strings.Contains(string(b), secret) {
			return nil, ErrInvalidLiveReport
		}
	}
	return append(b, '\n'), nil
}

func hasRequiredProofMarkers(s ScenarioEvidence) bool {
	required := map[string][]string{
		ScenarioGuestExecution:    {"control-plane", "guest-canary", "cleanup-observed"},
		ScenarioSnapshotAuthority: {"control-plane", "snapshot-observed", "rollback-restored-a", "stale-authority-denied", "cleanup-observed"},
		ScenarioEgressHTTP:        {"target-preflight", "guest-probe-exercised", "egress-denied", "cleanup-observed"},
		ScenarioEgressTCP:         {"target-preflight", "guest-probe-exercised", "egress-denied", "cleanup-observed"},
		ScenarioEgressUDP:         {"target-preflight", "guest-probe-exercised", "egress-denied", "cleanup-observed"},
		ScenarioEgressDNS:         {"target-preflight", "guest-probe-exercised", "egress-denied", "cleanup-observed"},
	}
	want, known := required[s.ID]
	if !known {
		return false
	}
	seen := make(map[string]bool, len(s.Markers))
	hasTargetDigest := false
	for _, marker := range s.Markers {
		seen[marker] = true
		if strings.HasPrefix(marker, "target-digest:") && len(marker) == len("target-digest:")+64 {
			hasTargetDigest = true
		}
	}
	for _, marker := range want {
		if !seen[marker] {
			return false
		}
	}
	if strings.HasPrefix(s.ID, "live.cube.egress-") && !hasTargetDigest {
		return false
	}
	return true
}
