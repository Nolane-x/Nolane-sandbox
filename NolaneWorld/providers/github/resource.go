package github

import (
	"strconv"
	"strings"
)

const resourcePrefix = "github:repo:"

func (r ContentsResource) String() string {
	return resourcePrefix + r.Owner + "/" + r.Repo + ":contents:" + r.Path + "@" + r.Branch
}

func (r IssueResource) String() string {
	return resourcePrefix + r.Owner + "/" + r.Repo + ":issue:" + strconv.FormatInt(r.Number, 10)
}

func (r PullResource) String() string {
	return resourcePrefix + r.Owner + "/" + r.Repo + ":pull:" + strconv.FormatInt(r.Number, 10)
}

func parseContentsResource(raw string) (ContentsResource, error) {
	if len(raw) == 0 || len(raw) > 1400 || strings.ContainsAny(raw, "%\\\x00") || !strings.HasPrefix(raw, resourcePrefix) {
		return ContentsResource{}, ErrInvalidProviderResource
	}
	rest := strings.TrimPrefix(raw, resourcePrefix)
	parts := strings.Split(rest, ":contents:")
	if len(parts) != 2 {
		return ContentsResource{}, ErrInvalidProviderResource
	}
	owner, repo, ok := parseRepository(parts[0])
	if !ok || strings.Count(parts[1], "@") != 1 {
		return ContentsResource{}, ErrInvalidProviderResource
	}
	pathBranch := strings.SplitN(parts[1], "@", 2)
	if !validContentPath(pathBranch[0]) || !validBranch(pathBranch[1]) {
		return ContentsResource{}, ErrInvalidProviderResource
	}
	out := ContentsResource{Owner: owner, Repo: repo, Path: pathBranch[0], Branch: pathBranch[1]}
	if out.String() != raw {
		return ContentsResource{}, ErrInvalidProviderResource
	}
	return out, nil
}

func parseIssueResource(raw string) (IssueResource, error) {
	owner, repo, number, ok := parseNumberResource(raw, ":issue:")
	if !ok {
		return IssueResource{}, ErrInvalidProviderResource
	}
	out := IssueResource{Owner: owner, Repo: repo, Number: number}
	if out.String() != raw {
		return IssueResource{}, ErrInvalidProviderResource
	}
	return out, nil
}

func parsePullResource(raw string) (PullResource, error) {
	owner, repo, number, ok := parseNumberResource(raw, ":pull:")
	if !ok {
		return PullResource{}, ErrInvalidProviderResource
	}
	out := PullResource{Owner: owner, Repo: repo, Number: number}
	if out.String() != raw {
		return PullResource{}, ErrInvalidProviderResource
	}
	return out, nil
}

func parseNumberResource(raw, marker string) (string, string, int64, bool) {
	if len(raw) == 0 || len(raw) > 512 || strings.ContainsAny(raw, "%@\\\x00") || !strings.HasPrefix(raw, resourcePrefix) {
		return "", "", 0, false
	}
	rest := strings.TrimPrefix(raw, resourcePrefix)
	parts := strings.Split(rest, marker)
	if len(parts) != 2 {
		return "", "", 0, false
	}
	owner, repo, ok := parseRepository(parts[0])
	if !ok || parts[1] == "" || (len(parts[1]) > 1 && parts[1][0] == '0') {
		return "", "", 0, false
	}
	for i := 0; i < len(parts[1]); i++ {
		if parts[1][i] < '0' || parts[1][i] > '9' {
			return "", "", 0, false
		}
	}
	number, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil || number < 1 || number > 2147483647 {
		return "", "", 0, false
	}
	return owner, repo, number, true
}

func parseRepository(raw string) (string, string, bool) {
	if strings.Count(raw, "/") != 1 {
		return "", "", false
	}
	parts := strings.SplitN(raw, "/", 2)
	if !validRepositoryIdentifier(parts[0]) || !validRepositoryIdentifier(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func validRepositoryIdentifier(s string) bool {
	if len(s) < 1 || len(s) > 100 || !asciiAlphaNumeric(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !asciiAlphaNumeric(c) && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func validContentPath(raw string) bool {
	if len(raw) < 1 || len(raw) > 1024 || strings.ContainsAny(raw, "@:%\\\x00") {
		return false
	}
	segments := strings.Split(raw, "/")
	for _, segment := range segments {
		if len(segment) < 1 || len(segment) > 255 || segment == "." || segment == ".." {
			return false
		}
		for i := 0; i < len(segment); i++ {
			c := segment[i]
			if !asciiAlphaNumeric(c) && c != '.' && c != '_' && c != '+' && c != '-' {
				return false
			}
		}
	}
	return true
}

func validBranch(raw string) bool {
	if len(raw) < 1 || len(raw) > 255 || strings.HasPrefix(raw, "/") || strings.HasSuffix(raw, "/") || strings.Contains(raw, "//") || strings.ContainsAny(raw, "@:%\\\x00") {
		return false
	}
	segments := strings.Split(raw, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		for i := 0; i < len(segment); i++ {
			c := segment[i]
			if !asciiAlphaNumeric(c) && c != '.' && c != '_' && c != '-' {
				return false
			}
		}
	}
	return true
}

func asciiAlphaNumeric(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
