package main

import (
	"fmt"
	"io"
	"os"

	"github.com/sito1912/review-pattern-corpus/internal/collect"
)

func main() {
	if err := run(os.Args[1:], os.Environ(), os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "review-patterns: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, env []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return fmt.Errorf("command is required")
	}

	switch args[0] {
	case "collect":
		return collect.Run(args[1:], collect.EnvMap(env), stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  review-patterns collect [flags]

Commands:
  collect    Collect human review comments from merged pull requests`)
}
