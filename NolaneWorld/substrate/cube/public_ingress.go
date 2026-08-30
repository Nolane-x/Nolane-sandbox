package cube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type PublicIngressObservation struct {
	CanaryReached         bool
	UnauthenticatedDenied bool
}

// ProbeRestrictedPublicIngress proves two facts in order: the canary is
// reachable through the sandbox public proxy with the host-owned traffic
// credential, and the same route is denied when no traffic credential is
// presented. A denial is never accepted as proof unless the positive control
// succeeded first.
func (s *GuestSession) ProbeRestrictedPublicIngress(ctx context.Context, port int, path, marker string) (PublicIngressObservation, error) {
	if s == nil || s.http == nil || s.sandboxID == "" || s.domain == "" || s.trafficAccessToken == "" {
		return PublicIngressObservation{}, ErrGuestUnavailable
	}
	if port < 1 || port > 65535 || !validPublicProbePath(path) || marker == "" || len(marker) > 4096 || strings.IndexByte(marker, 0) >= 0 {
		return PublicIngressObservation{}, ErrInvalidConfig
	}

	host := fmt.Sprintf("%d-%s.%s", port, s.sandboxID, s.domain)
	target := s.proxyScheme + "://" + host + path

	privileged, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return PublicIngressObservation{}, errors.Join(ErrGuestUnavailable, err)
	}
	privileged.Header.Set("cube-traffic-access-token", s.trafficAccessToken)
	resp, err := s.http.Do(privileged)
	if err != nil {
		return PublicIngressObservation{}, errors.Join(ErrGuestUnavailable, err)
	}
	body, readErr := readPublicProbeBody(resp)
	resp.Body.Close()
	if readErr != nil {
		return PublicIngressObservation{}, errors.Join(ErrGuestUnavailable, readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !strings.Contains(body, marker) {
		return PublicIngressObservation{}, ErrGuestUnavailable
	}

	observation := PublicIngressObservation{CanaryReached: true}
	unauthenticated, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return observation, errors.Join(ErrGuestUnavailable, err)
	}
	resp, err = s.http.Do(unauthenticated)
	if err != nil {
		return observation, errors.Join(ErrGuestUnavailable, err)
	}
	_, readErr = readPublicProbeBody(resp)
	resp.Body.Close()
	if readErr != nil {
		return observation, errors.Join(ErrGuestUnavailable, readErr)
	}
	observation.UnauthenticatedDenied = resp.StatusCode == http.StatusForbidden
	return observation, nil
}

func validPublicProbePath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.ContainsAny(path, "\r\n\x00")
}

func readPublicProbeBody(resp *http.Response) (string, error) {
	if resp == nil || resp.Body == nil {
		return "", ErrInvalidResponse
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil {
		return "", err
	}
	if len(body) > 64*1024 {
		return "", ErrResponseTooLarge
	}
	return string(body), nil
}
