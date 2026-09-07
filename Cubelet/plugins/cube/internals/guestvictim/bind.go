// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package guestvictim

import (
	"context"
	"encoding/hex"
	"fmt"
)

const (
	UpdateActionAnnotation     = "cube.shimapi.update.action"
	RealizationTokenAnnotation = "cube.shimapi.update.oom_victim_realization_token"
	BindAction                 = "BindOOMVictimRealization"
)

// TaskUpdateAnnotations encodes the exact internal one-shot Wave 21 binding
// action consumed by CubeShim. The token remains an opaque realization nonce;
// it is never derived from sandbox identity, process identity, or timestamps.
func TaskUpdateAnnotations(token [32]byte) (map[string]string, error) {
	if !validToken(token) {
		return nil, fmt.Errorf("guest OOM victim realization token must be non-zero")
	}
	return map[string]string{
		UpdateActionAnnotation:     BindAction,
		RealizationTokenAnnotation: hex.EncodeToString(token[:]),
	}, nil
}

// BindBeforeStartResult separates observational Wave 21 bind failures from
// workload Start failures. Evidence setup must never mask or replace the
// authoritative workload Start result.
type BindBeforeStartResult struct {
	BindErr  error
	StartErr error
}

// BindBeforeStart attempts the Wave 21 realization bind strictly before the
// workload Start call. Invalid evidence authority and bind failures are
// observational-only: Start is still attempted exactly once.
func BindBeforeStart(
	ctx context.Context,
	sandboxID string,
	token [32]byte,
	bind func(context.Context, string, [32]byte) error,
	start func(context.Context) error,
) BindBeforeStartResult {
	result := BindBeforeStartResult{}

	if sandboxID == "" {
		result.BindErr = fmt.Errorf("guest OOM victim sandbox ID is required")
	} else if !validToken(token) {
		result.BindErr = fmt.Errorf("guest OOM victim realization token must be non-zero")
	} else if bind == nil {
		result.BindErr = fmt.Errorf("guest OOM victim bind function is unavailable")
	} else {
		result.BindErr = bind(ctx, sandboxID, token)
	}

	if start == nil {
		result.StartErr = fmt.Errorf("workload start function is unavailable")
	} else {
		result.StartErr = start(ctx)
	}
	return result
}

func validToken(token [32]byte) bool {
	for _, b := range token {
		if b != 0 {
			return true
		}
	}
	return false
}
