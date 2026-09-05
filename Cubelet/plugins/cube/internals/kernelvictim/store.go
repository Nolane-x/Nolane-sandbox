// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

package kernelvictim

import (
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

const (
	maxStoredEvents    = 1024
	maxEventAgeBootNS  = uint64(10 * 60 * 1_000_000_000)
)

type eventKey struct {
	bootID         string
	tgid           uint32
	startTimeTicks uint64
}

// Store contains only positive kernel victim facts. Missing entries are always
// unknown because collector or ring-buffer loss can hide an event.
type Store struct {
	mu           sync.RWMutex
	events       map[eventKey]Event
	order        []eventKey
	latestBootNS uint64
}

func NewStore() *Store {
	return &Store{events: make(map[eventKey]Event)}
}

func canonicalBootID(raw string) bool {
	parsed, err := uuid.Parse(raw)
	return err == nil && parsed.String() == raw
}

func validateEvent(e Event) error {
	if !canonicalBootID(e.BootID) {
		return fmt.Errorf("kernel victim boot ID is not canonical")
	}
	if e.VictimTID == 0 || e.TGID == 0 || e.StartTimeTicks == 0 || e.EventBootTimeNS == 0 {
		return fmt.Errorf("kernel victim event identity is incomplete")
	}
	return nil
}

func sameEvent(a, b Event) bool {
	return a == b
}

func (s *Store) Add(e Event) error {
	if s == nil {
		return fmt.Errorf("kernel victim store is unavailable")
	}
	if err := validateEvent(e); err != nil {
		return err
	}
	key := eventKey{bootID: e.BootID, tgid: e.TGID, startTimeTicks: e.StartTimeTicks}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.events == nil {
		s.events = make(map[eventKey]Event)
	}
	if existing, ok := s.events[key]; ok {
		if sameEvent(existing, e) {
			return nil
		}
		return fmt.Errorf("conflicting kernel victim facts for one process lifetime")
	}

	if e.EventBootTimeNS > s.latestBootNS {
		s.latestBootNS = e.EventBootTimeNS
	}
	if s.latestBootNS > maxEventAgeBootNS && e.EventBootTimeNS < s.latestBootNS-maxEventAgeBootNS {
		return nil
	}

	s.events[key] = e
	s.order = append(s.order, key)
	s.compactLocked()
	return nil
}

func (s *Store) compactLocked() {
	if len(s.order) == 0 {
		return
	}
	sort.SliceStable(s.order, func(i, j int) bool {
		a, b := s.events[s.order[i]], s.events[s.order[j]]
		if a.EventBootTimeNS != b.EventBootTimeNS {
			return a.EventBootTimeNS < b.EventBootTimeNS
		}
		if a.TGID != b.TGID {
			return a.TGID < b.TGID
		}
		return a.StartTimeTicks < b.StartTimeTicks
	})

	ageFloor := uint64(0)
	if s.latestBootNS > maxEventAgeBootNS {
		ageFloor = s.latestBootNS - maxEventAgeBootNS
	}
	first := 0
	for first < len(s.order) {
		e := s.events[s.order[first]]
		if e.EventBootTimeNS >= ageFloor {
			break
		}
		delete(s.events, s.order[first])
		first++
	}
	if first > 0 {
		s.order = append([]eventKey(nil), s.order[first:]...)
	}
	for len(s.order) > maxStoredEvents {
		delete(s.events, s.order[0])
		s.order = s.order[1:]
	}
}

func (s *Store) Find(bootID string, tgid uint32, startTimeTicks, minBootNS, maxBootNS uint64) (Event, bool) {
	if s == nil || bootID == "" || tgid == 0 || startTimeTicks == 0 || maxBootNS < minBootNS {
		return Event{}, false
	}
	key := eventKey{bootID: bootID, tgid: tgid, startTimeTicks: startTimeTicks}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.events[key]
	if !ok || e.EventBootTimeNS < minBootNS || e.EventBootTimeNS > maxBootNS {
		return Event{}, false
	}
	return e, true
}
