package cube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
)

func TestApplyRealmProfileUsesHostOwnedFailClosedNetworkMapping(t *testing.T) {
	requests := make(chan map[string]any, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/sandboxes/sandbox-a/network" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := New(Config{APIURL: server.URL, TemplateID: "template-a", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []realm.NetworkProfile{realm.R0InternalOnly, realm.R1PublicRead, realm.R2SupplyChain} {
		if err := client.ApplyRealmProfile(context.Background(), substrate.Handle("sandbox-a"), profile); err != nil {
			t.Fatalf("profile %s: %v", profile, err)
		}
		body := <-requests
		if got, ok := body["allowInternetAccess"].(bool); !ok || got {
			t.Fatalf("profile %s raw internet=%v body=%v", profile, body["allowInternetAccess"], body)
		}
		if got, ok := body["allowPublicTraffic"].(bool); !ok || got {
			t.Fatalf("profile %s public traffic=%v body=%v", profile, body["allowPublicTraffic"], body)
		}
		for _, raw := range []string{"allowOut", "denyOut", "rules"} {
			if _, ok := body[raw]; ok {
				t.Fatalf("profile %s leaked raw network policy %q: %v", profile, raw, body)
			}
		}
	}
}
