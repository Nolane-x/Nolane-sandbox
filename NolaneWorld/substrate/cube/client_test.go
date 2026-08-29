package cube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestNewRejectsRemoteCleartextAPI(t *testing.T) {
	_, err := New(Config{APIURL: "http://example.com", TemplateID: "tpl"})
	if !errors.Is(err, ErrInsecureAPI) {
		t.Fatalf("error=%v want ErrInsecureAPI", err)
	}
}

func TestCreateIsFailClosedAndBindsWorldIdentity(t *testing.T) {
	var got map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sandboxes" || r.Method != "POST" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("missing api key")
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"sandboxID":"sb-1"}`)
	}))
	defer ts.Close()

	c, err := New(Config{APIURL: ts.URL, TemplateID: "tpl", APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Create(context.Background(), world.ID("w1"))
	if err != nil {
		t.Fatal(err)
	}
	if h != "sb-1" {
		t.Fatalf("handle=%q", h)
	}
	if got["templateID"] != "tpl" {
		t.Fatalf("templateID=%v", got["templateID"])
	}
	if got["allow_internet_access"] != false {
		t.Fatalf("allow_internet_access=%v want false", got["allow_internet_access"])
	}
	md := got["metadata"].(map[string]any)
	if md["nolane.world.id"] != "w1" {
		t.Fatalf("metadata=%v", md)
	}
	nw := got["network"].(map[string]any)
	if nw["allowPublicTraffic"] != false {
		t.Fatalf("network=%v", nw)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	var reached bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sandboxes" {
			http.Redirect(w, r, "/steal", http.StatusFound)
			return
		}
		if r.URL.Path == "/steal" {
			reached = true
			_, _ = io.WriteString(w, `{"sandboxID":"bad"}`)
		}
	}))
	defer ts.Close()
	c, _ := New(Config{APIURL: ts.URL, TemplateID: "tpl", APIKey: "secret"})
	_, err := c.Create(context.Background(), "w1")
	if err == nil {
		t.Fatal("expected redirect rejection")
	}
	if reached {
		t.Fatal("client followed redirect carrying host credentials")
	}
}

func TestSnapshotRollbackCloneWireContract(t *testing.T) {
	var calls []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/sandboxes/sb/snapshots":
			_, _ = io.WriteString(w, `{"snapshotID":"snap-1"}`)
		case r.Method == "POST" && r.URL.Path == "/sandboxes/sb/rollback":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["snapshotID"] != "snap-1" {
				t.Fatalf("rollback body=%v", body)
			}
			_, _ = io.WriteString(w, `{"status":"success"}`)
		case r.Method == "POST" && r.URL.Path == "/sandboxes":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["templateID"] != "snap-1" {
				t.Fatalf("clone template=%v", body)
			}
			if body["metadata"].(map[string]any)["nolane.world.id"] != "child" {
				t.Fatalf("clone metadata=%v", body)
			}
			_, _ = io.WriteString(w, `{"sandboxID":"sb-child"}`)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()
	c, _ := New(Config{APIURL: ts.URL, TemplateID: "tpl"})
	s, err := c.Snapshot(context.Background(), "sb")
	if err != nil || s != "snap-1" {
		t.Fatalf("snapshot %q %v", s, err)
	}
	if err := c.Rollback(context.Background(), "sb", s); err != nil {
		t.Fatal(err)
	}
	h, err := c.Clone(context.Background(), "sb", s, "child")
	if err != nil || h != "sb-child" {
		t.Fatalf("clone %q %v", h, err)
	}
	if strings.Join(calls, ",") != "POST /sandboxes/sb/snapshots,POST /sandboxes/sb/rollback,POST /sandboxes" {
		t.Fatalf("calls=%v", calls)
	}
}

func TestResponseLimitFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 2048))
	}))
	defer ts.Close()
	c, _ := New(Config{APIURL: ts.URL, TemplateID: "tpl", MaxResponseBytes: 128})
	_, err := c.Create(context.Background(), "w1")
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("error=%v", err)
	}
}

var _ substrate.SandboxSubstrate = (*Client)(nil)

func TestNewRejectsCredentialedOrDecoratedBaseURL(t *testing.T) {
	cases := []string{
		"https://user:pass@example.com",
		"https://example.com/base",
		"https://example.com?x=1",
		"https://example.com#fragment",
	}
	for _, raw := range cases {
		if _, err := New(Config{APIURL: raw, TemplateID: "tpl"}); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("New(%q) error=%v want ErrInvalidConfig", raw, err)
		}
	}
}
