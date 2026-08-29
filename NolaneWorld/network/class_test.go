package network

import "testing"

func TestParseAcceptsEveryDeclaredClass(t *testing.T) {
	classes := []Class{N0None, N1PublicRead, N2PublicSupplyChain, N3AuthenticatedRead, N4ReversibleWrite, N5ConsequentialWrite}
	for _, want := range classes {
		got, err := Parse(string(want))
		if err != nil {
			t.Fatalf("Parse(%q): %v", want, err)
		}
		if got != want || !got.Valid() {
			t.Fatalf("got %q valid=%v, want %q", got, got.Valid(), want)
		}
	}
}

func TestUnknownNetworkClassFailsClosed(t *testing.T) {
	if _, err := Parse("N6_MAGIC_ADMIN"); err != ErrInvalidNetworkClass {
		t.Fatalf("expected ErrInvalidNetworkClass, got %v", err)
	}
	if Class("N6_MAGIC_ADMIN").Valid() {
		t.Fatal("unknown network class must be invalid")
	}
}
