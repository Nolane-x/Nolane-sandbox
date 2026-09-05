package kernelvictim

import (
	"encoding/binary"
	"testing"
)

func TestV20DecodeCgroupV2HandleExact(t *testing.T) {
	b := make([]byte, 8)
	nativeEndian.PutUint64(b, 88)
	got, err := decodeCgroupV2Handle(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != 88 {
		t.Fatalf("got %d want 88", got)
	}
	if _, err := decodeCgroupV2Handle(make([]byte, 7)); err == nil {
		t.Fatal("short handle accepted")
	}
	if _, err := decodeCgroupV2Handle(make([]byte, 9)); err == nil {
		t.Fatal("long handle accepted")
	}
	zero := make([]byte, 8)
	binary.LittleEndian.PutUint64(zero, 0)
	if _, err := decodeCgroupV2Handle(zero); err == nil {
		t.Fatal("zero cgroup id accepted")
	}
}

func TestV20CanonicalCgroupHierarchyPath(t *testing.T) {
	for _, good := range []string{"/cube_sandbox_v1/42", "/a/b"} {
		if err := validateCgroupHierarchyPath(good); err != nil {
			t.Fatalf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{"", "/", "relative", "/a/../b", "/a//b", " /a/b"} {
		if err := validateCgroupHierarchyPath(bad); err == nil {
			t.Fatalf("bad path %q accepted", bad)
		}
	}
}
