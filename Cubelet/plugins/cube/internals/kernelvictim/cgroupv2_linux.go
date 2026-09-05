//go:build linux

package kernelvictim

import (
	"os"

	"golang.org/x/sys/unix"
)

func ResolveCgroupV2ID(cgroupPath string) (uint64, bool) {
	if err := validateCgroupHierarchyPath(cgroupPath); err != nil {
		return 0, false
	}
	mountInfo, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return 0, false
	}
	mount, err := parseCgroup2Mount(mountInfo)
	if err != nil {
		return 0, false
	}
	var fs unix.Statfs_t
	if err := unix.Statfs(mount, &fs); err != nil || uint64(fs.Type) != uint64(unix.CGROUP2_SUPER_MAGIC) {
		return 0, false
	}
	full, err := resolveCgroupPathUnderMount(mount, cgroupPath)
	if err != nil {
		return 0, false
	}
	handle, _, err := unix.NameToHandleAt(unix.AT_FDCWD, full, 0)
	if err != nil {
		return 0, false
	}
	id, err := decodeCgroupV2Handle(handle.Bytes())
	if err != nil {
		return 0, false
	}
	return id, true
}
