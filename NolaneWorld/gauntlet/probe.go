package gauntlet

import "sync"

type Probe struct {
	mu     sync.RWMutex
	events []Event
	sealed bool
}

func newProbe() *Probe { return &Probe{} }

func (p *Probe) Record(kind EventKind, marker, detail string) error {
	if p == nil || !kind.valid() || !nonBlank(marker) || !nonBlank(detail) || len(marker) > 256 || len(detail) > 4096 {
		return ErrInvalidEvent
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sealed {
		return ErrProbeSealed
	}
	p.events = append(p.events, Event{Marker: marker, Kind: kind, Detail: detail})
	return nil
}

func (p *Probe) Events() []Event {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]Event(nil), p.events...)
}

func (p *Probe) seal() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.sealed = true
	p.mu.Unlock()
}
