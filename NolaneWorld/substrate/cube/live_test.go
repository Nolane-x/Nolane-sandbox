package cube

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func connectFrame(flags byte, payload []byte) []byte {
	var h [5]byte
	h[0] = flags
	binary.BigEndian.PutUint32(h[1:], uint32(len(payload)))
	return append(h[:], payload...)
}

func TestLiveHealthAndConnectGuestKeepTokensInsideSession(t *testing.T) {
	var calls []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case "/sandboxes/sb-1/connect":
			_, _ = io.WriteString(w, `{"templateID":"tpl","sandboxID":"sb-1","clientID":"c","envdVersion":"v","envdAccessToken":"envd-secret","trafficAccessToken":"traffic-secret","domain":"sandbox.test"}`)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	c, err := New(Config{APIURL: ts.URL, TemplateID: "tpl", APIKey: "control-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err := c.ConnectGuest(context.Background(), substrate.Handle("sb-1"))
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || s.SandboxID() != "sb-1" {
		t.Fatalf("session=%#v", s)
	}
	if strings.Contains(strings.Join(calls, ","), "secret") {
		t.Fatalf("tokens leaked in request trace: %v", calls)
	}
	if got := strings.Join(calls, ","); got != "GET /health,POST /sandboxes/sb-1/connect" {
		t.Fatalf("calls=%s", got)
	}
}

func TestGuestSessionRunsFixedCommandThroughEnvdConnectStream(t *testing.T) {
	var mu sync.Mutex
	var gotHost, gotPath, gotAccess, gotTraffic, gotAuth string
	var gotPayload map[string]any

	dataClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		gotHost, gotPath = r.URL.Host, r.URL.Path
		gotAccess = r.Header.Get("X-Access-Token")
		gotTraffic = r.Header.Get("cube-traffic-access-token")
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		if len(raw) < 5 {
			t.Fatalf("short connect frame")
		}
		sz := binary.BigEndian.Uint32(raw[1:5])
		if int(sz) != len(raw)-5 {
			t.Fatalf("frame length=%d body=%d", sz, len(raw)-5)
		}
		if err := json.Unmarshal(raw[5:], &gotPayload); err != nil {
			t.Fatal(err)
		}

		start, _ := json.Marshal(map[string]any{"event": map[string]any{"start": map[string]any{"pid": 7}}})
		data, _ := json.Marshal(map[string]any{"event": map[string]any{"data": map[string]any{"stdout": base64.StdEncoding.EncodeToString([]byte("NOLANE_LIVE_V5_CANARY"))}}})
		end, _ := json.Marshal(map[string]any{"event": map[string]any{"end": map[string]any{"exitCode": 0}}})
		body := append(connectFrame(0, start), connectFrame(0, data)...)
		body = append(body, connectFrame(0, end)...)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}

	s := newGuestSessionForTest("sb-1", "sandbox.test", "envd-secret", "traffic-secret", dataClient, 1<<20)
	obs, err := s.RunCanary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if obs.ExitCode != 0 || obs.Stdout != "NOLANE_LIVE_V5_CANARY" || obs.Stderr != "" {
		t.Fatalf("obs=%+v", obs)
	}
	if gotHost != "49983-sb-1.sandbox.test" || gotPath != "/process.Process/Start" {
		t.Fatalf("target=%s%s", gotHost, gotPath)
	}
	if gotAccess != "envd-secret" || gotTraffic != "traffic-secret" {
		t.Fatalf("data tokens missing")
	}
	if gotAuth != "Basic cm9vdDo=" {
		t.Fatalf("authorization=%q", gotAuth)
	}
	process := gotPayload["process"].(map[string]any)
	if process["cmd"] != "/bin/bash" {
		t.Fatalf("process=%v", process)
	}
	args := process["args"].([]any)
	if args[2] != "printf %s NOLANE_LIVE_V5_CANARY" {
		t.Fatalf("args=%v", args)
	}
}

func TestUpdateNetworkUsesTypedFullReplacementBody(t *testing.T) {
	var got map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/sandboxes/sb/network" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	c, _ := New(Config{APIURL: ts.URL, TemplateID: "tpl"})
	deny := false
	p := NetworkPolicy{AllowInternetAccess: &deny, DenyOut: []string{"169.254.169.254/32"}}
	if err := c.UpdateNetwork(context.Background(), "sb", p); err != nil {
		t.Fatal(err)
	}
	if got["allowInternetAccess"] != false {
		t.Fatalf("body=%v", got)
	}
	denyOut := got["denyOut"].([]any)
	if denyOut[0] != "169.254.169.254/32" {
		t.Fatalf("body=%v", got)
	}
}

func TestDestroyObservedWaitsUntilGetReturnsNotFound(t *testing.T) {
	var gets int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet:
			gets++
			if gets < 3 {
				_, _ = io.WriteString(w, `{"sandboxID":"sb","state":"running"}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()
	c, _ := New(Config{APIURL: ts.URL, TemplateID: "tpl"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.DestroyObserved(ctx, "sb", 1*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if gets != 3 {
		t.Fatalf("gets=%d", gets)
	}
}

func TestGuestStreamPreservesEqualStdoutAndStderrPayloads(t *testing.T) {
	same := base64.StdEncoding.EncodeToString([]byte("same"))
	data, _ := json.Marshal(map[string]any{"event": map[string]any{"data": map[string]any{"stdout": same, "stderr": same}}})
	end, _ := json.Marshal(map[string]any{"event": map[string]any{"end": map[string]any{"exitCode": 0}}})
	body := append(connectFrame(0, data), connectFrame(0, end)...)
	obs, err := parseGuestProcessStream(bytes.NewReader(body), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Stdout != "same" || obs.Stderr != "same" {
		t.Fatalf("obs=%+v", obs)
	}
}
