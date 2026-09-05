package kernelvictim

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type Source interface {
	Find(bootID string, tgid uint32, startTimeTicks, minBootNS, maxBootNS uint64) (Event, bool)
}

func normalizeCollectorRecord(raw []byte, bootID string, offsetSeconds, offsetNanoseconds int64, goarch string) (Event, error) {
	if !canonicalBootID(bootID) {
		return Event{}, fmt.Errorf("kernel victim boot ID is not canonical")
	}
	record, err := DecodeRawVictimEvent(raw)
	if err != nil {
		return Event{}, err
	}
	startTicks, err := StartTimeTicks(record.StartBootTimeNS, offsetSeconds, offsetNanoseconds, goarch)
	if err != nil {
		return Event{}, err
	}
	if startTicks == 0 {
		return Event{}, fmt.Errorf("kernel victim process starttime is zero")
	}
	return Event{
		BootID:          bootID,
		VictimTID:       record.PID,
		TGID:            record.TGID,
		StartTimeTicks:  startTicks,
		EventBootTimeNS: record.EventBootTimeNS,
		CgroupV2ID:      record.CgroupV2ID,
	}, nil
}

func sameTimeNamespaceHandle(current, forChildren string) bool {
	if current == "" || current != forChildren || !strings.HasPrefix(current, "time:[") || !strings.HasSuffix(current, "]") {
		return false
	}
	inode := strings.TrimSuffix(strings.TrimPrefix(current, "time:["), "]")
	if inode == "" {
		return false
	}
	_, err := strconv.ParseUint(inode, 10, 64)
	return err == nil
}

func collectorIdentity() (bootID string, offsetSeconds, offsetNanoseconds int64, err error) {
	bootRaw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", 0, 0, fmt.Errorf("read kernel boot ID: %w", err)
	}
	bootID = strings.TrimSpace(string(bootRaw))
	if !canonicalBootID(bootID) {
		return "", 0, 0, fmt.Errorf("kernel boot ID is not canonical")
	}

	currentTimeNS, currentNSErr := os.Readlink("/proc/self/ns/time")
	childrenTimeNS, childrenNSErr := os.Readlink("/proc/self/ns/time_for_children")
	offsetRaw, offsetErr := os.ReadFile("/proc/self/timens_offsets")
	if offsetErr == nil {
		if currentNSErr != nil || childrenNSErr != nil {
			return "", 0, 0, fmt.Errorf("time namespace handles are unavailable for offset validation")
		}
		if !sameTimeNamespaceHandle(currentTimeNS, childrenTimeNS) {
			return "", 0, 0, fmt.Errorf("timens_offsets describes a different time namespace-for-children")
		}
		seconds, nanoseconds, parseErr := ParseTimeNamespaceBootOffset(offsetRaw)
		if parseErr != nil {
			return "", 0, 0, fmt.Errorf("parse Cubelet time namespace offset: %w", parseErr)
		}
		return bootID, seconds, nanoseconds, nil
	}
	if !os.IsNotExist(offsetErr) {
		return "", 0, 0, fmt.Errorf("read Cubelet time namespace offset: %w", offsetErr)
	}
	if currentNSErr == nil || childrenNSErr == nil {
		return "", 0, 0, fmt.Errorf("time namespaces are present but timens_offsets is unavailable")
	}
	if !os.IsNotExist(currentNSErr) || !os.IsNotExist(childrenNSErr) {
		return "", 0, 0, fmt.Errorf("inspect Cubelet time namespace capability")
	}
	return bootID, 0, 0, nil
}

func collectorGOARCH() string {
	return runtime.GOARCH
}
