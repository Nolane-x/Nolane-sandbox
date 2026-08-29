package world

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDurableAuthorityEpochAndCloseSurviveRestart(t *testing.T) {
	f, err := NewDurableFactory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.Create("world-A")
	if err != nil {
		t.Fatal(err)
	}
	if e, err := st.AdvanceAuthority(); err != nil || e != 2 {
		t.Fatalf("advance=(%d,%v)", e, err)
	}
	if err := st.Release(); err != nil {
		t.Fatal(err)
	}

	reopened, err := f.Open("world-A")
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.CurrentEpoch(); got != 2 {
		t.Fatalf("epoch=%d want 2", got)
	}
	if err := reopened.ValidateEpoch(1); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("old epoch=%v", err)
	}
	if e, err := reopened.CloseAuthority(); err != nil || e != 3 {
		t.Fatalf("close=(%d,%v)", e, err)
	}
	if err := reopened.Release(); err != nil {
		t.Fatal(err)
	}

	closed, err := f.Open("world-A")
	if err != nil {
		t.Fatal(err)
	}
	defer closed.Release()
	if !closed.Closed() {
		t.Fatal("terminal close did not survive restart")
	}
	if err := closed.ValidateEpoch(closed.CurrentEpoch()); !errors.Is(err, ErrClosedWorld) {
		t.Fatalf("closed validation=%v", err)
	}
}

func TestDurableAuthorityRejectsSecondWriter(t *testing.T) {
	f, _ := NewDurableFactory(t.TempDir())
	first, err := f.Create("w")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	second, err := f.Open("w")
	if second != nil {
		_ = second.Release()
	}
	if !errors.Is(err, ErrStateLocked) {
		t.Fatalf("second writer=%v", err)
	}
}

func TestDurableAuthorityDetectsTamperAndMalformedTail(t *testing.T) {
	dir := t.TempDir()
	f, _ := NewDurableFactory(dir)
	st, _ := f.Create("w")
	_, _ = st.AdvanceAuthority()
	_ = st.Release()
	path := f.Path("w")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"epoch":2`, `"epoch":9`, 1)
	if tampered == string(raw) {
		t.Fatal("test did not locate epoch")
	}
	if err := os.WriteFile(path, []byte(tampered), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Open("w"); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("tamper=%v", err)
	}

	f2, _ := NewDurableFactory(filepath.Join(dir, "other"))
	st2, _ := f2.Create("x")
	_ = st2.Release()
	p2 := f2.Path("x")
	b, _ := os.ReadFile(p2)
	if err := os.WriteFile(p2, append(b, []byte("{bad")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Open("x"); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("malformed=%v", err)
	}
}

func TestDurableAuthorityBindsExactWorldIdentityAndHashesPath(t *testing.T) {
	dir := t.TempDir()
	f, _ := NewDurableFactory(dir)
	st, _ := f.Create("../danger")
	_ = st.Release()
	path := f.Path("../danger")
	if filepath.Dir(path) != dir || strings.Contains(filepath.Base(path), "danger") {
		t.Fatalf("unsafe path=%q", path)
	}
	raw, _ := os.ReadFile(path)
	changed := strings.Replace(string(raw), `"world_id":"../danger"`, `"world_id":"other"`, 1)
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Open("../danger"); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("identity tamper=%v", err)
	}
}
