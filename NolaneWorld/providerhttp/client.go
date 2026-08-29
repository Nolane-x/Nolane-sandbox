package providerhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

var (
	ErrInvalidProviderConfig  = errors.New("provider http: invalid configuration")
	ErrInvalidProviderRoute   = errors.New("provider http: invalid route")
	ErrProviderTransport      = errors.New("provider http: transport failure")
	ErrProviderResponseTooLarge = errors.New("provider http: response too large")
)

const hardMaxResponseBytes int64 = 16 << 20

type Config struct {
	BaseURL string
	RootCAs *x509.CertPool
	Timeout time.Duration
}

type Client struct {
	base     *url.URL
	basePath string
	client   *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 || cfg.Timeout > 5*time.Minute {
		return nil, ErrInvalidProviderConfig
	}
	base, basePath, err := parseBase(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	var roots *x509.CertPool
	if cfg.RootCAs != nil {
		roots = cfg.RootCAs.Clone()
	}
	transport := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
		},
		ForceAttemptHTTP2: true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{base: base, basePath: basePath, client: client}, nil
}

func (c *Client) Do(ctx context.Context, method string, segments []string, headers http.Header, body []byte, maxResponseBytes int64) (int, []byte, error) {
	if c == nil || c.client == nil || c.base == nil || ctx == nil {
		return 0, nil, ErrInvalidProviderConfig
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut:
	default:
		return 0, nil, ErrInvalidProviderRoute
	}
	if maxResponseBytes <= 0 || maxResponseBytes > hardMaxResponseBytes {
		return 0, nil, ErrInvalidProviderConfig
	}
	u, err := buildURL(c.base, c.basePath, segments)
	if err != nil {
		return 0, nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(append([]byte(nil), body...))
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return 0, nil, ErrInvalidProviderRoute
	}
	if headers != nil {
		req.Header = headers.Clone()
		if req.Header.Get("Host") != "" || req.Header.Get("Proxy-Authorization") != "" {
			return 0, nil, ErrInvalidProviderRoute
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, ErrProviderTransport
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return 0, nil, ErrProviderTransport
	}
	if int64(len(raw)) > maxResponseBytes {
		for i := range raw {
			raw[i] = 0
		}
		return resp.StatusCode, nil, ErrProviderResponseTooLarge
	}
	return resp.StatusCode, raw, nil
}
