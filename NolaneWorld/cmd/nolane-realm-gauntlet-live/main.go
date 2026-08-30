package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	livecube "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/cube"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	cubewire "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate/cube"
)

func main() { os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) }

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("nolane-realm-gauntlet-live", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modeRaw := fs.String("mode", string(live.ModeProbe), "probe or require-live")
	profileRaw := fs.String("profile", string(realm.R0InternalOnly), "R0_INTERNAL_ONLY, R1_PUBLIC_READ, or R2_SUPPLY_CHAIN")
	targetKindRaw := fs.String("raw-public-kind", string(live.TargetHTTP), "http, tcp, udp, or dns")
	targetAddress := fs.String("raw-public-target", "", "raw public target used for the negative egress probe")
	targetExpect := fs.String("raw-public-expect", "", "optional host-side preflight expectation")
	outPath := fs.String("out", "", "write verified canonical JSON to path")
	apiURL := fs.String("api-url", getenv("NOLANE_CUBE_API_URL"), "CubeAPI URL")
	templateID := fs.String("template-id", getenv("NOLANE_CUBE_TEMPLATE_ID"), "Cube template ID")
	sandboxDomain := fs.String("sandbox-domain", getenv("NOLANE_CUBE_SANDBOX_DOMAIN"), "sandbox data-plane domain")
	proxyScheme := fs.String("proxy-scheme", getenv("NOLANE_CUBE_PROXY_SCHEME"), "sandbox data-plane scheme")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}

	mode := live.Mode(*modeRaw)
	if mode != live.ModeProbe && mode != live.ModeRequireLive {
		fmt.Fprintln(stderr, "invalid --mode")
		return 2
	}
	profile := realm.NetworkProfile(*profileRaw)
	if profile != realm.R0InternalOnly && profile != realm.R1PublicRead && profile != realm.R2SupplyChain {
		fmt.Fprintln(stderr, "invalid --profile")
		return 2
	}
	targetKind := live.TargetKind(*targetKindRaw)
	if targetKind != live.TargetHTTP && targetKind != live.TargetTCP && targetKind != live.TargetUDP && targetKind != live.TargetDNS {
		fmt.Fprintln(stderr, "invalid --raw-public-kind")
		return 2
	}

	apiKey := getenv("NOLANE_CUBE_API_KEY")
	var driver live.Driver
	if *apiURL != "" && *templateID != "" {
		cubeDriver, err := livecube.New(cubewire.Config{
			APIURL:        *apiURL,
			APIKey:        apiKey,
			TemplateID:    *templateID,
			SandboxDomain: *sandboxDomain,
			ProxyScheme:   *proxyScheme,
		})
		if err == nil {
			driver = cubeDriver
		} else {
			fmt.Fprintln(stderr, "live Cube configuration unavailable")
		}
	}

	runner := realmproof.Runner{
		Mode:    mode,
		Profile: profile,
		RawPublicTarget: live.Target{
			Kind:    targetKind,
			Address: *targetAddress,
			Expect:  *targetExpect,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	report, runErr := runner.Run(ctx, driver)
	body, marshalErr := realmproof.MarshalReport(report, credentialEncodings(apiKey)...)
	if marshalErr != nil {
		fmt.Fprintln(stderr, "realm live evidence verification failed")
		return 1
	}
	if *outPath != "" {
		if err := writeAtomic(*outPath, body); err != nil {
			fmt.Fprintln(stderr, "write realm live evidence failed")
			return 1
		}
	} else {
		_, _ = stdout.Write(body)
	}

	switch {
	case errors.Is(runErr, realmproof.ErrUnavailable):
		return 2
	case errors.Is(runErr, realmproof.ErrFailed):
		return 1
	case runErr != nil:
		return 1
	default:
		return 0
	}
}

func credentialEncodings(secret string) []string {
	if secret == "" {
		return nil
	}
	return []string{
		secret,
		base64.StdEncoding.EncodeToString([]byte(secret)),
		hex.EncodeToString([]byte(secret)),
	}
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".nolane-realm-live-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
