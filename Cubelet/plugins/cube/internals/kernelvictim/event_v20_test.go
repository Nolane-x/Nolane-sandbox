package kernelvictim

import (
	"encoding/binary"
	"math"
	"testing"
)

func rawV20Record(version, pid, tgid uint32, startNS, eventNS, cgroupID uint64) []byte {
	b := make([]byte, 40)
	binary.LittleEndian.PutUint32(b[0:4], version)
	binary.LittleEndian.PutUint32(b[4:8], 0)
	binary.LittleEndian.PutUint32(b[8:12], pid)
	binary.LittleEndian.PutUint32(b[12:16], tgid)
	binary.LittleEndian.PutUint64(b[16:24], startNS)
	binary.LittleEndian.PutUint64(b[24:32], eventNS)
	binary.LittleEndian.PutUint64(b[32:40], cgroupID)
	return b
}

func TestV20DecodeRawVictimEventExact(t *testing.T) {
	got, err := DecodeRawVictimEvent(rawV20Record(1, 43, 42, 12_345_678_999, 20_000_000_000, math.MaxUint64))
	if err != nil { t.Fatal(err) }
	if got.Version != 1 || got.PID != 43 || got.TGID != 42 || got.StartBootTimeNS != 12_345_678_999 || got.EventBootTimeNS != 20_000_000_000 || got.CgroupV2ID != math.MaxUint64 {
		t.Fatalf("unexpected decoded event: %+v", got)
	}
}

func TestV20DecodeRawVictimEventRejectsMalformedIdentity(t *testing.T) {
	cases := [][]byte{
		make([]byte, 39),
		rawV20Record(2, 43, 42, 1, 2, 0),
		rawV20Record(1, 0, 42, 1, 2, 0),
		rawV20Record(1, 43, 0, 1, 2, 0),
		rawV20Record(1, 43, 42, 0, 2, 0),
		rawV20Record(1, 43, 42, 2, 0, 0),
		rawV20Record(1, 43, 42, 3, 2, 0),
	}
	for i, raw := range cases {
		if _, err := DecodeRawVictimEvent(raw); err == nil { t.Fatalf("case %d unexpectedly accepted", i) }
	}
}

func TestV20StartTimeTicksExactOnSupportedArchitectures(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		got, err := StartTimeTicks(12_345_678_999, 0, 0, arch)
		if err != nil { t.Fatalf("%s: %v", arch, err) }
		if got != 1234 { t.Fatalf("%s: got %d want 1234", arch, got) }
	}
	got, err := StartTimeTicks(12_345_678_999, 1, 500_000_000, "amd64")
	if err != nil { t.Fatal(err) }
	if got != 1384 { t.Fatalf("got %d want 1384", got) }
	if _, err := StartTimeTicks(1, 0, 0, "386"); err == nil { t.Fatal("unsupported architecture accepted") }
	if _, err := StartTimeTicks(1, 0, -2, "amd64"); err == nil { t.Fatal("negative visible start accepted") }
	if _, err := StartTimeTicks(math.MaxUint64, 1, 0, "amd64"); err == nil { t.Fatal("overflow accepted") }
}

func TestV20ParseTimeNamespaceBootOffset(t *testing.T) {
	sec, ns, err := ParseTimeNamespaceBootOffset([]byte("monotonic 0 0\nboottime 1 500000000\n"))
	if err != nil { t.Fatal(err) }
	if sec != 1 || ns != 500000000 { t.Fatalf("got %d %d", sec, ns) }
	if _, _, err := ParseTimeNamespaceBootOffset([]byte("monotonic 0 0\n")); err == nil { t.Fatal("missing boottime accepted") }
	if _, _, err := ParseTimeNamespaceBootOffset([]byte("boottime 1 0\nboottime 1 0\n")); err == nil { t.Fatal("duplicate boottime accepted") }
}

func TestV20StoreIsBoundedAndPositiveOnly(t *testing.T) {
	s := NewStore()
	boot := "11111111-2222-3333-4444-555555555555"
	for i := uint64(1); i <= 1025; i++ {
		e := Event{BootID: boot, VictimTID: uint32(i), TGID: uint32(i), StartTimeTicks: i, EventBootTimeNS: 1_000_000_000_000 + i}
		if err := s.Add(e); err != nil { t.Fatalf("add %d: %v", i, err) }
	}
	if _, ok := s.Find(boot, 1, 1, 0, math.MaxUint64); ok { t.Fatal("oldest event was not evicted") }
	if _, ok := s.Find(boot, 1025, 1025, 0, math.MaxUint64); !ok { t.Fatal("newest event missing") }

	e := Event{BootID: boot, VictimTID: 50_000, TGID: 50_000, StartTimeTicks: 50_000, EventBootTimeNS: 2_000_000_000_000}
	if err := s.Add(e); err != nil { t.Fatal(err) }
	if err := s.Add(e); err != nil { t.Fatalf("exact duplicate must be idempotent: %v", err) }
	conflict := e; conflict.VictimTID = 50_001
	if err := s.Add(conflict); err == nil { t.Fatal("conflicting lifetime event was merged") }
}
