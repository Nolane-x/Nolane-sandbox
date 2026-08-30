package realm

import (
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestDurableRestartDoesNotTrustPriorRealizationAsReady(t *testing.T) {
	dir:=t.TempDir()
	s,err:=OpenDurableStore(dir);if err!=nil { t.Fatal(err) }
	spec:=validSpec();if _,err:=s.CreateRealm(spec);err!=nil { t.Fatal(err) }
	wr:=WorldRecord{RealmID:spec.ID,WorldID:world.ID("world-a"),RealizationRevision:7,Phase:WorldLeased,LeaseGeneration:4,LeaseExpiresUnix:time.Now().Add(time.Hour).Unix(),Handle:substrate.Handle("sandbox-private")}
	if err:=s.PutWorld(wr);err!=nil { t.Fatal(err) }
	if err:=s.Close();err!=nil { t.Fatal(err) }

	reopened,err:=OpenDurableStore(dir);if err!=nil { t.Fatal(err) }
	defer reopened.Close()
	got,ok:=reopened.World(spec.ID,wr.WorldID);if !ok { t.Fatal("world missing") }
	if got.Phase!=WorldCreating { t.Fatalf("recovered phase=%s want creating",got.Phase) }
	if got.Handle!="" { t.Fatalf("stale host handle survived recovery projection: %q",got.Handle) }
	if got.RealizationRevision!=wr.RealizationRevision || got.LeaseGeneration!=wr.LeaseGeneration { t.Fatalf("fences changed: got=%+v old=%+v",got,wr) }
}

func TestTerminalWorldRemainsTerminalAcrossRestart(t *testing.T) {
	dir:=t.TempDir();s,err:=OpenDurableStore(dir);if err!=nil { t.Fatal(err) }
	spec:=validSpec();if _,err:=s.CreateRealm(spec);err!=nil { t.Fatal(err) }
	wr:=WorldRecord{RealmID:spec.ID,WorldID:world.ID("world-a"),RealizationRevision:1,Phase:WorldTerminal,LeaseGeneration:1,LeaseExpiresUnix:time.Now().Add(time.Hour).Unix()}
	if err:=s.PutWorld(wr);err!=nil { t.Fatal(err) };if err:=s.Close();err!=nil { t.Fatal(err) }
	r,err:=OpenDurableStore(dir);if err!=nil { t.Fatal(err) };defer r.Close()
	got,_:=r.World(spec.ID,wr.WorldID);if got.Phase!=WorldTerminal { t.Fatalf("phase=%s",got.Phase) }
}
