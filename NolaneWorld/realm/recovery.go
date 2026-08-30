package realm

// FenceRealizationsForRecovery invalidates process-local realization truth after
// a host restart without rewriting durable history. The fabric must reconcile
// a realization before it can become observed-ready/leased again.
func (s *DurableStore) FenceRealizationsForRecovery() {
	if s == nil { return }
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed { return }
	for key, rec := range s.worlds {
		if rec.Phase == WorldTerminal { continue }
		rec.Phase = WorldCreating
		rec.Handle = ""
		s.worlds[key] = rec
	}
	for id, rec := range s.services {
		if rec.Ready {
			rec.Ready = false
			s.services[id] = rec
		}
	}
}
