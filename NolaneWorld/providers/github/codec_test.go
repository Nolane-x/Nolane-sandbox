package github

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

func TestProviderIdentityAndOperationsAreFixed(t *testing.T) {
	if Kind != delegation.AdapterKind("github.v1") {
		t.Fatalf("kind=%q", Kind)
	}
	want := []delegation.Operation{
		"github.repo.contents.write",
		"github.issue.comment.create",
		"github.pull_request.comment.create",
	}
	got := []delegation.Operation{OpContentsWrite, OpIssueComment, OpPullComment}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("operation %d=%q", i, got[i])
		}
	}
}

func TestContentsResourceCanonicalRoundTrip(t *testing.T) {
	raw := "github:repo:Nolane-x/Nolane-sandbox:contents:docs/spec-v7.md@release/v7"
	res, err := parseContentsResource(raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Owner != "Nolane-x" || res.Repo != "Nolane-sandbox" || res.Path != "docs/spec-v7.md" || res.Branch != "release/v7" {
		t.Fatalf("resource=%+v", res)
	}
	if res.String() != raw {
		t.Fatalf("roundtrip=%q", res.String())
	}
}

func TestCommentResourcesCanonicalRoundTrip(t *testing.T) {
	issueRaw := "github:repo:Nolane-x/Nolane-sandbox:issue:42"
	issue, err := parseIssueResource(issueRaw)
	if err != nil || issue.String() != issueRaw || issue.Number != 42 {
		t.Fatalf("issue=%+v err=%v", issue, err)
	}
	pullRaw := "github:repo:Nolane-x/Nolane-sandbox:pull:7"
	pull, err := parsePullResource(pullRaw)
	if err != nil || pull.String() != pullRaw || pull.Number != 7 {
		t.Fatalf("pull=%+v err=%v", pull, err)
	}
}

func TestResourcesRejectDelimiterTraversalAndNoncanonicalForms(t *testing.T) {
	badContents := []string{
		"github:repo:Nolane-x/Nolane-sandbox:contents:../secret@main",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a//b@main",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a\\b@main",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a%2Fb@main",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a@b@main",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a:b@main",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a@/main",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a@main/",
		"github:repo:Nolane-x/Nolane-sandbox:contents:a@main//x",
		"github:repo:-bad/Nolane-sandbox:contents:a@main",
		"github:repo:Nolane-x//bad:contents:a@main",
	}
	for _, raw := range badContents {
		if res, err := parseContentsResource(raw); !errors.Is(err, ErrInvalidProviderResource) {
			t.Fatalf("accepted %q as %+v err=%v", raw, res, err)
		}
	}
	for _, raw := range []string{
		"github:repo:Nolane-x/Nolane-sandbox:issue:0",
		"github:repo:Nolane-x/Nolane-sandbox:issue:01",
		"github:repo:Nolane-x/Nolane-sandbox:issue:2147483648",
		"github:repo:Nolane-x/Nolane-sandbox:pull:-1",
		"github:repo:Nolane-x/Nolane-sandbox:pull:1%30",
	} {
		if strings.Contains(raw, ":issue:") {
			if _, err := parseIssueResource(raw); !errors.Is(err, ErrInvalidProviderResource) {
				t.Fatalf("issue accepted %q err=%v", raw, err)
			}
		} else if _, err := parsePullResource(raw); !errors.Is(err, ErrInvalidProviderResource) {
			t.Fatalf("pull accepted %q err=%v", raw, err)
		}
	}
}

func TestContentsPayloadStrictCanonicalDecode(t *testing.T) {
	content := []byte("hello v7")
	raw := []byte(`{"content_b64":"` + base64.StdEncoding.EncodeToString(content) + `","commit_message":"update spec","expected_blob_sha":"0123456789abcdef0123456789abcdef01234567"}`)
	payload, err := decodeContentsPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload.Content) != string(content) || payload.CommitMessage != "update spec" || payload.ExpectedBlobSHA == "" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestContentsPayloadRejectsAmbiguousOversizedAndReservedInputs(t *testing.T) {
	tooLarge := base64.StdEncoding.EncodeToString(make([]byte, maxContentBytes+1))
	cases := [][]byte{
		[]byte(`{"content_b64":"aGVsbG8=","commit_message":"ok","extra":1}`),
		[]byte(`{"content_b64":"aGVsbG8=","content_b64":"aGVsbG8=","commit_message":"ok"}`),
		[]byte(`{ "content_b64":"aGVsbG8=","commit_message":"ok"}`),
		[]byte(`{"commit_message":"ok","content_b64":"aGVsbG8="}`),
		[]byte(`{"content_b64":"%%%","commit_message":"ok"}`),
		[]byte(`{"content_b64":"aGVsbG8=","commit_message":""}`),
		[]byte(`{"content_b64":"aGVsbG8=","commit_message":"contains nolane-provider-v7: marker"}`),
		[]byte(`{"content_b64":"aGVsbG8=","commit_message":"ok","expected_blob_sha":"ABCDEF0123456789abcdef0123456789abcdef01"}`),
		[]byte(`{"content_b64":"aGVsbG8=","commit_message":"ok","expected_blob_sha":"abc"}`),
		[]byte(`{"content_b64":"` + tooLarge + `","commit_message":"ok"}`),
	}
	for i, raw := range cases {
		if payload, err := decodeContentsPayload(raw); !errors.Is(err, ErrInvalidProviderPayload) {
			t.Fatalf("case %d payload=%+v err=%v", i, payload, err)
		}
	}
}

func TestCommentPayloadStrictCanonicalDecode(t *testing.T) {
	payload, err := decodeCommentPayload([]byte(`{"body":"hello from Nolane"}`))
	if err != nil || payload.Body != "hello from Nolane" {
		t.Fatalf("payload=%+v err=%v", payload, err)
	}
	cases := [][]byte{
		[]byte(`{"body":""}`),
		[]byte(`{"body":"x","extra":1}`),
		[]byte(`{"body":"x","body":"y"}`),
		[]byte(`{ "body":"x"}`),
		[]byte(`{"body":"nolane-provider-v7: forged"}`),
		[]byte(`{"body":"a\u0000b"}`),
		[]byte(`{"body":"` + strings.Repeat("x", maxCommentBytes+1) + `"}`),
	}
	for i, raw := range cases {
		if got, err := decodeCommentPayload(raw); !errors.Is(err, ErrInvalidProviderPayload) {
			t.Fatalf("case %d payload=%+v err=%v", i, got, err)
		}
	}
}

func TestActionMarkerIsDeterministicAndSecretIndependent(t *testing.T) {
	first := actionMarker("request-digest-1")
	second := actionMarker("request-digest-1")
	other := actionMarker("request-digest-2")
	if first != second || first == other || !strings.HasPrefix(first, markerPrefix) {
		t.Fatalf("first=%q second=%q other=%q", first, second, other)
	}
	if strings.Contains(first, "SYNTHETIC-V7-SECRET") {
		t.Fatal("secret material entered action marker")
	}
}
