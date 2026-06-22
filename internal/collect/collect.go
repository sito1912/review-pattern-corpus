package collect

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	schemaVersion = "1.0"
	defaultAPIURL = "https://api.github.com"
)

// Options describes a collect command invocation.
type Options struct {
	Repo                 string
	Since                time.Time
	Until                time.Time
	Output               string
	Token                string
	IncludeIssueComments bool

	APIBaseURL string
	HTTPClient *http.Client
	Now        func() time.Time
	Progress   io.Writer
}

// Record is the JSONL object emitted for each collected review item.
type Record struct {
	SchemaVersion     string         `json:"schema_version"`
	Repo              string         `json:"repo"`
	PRNumber          int            `json:"pr_number"`
	PRTitle           string         `json:"pr_title"`
	PRMergedAt        time.Time      `json:"pr_merged_at"`
	CommentType       string         `json:"comment_type"`
	CommentID         int64          `json:"comment_id"`
	ReviewID          *int64         `json:"review_id"`
	InReplyToID       *int64         `json:"in_reply_to_id"`
	ReviewState       *string        `json:"review_state"`
	Author            string         `json:"author"`
	AuthorType        string         `json:"author_type"`
	AuthorAssociation *string        `json:"author_association"`
	Path              *string        `json:"path"`
	Language          *string        `json:"language"`
	Line              *int           `json:"line"`
	StartLine         *int           `json:"start_line"`
	Side              *string        `json:"side"`
	DiffHunk          *string        `json:"diff_hunk"`
	Body              string         `json:"body"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Metadata          RecordMetadata `json:"metadata"`
}

// RecordMetadata stores contextual identifiers and links for a collected item.
type RecordMetadata struct {
	BaseRef string `json:"base_ref"`
	HeadSHA string `json:"head_sha"`
	HTMLURL string `json:"html_url"`
}

// EnvMap converts an os.Environ-style slice into a map.
func EnvMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

// Run parses collect flags, collects review data, and writes JSONL.
func Run(args []string, env map[string]string, stdout, stderr io.Writer) error {
	opts, err := ParseOptions(args, env, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	opts.Progress = stderr

	progressf(opts.Progress, "review-patterns: resolving GitHub authentication")
	token, err := resolveToken(context.Background(), opts.Token, githubCLIToken)
	if err != nil {
		return err
	}
	opts.Token = token

	progressf(opts.Progress, "review-patterns: collecting repo=%s since=%s until=%s", opts.Repo, opts.Since.Format(time.RFC3339), opts.Until.Format(time.RFC3339))
	records, err := Collect(context.Background(), opts)
	if err != nil {
		return err
	}

	if opts.Output == "" || opts.Output == "-" {
		progressf(opts.Progress, "review-patterns: writing %d record(s) to stdout", len(records))
		if err := WriteJSONL(stdout, records); err != nil {
			return err
		}
		progressf(opts.Progress, "review-patterns: done")
		return nil
	}

	outputDir := filepath.Dir(opts.Output)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	file, err := os.OpenFile(opts.Output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open output file: %w", err)
	}
	defer file.Close()

	progressf(opts.Progress, "review-patterns: writing %d record(s) to %s", len(records), opts.Output)
	if err := WriteJSONL(file, records); err != nil {
		return err
	}
	progressf(opts.Progress, "review-patterns: done")
	return nil
}

// ParseOptions parses collect flags and applies environment defaults.
func ParseOptions(args []string, env map[string]string, stderr io.Writer) (Options, error) {
	opts := Options{
		Output:     "-",
		APIBaseURL: defaultAPIURL,
		Now:        time.Now,
	}

	flags := flag.NewFlagSet("collect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.Repo, "repo", env["GITHUB_REPOSITORY"], "GitHub repository in owner/repo form")
	flags.StringVar(&opts.Output, "output", "-", "JSONL output path, or - for stdout")
	flags.BoolVar(&opts.IncludeIssueComments, "include-issue-comments", false, "include pull request conversation issue comments")

	var sinceText string
	var tokenText string
	var untilText string
	flags.StringVar(&sinceText, "since", "", "inclusive UTC start timestamp in RFC3339 format")
	flags.StringVar(&tokenText, "token", "", "GitHub token; defaults to GITHUB_TOKEN, GH_TOKEN, or GitHub CLI auth")
	flags.StringVar(&untilText, "until", "", "exclusive UTC end timestamp in RFC3339 format")

	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if flags.NArg() > 0 {
		return Options{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	if err := applyPeriod(&opts, sinceText, untilText); err != nil {
		return Options{}, err
	}
	opts.Token = firstNonEmpty(tokenText, env["GITHUB_TOKEN"], env["GH_TOKEN"])
	if err := validateOptions(opts); err != nil {
		return Options{}, err
	}
	return opts, nil
}

// Collect fetches matching pull requests and converts review items into records.
func Collect(ctx context.Context, opts Options) ([]Record, error) {
	if opts.APIBaseURL == "" {
		opts.APIBaseURL = defaultAPIURL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}

	owner, repoName, err := splitRepo(opts.Repo)
	if err != nil {
		return nil, err
	}

	client, err := newGitHubClient(opts.APIBaseURL, opts.Token, opts.HTTPClient)
	if err != nil {
		return nil, err
	}

	progressf(opts.Progress, "review-patterns: searching merged pull requests")
	prs, err := client.mergedPullRequests(ctx, opts.Repo, owner, repoName, opts.Since, opts.Until)
	if err != nil {
		return nil, err
	}
	progressf(opts.Progress, "review-patterns: found %d merged pull request(s)", len(prs))

	var records []Record
	for i, pr := range prs {
		beforeCount := len(records)
		progressf(opts.Progress, "review-patterns: collecting PR #%d (%d/%d)", pr.Number, i+1, len(prs))

		reviews, err := client.pullReviews(ctx, owner, repoName, pr.Number)
		if err != nil {
			return nil, err
		}
		reviewStates := make(map[int64]string, len(reviews))
		for _, review := range reviews {
			reviewStates[review.ID] = review.State
			if record, ok := reviewRecord(opts.Repo, pr, review); ok {
				records = append(records, record)
			}
		}

		comments, err := client.pullReviewComments(ctx, owner, repoName, pr.Number)
		if err != nil {
			return nil, err
		}
		for _, comment := range comments {
			if record, ok := reviewCommentRecord(opts.Repo, pr, comment, reviewStates); ok {
				records = append(records, record)
			}
		}

		if opts.IncludeIssueComments {
			issueComments, err := client.issueComments(ctx, owner, repoName, pr.Number)
			if err != nil {
				return nil, err
			}
			for _, comment := range issueComments {
				if record, ok := issueCommentRecord(opts.Repo, pr, comment); ok {
					records = append(records, record)
				}
			}
		}
		progressf(opts.Progress, "review-patterns: PR #%d collected %d record(s)", pr.Number, len(records)-beforeCount)
	}

	sortRecords(records)
	progressf(opts.Progress, "review-patterns: collected %d record(s) total", len(records))
	return records, nil
}

// WriteJSONL writes records as one compact JSON object per line.
func WriteJSONL(w io.Writer, records []Record) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			return fmt.Errorf("write jsonl: %w", err)
		}
	}
	return nil
}

func progressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}

func applyPeriod(opts *Options, sinceText, untilText string) error {
	if sinceText == "" && untilText == "" {
		nowUTC := opts.Now().UTC()
		until := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
		opts.Since = until.AddDate(0, 0, -1)
		opts.Until = until
		return nil
	}
	if sinceText == "" || untilText == "" {
		return errors.New("provide both --since and --until, or omit both to use the previous UTC day")
	}

	since, err := time.Parse(time.RFC3339, sinceText)
	if err != nil {
		return fmt.Errorf("parse --since: %w", err)
	}
	until, err := time.Parse(time.RFC3339, untilText)
	if err != nil {
		return fmt.Errorf("parse --until: %w", err)
	}
	opts.Since = since.UTC()
	opts.Until = until.UTC()
	return nil
}

func validateOptions(opts Options) error {
	if _, _, err := splitRepo(opts.Repo); err != nil {
		return err
	}
	if !opts.Since.Before(opts.Until) {
		return errors.New("--since must be before --until")
	}
	return nil
}

func resolveToken(ctx context.Context, token string, ghToken func(context.Context) (string, error)) (string, error) {
	if token != "" {
		return token, nil
	}

	token, err := ghToken(ctx)
	if err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token), nil
	}
	if err == nil {
		err = errors.New("GitHub CLI returned an empty token")
	}
	return "", fmt.Errorf("GitHub token is required; set GITHUB_TOKEN, set GH_TOKEN, pass --token, or authenticate GitHub CLI with `gh auth login`: %w", err)
}

func githubCLIToken(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("read GitHub CLI token with `gh auth token`: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func splitRepo(repo string) (string, string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("--repo must be in owner/repo form")
	}
	return owner, name, nil
}

func newGitHubClient(apiBaseURL, token string, httpClient *http.Client) (*gitHubClient, error) {
	baseURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("GitHub API URL must be absolute")
	}
	return &gitHubClient{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
	}, nil
}

type gitHubClient struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

func (c *gitHubClient) mergedPullRequests(ctx context.Context, repo, owner, repoName string, since, until time.Time) ([]pullRequest, error) {
	startDate := since.Format("2006-01-02")
	endDate := until.Add(-time.Nanosecond).Format("2006-01-02")
	query := fmt.Sprintf("repo:%s is:pr is:merged merged:%s..%s", repo, startDate, endDate)

	var search searchIssuesResponse
	err := c.getAll(ctx, "/search/issues", url.Values{
		"q":     []string{query},
		"sort":  []string{"created"},
		"order": []string{"asc"},
	}, func() any {
		return &searchIssuesResponse{}
	}, func(page any) error {
		response := page.(*searchIssuesResponse)
		if response.IncompleteResults {
			return errors.New("GitHub search returned incomplete results; narrow the collection period and retry")
		}
		if response.TotalCount > 1000 {
			return errors.New("GitHub search returned more than 1000 candidate pull requests; narrow the collection period and retry")
		}
		search.Items = append(search.Items, response.Items...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	prs := make([]pullRequest, 0, len(search.Items))
	seen := make(map[int]bool, len(search.Items))
	for _, item := range search.Items {
		if seen[item.Number] {
			continue
		}
		seen[item.Number] = true

		pr, err := c.pullRequest(ctx, owner, repoName, item.Number)
		if err != nil {
			return nil, err
		}
		if pr.MergedAt == nil {
			continue
		}
		mergedAt := pr.MergedAt.UTC()
		if !mergedAt.Before(since) && mergedAt.Before(until) {
			pr.MergedAt = &mergedAt
			prs = append(prs, pr)
		}
	}

	sort.Slice(prs, func(i, j int) bool {
		left := prs[i]
		right := prs[j]
		if !left.MergedAt.Equal(*right.MergedAt) {
			return left.MergedAt.Before(*right.MergedAt)
		}
		return left.Number < right.Number
	})
	return prs, nil
}

func (c *gitHubClient) pullRequest(ctx context.Context, owner, repoName string, number int) (pullRequest, error) {
	var pr pullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repoName), number)
	if err := c.get(ctx, path, nil, &pr); err != nil {
		return pullRequest{}, err
	}
	return pr, nil
}

func (c *gitHubClient) pullReviews(ctx context.Context, owner, repoName string, number int) ([]pullReview, error) {
	var reviews []pullReview
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", url.PathEscape(owner), url.PathEscape(repoName), number)
	err := c.getAll(ctx, path, nil, func() any {
		return &[]pullReview{}
	}, func(page any) error {
		reviews = append(reviews, (*page.(*[]pullReview))...)
		return nil
	})
	return reviews, err
}

func (c *gitHubClient) pullReviewComments(ctx context.Context, owner, repoName string, number int) ([]pullReviewComment, error) {
	var comments []pullReviewComment
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", url.PathEscape(owner), url.PathEscape(repoName), number)
	err := c.getAll(ctx, path, nil, func() any {
		return &[]pullReviewComment{}
	}, func(page any) error {
		comments = append(comments, (*page.(*[]pullReviewComment))...)
		return nil
	})
	return comments, err
}

func (c *gitHubClient) issueComments(ctx context.Context, owner, repoName string, number int) ([]issueComment, error) {
	var comments []issueComment
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", url.PathEscape(owner), url.PathEscape(repoName), number)
	err := c.getAll(ctx, path, nil, func() any {
		return &[]issueComment{}
	}, func(page any) error {
		comments = append(comments, (*page.(*[]issueComment))...)
		return nil
	})
	return comments, err
}

func (c *gitHubClient) getAll(ctx context.Context, path string, query url.Values, newPage func() any, appendPage func(any) error) error {
	for page := 1; ; page++ {
		pageQuery := cloneValues(query)
		pageQuery.Set("per_page", "100")
		pageQuery.Set("page", strconv.Itoa(page))

		target := newPage()
		headers, err := c.getWithHeaders(ctx, path, pageQuery, target)
		if err != nil {
			return err
		}
		if err := appendPage(target); err != nil {
			return err
		}
		if !hasNextPage(headers.Get("Link")) {
			return nil
		}
	}
}

func (c *gitHubClient) get(ctx context.Context, path string, query url.Values, target any) error {
	_, err := c.getWithHeaders(ctx, path, query, target)
	return err
}

func (c *gitHubClient) getWithHeaders(ctx context.Context, path string, query url.Values, target any) (http.Header, error) {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: strings.TrimRight(c.baseURL.Path, "/") + path})
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create GitHub request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "review-patterns")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.Header, apiError(resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return resp.Header, fmt.Errorf("decode GitHub API response: %w", err)
	}
	return resp.Header, nil
}

func apiError(resp *http.Response) error {
	var body struct {
		Message          string `json:"message"`
		DocumentationURL string `json:"documentation_url"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)

	message := strings.TrimSpace(body.Message)
	if message == "" {
		message = resp.Status
	}

	if isRateLimited(resp, message) {
		return RateLimitError{
			Status:     resp.StatusCode,
			Message:    message,
			ResetAt:    rateLimitReset(resp.Header),
			RetryAfter: resp.Header.Get("Retry-After"),
		}
	}

	if body.DocumentationURL != "" {
		message = message + " (" + body.DocumentationURL + ")"
	}
	return fmt.Errorf("GitHub API %s: %s", resp.Status, message)
}

// RateLimitError reports GitHub primary or secondary rate limiting.
type RateLimitError struct {
	Status     int
	Message    string
	ResetAt    *time.Time
	RetryAfter string
}

func (e RateLimitError) Error() string {
	parts := []string{fmt.Sprintf("GitHub API rate limited (%d): %s", e.Status, e.Message)}
	if e.ResetAt != nil {
		parts = append(parts, "reset at "+e.ResetAt.Format(time.RFC3339))
	}
	if e.RetryAfter != "" {
		parts = append(parts, "retry after "+e.RetryAfter+" seconds")
	}
	return strings.Join(parts, "; ")
}

func isRateLimited(resp *http.Response, message string) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	if resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(message), "rate limit") {
		return true
	}
	return resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(resp.Header.Get("X-RateLimit-Resource")), "search")
}

func rateLimitReset(headers http.Header) *time.Time {
	reset := headers.Get("X-RateLimit-Reset")
	if reset == "" {
		return nil
	}
	seconds, err := strconv.ParseInt(reset, 10, 64)
	if err != nil {
		return nil
	}
	resetAt := time.Unix(seconds, 0).UTC()
	return &resetAt
}

func reviewRecord(repo string, pr pullRequest, review pullReview) (Record, bool) {
	if !isHumanUser(review.User) || strings.TrimSpace(review.Body) == "" {
		return Record{}, false
	}

	submittedAt := prMergedAt(pr)
	if review.SubmittedAt != nil {
		submittedAt = review.SubmittedAt.UTC()
	}
	reviewID := review.ID
	state := review.State
	authorAssociation := nullableString(review.AuthorAssociation)

	return Record{
		SchemaVersion:     schemaVersion,
		Repo:              repo,
		PRNumber:          pr.Number,
		PRTitle:           pr.Title,
		PRMergedAt:        prMergedAt(pr),
		CommentType:       "review_summary",
		CommentID:         review.ID,
		ReviewID:          &reviewID,
		InReplyToID:       nil,
		ReviewState:       nullableString(state),
		Author:            review.User.Login,
		AuthorType:        review.User.Type,
		AuthorAssociation: authorAssociation,
		Path:              nil,
		Language:          nil,
		Line:              nil,
		StartLine:         nil,
		Side:              nil,
		DiffHunk:          nil,
		Body:              review.Body,
		CreatedAt:         submittedAt,
		UpdatedAt:         submittedAt,
		Metadata:          metadata(pr, review.HTMLURL),
	}, true
}

func reviewCommentRecord(repo string, pr pullRequest, comment pullReviewComment, reviewStates map[int64]string) (Record, bool) {
	if !isHumanUser(comment.User) || strings.TrimSpace(comment.Body) == "" {
		return Record{}, false
	}

	commentType := "review_comment"
	if comment.InReplyToID != nil {
		commentType = "review_comment_reply"
	}

	var reviewID *int64
	var reviewState *string
	if comment.PullRequestReviewID != 0 {
		value := comment.PullRequestReviewID
		reviewID = &value
		if state, ok := reviewStates[value]; ok {
			reviewState = nullableString(state)
		}
	}

	authorAssociation := nullableString(comment.AuthorAssociation)
	path := nullableString(comment.Path)
	side := nullableString(comment.Side)
	diffHunk := nullableString(comment.DiffHunk)

	return Record{
		SchemaVersion:     schemaVersion,
		Repo:              repo,
		PRNumber:          pr.Number,
		PRTitle:           pr.Title,
		PRMergedAt:        prMergedAt(pr),
		CommentType:       commentType,
		CommentID:         comment.ID,
		ReviewID:          reviewID,
		InReplyToID:       comment.InReplyToID,
		ReviewState:       reviewState,
		Author:            comment.User.Login,
		AuthorType:        comment.User.Type,
		AuthorAssociation: authorAssociation,
		Path:              path,
		Language:          languageForPath(comment.Path),
		Line:              comment.Line,
		StartLine:         comment.StartLine,
		Side:              side,
		DiffHunk:          diffHunk,
		Body:              comment.Body,
		CreatedAt:         comment.CreatedAt.UTC(),
		UpdatedAt:         comment.UpdatedAt.UTC(),
		Metadata:          metadata(pr, comment.HTMLURL),
	}, true
}

func issueCommentRecord(repo string, pr pullRequest, comment issueComment) (Record, bool) {
	if !isHumanUser(comment.User) || strings.TrimSpace(comment.Body) == "" {
		return Record{}, false
	}

	authorAssociation := nullableString(comment.AuthorAssociation)
	return Record{
		SchemaVersion:     schemaVersion,
		Repo:              repo,
		PRNumber:          pr.Number,
		PRTitle:           pr.Title,
		PRMergedAt:        prMergedAt(pr),
		CommentType:       "issue_comment",
		CommentID:         comment.ID,
		ReviewID:          nil,
		InReplyToID:       nil,
		ReviewState:       nil,
		Author:            comment.User.Login,
		AuthorType:        comment.User.Type,
		AuthorAssociation: authorAssociation,
		Path:              nil,
		Language:          nil,
		Line:              nil,
		StartLine:         nil,
		Side:              nil,
		DiffHunk:          nil,
		Body:              comment.Body,
		CreatedAt:         comment.CreatedAt.UTC(),
		UpdatedAt:         comment.UpdatedAt.UTC(),
		Metadata:          metadata(pr, comment.HTMLURL),
	}, true
}

func metadata(pr pullRequest, htmlURL string) RecordMetadata {
	return RecordMetadata{
		BaseRef: pr.Base.Ref,
		HeadSHA: pr.Head.SHA,
		HTMLURL: htmlURL,
	}
}

func prMergedAt(pr pullRequest) time.Time {
	if pr.MergedAt == nil {
		return time.Time{}
	}
	return pr.MergedAt.UTC()
}

func isHumanUser(user *gitHubUser) bool {
	if user == nil || user.Login == "" {
		return false
	}
	login := strings.ToLower(user.Login)
	if strings.EqualFold(user.Type, "Bot") || strings.HasSuffix(login, "[bot]") {
		return false
	}
	return true
}

func sortRecords(records []Record) {
	typeRank := map[string]int{
		"review_summary":       0,
		"review_comment":       1,
		"review_comment_reply": 2,
		"issue_comment":        3,
	}
	sort.Slice(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if !left.PRMergedAt.Equal(right.PRMergedAt) {
			return left.PRMergedAt.Before(right.PRMergedAt)
		}
		if left.PRNumber != right.PRNumber {
			return left.PRNumber < right.PRNumber
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		if typeRank[left.CommentType] != typeRank[right.CommentType] {
			return typeRank[left.CommentType] < typeRank[right.CommentType]
		}
		return left.CommentID < right.CommentID
	})
}

func hasNextPage(linkHeader string) bool {
	for _, part := range strings.Split(linkHeader, ",") {
		if strings.Contains(part, `rel="next"`) {
			return true
		}
	}
	return false
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, value := range values {
		cloned[key] = append([]string(nil), value...)
	}
	return cloned
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func languageForPath(path string) *string {
	ext := strings.ToLower(filepath.Ext(path))
	languages := map[string]string{
		".c":    "c",
		".cc":   "cpp",
		".cpp":  "cpp",
		".cs":   "csharp",
		".css":  "css",
		".go":   "go",
		".html": "html",
		".java": "java",
		".js":   "javascript",
		".jsx":  "javascript",
		".kt":   "kotlin",
		".md":   "markdown",
		".php":  "php",
		".py":   "python",
		".rb":   "ruby",
		".rs":   "rust",
		".sh":   "shell",
		".ts":   "typescript",
		".tsx":  "typescript",
		".yaml": "yaml",
		".yml":  "yaml",
	}
	if language, ok := languages[ext]; ok {
		return &language
	}
	return nil
}

type searchIssuesResponse struct {
	TotalCount        int           `json:"total_count"`
	IncompleteResults bool          `json:"incomplete_results"`
	Items             []searchIssue `json:"items"`
}

type searchIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type pullRequest struct {
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	MergedAt *time.Time `json:"merged_at"`
	Base     struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type pullReview struct {
	ID                int64       `json:"id"`
	State             string      `json:"state"`
	Body              string      `json:"body"`
	User              *gitHubUser `json:"user"`
	AuthorAssociation string      `json:"author_association"`
	SubmittedAt       *time.Time  `json:"submitted_at"`
	HTMLURL           string      `json:"html_url"`
}

type pullReviewComment struct {
	ID                  int64       `json:"id"`
	PullRequestReviewID int64       `json:"pull_request_review_id"`
	InReplyToID         *int64      `json:"in_reply_to_id"`
	Body                string      `json:"body"`
	User                *gitHubUser `json:"user"`
	AuthorAssociation   string      `json:"author_association"`
	Path                string      `json:"path"`
	Line                *int        `json:"line"`
	StartLine           *int        `json:"start_line"`
	Side                string      `json:"side"`
	DiffHunk            string      `json:"diff_hunk"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	HTMLURL             string      `json:"html_url"`
}

type issueComment struct {
	ID                int64       `json:"id"`
	Body              string      `json:"body"`
	User              *gitHubUser `json:"user"`
	AuthorAssociation string      `json:"author_association"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	HTMLURL           string      `json:"html_url"`
}

type gitHubUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}
