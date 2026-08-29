package gauntlet

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

func hashFields(domain string, fields ...string) string {
	h := sha256.New()
	write := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(s))
	}
	write(domain)
	for _, field := range fields {
		write(field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func policyDigest(p Policy) string {
	return hashFields("nolane.gauntlet.policy.v1", p.ProductID, strconv.FormatInt(int64(p.ScenarioTimeout), 10))
}

func scenarioDigest(s ScenarioEvidence) string {
	fields := []string{s.ID, s.Invariant, s.Attack, s.ExpectedDefense, string(s.Severity), string(s.Outcome), string(s.FailureCode), s.FailureMessage}
	for _, marker := range s.RequiredMarkers {
		fields = append(fields, marker)
	}
	for _, e := range s.Events {
		fields = append(fields, string(e.Kind), e.Marker, e.Detail)
	}
	return hashFields("nolane.gauntlet.scenario.v1", fields...)
}

func reportDigest(r Report) string {
	fields := []string{strconv.Itoa(r.Version), r.ProductID, r.PolicyDigest, strconv.FormatBool(r.Approved)}
	for _, s := range r.Scenarios {
		fields = append(fields, s.EvidenceDigest)
	}
	return hashFields("nolane.gauntlet.report.v1", fields...)
}

func VerifyReport(r Report) error {
	if r.Version != ReportVersion || r.ProductID != ProductNolaneSandbox || r.PolicyDigest == "" || len(r.Scenarios) == 0 || r.Policy.validate() != nil || r.Policy.ProductID != r.ProductID || policyDigest(r.Policy) != r.PolicyDigest {
		return ErrInvalidReport
	}
	approved := true
	lastID := ""
	for i := range r.Scenarios {
		s := r.Scenarios[i]
		if !nonBlank(s.ID) || !nonBlank(s.Invariant) || !nonBlank(s.Attack) || !nonBlank(s.ExpectedDefense) || !s.Severity.valid() {
			return ErrInvalidReport
		}
		if i > 0 && s.ID <= lastID {
			return ErrInvalidReport
		}
		lastID = s.ID
		if invalidMarkers(s.RequiredMarkers) {
			return ErrInvalidReport
		}
		if !s.FailureCode.valid() {
			return ErrInvalidReport
		}
		if s.Outcome != OutcomePass && s.Outcome != OutcomeFail {
			return ErrInvalidReport
		}
		if s.Outcome == OutcomePass {
			if s.FailureCode != FailureNone || s.FailureMessage != "" || !proofSatisfied(s.Events) || missingMarker(s.RequiredMarkers, s.Events) {
				return ErrInvalidReport
			}
		} else {
			approved = false
			if s.FailureCode == FailureNone || s.FailureMessage == "" {
				return ErrInvalidReport
			}
		}
		for _, e := range s.Events {
			if !e.Kind.valid() || !nonBlank(e.Marker) || !nonBlank(e.Detail) {
				return ErrInvalidReport
			}
		}
		if scenarioDigest(s) != s.EvidenceDigest {
			return ErrInvalidReport
		}
	}
	if approved != r.Approved {
		return ErrInvalidReport
	}
	if reportDigest(r) != r.ReportDigest {
		return ErrInvalidReport
	}
	return nil
}

func MarshalReport(r Report) ([]byte, error) {
	if err := VerifyReport(r); err != nil {
		return nil, err
	}
	return json.MarshalIndent(r, "", "  ")
}

func sortedCopy(scenarios []ScenarioEvidence) []ScenarioEvidence {
	out := append([]ScenarioEvidence(nil), scenarios...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func invalidMarkers(markers []string) bool {
	seen := make(map[string]struct{}, len(markers))
	for _, marker := range markers {
		if !nonBlank(marker) || len(marker) > 256 {
			return true
		}
		if _, ok := seen[marker]; ok {
			return true
		}
		seen[marker] = struct{}{}
	}
	return false
}
