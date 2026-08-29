package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	livecube "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/cube"
	cubewire "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate/cube"
)

func main() { os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) }

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("nolane-gauntlet-live", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modeRaw := fs.String("mode", string(live.ModeProbe), "probe or require-live")
	profileRaw := fs.String("profile", string(live.ProfileCore), "core or full-egress")
	outPath := fs.String("out", "", "write verified JSON to path")
	apiURL := fs.String("api-url", getenv("NOLANE_CUBE_API_URL"), "CubeAPI URL")
	templateID := fs.String("template-id", getenv("NOLANE_CUBE_TEMPLATE_ID"), "Cube template ID")
	sandboxDomain := fs.String("sandbox-domain", getenv("NOLANE_CUBE_SANDBOX_DOMAIN"), "sandbox data-plane domain")
	proxyScheme := fs.String("proxy-scheme", getenv("NOLANE_CUBE_PROXY_SCHEME"), "sandbox data-plane scheme")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	mode := live.Mode(*modeRaw)
	profile := live.Profile(*profileRaw)
	if mode != live.ModeProbe && mode != live.ModeRequireLive {
		fmt.Fprintln(stderr, "invalid --mode")
		return 2
	}
	if profile != live.ProfileCore && profile != live.ProfileFullEgress {
		fmt.Fprintln(stderr, "invalid --profile")
		return 2
	}

	var driver live.Driver
	apiKey := getenv("NOLANE_CUBE_API_KEY")
	if *apiURL != "" && *templateID != "" {
		d, err := livecube.New(cubewire.Config{APIURL: *apiURL, APIKey: apiKey, TemplateID: *templateID, SandboxDomain: *sandboxDomain, ProxyScheme: *proxyScheme})
		if err == nil {
			driver = d
		} else {
			fmt.Fprintln(stderr, "live Cube configuration unavailable")
		}
	}
	runner := live.Runner{Mode: mode, Profile: profile, Targets: targetsFromEnv(getenv)}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	report, runErr := runner.Run(ctx, driver)
	body, marshalErr := live.MarshalReport(report, apiKey)
	if marshalErr != nil {
		fmt.Fprintln(stderr, "live evidence verification failed")
		return 1
	}
	if *outPath != "" {
		if err := writeAtomic(*outPath, body); err != nil {
			fmt.Fprintln(stderr, "write live evidence failed")
			return 1
		}
	} else {
		_, _ = stdout.Write(body)
	}
	if errors.Is(runErr, live.ErrLiveUnavailable) {
		return 2
	}
	if errors.Is(runErr, live.ErrLiveFailed) {
		return 1
	}
	if runErr != nil {
		return 1
	}
	return 0
}

func targetsFromEnv(getenv func(string) string) []live.Target {
	defs := []struct {
		k            live.TargetKind
		addr, expect string
	}{
		{live.TargetHTTP, "NOLANE_GAUNTLET_HTTP_TARGET", "NOLANE_GAUNTLET_HTTP_EXPECT"},
		{live.TargetTCP, "NOLANE_GAUNTLET_TCP_TARGET", "NOLANE_GAUNTLET_TCP_EXPECT"},
		{live.TargetUDP, "NOLANE_GAUNTLET_UDP_TARGET", "NOLANE_GAUNTLET_UDP_EXPECT"},
		{live.TargetDNS, "NOLANE_GAUNTLET_DNS_TARGET", "NOLANE_GAUNTLET_DNS_EXPECT"},
	}
	out := make([]live.Target, 0, 4)
	for _, d := range defs {
		if a := getenv(d.addr); a != "" {
			out = append(out, live.Target{Kind: d.k, Address: a, Expect: getenv(d.expect)})
		}
	}
	return out
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".nolane-live-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
