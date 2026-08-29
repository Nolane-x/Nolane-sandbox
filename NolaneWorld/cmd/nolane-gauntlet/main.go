package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/scenarios"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("nolane-gauntlet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("out", "", "write verified release evidence JSON to this path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "nolane-gauntlet: unexpected positional argument")
		return 2
	}

	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 2 * time.Second}
	report, err := scenarios.RunStandard(context.Background(), policy)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nolane-gauntlet: run failed: %v\n", err)
		return 1
	}
	raw, err := gauntlet.MarshalReport(report)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nolane-gauntlet: evidence verification failed: %v\n", err)
		return 1
	}

	if *outPath == "" {
		if _, err := stdout.Write(append(raw, '\n')); err != nil {
			_, _ = fmt.Fprintf(stderr, "nolane-gauntlet: write stdout: %v\n", err)
			return 1
		}
	} else {
		dir := filepath.Dir(*outPath)
		if dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				_, _ = fmt.Fprintf(stderr, "nolane-gauntlet: create output directory: %v\n", err)
				return 1
			}
		}
		if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
			_, _ = fmt.Fprintf(stderr, "nolane-gauntlet: write report: %v\n", err)
			return 1
		}
	}
	if !report.Approved {
		_, _ = fmt.Fprintln(stderr, "nolane-gauntlet: release rejected")
		return 1
	}
	return 0
}
