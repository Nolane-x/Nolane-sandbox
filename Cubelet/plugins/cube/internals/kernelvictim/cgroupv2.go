package kernelvictim

import (
	"encoding/binary"
	"fmt"
	"path"
	"strings"
)

var nativeEndian binary.ByteOrder = binary.NativeEndian

func decodeCgroupV2Handle(raw []byte) (uint64, error) {
	if len(raw) != 8 {
		return 0, fmt.Errorf("cgroup-v2 handle payload must be exactly 8 bytes, got %d", len(raw))
	}
	id := nativeEndian.Uint64(raw)
	if id == 0 {
		return 0, fmt.Errorf("cgroup-v2 handle resolved zero identity")
	}
	return id, nil
}

func validateCgroupHierarchyPath(cgroupPath string) error {
	if cgroupPath == "" || strings.TrimSpace(cgroupPath) != cgroupPath {
		return fmt.Errorf("cgroup hierarchy path is not canonical")
	}
	if !strings.HasPrefix(cgroupPath, "/") || cgroupPath == "/" {
		return fmt.Errorf("cgroup hierarchy path must be a non-root absolute path")
	}
	if strings.ContainsRune(cgroupPath, '\x00') || path.Clean(cgroupPath) != cgroupPath {
		return fmt.Errorf("cgroup hierarchy path is not canonical")
	}
	return nil
}
