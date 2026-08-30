package realm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestDurableStoreRoundTripAndTamperDetection(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenDurableStore(dir)
	if err != nil { t.Fatal(err) }
	spec := validSpec()
	rec, err := store.CreateRealm(spec)
	if err != nil { t.Fatal(err) }
	wr := WorldRecord{
		RealmID: spec.ID, WorldID: world.ID("realm-test-world"), RealizationRevision: 1,
		Phase: WorldObservedReady, LeaseGeneration: 1, LeaseExpiresUnix: time.Now().Add(time.Minute).Unix(),
	}
	if err := store.PutWorld(wr); err != nil { t.Fatal(err) }
	if err := store.Close(); err != nil { t.Fatal(err) }

	reopened, err := OpenDurableStore(dir)
	if err != nil { t.Fatal(err) }
	got, ok := reopened.Realm(spec.ID)
	if !ok || got.Revision != rec.Revision { t.Fatalf("realm=%+v ok=%v", got, ok) }
	gotWorld, ok := reopened.World(spec.ID, wr.WorldID)
	if !ok || gotWorld.WorldID != wr.WorldID { t.Fatalf("world=%+v ok=%v", gotWorld, ok) }
	if err := reopened.Close(); err != nil { t.Fatal(err) }

	path := filepath.Join(dir, "realm-state.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	if len(raw) < 20 { t.Fatal("journal unexpectedly small") }
	raw[len(raw)/2] ^= 1
	if err := os.WriteFile(path, raw, 0o600); err != nil { t.Fatal(err) }
	if _, err := OpenDurableStore(dir); err == nil { t.Fatal("tampered journal reopened") }
}

func TestDurableStoreRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenDurableStore(dir)
	if err != nil { t.Fatal(err) }
	if _, err := store.CreateRealm(validSpec()); err != nil { t.Fatal(err) }
	if err := store.Close(); err != nil { t.Fatal(err) }

	path := filepath.Join(dir, "realm-state.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	lines[0] = strings.TrimSuffix(lines[0], "}") + `,"unknown":true}`
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil { t.Fatal(err) }
	if _, err := OpenDurableStore(dir); err == nil { t.Fatal("unknown field accepted") }
}
