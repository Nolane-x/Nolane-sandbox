package github

import (
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

const Kind delegation.AdapterKind = "github.v1"

const (
	OpContentsWrite delegation.Operation = "github.repo.contents.write"
	OpIssueComment  delegation.Operation = "github.issue.comment.create"
	OpPullComment   delegation.Operation = "github.pull_request.comment.create"
)

const (
	maxContentBytes = 1 << 20
	maxCommentBytes = 65536
	markerPrefix    = "nolane-provider-v7:"
)

var (
	ErrInvalidProviderConfig   = errors.New("github provider: invalid configuration")
	ErrInvalidProviderResource = errors.New("github provider: invalid resource")
	ErrInvalidProviderPayload  = errors.New("github provider: invalid payload")
	ErrProviderTransport       = errors.New("github provider: transport failure")
	ErrProviderRejected        = errors.New("github provider: provider rejected request")
	ErrProviderResponse        = errors.New("github provider: invalid response")
)

type ContentsResource struct {
	Owner  string
	Repo   string
	Path   string
	Branch string
}

type IssueResource struct {
	Owner  string
	Repo   string
	Number int64
}

type PullResource struct {
	Owner  string
	Repo   string
	Number int64
}

type ContentsWritePayload struct {
	Content         []byte
	CommitMessage   string
	ExpectedBlobSHA string
}

type CommentPayload struct {
	Body string
}
