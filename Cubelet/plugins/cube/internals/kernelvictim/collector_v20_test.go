package kernelvictim

import (
	"math"
	"testing"
)

func TestV20NormalizeCollectorRecordBindsBootAndStartLifetime(t *testing.T) {
	event, err := normalizeCollectorRecord(
		rawV20Record(1, 4247, 4242, 12_345_678_999, 20_000_000_000, 88),
		"11111111-2222-3333-4444-555555555555",
		0,
		0,
		"amd64",
	)
	if err != nil {
		t.Fatal(err)
	}
	if event.BootID != "11111111-2222-3333-4444-555555555555" || event.VictimTID != 4247 || event.TGID != 4242 || event.StartTimeTicks != 1234 || event.EventBootTimeNS != 20_000_000_000 || event.CgroupV2ID != 88 {
		t.Fatalf("unexpected normalized event: %+v", event)
	}
}

func TestV20NormalizeCollectorRecordRejectsInvalidBootAndBridge(t *testing.T) {
	raw := rawV20Record(1, 4247, 4242, 12_345_678_999, 20_000_000_000, 0)
	if _, err := normalizeCollectorRecord(raw, "NOT-A-UUID", 0, 0, "amd64"); err == nil {
		t.Fatal("invalid boot ID accepted")
	}
	if _, err := normalizeCollectorRecord(raw, "11111111-2222-3333-4444-555555555555", 0, 0, "386"); err == nil {
		t.Fatal("unsupported starttime bridge accepted")
	}
}

func TestV20ParseCgroup2MountAndResolveHierarchyPath(t *testing.T) {
	mountInfo := []byte(
		"30 23 0:26 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw\n" +
			"42 23 0:29 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw\n",
	)
	mount, err := parseCgroup2Mount(mountInfo)
	if err != nil {
		t.Fatal(err)
	}
	if mount != "/sys/fs/cgroup" {
		t.Fatalf("mount = %q", mount)
	}
	full, err := resolveCgroupPathUnderMount(mount, "/cube_sandbox_v1/42")
	if err != nil {
		t.Fatal(err)
	}
	if full != "/sys/fs/cgroup/cube_sandbox_v1/42" {
		t.Fatalf("full path = %q", full)
	}
	if _, err := resolveCgroupPathUnderMount(mount, "/cube_sandbox_v1/../escape"); err == nil {
		t.Fatal("non-canonical hierarchy path escaped mount")
	}
}

func TestV20ParseCgroup2MountRejectsMissingAndAmbiguousAuthority(t *testing.T) {
	if _, err := parseCgroup2Mount([]byte("30 23 0:26 / /proc rw - proc proc rw\n")); err == nil {
		t.Fatal("missing cgroup2 mount accepted")
	}
	ambiguous := []byte(
		"42 23 0:29 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n" +
			"43 23 0:30 / /other/cgroup rw - cgroup2 cgroup rw\n",
	)
	if _, err := parseCgroup2Mount(ambiguous); err == nil {
		t.Fatal("ambiguous cgroup2 mount accepted")
	}
}

func TestV20TimespecToBootNSIsExact(t *testing.T) {
	got, err := timespecToBootNS(12, 345)
	if err != nil {
		t.Fatal(err)
	}
	if got != 12_000_000_345 {
		t.Fatalf("got %d", got)
	}
	if _, err := timespecToBootNS(-1, 0); err == nil {
		t.Fatal("negative seconds accepted")
	}
	if _, err := timespecToBootNS(1, 1_000_000_000); err == nil {
		t.Fatal("invalid nanoseconds accepted")
	}
	if _, err := timespecToBootNS(math.MaxInt64, 0); err == nil {
		t.Fatal("overflow accepted")
	}
}
