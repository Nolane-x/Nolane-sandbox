package kernelvictim

import (
	"fmt"
	"os"
	"runtime"
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

func canonicalBootID(raw string) bool {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		c := raw[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
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

	offsetRaw, err := os.ReadFile("/proc/self/timens_offsets")
	if err == nil {
		seconds, nanoseconds, parseErr := ParseTimeNamespaceBootOffset(offsetRaw)
		if parseErr != nil {
			return "", 0, 0, fmt.Errorf("parse Cubelet time namespace offset: %w", parseErr)
		}
		return bootID, seconds, nanoseconds, nil
	}
	if !os.IsNotExist(err) {
		return "", 0, 0, fmt.Errorf("read Cubelet time namespace offset: %w", err)
	}
	if _, nsErr := os.Stat("/proc/self/ns/time"); nsErr == nil {
		return "", 0, 0, fmt.Errorf("time namespaces are present but timens_offsets is unavailable")
	} else if !os.IsNotExist(nsErr) {
		return "", 0, 0, fmt.Errorf("inspect Cubelet time namespace capability: %w", nsErr)
	}
	return bootID, 0, 0, nil
}

func collectorGOARCH() string {
	return runtime.GOARCH
}
