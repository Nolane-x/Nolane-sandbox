package cube

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
)

const (
	envdPort              = 49983
	connectEndStreamFlag  = byte(0x02)
	connectCompressedFlag = byte(0x01)
	liveCanary            = "NOLANE_LIVE_V5_CANARY"
	liveCanaryCommand     = "printf %s " + liveCanary
	maxLiveCommandBytes   = 16 * 1024
)

var (
	ErrGuestUnavailable = errors.New("cube: guest data plane unavailable")
	ErrGuestProtocol    = errors.New("cube: invalid guest protocol")
	ErrCleanupUncertain = errors.New("cube: cleanup not observed")
)

type NetworkPolicy struct {
	AllowInternetAccess *bool
	AllowPublicTraffic  *bool
	AllowOut            []string
	DenyOut             []string
	Rules               []EgressRule
}

type EgressRule struct {
	Name   string           `json:"name"`
	Match  EgressMatch      `json:"match"`
	Action EgressRuleAction `json:"action"`
}

type EgressMatch struct {
	SNI    string   `json:"sni,omitempty"`
	Host   string   `json:"host,omitempty"`
	Method []string `json:"method,omitempty"`
	Path   string   `json:"path,omitempty"`
	Scheme string   `json:"scheme,omitempty"`
	Port   int      `json:"port,omitempty"`
}

type EgressRuleAction struct {
	Allow bool   `json:"allow"`
	Audit string `json:"audit,omitempty"`
}

type GuestObservation struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type GuestSession struct {
	sandboxID          string
	domain             string
	envdAccessToken    string
	trafficAccessToken string
	proxyScheme        string
	maxBytes           int64
	http               *http.Client
}

func (s *GuestSession) SandboxID() string {
	if s == nil {
		return ""
	}
	return s.sandboxID
}

func (s *GuestSession) SandboxDigest() string {
	if s == nil {
		return ""
	}
	h := sha256.Sum256([]byte(s.sandboxID))
	return hex.EncodeToString(h[:])
}

func (c *Client) Health(ctx context.Context) error {
	var out map[string]any
	return c.doJSON(ctx, http.MethodGet, "/health", nil, &out, false)
}

func (c *Client) ConnectGuest(ctx context.Context, h substrate.Handle) (*GuestSession, error) {
	if c == nil || h == "" {
		return nil, ErrInvalidConfig
	}
	var out struct {
		SandboxID          string `json:"sandboxID"`
		EnvdAccessToken    string `json:"envdAccessToken"`
		TrafficAccessToken string `json:"trafficAccessToken"`
		Domain             string `json:"domain"`
	}
	path := "/sandboxes/" + url.PathEscape(string(h)) + "/connect"
	if err := c.doJSON(ctx, http.MethodPost, path, map[string]any{}, &out, false); err != nil {
		return nil, err
	}
	if out.SandboxID == "" || out.SandboxID != string(h) {
		return nil, ErrInvalidResponse
	}
	domain := out.Domain
	if domain == "" {
		domain = c.sandboxDomain
	}
	if domain == "" || strings.ContainsAny(domain, "/?#@") {
		return nil, errors.Join(ErrGuestUnavailable, ErrInvalidResponse)
	}
	if c.dataHTTP == nil {
		return nil, ErrGuestUnavailable
	}
	return &GuestSession{
		sandboxID: out.SandboxID, domain: domain, envdAccessToken: out.EnvdAccessToken,
		trafficAccessToken: out.TrafficAccessToken, proxyScheme: c.proxyScheme,
		maxBytes: c.maxBytes, http: c.dataHTTP,
	}, nil
}

func (c *Client) UpdateNetwork(ctx context.Context, h substrate.Handle, policy NetworkPolicy) error {
	if c == nil || h == "" {
		return ErrInvalidConfig
	}
	body := make(map[string]any)
	if policy.AllowInternetAccess != nil {
		body["allowInternetAccess"] = *policy.AllowInternetAccess
	}
	if policy.AllowPublicTraffic != nil {
		body["allowPublicTraffic"] = *policy.AllowPublicTraffic
	}
	if policy.AllowOut != nil {
		body["allowOut"] = policy.AllowOut
	}
	if policy.DenyOut != nil {
		body["denyOut"] = policy.DenyOut
	}
	if policy.Rules != nil {
		body["rules"] = policy.Rules
	}
	return c.doJSON(ctx, http.MethodPut, "/sandboxes/"+url.PathEscape(string(h))+"/network", body, nil, false)
}

func (c *Client) DestroyObserved(ctx context.Context, h substrate.Handle, poll time.Duration) error {
	if c == nil || h == "" {
		return ErrInvalidConfig
	}
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	if err := c.Destroy(ctx, h); err != nil {
		return err
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		present, err := c.sandboxPresent(ctx, h)
		if err != nil {
			return errors.Join(ErrCleanupUncertain, err)
		}
		if !present {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(ErrCleanupUncertain, ctx.Err())
		case <-t.C:
		}
	}
}

func (c *Client) sandboxPresent(ctx context.Context, h substrate.Handle) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/sandboxes/"+url.PathEscape(string(h)), nil)
	if err != nil {
		return false, errors.Join(ErrRequestFailed, err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, errors.Join(ErrRequestFailed, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, c.maxBytes+1))
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("%w: status=%d", ErrRequestFailed, resp.StatusCode)
	}
	return true, nil
}

func (s *GuestSession) RunCanary(ctx context.Context) (GuestObservation, error) {
	return s.RunCommand(ctx, liveCanaryCommand)
}

func (s *GuestSession) RunCommand(ctx context.Context, command string) (GuestObservation, error) {
	if s == nil || s.http == nil || s.sandboxID == "" || s.domain == "" {
		return GuestObservation{}, ErrGuestUnavailable
	}
	if command == "" || len(command) > maxLiveCommandBytes || strings.IndexByte(command, 0) >= 0 {
		return GuestObservation{}, ErrInvalidConfig
	}
	payload := map[string]any{
		"process": map[string]any{"cmd": "/bin/bash", "args": []string{"-l", "-c", command}, "envs": map[string]string{}},
		"stdin":   false,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return GuestObservation{}, errors.Join(ErrGuestProtocol, err)
	}
	framed := encodeConnectEnvelope(raw)
	host := fmt.Sprintf("%d-%s.%s", envdPort, s.sandboxID, s.domain)
	target := s.proxyScheme + "://" + host + "/process.Process/Start"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(framed))
	if err != nil {
		return GuestObservation{}, errors.Join(ErrGuestProtocol, err)
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Connect-Content-Encoding", "identity")
	req.Header.Set("Authorization", "Basic cm9vdDo=")
	if s.envdAccessToken != "" {
		req.Header.Set("X-Access-Token", s.envdAccessToken)
	}
	if s.trafficAccessToken != "" {
		req.Header.Set("e2b-traffic-access-token", s.trafficAccessToken)
		req.Header.Set("cube-traffic-access-token", s.trafficAccessToken)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return GuestObservation{}, errors.Join(ErrGuestUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GuestObservation{}, fmt.Errorf("%w: status=%d", ErrGuestUnavailable, resp.StatusCode)
	}
	return parseGuestProcessStream(io.LimitReader(resp.Body, s.maxBytes+1), s.maxBytes)
}

func newGuestSessionForTest(id, domain, envdToken, trafficToken string, hc *http.Client, maxBytes int64) *GuestSession {
	return &GuestSession{sandboxID: id, domain: domain, envdAccessToken: envdToken, trafficAccessToken: trafficToken, proxyScheme: "https", http: hardenedHTTPClient(hc, 30*time.Second), maxBytes: maxBytes}
}

func encodeConnectEnvelope(payload []byte) []byte {
	out := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:], payload)
	return out
}

type guestProcessResponse struct {
	Event *guestProcessEvent `json:"event"`
}
type guestProcessEvent struct {
	Data *struct {
		Stdout string `json:"stdout,omitempty"`
		Stderr string `json:"stderr,omitempty"`
	} `json:"data,omitempty"`
	End *struct {
		ExitCode      *int   `json:"exitCode,omitempty"`
		ExitCodeSnake *int   `json:"exit_code,omitempty"`
		Error         string `json:"error,omitempty"`
	} `json:"end,omitempty"`
}

func parseGuestProcessStream(r io.Reader, maxBytes int64) (GuestObservation, error) {
	var obs GuestObservation
	var stdout, stderr strings.Builder
	var total int64
	sawEnd := false
	for {
		var hdr [5]byte
		_, err := io.ReadFull(r, hdr[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return GuestObservation{}, errors.Join(ErrGuestProtocol, err)
		}
		sz := int64(binary.BigEndian.Uint32(hdr[1:]))
		total += 5 + sz
		if sz < 0 || total > maxBytes {
			return GuestObservation{}, ErrResponseTooLarge
		}
		payload := make([]byte, sz)
		if _, err := io.ReadFull(r, payload); err != nil {
			return GuestObservation{}, errors.Join(ErrGuestProtocol, err)
		}
		if hdr[0]&connectCompressedFlag != 0 {
			return GuestObservation{}, ErrGuestProtocol
		}
		if hdr[0]&connectEndStreamFlag != 0 {
			continue
		}
		var frame guestProcessResponse
		if err := json.Unmarshal(payload, &frame); err != nil {
			return GuestObservation{}, errors.Join(ErrGuestProtocol, err)
		}
		if frame.Event == nil {
			continue
		}
		if frame.Event.Data != nil {
			if frame.Event.Data.Stdout != "" {
				b, err := base64.StdEncoding.DecodeString(frame.Event.Data.Stdout)
				if err != nil {
					return GuestObservation{}, errors.Join(ErrGuestProtocol, err)
				}
				stdout.Write(b)
			}
			if frame.Event.Data.Stderr != "" {
				b, err := base64.StdEncoding.DecodeString(frame.Event.Data.Stderr)
				if err != nil {
					return GuestObservation{}, errors.Join(ErrGuestProtocol, err)
				}
				stderr.Write(b)
			}
		}
		if frame.Event.End != nil {
			code := frame.Event.End.ExitCode
			if code == nil {
				code = frame.Event.End.ExitCodeSnake
			}
			if code == nil {
				return GuestObservation{}, ErrGuestProtocol
			}
			obs.ExitCode = *code
			if frame.Event.End.Error != "" && *code == 0 {
				return GuestObservation{}, ErrGuestProtocol
			}
			sawEnd = true
		}
	}
	if !sawEnd {
		return GuestObservation{}, ErrGuestProtocol
	}
	obs.Stdout, obs.Stderr = stdout.String(), stderr.String()
	return obs, nil
}
