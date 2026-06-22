package filter

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// Options describes a filter command invocation.
type Options struct {
	Input  string
	Output string
	Path   string

	Progress io.Writer
}

// Result summarizes a JSONL filtering run.
type Result struct {
	Read    int
	Written int
}

// Run parses filter flags and writes matching JSONL records.
func Run(args []string, stdout, stderr io.Writer) error {
	opts, err := ParseOptions(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	opts.Progress = stderr

	input, err := os.Open(opts.Input)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	defer input.Close()

	var output io.Writer = stdout
	var outputFile *os.File
	if opts.Output != "-" {
		outputDir := filepath.Dir(opts.Output)
		if outputDir != "." {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}
		}

		outputFile, err = os.OpenFile(opts.Output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open output file: %w", err)
		}
		defer outputFile.Close()
		output = outputFile
	}

	progressf(opts.Progress, "review-patterns: filtering review corpus from %s", opts.Input)
	result, err := FilterJSONL(input, output, opts.Path)
	if err != nil {
		return err
	}
	if opts.Output == "-" {
		progressf(opts.Progress, "review-patterns: wrote %d of %d record(s) to stdout", result.Written, result.Read)
	} else {
		progressf(opts.Progress, "review-patterns: wrote %d of %d record(s) to %s", result.Written, result.Read, opts.Output)
	}
	progressf(opts.Progress, "review-patterns: done")
	return nil
}

// ParseOptions parses filter flags.
func ParseOptions(args []string, stderr io.Writer) (Options, error) {
	var opts Options
	flags := flag.NewFlagSet("filter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.Input, "input", "", "JSONL review corpus input path")
	flags.StringVar(&opts.Output, "output", "", "JSONL output path, or - for stdout")
	flags.StringVar(&opts.Path, "path", "", "path segment to match against each JSON object's path value")

	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if flags.NArg() > 0 {
		return Options{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if err := validateOptions(opts); err != nil {
		return Options{}, err
	}
	return opts, nil
}

// FilterJSONL copies JSONL objects whose path field matches searchPath.
func FilterJSONL(input io.Reader, output io.Writer, searchPath string) (Result, error) {
	reader := bufio.NewReader(input)
	var result Result
	lineNumber := 0
	for {
		rawLine, err := reader.ReadString('\n')
		if rawLine != "" {
			lineNumber++
			if strings.TrimSpace(rawLine) != "" {
				result.Read++
				matched, err := matchesJSONLPath(rawLine, lineNumber, searchPath)
				if err != nil {
					return Result{}, err
				}
				if matched {
					line := strings.TrimRight(rawLine, "\r\n")
					if _, err := io.WriteString(output, line+"\n"); err != nil {
						return Result{}, fmt.Errorf("write output JSONL: %w", err)
					}
					result.Written++
				}
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return Result{}, fmt.Errorf("read input JSONL: %w", err)
	}
	return result, nil
}

func validateOptions(opts Options) error {
	if strings.TrimSpace(opts.Input) == "" {
		return errors.New("--input is required")
	}
	if strings.TrimSpace(opts.Output) == "" {
		return errors.New("--output is required")
	}
	if strings.TrimSpace(opts.Path) == "" {
		return errors.New("--path is required")
	}
	return nil
}

func matchesJSONLPath(rawLine string, lineNumber int, searchPath string) (bool, error) {
	line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
	if line == "" {
		return false, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &object); err != nil {
		return false, fmt.Errorf("parse input JSONL line %d: %w", lineNumber, err)
	}
	if object == nil {
		return false, fmt.Errorf("parse input JSONL line %d: expected JSON object", lineNumber)
	}

	rawPath, ok := object["path"]
	if !ok || len(rawPath) == 0 || string(rawPath) == "null" {
		return false, nil
	}
	var recordPath string
	if err := json.Unmarshal(rawPath, &recordPath); err != nil {
		return false, fmt.Errorf("parse input JSONL line %d: path must be a string or null", lineNumber)
	}
	if strings.TrimSpace(recordPath) == "" {
		return false, nil
	}
	return matchPath(recordPath, searchPath), nil
}

func matchPath(recordPath, searchPath string) bool {
	record := normalizePath(recordPath)
	search := normalizePath(searchPath)
	if search == "/" {
		return record != "/"
	}
	return record == search ||
		strings.HasPrefix(record, search+"/") ||
		strings.Contains(record, search+"/") ||
		strings.HasSuffix(record, search)
}

func normalizePath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	if value == "" {
		return "/"
	}
	return pathpkg.Clean("/" + value)
}

func progressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}
