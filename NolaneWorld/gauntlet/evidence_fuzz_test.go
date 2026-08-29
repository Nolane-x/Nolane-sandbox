package gauntlet

import (
	"context"
	"testing"
	"time"
)

func fuzzReport(t *testing.T) Report {
	t.Helper()
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	report, err := r.Run(context.Background(), []Scenario{passingScenario("fuzz")})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func FuzzVerifyReportRejectsTrustFieldMutation(f *testing.F) {
	f.Add(uint8(0), "mutated")
	f.Add(uint8(5), "marker-mutated")
	f.Fuzz(func(t *testing.T, selector uint8, mutation string) {
		if mutation == "" {
			t.Skip()
		}
		report := fuzzReport(t)
		s := &report.Scenarios[0]
		switch selector % 8 {
		case 0:
			if mutation == s.Invariant {
				t.Skip()
			}
			s.Invariant = mutation
		case 1:
			if mutation == s.Attack {
				t.Skip()
			}
			s.Attack = mutation
		case 2:
			if mutation == s.ExpectedDefense {
				t.Skip()
			}
			s.ExpectedDefense = mutation
		case 3:
			if mutation == s.RequiredMarkers[0] {
				t.Skip()
			}
			s.RequiredMarkers[0] = mutation
		case 4:
			if mutation == s.Events[0].Marker {
				t.Skip()
			}
			s.Events[0].Marker = mutation
		case 5:
			if mutation == s.Events[0].Detail {
				t.Skip()
			}
			s.Events[0].Detail = mutation
		case 6:
			if mutation == report.ProductID {
				t.Skip()
			}
			report.ProductID = mutation
		case 7:
			report.Policy.ScenarioTimeout++
		}
		if err := VerifyReport(report); err == nil {
			t.Fatalf("trust-bearing mutation verified: selector=%d mutation=%q", selector, mutation)
		}
	})
}

func FuzzScenarioSpecRejectsDuplicateMarkers(f *testing.F) {
	f.Add("marker")
	f.Add("../marker")
	f.Fuzz(func(t *testing.T, marker string) {
		s := ScenarioSpec{ID: "fuzz-duplicate", Invariant: "i", Attack: "a", ExpectedDefense: "d", Severity: SeverityHigh, RequiredMarkers: []string{marker, marker}}
		if err := s.Validate(); err == nil {
			t.Fatalf("duplicate marker accepted: %q", marker)
		}
	})
}

func FuzzLengthPrefixedHashSeparatesBoundaries(f *testing.F) {
	f.Add("a", "b", "c")
	f.Add("prefix", "middle", "suffix")
	f.Fuzz(func(t *testing.T, a, b, c string) {
		if b == "" {
			t.Skip()
		}
		left := hashFields("fuzz-domain", a+b, c)
		right := hashFields("fuzz-domain", a, b+c)
		if left == right {
			t.Fatalf("length-prefixed fields collided for a=%q b=%q c=%q", a, b, c)
		}
	})
}
