package constitution

import "testing"

func TestLawsContainStableUniqueCatalog(t *testing.T) {
	laws := Laws()
	if len(laws) != 12 {
		t.Fatalf("expected 12 laws, got %d", len(laws))
	}
	seen := map[string]bool{}
	for _, law := range laws {
		if law.ID == "" || law.Title == "" || law.Rule == "" {
			t.Fatalf("law must be complete: %#v", law)
		}
		if seen[law.ID] {
			t.Fatalf("duplicate law id %q", law.ID)
		}
		seen[law.ID] = true
	}
	for i := 1; i <= 12; i++ {
		id := lawID(i)
		if !seen[id] {
			t.Fatalf("missing %s", id)
		}
	}
	if err := Validate(); err != nil {
		t.Fatalf("built-in catalog invalid: %v", err)
	}
}

func TestValidateLawsRejectsDuplicateOrIncompleteLaw(t *testing.T) {
	valid := Law{ID: "NS-LAW-001", Title: "a", Rule: "b"}
	cases := [][]Law{
		{{ID: "", Title: "a", Rule: "b"}},
		{{ID: "NS-LAW-001", Title: "", Rule: "b"}},
		{{ID: "NS-LAW-001", Title: "a", Rule: ""}},
		{valid, valid},
	}
	for i, laws := range cases {
		if err := validateLaws(laws); err == nil {
			t.Fatalf("case %d should fail", i)
		}
	}
}
