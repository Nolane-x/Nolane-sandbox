package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	freedomgauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/freedom"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("nolane-freedom-gauntlet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("out", "", "write verified Freedom Plane v8 evidence JSON to this path")
	if err := fs.Parse(args); err != nil { return 2 }
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "nolane-freedom-gauntlet: unexpected positional argument")
		return 2
	}
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 5 * time.Second}
	report, err := freedomgauntlet.RunStandard(context.Background(), policy)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nolane-freedom-gauntlet: run failed")
		return 1
	}
	raw, err := gauntlet.MarshalReport(report)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nolane-freedom-gauntlet: evidence verification failed")
		return 1
	}
	forms := [][]byte{
		[]byte(freedomgauntlet.SyntheticSecret),
		[]byte(base64.StdEncoding.EncodeToString([]byte(freedomgauntlet.SyntheticSecret))),
		[]byte(hex.EncodeToString([]byte(freedomgauntlet.SyntheticSecret))),
	}
	for _, form := range forms {
		if bytes.Contains(bytes.ToLower(raw), bytes.ToLower(form)) {
			_, _ = fmt.Fprintln(stderr, "nolane-freedom-gauntlet: secret material in report")
			return 1
		}
	}
	if bytes.Contains(bytes.ToLower(raw), []byte("authorization: bearer")) {
		_, _ = fmt.Fprintln(stderr, "nolane-freedom-gauntlet: credential-bearing header in report")
		return 1
	}
	if *outPath == "" {
		if _, err := stdout.Write(append(raw, '\n')); err != nil { return 1 }
	} else {
		dir := filepath.Dir(*outPath)
		if dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil { return 1 }
		}
		if err := os.WriteFile(*outPath, raw, 0o644); err != nil { return 1 }
	}
	if !report.Approved {
		_, _ = fmt.Fprintln(stderr, "nolane-freedom-gauntlet: release rejected")
		return 1
	}
	return 0
}
