package cube

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	cubewire "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate/cube"
)

func TestDriverFingerprintHashesEndpointAndTemplate(t *testing.T) {
	d, err := New(cubewire.Config{APIURL: "http://127.0.0.1:9999", TemplateID: "tpl"})
	if err != nil {
		t.Fatal(err)
	}
	fp := d.Fingerprint()
	if len(fp.EndpointDigest) != 64 || len(fp.TemplateDigest) != 64 {
		t.Fatalf("fp=%+v", fp)
	}
	if strings.Contains(fp.EndpointDigest, "127.0.0.1") || strings.Contains(fp.TemplateDigest, "tpl") {
		t.Fatalf("raw config leaked: %+v", fp)
	}
}

func TestProbeCommandRejectsMalformedTargetsAndNeverEmbedsExpectSecret(t *testing.T) {
	if _, err := probeCommand(live.Target{Kind: live.TargetTCP, Address: "not-host-port", Expect: "TOP-SECRET"}); err == nil {
		t.Fatal("malformed target accepted")
	}
	for _, target := range []live.Target{
		{Kind: live.TargetHTTP, Address: "https://example.test/path", Expect: "TOP-SECRET"},
		{Kind: live.TargetUDP, Address: "127.0.0.1:9000", Expect: "TOP-SECRET"},
	} {
		cmd, err := probeCommand(target)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(cmd, "TOP-SECRET") {
			t.Fatalf("expect secret embedded in guest command: %s", cmd)
		}
	}
}

func TestPreflightRejectsUnknownTargetKind(t *testing.T) {
	d, err := New(cubewire.Config{APIURL: "http://127.0.0.1:9999", TemplateID: "tpl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Preflight(context.Background(), live.Target{Kind: "other", Address: "x"}); err == nil {
		t.Fatal("unknown target accepted")
	}
}

func TestTCPProbeRejectsShellMetacharacterHost(t *testing.T) {
	if _, err := probeCommand(live.Target{Kind: live.TargetTCP, Address: "$(id):80"}); err == nil {
		t.Fatal("shell metacharacter target accepted")
	}
}

func TestUDPProbeCommandExecutesInsteadOfSyntaxFailing(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	cmd, err := probeCommand(live.Target{Kind: live.TargetUDP, Address: "127.0.0.1:9", Expect: "x"})
	if err != nil {
		t.Fatal(err)
	}
	err = exec.Command("bash", "-c", cmd).Run()
	if err == nil {
		return
	}
	exit, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatal(err)
	}
	if exit.ExitCode() != 42 {
		t.Fatalf("exit=%d want 42 (network denial/timeout), cmd=%s", exit.ExitCode(), cmd)
	}
}
