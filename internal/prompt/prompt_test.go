package prompt

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOptionsDefaults(t *testing.T) {
	opts, err := ParseOptions([]string{"--input", "reviews.jsonl"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Output != "-" {
		t.Fatalf("Output = %q, want -", opts.Output)
	}
	if opts.PatternsDir != ".review-patterns/patterns" {
		t.Fatalf("PatternsDir = %q, want default", opts.PatternsDir)
	}
	if opts.Mode != "auto" {
		t.Fatalf("Mode = %q, want auto", opts.Mode)
	}
}

func TestParseOptionsRequiresInput(t *testing.T) {
	_, err := ParseOptions(nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--input is required") {
		t.Fatalf("error = %q", err)
	}
}

func TestParseOptionsHelp(t *testing.T) {
	_, err := ParseOptions([]string{"--help"}, &bytes.Buffer{})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
}

func TestRunAutoExtractWithoutPatterns(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "reviews.jsonl")
	output := filepath.Join(dir, "prompt.md")
	writeFile(t, input, strings.Join([]string{
		`{"schema_version":"1.0","repo":"owner/repo","comment_type":"review_comment","path":"internal/app.go","body":"Could this return a typed error?"}`,
		`{"schema_version":"1.0","repo":"owner/repo","comment_type":"review_summary","body":"Prefer preserving caller context."}`,
	}, "\n")+"\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Run([]string{
		"--input", input,
		"--patterns-dir", filepath.Join(dir, "missing-patterns"),
		"--output", output,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	got := readFile(t, output)
	for _, want := range []string{
		"# review-patterns prompt: extract",
		"初期版を抽出してください",
		"レコード数: 2",
		"review_comment (1)",
		"internal/app.go (1)",
		"Could this return a typed error?",
		"生のソースコード、長いdiff hunk、長いレビューコメントをパタンファイルへコピーしない",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "## 既存パタンファイル") {
		t.Fatalf("extract prompt unexpectedly included existing pattern section:\n%s", got)
	}
	if !strings.Contains(stderr.String(), "generating extract prompt") {
		t.Fatalf("stderr = %q, want extract progress", stderr.String())
	}
}

func TestRunAutoUpdateWithPatterns(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "reviews.jsonl")
	patternsDir := filepath.Join(dir, "patterns")
	output := filepath.Join(dir, "prompt.md")
	writeFile(t, input, `{"schema_version":"1.0","repo":"owner/repo","comment_type":"review_comment","path":"cmd/main.go","body":"Keep output scriptable."}`+"\n")
	writeFile(t, filepath.Join(patternsDir, "P001-scriptable-output.md"), "# Scriptable Output\n")
	writeFile(t, filepath.Join(patternsDir, "catalog.yaml"), "patterns:\n  - id: P001\n")
	writeFile(t, filepath.Join(patternsDir, "notes.txt"), "this should not be included\n")

	err := Run([]string{
		"--input", input,
		"--patterns-dir", patternsDir,
		"--output", output,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	got := readFile(t, output)
	for _, want := range []string{
		"# review-patterns prompt: update",
		"差分更新してください",
		"## 既存パタンファイル",
		"### `P001-scriptable-output.md`",
		"### `catalog.yaml`",
		"Keep output scriptable.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "notes.txt") {
		t.Fatalf("prompt included non-pattern file:\n%s", got)
	}
	if strings.Index(got, "P001-scriptable-output.md") > strings.Index(got, "catalog.yaml") {
		t.Fatalf("pattern files are not sorted:\n%s", got)
	}
}

func TestRunIgnoresSymlinkedPatternFiles(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "reviews.jsonl")
	patternsDir := filepath.Join(dir, "patterns")
	output := filepath.Join(dir, "prompt.md")
	outside := filepath.Join(dir, "outside.md")
	writeFile(t, input, `{"schema_version":"1.0","repo":"owner/repo","comment_type":"review_comment","body":"Review note."}`+"\n")
	writeFile(t, filepath.Join(patternsDir, "catalog.yaml"), "patterns: []\n")
	writeFile(t, outside, "secret outside content\n")
	if err := os.Symlink(outside, filepath.Join(patternsDir, "linked.md")); err != nil {
		t.Skipf("symlink is not available: %v", err)
	}

	err := Run([]string{
		"--input", input,
		"--patterns-dir", patternsDir,
		"--output", output,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	got := readFile(t, output)
	if strings.Contains(got, "secret outside content") || strings.Contains(got, "linked.md") {
		t.Fatalf("prompt included symlinked pattern file:\n%s", got)
	}
	if !strings.Contains(got, "# review-patterns prompt: update") {
		t.Fatalf("prompt = %q, want update mode", got)
	}
}

func TestRunRejectsUpdateWithoutPatterns(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "reviews.jsonl")
	writeFile(t, input, `{"schema_version":"1.0","repo":"owner/repo","comment_type":"review_comment","body":"Review note."}`+"\n")

	err := Run([]string{
		"--input", input,
		"--patterns-dir", filepath.Join(dir, "missing-patterns"),
		"--mode", "update",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--mode update requires") {
		t.Fatalf("error = %q", err)
	}
}

func TestRunRejectsInvalidJSONLWithLineNumber(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "reviews.jsonl")
	writeFile(t, input, `{"schema_version":"1.0"}`+"\nnot-json\n")

	err := Run([]string{"--input", input}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error = %q", err)
	}
}

func TestRunWritesToStdoutByDefault(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "reviews.jsonl")
	writeFile(t, input, `{"schema_version":"1.0","repo":"owner/repo","comment_type":"review_comment","body":"Review note."}`+"\n")

	var stdout bytes.Buffer
	if err := Run([]string{"--input", input}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "# review-patterns prompt: extract") {
		t.Fatalf("stdout = %q", stdout.String())
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
