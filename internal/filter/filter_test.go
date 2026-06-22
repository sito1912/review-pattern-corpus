package filter

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	opts, err := ParseOptions([]string{
		"--input", "reviews.jsonl",
		"--output", "filtered.jsonl",
		"--path", "/app/controllers",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Input != "reviews.jsonl" {
		t.Fatalf("Input = %q, want reviews.jsonl", opts.Input)
	}
	if opts.Output != "filtered.jsonl" {
		t.Fatalf("Output = %q, want filtered.jsonl", opts.Output)
	}
	if opts.Path != "/app/controllers" {
		t.Fatalf("Path = %q, want /app/controllers", opts.Path)
	}
}

func TestParseOptionsRequiresFlags(t *testing.T) {
	_, err := ParseOptions([]string{"--input", "reviews.jsonl", "--output", "filtered.jsonl"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--path is required") {
		t.Fatalf("error = %q, want --path error", err)
	}
}

func TestParseOptionsHelp(t *testing.T) {
	_, err := ParseOptions([]string{"--help"}, &bytes.Buffer{})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
}

func TestFilterJSONLMatchesPathSegments(t *testing.T) {
	input := strings.Join([]string{
		`{"schema_version":"1.0","path":"app/controllers/users_controller.go","body":"root app controller"}`,
		`{"schema_version":"1.0","path":"./app/controllers/admin_controller.go","body":"dot slash path"}`,
		`{"schema_version":"1.0","path":"packages/backend/app/controllers/session.go","body":"nested app controller"}`,
		`{"schema_version":"1.0","path":"app/models/user.go","body":"different app area"}`,
		`{"schema_version":"1.0","path":"application/controllers/user.go","body":"similar prefix only"}`,
		`{"schema_version":"1.0","comment_type":"review_summary","body":"no path"}`,
		`{"schema_version":"1.0","path":null,"body":"null path"}`,
	}, "\n")

	var output bytes.Buffer
	result, err := FilterJSONL(strings.NewReader(input), &output, "/app/controllers")
	if err != nil {
		t.Fatal(err)
	}
	if result.Read != 7 {
		t.Fatalf("Read = %d, want 7", result.Read)
	}
	if result.Written != 3 {
		t.Fatalf("Written = %d, want 3", result.Written)
	}

	got := strings.TrimSpace(output.String())
	for _, want := range []string{
		"root app controller",
		"dot slash path",
		"nested app controller",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output does not contain %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"different app area",
		"similar prefix only",
		"no path",
		"null path",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("output contains %q:\n%s", unwanted, got)
		}
	}
}

func TestFilterJSONLRejectsInvalidJSONWithLineNumber(t *testing.T) {
	_, err := FilterJSONL(strings.NewReader(`{"path":"app/a.go"}`+"\nnot-json\n"), &bytes.Buffer{}, "/app")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error = %q, want line number", err)
	}
}

func TestFilterJSONLRejectsNonStringPath(t *testing.T) {
	_, err := FilterJSONL(strings.NewReader(`{"path":123}`+"\n"), &bytes.Buffer{}, "/app")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "path must be a string or null") {
		t.Fatalf("error = %q, want path type error", err)
	}
}

func TestRunWritesFilteredJSONLFile(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "reviews.jsonl")
	output := filepath.Join(dir, "nested", "filtered.jsonl")
	writeFile(t, input, strings.Join([]string{
		`{"path":"cmd/review-patterns/main.go","body":"include"}`,
		`{"path":"internal/prompt/prompt.go","body":"exclude"}`,
	}, "\n")+"\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Run([]string{
		"--input", input,
		"--output", output,
		"--path", "/cmd",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	got := readFile(t, output)
	if strings.Contains(got, "exclude") {
		t.Fatalf("output contains excluded record:\n%s", got)
	}
	if !strings.Contains(got, "include") {
		t.Fatalf("output does not contain included record:\n%s", got)
	}
	if !strings.Contains(stderr.String(), "wrote 1 of 2 record(s)") {
		t.Fatalf("stderr = %q, want progress summary", stderr.String())
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
