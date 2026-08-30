package providerhttp

import (
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

func parseBase(raw string) (*url.URL, string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return nil, "", ErrInvalidProviderConfig
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
		return nil, "", ErrInvalidProviderConfig
	}
	if u.Path != "" {
		if strings.HasSuffix(u.Path, "/") || strings.Contains(u.Path, "//") || path.Clean(u.Path) != u.Path {
			return nil, "", ErrInvalidProviderConfig
		}
		for _, segment := range strings.Split(strings.TrimPrefix(u.Path, "/"), "/") {
			if !safeSegment(segment) {
				return nil, "", ErrInvalidProviderConfig
			}
		}
	}
	basePath := u.Path
	u.RawQuery = ""
	u.Fragment = ""
	return u, basePath, nil
}

func safeSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." || len(segment) > 1024 || !utf8.ValidString(segment) || strings.ContainsAny(segment, "/\\") {
		return false
	}
	for _, r := range segment {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func buildURL(base *url.URL, basePath string, segments []string) (*url.URL, error) {
	if base == nil || len(segments) == 0 || len(segments) > 128 {
		return nil, ErrInvalidProviderRoute
	}
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if !safeSegment(segment) {
			return nil, ErrInvalidProviderRoute
		}
		parts = append(parts, segment)
	}
	u := *base
	u.Path = basePath + "/" + strings.Join(parts, "/")
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return &u, nil
}
