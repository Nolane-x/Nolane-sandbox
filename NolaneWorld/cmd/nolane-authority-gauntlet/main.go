package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	delegationgauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/delegation"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("nolane-authority-gauntlet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("out", "", "write verified delegated-authority evidence JSON to this path")
	if err := fs.Parse(args); err != nil { return 2 }
	if fs.NArg() != 0 { _, _ = fmt.Fprintln(stderr, "nolane-authority-gauntlet: unexpected positional argument"); return 2 }
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 3 * time.Second}
	report, err := delegationgauntlet.RunStandard(context.Background(), policy)
	if err != nil { _, _ = fmt.Fprintln(stderr, "nolane-authority-gauntlet: run failed"); return 1 }
	raw, err := gauntlet.MarshalReport(report)
	if err != nil { _, _ = fmt.Fprintln(stderr, "nolane-authority-gauntlet: evidence verification failed"); return 1 }
	if bytes.Contains(raw, []byte(delegationgauntlet.SyntheticSecret)) { _, _ = fmt.Fprintln(stderr, "nolane-authority-gauntlet: secret material in report"); return 1 }
	if *outPath == "" {
		if _, err := stdout.Write(append(raw, '\n')); err != nil { return 1 }
	} else {
		dir := filepath.Dir(*outPath); if dir != "." { if err := os.MkdirAll(dir, 0o755); err != nil { return 1 } }
		if err := os.WriteFile(*outPath, raw, 0o644); err != nil { return 1 }
	}
	if !report.Approved { _, _ = fmt.Fprintln(stderr, "nolane-authority-gauntlet: release rejected"); return 1 }
	return 0
}
