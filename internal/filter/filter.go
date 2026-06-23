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
	Author string

	Progress io.Writer
}

// Criteria describes the JSONL fields used by the filter command.
type Criteria struct {
	Path   string
	Author string
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
	result, err := FilterJSONL(input, output, Criteria{Path: opts.Path, Author: opts.Author})
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
	flags.StringVar(&opts.Author, "author", "", "author login to match against each JSON object's author value")

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

// FilterJSONL copies JSONL objects that match all requested criteria.
func FilterJSONL(input io.Reader, output io.Writer, criteria Criteria) (Result, error) {
	if !hasPathCriterion(criteria) && !hasAuthorCriterion(criteria) {
		return Result{}, errors.New("--path or --author is required")
	}

	reader := bufio.NewReader(input)
	var result Result
	lineNumber := 0
	for {
		rawLine, err := reader.ReadString('\n')
		if rawLine != "" {
			lineNumber++
			if strings.TrimSpace(rawLine) != "" {
				result.Read++
				matched, err := matchesJSONL(rawLine, lineNumber, criteria)
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
	criteria := Criteria{Path: opts.Path, Author: opts.Author}
	if !hasPathCriterion(criteria) && !hasAuthorCriterion(criteria) {
		return errors.New("--path or --author is required")
	}
	return nil
}

func matchesJSONL(rawLine string, lineNumber int, criteria Criteria) (bool, error) {
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

	if hasPathCriterion(criteria) {
		matched, err := matchesJSONLPath(object, lineNumber, criteria.Path)
		if err != nil || !matched {
			return matched, err
		}
	}
	if hasAuthorCriterion(criteria) {
		return matchesJSONLAuthor(object, lineNumber, criteria.Author)
	}
	return true, nil
}

func matchesJSONLPath(object map[string]json.RawMessage, lineNumber int, searchPath string) (bool, error) {
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

func matchesJSONLAuthor(object map[string]json.RawMessage, lineNumber int, searchAuthor string) (bool, error) {
	rawAuthor, ok := object["author"]
	if !ok || len(rawAuthor) == 0 || string(rawAuthor) == "null" {
		return false, nil
	}
	var recordAuthor string
	if err := json.Unmarshal(rawAuthor, &recordAuthor); err != nil {
		return false, fmt.Errorf("parse input JSONL line %d: author must be a string or null", lineNumber)
	}
	return strings.TrimSpace(recordAuthor) == strings.TrimSpace(searchAuthor), nil
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

func hasPathCriterion(criteria Criteria) bool {
	return strings.TrimSpace(criteria.Path) != ""
}

func hasAuthorCriterion(criteria Criteria) bool {
	return strings.TrimSpace(criteria.Author) != ""
}

func progressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}
