package prompt

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultOutput      = "-"
	defaultPatternsDir = ".review-patterns/patterns"
)

// Options describes a prompt command invocation.
type Options struct {
	Input            string
	PatternsDir      string
	Output           string
	Mode             string
	ReviewerPatterns bool

	Progress io.Writer
}

type corpus struct {
	Path           string
	Stats          corpusStats
	ReviewerAuthor string
}

type corpusStats struct {
	Total        int
	Repos        map[string]int
	CommentTypes map[string]int
	Paths        map[string]int
}

type patternFile struct {
	Path string
}

// Run parses prompt flags and writes a generated agent prompt.
func Run(args []string, stdout, stderr io.Writer) error {
	opts, err := ParseOptions(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	opts.Progress = stderr

	progressf(opts.Progress, "review-patterns: reading review corpus from %s", opts.Input)
	corpus, err := readCorpus(opts.Input, opts.ReviewerPatterns)
	if err != nil {
		return err
	}

	progressf(opts.Progress, "review-patterns: reading patterns from %s", opts.PatternsDir)
	patterns, err := readPatternFiles(opts.PatternsDir)
	if err != nil {
		return err
	}

	selectedMode, err := selectMode(opts.Mode, patterns)
	if err != nil {
		return err
	}
	progressf(opts.Progress, "review-patterns: generating %s prompt", selectedMode)

	content := renderPrompt(selectedMode, corpus, patterns, opts.PatternsDir, opts.ReviewerPatterns)
	if opts.Output == "" || opts.Output == "-" {
		progressf(opts.Progress, "review-patterns: writing prompt to stdout")
		_, err := io.WriteString(stdout, content)
		if err != nil {
			return fmt.Errorf("write prompt: %w", err)
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

	progressf(opts.Progress, "review-patterns: writing prompt to %s", opts.Output)
	if err := os.WriteFile(opts.Output, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}
	progressf(opts.Progress, "review-patterns: done")
	return nil
}

// ParseOptions parses prompt flags.
func ParseOptions(args []string, stderr io.Writer) (Options, error) {
	opts := Options{
		Output:      defaultOutput,
		PatternsDir: defaultPatternsDir,
		Mode:        "auto",
	}

	flags := flag.NewFlagSet("prompt", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.Input, "input", "", "JSONL review corpus input path")
	flags.StringVar(&opts.PatternsDir, "patterns-dir", defaultPatternsDir, "directory containing existing pattern files")
	flags.StringVar(&opts.Output, "output", defaultOutput, "prompt output path, or - for stdout")
	flags.StringVar(&opts.Mode, "mode", "auto", "prompt mode: auto, extract, or update")
	flags.BoolVar(&opts.ReviewerPatterns, "reviewer-patterns", false, "specialize pattern mining for a single reviewer author")

	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if flags.NArg() > 0 {
		return Options{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	opts.Mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	if err := validateOptions(opts); err != nil {
		return Options{}, err
	}
	return opts, nil
}

func validateOptions(opts Options) error {
	if strings.TrimSpace(opts.Input) == "" {
		return errors.New("--input is required")
	}
	if strings.TrimSpace(opts.PatternsDir) == "" {
		return errors.New("--patterns-dir is required")
	}
	switch opts.Mode {
	case "auto", "extract", "update":
		return nil
	default:
		return fmt.Errorf("--mode must be auto, extract, or update")
	}
}

func readCorpus(path string, requireSingleAuthor bool) (corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return corpus{}, fmt.Errorf("read input JSONL: %w", err)
	}

	stats := corpusStats{
		Repos:        make(map[string]int),
		CommentTypes: make(map[string]int),
		Paths:        make(map[string]int),
	}
	var reviewerAuthor string
	var reviewerAuthorLine int

	for i, rawLine := range strings.Split(string(data), "\n") {
		lineNumber := i + 1
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" {
			continue
		}

		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &object); err != nil {
			return corpus{}, fmt.Errorf("parse input JSONL line %d: %w", lineNumber, err)
		}
		if object == nil {
			return corpus{}, fmt.Errorf("parse input JSONL line %d: expected JSON object", lineNumber)
		}

		stats.Total++
		if requireSingleAuthor {
			author, err := requiredReviewerAuthor(object, lineNumber)
			if err != nil {
				return corpus{}, fmt.Errorf("%w. %s", err, reviewerFilterSuggestion(path, reviewerAuthor))
			}
			if reviewerAuthor == "" {
				reviewerAuthor = author
				reviewerAuthorLine = lineNumber
			} else if author != reviewerAuthor {
				return corpus{}, fmt.Errorf("--reviewer-patterns requires all input JSONL records to have the same author; line %d has author %q, but line %d established author %q. %s", lineNumber, author, reviewerAuthorLine, reviewerAuthor, reviewerFilterSuggestion(path, reviewerAuthor))
			}
		}
		incrementJSONText(stats.Repos, object["repo"])
		incrementJSONText(stats.CommentTypes, object["comment_type"])
		incrementJSONText(stats.Paths, object["path"])
	}
	if requireSingleAuthor && stats.Total == 0 {
		return corpus{}, fmt.Errorf("--reviewer-patterns requires at least one JSONL record with a non-empty author. %s", reviewerFilterSuggestion(path, ""))
	}

	return corpus{Path: path, Stats: stats, ReviewerAuthor: reviewerAuthor}, nil
}

func requiredReviewerAuthor(object map[string]json.RawMessage, lineNumber int) (string, error) {
	raw, ok := object["author"]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("--reviewer-patterns requires every input JSONL record to have the same non-empty author; line %d has no author", lineNumber)
	}
	var author string
	if err := json.Unmarshal(raw, &author); err != nil {
		return "", fmt.Errorf("--reviewer-patterns requires every input JSONL record to have a string author; line %d author is not a string", lineNumber)
	}
	if strings.TrimSpace(author) == "" {
		return "", fmt.Errorf("--reviewer-patterns requires every input JSONL record to have the same non-empty author; line %d has an empty author", lineNumber)
	}
	return author, nil
}

func reviewerFilterSuggestion(inputPath, author string) string {
	if author == "" {
		return fmt.Sprintf("Filter the corpus first, for example: review-patterns filter --input %q --author <login> --output <filtered.jsonl>", inputPath)
	}
	return fmt.Sprintf("Filter the corpus first, for example: review-patterns filter --input %q --author %q --output <filtered.jsonl>", inputPath, author)
}

func incrementJSONText(counts map[string]int, raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		counts[text]++
	}
}

func readPatternFiles(dir string) ([]patternFile, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read patterns directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("--patterns-dir must be a directory")
	}

	var files []patternFile
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect pattern file %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if !isPatternFile(path) {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relativize pattern file %s: %w", path, err)
		}
		files = append(files, patternFile{
			Path: filepath.ToSlash(rel),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk patterns directory: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func isPatternFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func selectMode(mode string, patterns []patternFile) (string, error) {
	if mode == "auto" {
		if len(patterns) == 0 {
			return "extract", nil
		}
		return "update", nil
	}
	if mode == "update" && len(patterns) == 0 {
		return "", errors.New("--mode update requires at least one .md, .yaml, or .yml file in --patterns-dir")
	}
	return mode, nil
}

func renderPrompt(mode string, corpus corpus, patterns []patternFile, patternsDir string, reviewerPatterns bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# review-patterns prompt: %s", mode)
	if reviewerPatterns {
		b.WriteString(" reviewer-patterns")
	}
	b.WriteString("\n\n")
	if mode == "extract" {
		writeExtractInstructions(&b, patternsDir, reviewerPatterns, corpus.ReviewerAuthor)
	} else {
		writeUpdateInstructions(&b, patternsDir, reviewerPatterns, corpus.ReviewerAuthor)
		writePatternReferences(&b, patternsDir, patterns)
	}
	writeCorpusSummary(&b, corpus.Stats, corpus.ReviewerAuthor)
	writeCorpusReference(&b, corpus.Path)
	b.WriteString("\n")
	return b.String()
}

func writeExtractInstructions(b *strings.Builder, patternsDir string, reviewerPatterns bool, reviewerAuthor string) {
	fmt.Fprintf(b, "## 目的\n\n")
	if reviewerPatterns {
		fmt.Fprintf(b, "指定されたレビューJSONLファイルから、author=%q のレビュワーが繰り返し見ている兆候、判断軸、フォースの重みづけに特化したパタンランゲージの初期版を抽出してください。出力先は `%s` です。\n\n", reviewerAuthor, patternsDir)
	} else {
		fmt.Fprintf(b, "指定されたレビューJSONLファイルから、リポジトリローカルなコードレビュー用パタンランゲージの初期版を抽出してください。出力先は `%s` です。\n\n", patternsDir)
	}
	writeSharedInstructions(b, reviewerPatterns, reviewerAuthor)
	b.WriteString("## 初回抽出での判断\n\n")
	b.WriteString("- 繰り返し現れる問題、または今後も再利用できる強いレビュー知識だけをパタン化する。\n")
	b.WriteString("- 一回限りの好み、文脈依存の指摘、根拠の弱い一般化はパタンにしない。\n")
	b.WriteString("- 必要なら `catalog.yaml` と `P###-slug.md` を新規作成する。\n\n")
}

func writeUpdateInstructions(b *strings.Builder, patternsDir string, reviewerPatterns bool, reviewerAuthor string) {
	fmt.Fprintf(b, "## 目的\n\n")
	if reviewerPatterns {
		fmt.Fprintf(b, "指定されたレビューJSONLファイルと既存パタンを比較し、author=%q のレビュワーが繰り返し見ている兆候、判断軸、フォースの重みづけに特化したパタンランゲージを差分更新してください。出力先は `%s` です。\n\n", reviewerAuthor, patternsDir)
	} else {
		fmt.Fprintf(b, "指定されたレビューJSONLファイルと既存パタンを比較し、リポジトリローカルなコードレビュー用パタンランゲージを差分更新してください。出力先は `%s` です。\n\n", patternsDir)
	}
	writeSharedInstructions(b, reviewerPatterns, reviewerAuthor)
	b.WriteString("## 差分更新での判断\n\n")
	b.WriteString("- 新しいレビュー例によって適用範囲、フォース、例外、誤検知が明確になる場合は既存パタンを更新する。\n")
	b.WriteString("- 既存パタンでは扱えない繰り返し問題がある場合だけ新規パタンを追加する。\n")
	b.WriteString("- 重複するパタンが見つかった場合は統合するか、deprecatedとして扱う方針を示す。\n")
	b.WriteString("- 変更不要なら、なぜ変更不要かを簡潔に説明する。\n\n")
}

func writeSharedInstructions(b *strings.Builder, reviewerPatterns bool, reviewerAuthor string) {
	b.WriteString("## 守ること\n\n")
	b.WriteString("- AIエージェント自体の実行や外部サービス呼び出しは行わず、パタンファイルの編集内容だけを提案または適用する。\n")
	b.WriteString("- パタン本文は「文脈」「問題」「フォース」「解決」「結果として生じる文脈」を中心に構成する。\n")
	b.WriteString("- レビューエージェントが使うときの観察ポイント、誤用、例外、よくある誤検知、関連パタンを必要に応じて加える。\n")
	b.WriteString("- コミットされるパタンファイルは、抽象的で再利用可能なレビュー知識として書く。\n")
	b.WriteString("- 生のソースコード、長いdiff hunk、長いレビューコメントをパタンファイルへコピーしない。\n")
	b.WriteString("- 個人を責める表現、不要な個人名、不要なプロプライエタリ情報を残さない。\n")
	b.WriteString("- 「必ず」「常に」のような断定は、フォースや例外を説明できる場合だけ使う。\n\n")
	writeCorpusReadingProcess(b)
	if reviewerPatterns {
		writeReviewerPatternMiningProcess(b, reviewerAuthor)
		writeReviewerAcceptanceCriteria(b)
	} else {
		writePatternMiningProcess(b)
		writeAcceptanceCriteria(b)
	}
	writePatternWritingInstructions(b, reviewerPatterns)
	writeSelfShepherdingInstructions(b)
}

func writeCorpusReadingProcess(b *strings.Builder) {
	b.WriteString("## JSONLの読み方\n\n")
	b.WriteString("- JSONLを全件読み、`pr_number`、`review_id`、`in_reply_to_id` を使ってPull Request内の議論と返信関係を復元する。\n")
	b.WriteString("- `review_comment` と意味のある `review_summary` を主証拠にし、`review_comment_reply` は指摘の受理、誤解、スコープ外判断を読む補助証拠として扱う。\n")
	b.WriteString("- 感謝、単なる賛同、対応完了だけの返信、空に近いapproveは、パタン候補の主証拠にしない。\n")
	b.WriteString("- `path`、`language`、`diff_hunk`、Pull Requestタイトルは文脈理解に使う。ただし、長いコードやコメント本文をパタンファイルへ残さない。\n\n")
}

func writePatternMiningProcess(b *strings.Builder) {
	b.WriteString("## パタン候補の見つけ方\n\n")
	b.WriteString("各レビュー指摘を、まず観察カードとして短く分解する。\n\n")
	b.WriteString("- 文脈: どの種類の変更、コード、API、UI、テスト、運用で起きたか。\n")
	b.WriteString("- 表面上の指摘: レビュアーが直接求めた修正。\n")
	b.WriteString("- 背後の問題: そのままだと何が悪くなるか。\n")
	b.WriteString("- フォース: なぜ単純に解けないか。互換性、明瞭さ、性能、セキュリティ、API一貫性、既存設計、ユーザー体験、テスト容易性などの緊張関係を書く。\n")
	b.WriteString("- 解決の核: レビューコメント文面ではなく、再利用可能な設計判断として言い換える。\n")
	b.WriteString("- 証拠: PR番号、コメント種別、関連パスを短く記録する。生コメント本文は残さない。\n\n")
	b.WriteString("その後、観察カードをファイル名や単語一致ではなく、同じ問題とフォースを共有するもの同士でクラスタリングする。\n\n")
}

func writeReviewerPatternMiningProcess(b *strings.Builder, reviewerAuthor string) {
	fmt.Fprintf(b, "## パタン候補の見つけ方\n\nこの入力コーパスは author=%q の単一レビュワーに絞り込まれている前提で読む。\n\n", reviewerAuthor)
	b.WriteString(`各レビュー指摘を、まず「思考の癖」を読むための観察カードとして短く分解する。

- 文脈: どの種類の変更、コード、API、UI、テスト、運用で起きたか。
- 表面上の指摘: レビュアーが直接求めた修正。
- 着眼点: レビュアーが何に最初に反応しているか。命名、責務境界、データ整合性、例外系、将来変更、利用者視点、運用影響、テスト容易性など。
- 問題の立て方: そのレビュアーが「何を問題だとみなしたか」。バグそのものではなく、曖昧さ、責務の混在、前提の隠れ、境界条件の未整理、読み手の誤解、変更耐性の低さなどとして読む。
- 背後の懸念: そのままだと何が悪くなると見ているか。将来の変更ミス、仕様理解のずれ、利用者体験の劣化、データ不整合、運用時の調査困難、レビュー不能性など。
- 重視しているフォース: そのレビュアーがどの価値の衝突を調停しようとしているか。明瞭さと簡潔さ、局所修正と設計一貫性、柔軟性と制約、実装速度と将来保守、ユーザー都合と内部都合など。
- 推論の型: どのような考え方で指摘しているか。境界条件から考える、責務の置き場所を問う、利用者の誤解から逆算する、既存設計との対称性を見る、将来の変更経路を先読みする、テスト不能な暗黙前提を嫌う、など。
- 介入の粒度: そのレビュアーがどの粒度で直そうとしているか。文言修正、条件分岐、メソッド分割、責務移動、API設計、テスト追加、仕様確認など。
- 解決の核: レビューコメント文面ではなく、そのレビュアーが繰り返し使っている再利用可能な判断として言い換える。
- 証拠: PR番号、コメント種別、関連パスを短く記録する。生コメント本文は残さない。

その後、観察カードをファイル名、技術要素、単語一致ではなく、同じ「着眼点」「問題の立て方」「重視するフォース」「推論の型」を共有するもの同士でクラスタリングする。

クラスタ名は、人格評価や性格診断のように書かず、レビュー時に再利用できる思考パタンとして命名する。たとえば「心配性」ではなく「境界条件から仕様を固める」、「細かい」ではなく「読み手の誤解を先回りして命名を整える」のように書く。

パタン化するときは、その人が何を好むかではなく、どの文脈で、どのような兆候を見て、どのフォースを重視し、どの方向に判断を寄せる傾向があるかを抽出する。

`)
}

func writeAcceptanceCriteria(b *strings.Builder) {
	b.WriteString("## 採用基準\n\n")
	b.WriteString("新規または更新対象のパタン候補は、次のいずれかを満たす場合だけ採用する。\n\n")
	b.WriteString("- 複数PRまたは複数箇所で繰り返し現れている。\n")
	b.WriteString("- 例は少なくても、このリポジトリのレビュー文化として強い判断基準が読み取れる。\n")
	b.WriteString("- レビューエージェントが将来の差分で観察できる兆候がある。\n")
	b.WriteString("- 解決が単なるスタイル修正ではなく、フォースを調停している。\n\n")
	b.WriteString("次の候補は採用しない。\n\n")
	b.WriteString("- 一回限りの文脈依存判断。\n")
	b.WriteString("- 個人の好みだけに見えるもの。\n")
	b.WriteString("- 一般的すぎてこのリポジトリ固有の判断になっていないもの。\n")
	b.WriteString("- 生のコードやレビューコメントを残さないと意味が通らないもの。\n")
	b.WriteString("- 信頼度が低いのに硬いルールにしないと成立しないもの。\n\n")
}

func writeReviewerAcceptanceCriteria(b *strings.Builder) {
	b.WriteString(`## 採用基準

新規または更新対象のパタン候補は、次のいずれかを満たす場合だけ採用する。

- 同じレビュアーが、複数PRまたは複数箇所で、同じ着眼点や問題の立て方を繰り返している。
- 表面的な修正内容は異なっていても、背後にあるフォースの捉え方や調停の方向が共通している。
- そのレビュアーがレビュー時に優先しやすい価値判断として読み取れる。たとえば、明瞭さ、責務分離、既存設計との整合、境界条件、将来変更、利用者視点、運用容易性など。
- 将来の差分に対して、レビューエージェントが「このレビュアーなら反応しそうな兆候」として観察できる。
- 単なるコードスタイルではなく、判断の背景にある問題意識、フォース、例外条件を説明できる。
- 出現回数は少なくても、コメントの密度が高く、そのレビュアーの特徴的な思考手順が明確に読める。
- 既存パタンに吸収できる場合は新規作成せず、既存パタンの文脈、フォース、誤用、信頼度を更新する。

次の候補は採用しない。

- 一回限りの文脈依存判断で、そのレビュアー固有の再利用可能な着眼点として読めないもの。
- 単なる表記ゆれ、文体、命名の好みだけに見えるもの。
- リポジトリやチームの規約を述べているだけで、そのレビュアーの思考の癖としては区別できないもの。
- 一般的すぎて、どのレビュアーにも当てはまるコードレビュー原則にしかなっていないもの。
- 生のコードやレビューコメントを残さないと意味が通らないもの。
- 人格評価、能力評価、性格診断、感情の推測になってしまうもの。
- コメントの口調や強さだけを根拠にしたもの。
- 信頼度が低いのに硬いルールにしないと成立しないもの。
- 将来の差分から観察できる兆候がなく、レビューエージェントが実用できないもの。

採用時は、パタンの信頼度を次の観点で明示する。

- high: 複数の文脈で繰り返し現れ、同じ着眼点、フォース、解決方向が安定している。
- medium: 繰り返しは限定的だが、コメント内容から特徴的な判断軸が読み取れる。
- low: 有望な仮説ではあるが、出典が少ない、または文脈依存の可能性が残る。low の候補は硬いルールとして書かず、仮説的な観察パタンとして扱う。

`)
}

func writePatternWritingInstructions(b *strings.Builder, reviewerPatterns bool) {
	b.WriteString("## 書き方\n\n")
	b.WriteString("各パタンは `P###-slug.md` として作成または更新し、`catalog.yaml` に登録する。Markdownは次の見出しを使う。\n\n")
	b.WriteString("- 要約\n")
	b.WriteString("- 文脈\n")
	b.WriteString("- 問題\n")
	b.WriteString("- フォース\n")
	b.WriteString("- 解決\n")
	b.WriteString("- 結果として生じる文脈\n")
	b.WriteString("- レビューでの使い方\n")
	b.WriteString("- 具体化の方向\n")
	b.WriteString("- 誤用と例外\n")
	b.WriteString("- 信頼度\n")
	b.WriteString("- 出典メモ\n")
	b.WriteString("- 関連パタン\n")
	b.WriteString("- 変更履歴\n\n")
	b.WriteString("特に `フォース` を厚く書く。パタンは「この場合はこう直す」というレシピではなく、「この文脈では、これらの力が衝突するので、この中心的判断で調停する」という形にする。\n\n")
	if reviewerPatterns {
		b.WriteString("パタンは「このリポジトリではこうすべき」という規約集ではなく、「このレビュアーはどのような兆候を問題化し、どのフォースを重視して判断する傾向があるか」を、将来のレビュー支援に使える形で書く。\n\n")
	}
}

func writeSelfShepherdingInstructions(b *strings.Builder) {
	b.WriteString("## 自己シェパーディング\n\n")
	b.WriteString("パタンファイルを書き出す前に、各候補を次の観点で見直す。\n\n")
	b.WriteString("- これはチェックリストではなく、文脈、問題、フォース、解決の関係を持つパタンになっているか。\n")
	b.WriteString("- 解決が表面的な修正手順ではなく、フォースを調停する中心的判断として書かれているか。\n")
	b.WriteString("- 適用しない条件、例外、誤検知しやすい条件があるか。\n")
	b.WriteString("- 既存パタンと重複していないか。差分更新では、既存パタンに吸収できる候補を新規作成しない。\n")
	b.WriteString("- 生のコメント、長いdiff、個人名、不要な固有情報を残していないか。\n")
	b.WriteString("- 信頼度が低い候補を硬いルールや `high` として扱っていないか。\n\n")
}

func writePatternReferences(b *strings.Builder, patternsDir string, patterns []patternFile) {
	b.WriteString("## 既存パタンファイル\n\n")
	b.WriteString("以下の既存パタンファイルを読んでから差分更新してください。プロンプト本文には既存パタン本文を埋め込んでいません。\n\n")
	var paths []string
	for _, file := range patterns {
		paths = append(paths, filepath.ToSlash(filepath.Join(patternsDir, filepath.FromSlash(file.Path))))
	}
	b.WriteString(markdownFence(strings.Join(paths, "\n"), "text"))
	b.WriteString("\n")
	b.WriteString("既存パタンの内容は根拠として読みますが、更新後のパタンファイルにも生のコメントや生のコードを不要に残さないでください。\n\n")
}

func writeCorpusSummary(b *strings.Builder, stats corpusStats, reviewerAuthor string) {
	b.WriteString("## コーパス概要\n\n")
	fmt.Fprintf(b, "- レコード数: %d\n", stats.Total)
	if reviewerAuthor != "" {
		fmt.Fprintf(b, "- 対象レビュワー: %s\n", reviewerAuthor)
	}
	writeCountList(b, "リポジトリ", stats.Repos, 8)
	writeCountList(b, "コメント種別", stats.CommentTypes, 8)
	writeCountList(b, "主な対象パス", stats.Paths, 12)
	b.WriteString("\n")
}

func writeCorpusReference(b *strings.Builder, path string) {
	b.WriteString("## 入力コーパスファイル\n\n")
	b.WriteString("以下のJSONLファイルを今回の入力コーパスとして読んでください。プロンプト本文にはJSONL本文を埋め込んでいません。\n\n")
	b.WriteString(markdownFence(path, "text"))
	b.WriteString("\n")
	b.WriteString("パタン更新の根拠として読み、コミットされるパタンファイルには生のコメントや生のコードを不要に残さないでください。\n")
}

func writeCountList(b *strings.Builder, label string, counts map[string]int, limit int) {
	if len(counts) == 0 {
		fmt.Fprintf(b, "- %s: なし\n", label)
		return
	}
	items := sortedCountItems(counts)
	values := make([]string, 0, min(len(items), limit))
	for i, item := range items {
		if i >= limit {
			break
		}
		values = append(values, fmt.Sprintf("%s (%d)", item.Value, item.Count))
	}
	if len(items) > limit {
		values = append(values, fmt.Sprintf("ほか%d件", len(items)-limit))
	}
	fmt.Fprintf(b, "- %s: %s\n", label, strings.Join(values, ", "))
}

type countItem struct {
	Value string
	Count int
}

func sortedCountItems(counts map[string]int) []countItem {
	items := make([]countItem, 0, len(counts))
	for value, count := range counts {
		items = append(items, countItem{Value: value, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Value < items[j].Value
	})
	return items
}

func markdownFence(content, info string) string {
	fence := strings.Repeat("`", longestBacktickRun(content)+1)
	if len(fence) < 3 {
		fence = "```"
	}
	if info != "" {
		return fence + info + "\n" + content + ensureTrailingNewline(content) + fence + "\n"
	}
	return fence + "\n" + content + ensureTrailingNewline(content) + fence + "\n"
}

func longestBacktickRun(content string) int {
	longest := 0
	current := 0
	for _, r := range content {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	return longest
}

func ensureTrailingNewline(content string) string {
	if content == "" || strings.HasSuffix(content, "\n") {
		return ""
	}
	return "\n"
}

func progressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}
