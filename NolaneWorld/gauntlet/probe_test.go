package gauntlet

import "testing"

func TestScenarioSpecRejectsEmptyTrustFields(t *testing.T) {
	base := ScenarioSpec{ID: "x", Invariant: "i", Attack: "a", ExpectedDefense: "d", Severity: SeverityHigh, RequiredMarkers: []string{"m"}}
	cases := []struct {
		name string
		edit func(*ScenarioSpec)
	}{
		{"id", func(s *ScenarioSpec) { s.ID = "" }},
		{"invariant", func(s *ScenarioSpec) { s.Invariant = "" }},
		{"attack", func(s *ScenarioSpec) { s.Attack = "" }},
		{"defense", func(s *ScenarioSpec) { s.ExpectedDefense = "" }},
		{"marker", func(s *ScenarioSpec) { s.RequiredMarkers = []string{""} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			s.RequiredMarkers = append([]string(nil), base.RequiredMarkers...)
			tc.edit(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("expected invalid spec")
			}
		})
	}
}

func TestProbeReturnsDefensiveEventCopy(t *testing.T) {
	p := newProbe()
	if err := p.Record(EventAttack, "attack.sent", "sent hostile request"); err != nil {
		t.Fatal(err)
	}
	events := p.Events()
	events[0].Marker = "mutated"
	if got := p.Events()[0].Marker; got != "attack.sent" {
		t.Fatalf("probe events mutated through caller: %q", got)
	}
}

func TestProbeRejectsInvalidEvents(t *testing.T) {
	p := newProbe()
	if err := p.Record(EventKind("invalid"), "x", "y"); err == nil {
		t.Fatal("invalid kind accepted")
	}
	if err := p.Record(EventAttack, "", "y"); err == nil {
		t.Fatal("empty marker accepted")
	}
	if err := p.Record(EventAttack, "x", ""); err == nil {
		t.Fatal("empty detail accepted")
	}
}
