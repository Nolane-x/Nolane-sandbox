package realm

import "sort"

func (s *DurableStore) Worlds(realmID ID) []WorldRecord {
	if s == nil { return nil }
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed { return nil }
	out := make([]WorldRecord, 0)
	for _, rec := range s.worlds {
		if rec.RealmID == realmID { out = append(out, rec) }
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i].WorldID) < string(out[j].WorldID) })
	return out
}
