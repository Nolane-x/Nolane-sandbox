package github

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/providerhttp"
)

const providerResponseLimit int64 = 64 * 1024

type Config struct {
	BaseURL string
	RootCAs *x509.CertPool
	Timeout time.Duration
}

type Adapter struct {
	http *providerhttp.Client
}

func New(cfg Config) (*Adapter, error) {
	client, err := providerhttp.New(providerhttp.Config{BaseURL: cfg.BaseURL, RootCAs: cfg.RootCAs, Timeout: cfg.Timeout})
	if err != nil {
		return nil, ErrInvalidProviderConfig
	}
	return &Adapter{http: client}, nil
}

func (a *Adapter) Kind() delegation.AdapterKind { return Kind }

func (a *Adapter) Execute(ctx context.Context, request delegation.AdapterRequest, secret delegation.Secret) (delegation.Effect, error) {
	if a == nil || a.http == nil || ctx == nil || request.IdempotencyKey == "" || len(secret.Bytes()) == 0 {
		return delegation.Effect{}, ErrInvalidProviderPayload
	}
	marker := actionMarker(request.IdempotencyKey)
	switch request.Operation {
	case OpIssueComment:
		resource, err := parseIssueResource(request.Resource)
		if err != nil {
			return delegation.Effect{}, err
		}
		payload, err := decodeCommentPayload(request.Payload)
		if err != nil {
			return delegation.Effect{}, err
		}
		return a.executeComment(ctx, request, secret, resource.Owner, resource.Repo, resource.Number, payload, marker)
	case OpPullComment:
		resource, err := parsePullResource(request.Resource)
		if err != nil {
			return delegation.Effect{}, err
		}
		payload, err := decodeCommentPayload(request.Payload)
		if err != nil {
			return delegation.Effect{}, err
		}
		return a.executeComment(ctx, request, secret, resource.Owner, resource.Repo, resource.Number, payload, marker)
	case OpContentsWrite:
		resource, err := parseContentsResource(request.Resource)
		if err != nil {
			return delegation.Effect{}, err
		}
		payload, err := decodeContentsPayload(request.Payload)
		if err != nil {
			return delegation.Effect{}, err
		}
		defer zeroBytes(payload.Content)
		return a.executeContents(ctx, request, secret, resource, payload, marker)
	default:
		return delegation.Effect{}, ErrInvalidProviderPayload
	}
}

func (a *Adapter) executeComment(ctx context.Context, request delegation.AdapterRequest, secret delegation.Secret, owner, repo string, number int64, payload CommentPayload, marker string) (delegation.Effect, error) {
	wire := struct {
		Body string `json:"body"`
	}{Body: payload.Body + "\n\n<!-- " + marker + " -->"}
	body, err := json.Marshal(wire)
	if err != nil {
		return delegation.Effect{}, ErrInvalidProviderPayload
	}
	headers := providerHeaders(secret)
	defer clearHeaders(headers)
	segments := []string{"repos", owner, repo, "issues", strconv.FormatInt(number, 10), "comments"}
	status, response, err := a.http.Do(ctx, http.MethodPost, segments, headers, body, providerResponseLimit)
	if err != nil {
		return delegation.Effect{}, mapTransportError(err)
	}
	defer zeroBytes(response)
	if status != http.StatusCreated {
		return delegation.Effect{}, ErrProviderRejected
	}
	var parsed struct {
		ID int64 `json:"id"`
	}
	if err := decodeProviderJSON(response, &parsed); err != nil || parsed.ID <= 0 {
		return delegation.Effect{}, ErrProviderResponse
	}
	evidence, err := sanitizedEvidence(request.Operation, request.Resource, strconv.FormatInt(parsed.ID, 10), marker, status)
	if err != nil {
		return delegation.Effect{}, err
	}
	return delegation.Effect{Evidence: evidence}, nil
}

func (a *Adapter) executeContents(ctx context.Context, request delegation.AdapterRequest, secret delegation.Secret, resource ContentsResource, payload ContentsWritePayload, marker string) (delegation.Effect, error) {
	wire := struct {
		Message string `json:"message"`
		Content string `json:"content"`
		Branch  string `json:"branch"`
		SHA     string `json:"sha,omitempty"`
	}{
		Message: payload.CommitMessage + "\n\n" + marker,
		Content: base64.StdEncoding.EncodeToString(payload.Content),
		Branch:  resource.Branch,
		SHA:     payload.ExpectedBlobSHA,
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return delegation.Effect{}, ErrInvalidProviderPayload
	}
	headers := providerHeaders(secret)
	defer clearHeaders(headers)
	segments := []string{"repos", resource.Owner, resource.Repo, "contents"}
	segments = append(segments, strings.Split(resource.Path, "/")...)
	status, response, err := a.http.Do(ctx, http.MethodPut, segments, headers, body, providerResponseLimit)
	if err != nil {
		return delegation.Effect{}, mapTransportError(err)
	}
	defer zeroBytes(response)
	if status != http.StatusOK && status != http.StatusCreated {
		return delegation.Effect{}, ErrProviderRejected
	}
	var parsed struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := decodeProviderJSON(response, &parsed); err != nil || !validLowerHexSHA(parsed.Commit.SHA) {
		return delegation.Effect{}, ErrProviderResponse
	}
	evidence, err := sanitizedEvidence(request.Operation, request.Resource, parsed.Commit.SHA, marker, status)
	if err != nil {
		return delegation.Effect{}, err
	}
	return delegation.Effect{Evidence: evidence}, nil
}

func providerHeaders(secret delegation.Secret) http.Header {
	return http.Header{
		"Accept":               []string{"application/vnd.github+json"},
		"Authorization":        []string{"Bearer " + string(secret.Bytes())},
		"Content-Type":         []string{"application/json"},
		"X-GitHub-Api-Version": []string{"2022-11-28"},
	}
}

func clearHeaders(headers http.Header) {
	for key := range headers {
		headers.Del(key)
	}
}

func mapTransportError(err error) error {
	switch err {
	case providerhttp.ErrProviderResponseTooLarge:
		return ErrProviderResponse
	default:
		return ErrProviderTransport
	}
}

func decodeProviderJSON(raw []byte, out any) error {
	if len(raw) == 0 || int64(len(raw)) > providerResponseLimit {
		return ErrProviderResponse
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(out); err != nil {
		return ErrProviderResponse
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return ErrProviderResponse
	}
	return nil
}
