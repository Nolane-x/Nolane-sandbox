package cube

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProbeRestrictedPublicIngressUsesPrivilegedControlThenUnauthenticatedDenial(t *testing.T) {
	var calls int
	dataClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet || r.URL.Host != "18080-sb-1.sandbox.test" || r.URL.Path != "/nolane-live-v9-ingress" {
			t.Fatalf("unexpected request %s %s%s", r.Method, r.URL.Host, r.URL.Path)
		}
		if calls == 1 {
			if got := r.Header.Get("cube-traffic-access-token"); got != "traffic-secret" {
				t.Fatalf("privileged control token=%q", got)
			}
			if r.Header.Get("X-Access-Token") != "" || r.Header.Get("Authorization") != "" {
				t.Fatalf("public control request carried envd/basic authority: %v", r.Header)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("NOLANE_LIVE_V9_INGRESS")), Header: make(http.Header)}, nil
		}
		for _, header := range []string{"cube-traffic-access-token", "e2b-traffic-access-token", "X-Access-Token", "Authorization"} {
			if got := r.Header.Get(header); got != "" {
				t.Fatalf("unauthenticated probe leaked %s=%q", header, got)
			}
		}
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("forbidden")), Header: make(http.Header)}, nil
	})}

	s := newGuestSessionForTest("sb-1", "sandbox.test", "envd-secret", "traffic-secret", dataClient, 1<<20)
	obs, err := s.ProbeRestrictedPublicIngress(context.Background(), 18080, "/nolane-live-v9-ingress", "NOLANE_LIVE_V9_INGRESS")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !obs.CanaryReached || !obs.UnauthenticatedDenied {
		t.Fatalf("calls=%d obs=%+v", calls, obs)
	}
}

func TestProbeRestrictedPublicIngressObservesPolicyViolationWithoutCallingItTransportFailure(t *testing.T) {
	var calls int
	dataClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body := "NOLANE_LIVE_V9_INGRESS"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	s := newGuestSessionForTest("sb-1", "sandbox.test", "", "traffic-secret", dataClient, 1<<20)
	obs, err := s.ProbeRestrictedPublicIngress(context.Background(), 18080, "/nolane-live-v9-ingress", "NOLANE_LIVE_V9_INGRESS")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !obs.CanaryReached || obs.UnauthenticatedDenied {
		t.Fatalf("calls=%d obs=%+v", calls, obs)
	}
}

func TestProbeRestrictedPublicIngressRequiresPositiveControlBeforeDenialProof(t *testing.T) {
	var calls int
	dataClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("forbidden")), Header: make(http.Header)}, nil
	})}
	s := newGuestSessionForTest("sb-1", "sandbox.test", "", "traffic-secret", dataClient, 1<<20)
	obs, err := s.ProbeRestrictedPublicIngress(context.Background(), 18080, "/nolane-live-v9-ingress", "NOLANE_LIVE_V9_INGRESS")
	if err == nil || !errors.Is(err, ErrGuestUnavailable) {
		t.Fatalf("err=%v obs=%+v", err, obs)
	}
	if calls != 1 || obs.CanaryReached || obs.UnauthenticatedDenied {
		t.Fatalf("calls=%d obs=%+v", calls, obs)
	}
}

func TestProbeRestrictedPublicIngressRejectsInvalidProbeParameters(t *testing.T) {
	s := newGuestSessionForTest("sb-1", "sandbox.test", "", "traffic-secret", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid probe reached transport")
		return nil, nil
	})}, 1<<20)
	for _, tc := range []struct {
		port   int
		path   string
		marker string
	}{
		{0, "/x", "m"},
		{65536, "/x", "m"},
		{18080, "relative", "m"},
		{18080, "//evil.example/x", "m"},
		{18080, "/x", ""},
	} {
		if _, err := s.ProbeRestrictedPublicIngress(context.Background(), tc.port, tc.path, tc.marker); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("case=%+v err=%v", tc, err)
		}
	}
}
