package cube

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	realmproof "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	cubewire "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate/cube"
)

var _ realmproof.RealmProfileSandbox = (*box)(nil)

func TestLiveBoxAppliesExactRealmProfileThroughCubeNetworkAPI(t *testing.T) {
	var body map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/sandboxes/sb/network" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer api.Close()
	client, err := cubewire.New(cubewire.Config{APIURL: api.URL, TemplateID: "tpl"})
	if err != nil {
		t.Fatal(err)
	}
	b := &box{client: client, handle: substrate.Handle("sb")}
	if err := b.ApplyRealmProfile(context.Background(), realm.R2SupplyChain); err != nil {
		t.Fatal(err)
	}
	if body["allowInternetAccess"] != false || body["allowPublicTraffic"] != false {
		t.Fatalf("profile did not remain fail-closed: %v", body)
	}
	if _, ok := body["allowOut"]; ok {
		t.Fatalf("Realm profile escaped into raw allowOut: %v", body)
	}
	if _, ok := body["rules"]; ok {
		t.Fatalf("Realm profile escaped into raw rules: %v", body)
	}
}

func TestLiveBoxPublicIngressProofStartsCanaryThenUsesRestrictedProxyProbe(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sandboxes/sb/connect" {
			t.Fatalf("unexpected control request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"sandboxID":"sb","envdAccessToken":"envd-secret","trafficAccessToken":"traffic-secret","domain":"sandbox.test"}`)
	}))
	defer api.Close()

	var calls int
	data := &http.Client{Transport: realmRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if r.Method != http.MethodPost || r.URL.Host != "49983-sb.sandbox.test" {
				t.Fatalf("unexpected canary start request %s %s", r.Method, r.URL.String())
			}
			raw, _ := io.ReadAll(r.Body)
			if !bytes.Contains(raw, []byte("NOLANE_LIVE_V9_INGRESS")) || bytes.Contains(raw, []byte("traffic-secret")) {
				t.Fatalf("unsafe canary command payload: %q", raw)
			}
			return connectSuccessResponse(), nil
		case 2:
			if r.Method != http.MethodGet || r.URL.Host != "18080-sb.sandbox.test" || r.URL.Path != "/nolane-live-v9-ingress" {
				t.Fatalf("unexpected privileged probe %s %s", r.Method, r.URL.String())
			}
			if r.Header.Get("cube-traffic-access-token") != "traffic-secret" {
				t.Fatalf("positive control missing host-owned traffic token")
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("NOLANE_LIVE_V9_INGRESS")), Header: make(http.Header)}, nil
		case 3:
			if r.Method != http.MethodGet || r.URL.Host != "18080-sb.sandbox.test" {
				t.Fatalf("unexpected unauthenticated probe %s %s", r.Method, r.URL.String())
			}
			for _, header := range []string{"cube-traffic-access-token", "e2b-traffic-access-token", "X-Access-Token", "Authorization"} {
				if r.Header.Get(header) != "" {
					t.Fatalf("unauthenticated public probe carried %s", header)
				}
			}
			return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("forbidden")), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected data-plane request %d: %s", calls, r.URL.String())
			return nil, nil
		}
	})}

	client, err := cubewire.New(cubewire.Config{APIURL: api.URL, TemplateID: "tpl", DataHTTPClient: data})
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.ConnectGuest(context.Background(), substrate.Handle("sb"))
	if err != nil {
		t.Fatal(err)
	}
	b := &box{client: client, handle: substrate.Handle("sb"), session: session}
	observation, err := b.ProbePublicIngress(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || !observation.Denied || observation.Marker != "external-restricted-proxy-denied" {
		t.Fatalf("calls=%d observation=%+v", calls, observation)
	}
}

type realmRoundTripFunc func(*http.Request) (*http.Response, error)

func (f realmRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func connectSuccessResponse() *http.Response {
	start, _ := json.Marshal(map[string]any{"event": map[string]any{"start": map[string]any{"pid": 9}}})
	end, _ := json.Marshal(map[string]any{"event": map[string]any{"end": map[string]any{"exitCode": 0}}})
	body := append(realmConnectFrame(0, start), realmConnectFrame(0, end)...)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}
}

func realmConnectFrame(flags byte, payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = flags
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}
