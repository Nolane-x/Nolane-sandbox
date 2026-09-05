// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const hostProcessRuntimeRoleCubeShimVMM = "cube-shim-vmm"

type HostProcessPlacementSource string

const HostProcessPlacementSourceCubeBoxAddProc HostProcessPlacementSource = "cubebox.cgroup.add_proc"

type HostProcessPlacementProof struct {
	SandboxID      string
	CGroupPath     string
	BootID         string
	HostPID        uint32
	StartTimeTicks uint64
	PlacedAt       time.Time
	ObservedAt     time.Time
	Source         HostProcessPlacementSource
}

type HostProcessRealizationBinding struct {
	SandboxID      string
	Generation     uint64
	CGroupPath     string
	BootID         string
	HostPID        uint32
	StartTimeTicks uint64
	PlacedAt       time.Time
	BoundAt        time.Time
	Source         HostProcessPlacementSource
}

type hostProcessLifetime struct{}

type hostProcessPlacementToken struct {
	sandboxID  string
	generation uint64
	lifetime   *hostProcessLifetime
}

type hostProcessInspector struct {
	readFile func(string) ([]byte, error)
	now      func() time.Time
}

func newHostProcessInspector(readFile func(string) ([]byte, error), now func() time.Time) *hostProcessInspector {
	if readFile == nil {
		readFile = os.ReadFile
	}
	if now == nil {
		now = time.Now
	}
	return &hostProcessInspector{readFile: readFile, now: now}
}

func parseHostProcessStatStartTime(raw string, expectedPID uint32) (uint64, error) {
	if expectedPID == 0 {
		return 0, fmt.Errorf("host process PID is required")
	}
	if strings.HasSuffix(raw, "\n") {
		raw = strings.TrimSuffix(raw, "\n")
	}
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return 0, fmt.Errorf("host process stat is malformed")
	}
	open := strings.Index(raw, " (")
	close := strings.LastIndex(raw, ") ")
	if open <= 0 || close <= open+1 {
		return 0, fmt.Errorf("host process stat command field is malformed")
	}
	pidToken := raw[:open]
	pid64, err := strconv.ParseUint(pidToken, 10, 32)
	if err != nil || strconv.FormatUint(pid64, 10) != pidToken || uint32(pid64) != expectedPID {
		return 0, fmt.Errorf("host process stat PID does not match requested PID")
	}

	fields := strings.Fields(raw[close+2:])
	const startTimeIndex = 22 - 3
	if len(fields) <= startTimeIndex {
		return 0, fmt.Errorf("host process stat is missing starttime")
	}
	startToken := fields[startTimeIndex]
	startTime, err := strconv.ParseUint(startToken, 10, 64)
	if err != nil || startTime == 0 || strconv.FormatUint(startTime, 10) != startToken {
		return 0, fmt.Errorf("host process starttime is not canonical")
	}
	return startTime, nil
}

func validateHostProcessCgroupPath(group string) error {
	if group == "" || strings.TrimSpace(group) != group || !path.IsAbs(group) || path.Clean(group) != group || group == "/" {
		return fmt.Errorf("host process cgroup path %q is not canonical", group)
	}
	return nil
}

func validateHostProcessCgroupMembership(raw, expected string) error {
	if err := validateHostProcessCgroupPath(expected); err != nil {
		return err
	}
	if strings.HasSuffix(raw, "\n") {
		raw = strings.TrimSuffix(raw, "\n")
	}
	if raw == "" {
		return fmt.Errorf("host process cgroup membership is empty")
	}
	matched := false
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			return fmt.Errorf("host process cgroup membership contains an empty entry")
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			return fmt.Errorf("host process cgroup membership entry is malformed")
		}
		hierarchy, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil || strconv.FormatUint(hierarchy, 10) != parts[0] {
			return fmt.Errorf("host process cgroup hierarchy is not canonical")
		}
		if hierarchy != 0 && parts[1] == "" {
			return fmt.Errorf("host process cgroup v1 controller list is empty")
		}
		if hierarchy == 0 && parts[1] != "" {
			return fmt.Errorf("host process cgroup v2 controller field is not empty")
		}
		if err := validateHostProcessCgroupPath(parts[2]); err != nil {
			return fmt.Errorf("host process cgroup membership path: %w", err)
		}
		if parts[2] == expected {
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("host process is not in exact cgroup %q", expected)
	}
	return nil
}

func parseCanonicalHostBootID(raw []byte) (string, error) {
	normalized := strings.TrimSpace(string(raw))
	if normalized == "" {
		return "", fmt.Errorf("host boot ID is empty")
	}
	parsed, err := uuid.Parse(normalized)
	if err != nil || parsed.String() != normalized {
		return "", fmt.Errorf("host boot ID is not canonical")
	}
	return normalized, nil
}

func (i *hostProcessInspector) observeIdentity(sandboxID, cgroupPath string, pid uint32) (string, uint64, time.Time, error) {
	if i == nil || i.readFile == nil || i.now == nil {
		return "", 0, time.Time{}, fmt.Errorf("host process inspector is unavailable")
	}
	if sandboxID == "" || strings.TrimSpace(sandboxID) != sandboxID {
		return "", 0, time.Time{}, fmt.Errorf("host process sandbox ID is not canonical")
	}
	if err := validateHostProcessCgroupPath(cgroupPath); err != nil {
		return "", 0, time.Time{}, err
	}
	if pid == 0 {
		return "", 0, time.Time{}, fmt.Errorf("host process PID is required")
	}

	bootRaw, err := i.readFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("read host boot ID: %w", err)
	}
	bootID, err := parseCanonicalHostBootID(bootRaw)
	if err != nil {
		return "", 0, time.Time{}, err
	}

	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statA, err := i.readFile(statPath)
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("read host process stat A: %w", err)
	}
	startA, err := parseHostProcessStatStartTime(string(statA), pid)
	if err != nil {
		return "", 0, time.Time{}, err
	}

	cgroupRaw, err := i.readFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("read host process cgroup: %w", err)
	}
	if err := validateHostProcessCgroupMembership(string(cgroupRaw), cgroupPath); err != nil {
		return "", 0, time.Time{}, err
	}

	statB, err := i.readFile(statPath)
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("read host process stat B: %w", err)
	}
	startB, err := parseHostProcessStatStartTime(string(statB), pid)
	if err != nil {
		return "", 0, time.Time{}, err
	}
	if startA != startB {
		return "", 0, time.Time{}, fmt.Errorf("host process identity changed during cgroup observation")
	}

	observedAt := i.now().UTC()
	if observedAt.IsZero() {
		return "", 0, time.Time{}, fmt.Errorf("host process observation timestamp is zero")
	}
	return bootID, startA, observedAt, nil
}

func (i *hostProcessInspector) CapturePlacement(sandboxID, cgroupPath string, pid uint32, placedAt time.Time) (HostProcessPlacementProof, error) {
	if placedAt.IsZero() {
		return HostProcessPlacementProof{}, fmt.Errorf("host process placement timestamp is required")
	}
	placedAt = placedAt.UTC()
	bootID, startTime, observedAt, err := i.observeIdentity(sandboxID, cgroupPath, pid)
	if err != nil {
		return HostProcessPlacementProof{}, err
	}
	if observedAt.Before(placedAt) {
		return HostProcessPlacementProof{}, fmt.Errorf("host process observation predates placement")
	}
	return HostProcessPlacementProof{
		SandboxID:      sandboxID,
		CGroupPath:     cgroupPath,
		BootID:         bootID,
		HostPID:        pid,
		StartTimeTicks: startTime,
		PlacedAt:       placedAt,
		ObservedAt:     observedAt,
		Source:         HostProcessPlacementSourceCubeBoxAddProc,
	}, nil
}

func (i *hostProcessInspector) ValidatePlacement(proof HostProcessPlacementProof) (time.Time, error) {
	if err := validateHostProcessPlacementProof(proof); err != nil {
		return time.Time{}, err
	}
	bootID, startTime, observedAt, err := i.observeIdentity(proof.SandboxID, proof.CGroupPath, proof.HostPID)
	if err != nil {
		return time.Time{}, err
	}
	if bootID != proof.BootID || startTime != proof.StartTimeTicks {
		return time.Time{}, fmt.Errorf("host process placement identity no longer matches")
	}
	return observedAt, nil
}

func validateHostProcessPlacementProof(proof HostProcessPlacementProof) error {
	if proof.SandboxID == "" || strings.TrimSpace(proof.SandboxID) != proof.SandboxID {
		return fmt.Errorf("host process placement sandbox ID is not canonical")
	}
	if err := validateHostProcessCgroupPath(proof.CGroupPath); err != nil {
		return err
	}
	if proof.HostPID == 0 || proof.StartTimeTicks == 0 {
		return fmt.Errorf("host process placement identity is incomplete")
	}
	if _, err := parseCanonicalHostBootID([]byte(proof.BootID)); err != nil {
		return err
	}
	if proof.Source != HostProcessPlacementSourceCubeBoxAddProc {
		return fmt.Errorf("host process placement source %q is not authoritative", proof.Source)
	}
	if proof.PlacedAt.IsZero() || proof.ObservedAt.IsZero() || proof.ObservedAt.Before(proof.PlacedAt) {
		return fmt.Errorf("host process placement timestamps are invalid")
	}
	return nil
}

func sameHostProcessPlacementIdentity(a, b HostProcessPlacementProof) bool {
	return a.SandboxID == b.SandboxID &&
		a.CGroupPath == b.CGroupPath &&
		a.BootID == b.BootID &&
		a.HostPID == b.HostPID &&
		a.StartTimeTicks == b.StartTimeTicks &&
		a.Source == b.Source &&
		a.PlacedAt.Equal(b.PlacedAt)
}

func (s *taskOutcomeProofStore) ensureHostProcessMapsLocked(sandboxID string) {
	if s.hostProcessLifetimes == nil {
		s.hostProcessLifetimes = make(map[string]*hostProcessLifetime)
	}
	if s.hostProcessPlacements == nil {
		s.hostProcessPlacements = make(map[string]HostProcessPlacementProof)
	}
	if s.hostProcessBindings == nil {
		s.hostProcessBindings = make(map[string]HostProcessRealizationBinding)
	}
	if sandboxID != "" && s.hostProcessLifetimes[sandboxID] == nil {
		s.hostProcessLifetimes[sandboxID] = &hostProcessLifetime{}
	}
}

func (s *taskOutcomeProofStore) BeginHostProcessPlacementCapture(sandboxID string) hostProcessPlacementToken {
	if s == nil || sandboxID == "" {
		return hostProcessPlacementToken{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureHostProcessMapsLocked(sandboxID)
	return hostProcessPlacementToken{
		sandboxID:  sandboxID,
		generation: s.generations[sandboxID],
		lifetime:   s.hostProcessLifetimes[sandboxID],
	}
}

func (s *taskOutcomeProofStore) CommitHostProcessPlacement(token hostProcessPlacementToken, proof HostProcessPlacementProof) (HostProcessRealizationBinding, bool) {
	if s == nil || token.sandboxID == "" || token.lifetime == nil || validateHostProcessPlacementProof(proof) != nil || token.sandboxID != proof.SandboxID {
		return HostProcessRealizationBinding{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureHostProcessMapsLocked(proof.SandboxID)
	if s.hostProcessLifetimes[proof.SandboxID] != token.lifetime || s.generations[proof.SandboxID] != token.generation {
		return HostProcessRealizationBinding{}, false
	}

	closed := false
	if outcome, ok := s.proofs[proof.SandboxID]; ok && outcome.Generation == token.generation {
		closed = true
	}
	if existing, ok := s.hostProcessPlacements[proof.SandboxID]; ok && !sameHostProcessPlacementIdentity(existing, proof) && !closed {
		delete(s.hostProcessBindings, proof.SandboxID)
	}
	s.hostProcessPlacements[proof.SandboxID] = proof
	if token.generation == 0 || closed || (s.fenced != nil && s.fenced[proof.SandboxID]) {
		return HostProcessRealizationBinding{}, false
	}

	binding := HostProcessRealizationBinding{
		SandboxID:      proof.SandboxID,
		Generation:     token.generation,
		CGroupPath:     proof.CGroupPath,
		BootID:         proof.BootID,
		HostPID:        proof.HostPID,
		StartTimeTicks: proof.StartTimeTicks,
		PlacedAt:       proof.PlacedAt.UTC(),
		BoundAt:        proof.ObservedAt.UTC(),
		Source:         proof.Source,
	}
	s.hostProcessBindings[proof.SandboxID] = binding
	return binding, true
}

func (s *taskOutcomeProofStore) BeginHostProcessRevalidation(sandboxID string, generation uint64) (hostProcessPlacementToken, HostProcessPlacementProof, bool) {
	if s == nil || sandboxID == "" || generation == 0 {
		return hostProcessPlacementToken{}, HostProcessPlacementProof{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureHostProcessMapsLocked(sandboxID)
	if s.generations[sandboxID] != generation || (s.fenced != nil && s.fenced[sandboxID]) {
		return hostProcessPlacementToken{}, HostProcessPlacementProof{}, false
	}
	proof, ok := s.hostProcessPlacements[sandboxID]
	if !ok {
		return hostProcessPlacementToken{}, HostProcessPlacementProof{}, false
	}
	return hostProcessPlacementToken{sandboxID: sandboxID, generation: generation, lifetime: s.hostProcessLifetimes[sandboxID]}, proof, true
}

func (s *taskOutcomeProofStore) CommitHostProcessRevalidation(token hostProcessPlacementToken, proof HostProcessPlacementProof, boundAt time.Time) (HostProcessRealizationBinding, bool) {
	if s == nil || token.sandboxID == "" || token.lifetime == nil || boundAt.IsZero() || validateHostProcessPlacementProof(proof) != nil {
		return HostProcessRealizationBinding{}, false
	}
	boundAt = boundAt.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureHostProcessMapsLocked(proof.SandboxID)
	if token.sandboxID != proof.SandboxID || s.hostProcessLifetimes[proof.SandboxID] != token.lifetime || s.generations[proof.SandboxID] != token.generation || token.generation == 0 {
		return HostProcessRealizationBinding{}, false
	}
	if s.fenced != nil && s.fenced[proof.SandboxID] {
		return HostProcessRealizationBinding{}, false
	}
	if outcome, ok := s.proofs[proof.SandboxID]; ok && outcome.Generation == token.generation {
		return HostProcessRealizationBinding{}, false
	}
	current, ok := s.hostProcessPlacements[proof.SandboxID]
	if !ok || !sameHostProcessPlacementIdentity(current, proof) || boundAt.Before(proof.PlacedAt) {
		return HostProcessRealizationBinding{}, false
	}
	binding := HostProcessRealizationBinding{
		SandboxID: proof.SandboxID, Generation: token.generation, CGroupPath: proof.CGroupPath,
		BootID: proof.BootID, HostPID: proof.HostPID, StartTimeTicks: proof.StartTimeTicks,
		PlacedAt: proof.PlacedAt.UTC(), BoundAt: boundAt, Source: proof.Source,
	}
	s.hostProcessBindings[proof.SandboxID] = binding
	return binding, true
}

func (s *taskOutcomeProofStore) ListHostProcessIdentityProofs() []HostProcessRealizationBinding {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	proofs := make([]HostProcessRealizationBinding, 0, len(s.hostProcessBindings))
	for _, proof := range s.hostProcessBindings {
		proofs = append(proofs, proof)
	}
	s.mu.RUnlock()
	sort.Slice(proofs, func(i, j int) bool {
		if proofs[i].SandboxID != proofs[j].SandboxID {
			return proofs[i].SandboxID < proofs[j].SandboxID
		}
		return proofs[i].Generation < proofs[j].Generation
	})
	return proofs
}

func (c *controllerLocal) RecordHostProcessPlacement(_ context.Context, sandboxID, cgroupPath string, pid uint32, placedAt time.Time) error {
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return fmt.Errorf("host process identity store is unavailable")
	}
	token := store.BeginHostProcessPlacementCapture(sandboxID)
	inspector := c.ensureHostProcessInspector()
	proof, err := inspector.CapturePlacement(sandboxID, cgroupPath, pid, placedAt)
	if err != nil {
		return err
	}
	store.CommitHostProcessPlacement(token, proof)
	return nil
}

func (c *controllerLocal) revalidateHostProcessIdentity(sandboxID string, generation uint64) {
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	token, proof, ok := store.BeginHostProcessRevalidation(sandboxID, generation)
	if !ok {
		return
	}
	boundAt, err := c.ensureHostProcessInspector().ValidatePlacement(proof)
	if err != nil {
		return
	}
	store.CommitHostProcessRevalidation(token, proof, boundAt)
}

func (c *controllerLocal) VisitHostProcessIdentityProofs(visit func(string, uint64, uint32, uint64, string, string, string, string, time.Time, time.Time)) {
	if visit == nil {
		return
	}
	store := c.ensureTaskOutcomeProofStore()
	if store == nil {
		return
	}
	for _, proof := range store.ListHostProcessIdentityProofs() {
		visit(
			proof.SandboxID,
			proof.Generation,
			proof.HostPID,
			proof.StartTimeTicks,
			proof.BootID,
			proof.CGroupPath,
			hostProcessRuntimeRoleCubeShimVMM,
			string(proof.Source),
			proof.PlacedAt,
			proof.BoundAt,
		)
	}
}
