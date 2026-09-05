package kernelvictim

import (
	"encoding/binary"
	"fmt"
	"path"
	"path/filepath"
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

func parseCgroup2Mount(raw []byte) (string, error) {
	var mount string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		post := strings.Fields(parts[1])
		if len(post) < 1 || post[0] != "cgroup2" {
			continue
		}
		pre := strings.Fields(parts[0])
		if len(pre) < 5 {
			return "", fmt.Errorf("malformed cgroup2 mountinfo record")
		}
		candidate := unescapeMountInfoPath(pre[4])
		if candidate == "" || !filepath.IsAbs(candidate) || filepath.Clean(candidate) != candidate {
			return "", fmt.Errorf("invalid cgroup2 mount point")
		}
		if mount != "" && mount != candidate {
			return "", fmt.Errorf("multiple cgroup2 mounts are ambiguous")
		}
		mount = candidate
	}
	if mount == "" {
		return "", fmt.Errorf("cgroup2 mount is unavailable")
	}
	return mount, nil
}

func unescapeMountInfoPath(raw string) string {
	return strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	).Replace(raw)
}

func resolveCgroupPathUnderMount(mount, cgroupPath string) (string, error) {
	if mount == "" || !filepath.IsAbs(mount) || filepath.Clean(mount) != mount {
		return "", fmt.Errorf("cgroup2 mount point is not canonical")
	}
	if err := validateCgroupHierarchyPath(cgroupPath); err != nil {
		return "", err
	}
	full := filepath.Join(mount, strings.TrimPrefix(cgroupPath, "/"))
	mountWithSep := mount + string(filepath.Separator)
	if full == mount || !strings.HasPrefix(full, mountWithSep) {
		return "", fmt.Errorf("cgroup path escapes cgroup2 mount")
	}
	return full, nil
}
