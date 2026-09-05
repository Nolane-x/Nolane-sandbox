// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package kernelvictim

import (
	"bufio"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	userHZSupported        = 100
	nanosecondsPerSecond   = int64(1_000_000_000)
	nanosecondsPerUserTick = int64(10_000_000)
)

func StartTimeTicks(startBootNS uint64, offsetSeconds int64, offsetNanoseconds int64, goarch string) (uint64, error) {
	if goarch != "amd64" && goarch != "arm64" {
		return 0, fmt.Errorf("kernel victim start-time bridge does not support architecture %q", goarch)
	}
	if startBootNS == 0 {
		return 0, fmt.Errorf("kernel victim start boottime is required")
	}
	if offsetNanoseconds <= -nanosecondsPerSecond || offsetNanoseconds >= nanosecondsPerSecond {
		return 0, fmt.Errorf("time namespace nanosecond offset is not normalized")
	}

	visible := new(big.Int).SetUint64(startBootNS)
	sec := new(big.Int).Mul(big.NewInt(offsetSeconds), big.NewInt(nanosecondsPerSecond))
	visible.Add(visible, sec)
	visible.Add(visible, big.NewInt(offsetNanoseconds))
	if visible.Sign() <= 0 || visible.BitLen() > 64 {
		return 0, fmt.Errorf("visible process start time is outside uint64 domain")
	}

	ticks := new(big.Int).Quo(visible, big.NewInt(nanosecondsPerUserTick))
	if ticks.Sign() <= 0 || ticks.BitLen() > 64 {
		return 0, fmt.Errorf("visible process start ticks are outside uint64 domain")
	}
	return ticks.Uint64(), nil
}

func ParseTimeNamespaceBootOffset(raw []byte) (seconds int64, nanoseconds int64, err error) {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	found := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, 0, fmt.Errorf("time namespace offset row is malformed")
		}
		if fields[0] != "boottime" {
			continue
		}
		if found {
			return 0, 0, fmt.Errorf("time namespace contains duplicate boottime offsets")
		}
		sec, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil || strconv.FormatInt(sec, 10) != fields[1] {
			return 0, 0, fmt.Errorf("boottime second offset is not canonical")
		}
		ns, parseErr := strconv.ParseInt(fields[2], 10, 64)
		if parseErr != nil || strconv.FormatInt(ns, 10) != fields[2] || ns < 0 || ns >= nanosecondsPerSecond {
			return 0, 0, fmt.Errorf("boottime nanosecond offset is not canonical")
		}
		seconds, nanoseconds, found = sec, ns, true
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if !found {
		return 0, 0, fmt.Errorf("time namespace has no boottime offset")
	}
	return seconds, nanoseconds, nil
}
