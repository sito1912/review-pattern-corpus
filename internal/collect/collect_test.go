package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseOptionsDefaultPeriod(t *testing.T) {
	now := time.Date(2026, 6, 22, 15, 30, 0, 0, time.FixedZone("JST", 9*60*60))
	opts, err := ParseOptions([]string{"--repo", "owner/repo", "--token", "token"}, nil, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatal(err)
	}
	opts.Now = func() time.Time { return now }
	if err := applyPeriod(&opts, "", ""); err != nil {
		t.Fatal(err)
	}

	wantSince := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	wantUntil := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	if !opts.Since.Equal(wantSince) || !opts.Until.Equal(wantUntil) {
		t.Fatalf("period = %s..%s, want %s..%s", opts.Since, opts.Until, wantSince, wantUntil)
	}
}

func TestParseOptionsRejectsPartialPeriod(t *testing.T) {
	_, err := ParseOptions([]string{
		"--repo", "owner/repo",
		"--token", "token",
		"--since", "2026-06-21T00:00:00Z",
	}, nil, bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "provide both --since and --until") {
		t.Fatalf("error = %q", err)
	}
}

func TestParseOptionsHelpDoesNotPrintTokenDefault(t *testing.T) {
	var stderr bytes.Buffer
	_, err := ParseOptions([]string{"--help"}, map[string]string{
		"GITHUB_TOKEN": "secret-token",
	}, &stderr)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
	if strings.Contains(stderr.String(), "secret-token") {
		t.Fatalf("help leaked token: %s", stderr.String())
	}
}

func TestResolveTokenFallsBackToGitHubCLI(t *testing.T) {
	token, err := resolveToken(context.Background(), "", func(context.Context) (string, error) {
		return " gh-token\n", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "gh-token" {
		t.Fatalf("token = %q, want gh-token", token)
	}
}

func TestResolveTokenPrefersExplicitToken(t *testing.T) {
	token, err := resolveToken(context.Background(), "explicit-token", func(context.Context) (string, error) {
		t.Fatal("GitHub CLI token resolver should not be called")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "explicit-token" {
		t.Fatalf("token = %q, want explicit-token", token)
	}
}

func TestCollectFiltersAndOrdersRecords(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		var body string
		switch r.URL.Path {
		case "/search/issues":
			body = `{"total_count":1,"incomplete_results":false,"items":[{"number":2,"title":"Improve validation"}]}`
		case "/repos/owner/repo/pulls/2":
			body = `{
				"number":2,
				"title":"Improve validation",
				"merged_at":"2026-06-21T03:00:00Z",
				"base":{"ref":"main"},
				"head":{"sha":"abc123"}
			}`
		case "/repos/owner/repo/pulls/2/reviews":
			body = `[
				{"id":20,"state":"APPROVED","body":"","user":{"login":"alice","type":"User"},"author_association":"MEMBER","submitted_at":"2026-06-21T03:05:00Z","html_url":"https://github.test/review/20"},
				{"id":21,"state":"COMMENTED","body":"Please keep the caller context.","user":{"login":"alice","type":"User"},"author_association":"MEMBER","submitted_at":"2026-06-21T03:04:00Z","html_url":"https://github.test/review/21"},
				{"id":22,"state":"APPROVED","body":"Looks good after the cleanup.","user":{"login":"bob","type":"User"},"author_association":"CONTRIBUTOR","submitted_at":"2026-06-21T03:07:00Z","html_url":"https://github.test/review/22"},
				{"id":23,"state":"COMMENTED","body":"Generated summary","user":{"login":"ci[bot]","type":"Bot"},"author_association":"NONE","submitted_at":"2026-06-21T03:08:00Z","html_url":"https://github.test/review/23"}
			]`
		case "/repos/owner/repo/pulls/2/comments":
			body = `[
				{"id":31,"pull_request_review_id":21,"in_reply_to_id":null,"body":"Could this return a typed error?","user":{"login":"carol","type":"User"},"author_association":"MEMBER","path":"internal/app.go","line":40,"start_line":null,"side":"RIGHT","diff_hunk":"@@ -1 +1 @@","created_at":"2026-06-21T03:06:00Z","updated_at":"2026-06-21T03:06:30Z","html_url":"https://github.test/comment/31"},
				{"id":32,"pull_request_review_id":21,"in_reply_to_id":31,"body":"Good point, I will adjust it.","user":{"login":"dave","type":"User"},"author_association":"MEMBER","path":"internal/app.go","line":41,"start_line":40,"side":"RIGHT","diff_hunk":"@@ -1 +1 @@","created_at":"2026-06-21T03:06:10Z","updated_at":"2026-06-21T03:06:40Z","html_url":"https://github.test/comment/32"},
				{"id":33,"pull_request_review_id":21,"in_reply_to_id":null,"body":"Bot note","user":{"login":"github-actions[bot]","type":"Bot"},"author_association":"NONE","path":"internal/app.go","line":42,"start_line":null,"side":"RIGHT","diff_hunk":"@@ -1 +1 @@","created_at":"2026-06-21T03:06:20Z","updated_at":"2026-06-21T03:06:20Z","html_url":"https://github.test/comment/33"}
			]`
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, headers, body), nil
	})}

	var progress bytes.Buffer
	records, err := Collect(context.Background(), Options{
		Repo:       "owner/repo",
		Since:      time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC),
		Until:      time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC),
		Token:      "token",
		APIBaseURL: "https://api.github.test",
		HTTPClient: client,
		Progress:   &progress,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(records) != 4 {
		t.Fatalf("len(records) = %d, want 4", len(records))
	}

	gotOrder := make([]string, len(records))
	for i, record := range records {
		gotOrder[i] = fmt.Sprintf("%s:%d", record.CommentType, record.CommentID)
	}
	wantOrder := []string{
		"review_summary:21",
		"review_comment:31",
		"review_comment_reply:32",
		"review_summary:22",
	}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("order = %v, want %v", gotOrder, wantOrder)
	}
	if records[1].Language == nil || *records[1].Language != "go" {
		t.Fatalf("language = %v, want go", records[1].Language)
	}
	if records[1].ReviewState == nil || *records[1].ReviewState != "COMMENTED" {
		t.Fatalf("review_state = %v, want COMMENTED", records[1].ReviewState)
	}
	for _, want := range []string{
		"searching merged pull requests",
		"found 1 merged pull request(s)",
		"collecting PR #2 (1/1)",
		"collected 4 record(s) total",
	} {
		if !strings.Contains(progress.String(), want) {
			t.Fatalf("progress output does not contain %q:\n%s", want, progress.String())
		}
	}

	var buf bytes.Buffer
	if err := WriteJSONL(&buf, records); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("jsonl lines = %d, want 4", len(lines))
	}
	for _, line := range lines {
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid jsonl line %q: %v", line, err)
		}
	}
}

func TestCollectRateLimitError(t *testing.T) {
	resetAt := time.Date(2026, 6, 22, 0, 10, 0, 0, time.UTC)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		headers.Set("X-RateLimit-Remaining", "0")
		headers.Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))
		return jsonResponse(http.StatusForbidden, headers, `{"message":"API rate limit exceeded"}`), nil
	})}

	_, err := Collect(context.Background(), Options{
		Repo:       "owner/repo",
		Since:      time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC),
		Until:      time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC),
		Token:      "token",
		APIBaseURL: "https://api.github.test",
		HTTPClient: client,
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var rateLimitErr RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("error = %T %v, want RateLimitError", err, err)
	}
	if rateLimitErr.ResetAt == nil || !rateLimitErr.ResetAt.Equal(resetAt) {
		t.Fatalf("reset = %v, want %v", rateLimitErr.ResetAt, resetAt)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, headers http.Header, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
