package cube

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrInvalidConfig                          = errors.New("cube: invalid config")
	ErrInsecureAPI                            = errors.New("cube: insecure api url")
	ErrRequestFailed                          = errors.New("cube: request failed")
	ErrResponseTooLarge                       = errors.New("cube: response too large")
	ErrInvalidResponse                        = errors.New("cube: invalid response")
	ErrInvalidRuntimeCPUPolicyCreateAuthority = errors.New("cube: invalid runtime CPU policy create authority")
)

const runtimeCPUPolicyCreatePropagationDigestDomain = "nolane.runtime-cpu-policy-create-propagation.v32\x00"

type Config struct {
	APIURL           string
	APIKey           string
	TemplateID       string
	SandboxDomain    string
	ProxyScheme      string
	MaxResponseBytes int64
	HTTPClient       *http.Client
	DataHTTPClient   *http.Client
	EndpointSPKIPins []string
}

type Client struct {
	apiURL           string
	apiKey           string
	templateID       string
	sandboxDomain    string
	proxyScheme      string
	maxBytes         int64
	http             *http.Client
	dataHTTP         *http.Client
	endpointSPKIPins map[[32]byte]struct{}
}

func New(cfg Config) (*Client, error) {
	if cfg.APIURL == "" || cfg.TemplateID == "" {
		return nil, ErrInvalidConfig
	}
	u, err := url.Parse(cfg.APIURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, ErrInvalidConfig
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrInvalidConfig
	}
	if u.Scheme == "http" && !loopbackHost(u.Hostname()) {
		return nil, ErrInsecureAPI
	}

	endpointSPKIPins, err := parseEndpointSPKIPins(cfg.EndpointSPKIPins)
	if err != nil {
		return nil, err
	}
	if len(endpointSPKIPins) > 0 && u.Scheme != "https" {
		return nil, ErrInvalidEndpointSPKIConfig
	}

	maxBytes := cfg.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = 1 << 20
	}
	if maxBytes < 1 {
		return nil, ErrInvalidConfig
	}

	var hc *http.Client
	if len(endpointSPKIPins) > 0 {
		hc, err = hardenedPinnedHTTPClient(cfg.HTTPClient, 30*time.Second, endpointSPKIPins)
		if err != nil {
			return nil, err
		}
	} else {
		hc = hardenedHTTPClient(cfg.HTTPClient, 30*time.Second)
	}

	dc := hardenedHTTPClient(cfg.DataHTTPClient, 30*time.Second)
	if cfg.DataHTTPClient == nil && cfg.HTTPClient != nil {
		dc = hardenedHTTPClient(cfg.HTTPClient, 30*time.Second)
	}
	proxyScheme := cfg.ProxyScheme
	if proxyScheme == "" {
		proxyScheme = "https"
	}
	if proxyScheme != "https" && proxyScheme != "http" {
		return nil, ErrInvalidConfig
	}
	return &Client{
		apiURL: strings.TrimRight(cfg.APIURL, "/"), apiKey: cfg.APIKey,
		templateID: cfg.TemplateID, sandboxDomain: cfg.SandboxDomain, proxyScheme: proxyScheme,
		maxBytes: maxBytes, http: hc, dataHTTP: dc, endpointSPKIPins: endpointSPKIPins,
	}, nil
}

func (c *Client) Create(ctx context.Context, id world.ID) (substrate.Handle, error) {
	return c.createFromTemplate(ctx, c.templateID, id)
}

func (c *Client) CreateWithRuntimeCPUPolicy(ctx context.Context, id world.ID, authority realm.RuntimeCPUPolicyAuthority) (substrate.Handle, substrate.RuntimeCPUPolicyCreatePropagation, error) {
	if c == nil || c.templateID == "" || id == "" {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, ErrInvalidConfig
	}
	binding, ok := authority.Binding()
	if !ok {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, ErrInvalidRuntimeCPUPolicyCreateAuthority
	}
	body := map[string]any{
		"templateID":            c.templateID,
		"allow_internet_access": false,
		"metadata": map[string]string{
			"nolane.world.id":                         string(id),
			"nolane.realm.id":                         string(binding.RealmID),
			"nolane.realm.revision":                   strconv.FormatUint(binding.RealmRevision, 10),
			"nolane.realm.policy_digest":              binding.PolicyDigest,
			"nolane.realm.runtime_cpu_policy_digest":  binding.Digest,
			"nolane.realm.runtime_cpu_limit_millicpu": strconv.FormatUint(binding.LimitMilliCPU, 10),
		},
		"network": map[string]any{
			"allowPublicTraffic": false,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, errors.Join(ErrInvalidConfig, err)
	}
	hash := sha256.Sum256(append([]byte(runtimeCPUPolicyCreatePropagationDigestDomain), raw...))
	receipt := substrate.RuntimeCPUPolicyCreatePropagation{
		RealmID:         string(binding.RealmID),
		RealmRevision:   binding.RealmRevision,
		PolicyDigest:    binding.PolicyDigest,
		LimitMilliCPU:   binding.LimitMilliCPU,
		AuthorityDigest: binding.Digest,
		WorldID:         id,
		RequestDigest:   substrate.RuntimeCPUPolicyCreatePropagationDigestPrefix + hex.EncodeToString(hash[:]),
	}
	if !receipt.Valid() {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, ErrInvalidRuntimeCPUPolicyCreateAuthority
	}
	var out struct {
		SandboxID string `json:"sandboxID"`
	}
	if err := c.doJSONBytes(ctx, http.MethodPost, "/sandboxes", raw, &out, false); err != nil {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, err
	}
	if out.SandboxID == "" {
		return "", substrate.RuntimeCPUPolicyCreatePropagation{}, ErrInvalidResponse
	}
	return substrate.Handle(out.SandboxID), receipt, nil
}

func (c *Client) Clone(ctx context.Context, source substrate.Handle, snap substrate.Snapshot, id world.ID) (substrate.Handle, error) {
	if source == "" || snap == "" || id == "" {
		return "", ErrInvalidConfig
	}
	return c.createFromTemplate(ctx, string(snap), id)
}

func (c *Client) createFromTemplate(ctx context.Context, template string, id world.ID) (substrate.Handle, error) {
	if c == nil || template == "" || id == "" {
		return "", ErrInvalidConfig
	}
	body := map[string]any{
		"templateID":            template,
		"allow_internet_access": false,
		"metadata": map[string]string{
			"nolane.world.id": string(id),
		},
		"network": map[string]any{
			"allowPublicTraffic": false,
		},
	}
	var out struct {
		SandboxID string `json:"sandboxID"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/sandboxes", body, &out, false); err != nil {
		return "", err
	}
	if out.SandboxID == "" {
		return "", ErrInvalidResponse
	}
	return substrate.Handle(out.SandboxID), nil
}

func (c *Client) Destroy(ctx context.Context, h substrate.Handle) error {
	if h == "" {
		return ErrInvalidConfig
	}
	return c.doJSON(ctx, http.MethodDelete, "/sandboxes/"+url.PathEscape(string(h)), nil, nil, true)
}

func (c *Client) Pause(ctx context.Context, h substrate.Handle) error {
	if h == "" {
		return ErrInvalidConfig
	}
	return c.doJSON(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(string(h))+"/pause", nil, nil, false)
}

func (c *Client) Resume(ctx context.Context, h substrate.Handle) error {
	if h == "" {
		return ErrInvalidConfig
	}
	return c.doJSON(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(string(h))+"/resume", map[string]any{}, nil, false)
}

func (c *Client) Snapshot(ctx context.Context, h substrate.Handle) (substrate.Snapshot, error) {
	if h == "" {
		return "", ErrInvalidConfig
	}
	var out struct {
		SnapshotID string `json:"snapshotID"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(string(h))+"/snapshots", map[string]any{}, &out, false); err != nil {
		return "", err
	}
	if out.SnapshotID == "" {
		return "", ErrInvalidResponse
	}
	return substrate.Snapshot(out.SnapshotID), nil
}

func (c *Client) Rollback(ctx context.Context, h substrate.Handle, snap substrate.Snapshot) error {
	if h == "" || snap == "" {
		return ErrInvalidConfig
	}
	return c.doJSON(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(string(h))+"/rollback",
		map[string]string{"snapshotID": string(snap)}, nil, false)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any, allowNotFound bool) error {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return errors.Join(ErrInvalidConfig, err)
		}
	}
	return c.doJSONBytes(ctx, method, path, raw, out, allowNotFound)
}

func (c *Client) doJSONBytes(ctx context.Context, method, path string, raw []byte, out any, allowNotFound bool) error {
	if c == nil || c.http == nil {
		return ErrInvalidConfig
	}
	var reader io.Reader
	if raw != nil {
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, reader)
	if err != nil {
		return errors.Join(ErrRequestFailed, err)
	}
	req.Header.Set("Accept", "application/json")
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return errors.Join(ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	responseRaw, err := io.ReadAll(limited)
	if err != nil {
		return errors.Join(ErrRequestFailed, err)
	}
	if int64(len(responseRaw)) > c.maxBytes {
		return ErrResponseTooLarge
	}

	if allowNotFound && resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status=%d", ErrRequestFailed, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if len(responseRaw) == 0 {
		return ErrInvalidResponse
	}
	if err := json.Unmarshal(responseRaw, out); err != nil {
		return errors.Join(ErrInvalidResponse, err)
	}
	return nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hardenedHTTPClient(src *http.Client, timeout time.Duration) *http.Client {
	hc := &http.Client{Timeout: timeout}
	if src != nil {
		copyClient := *src
		hc = &copyClient
		if hc.Timeout == 0 {
			hc.Timeout = timeout
		}
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return hc
}
