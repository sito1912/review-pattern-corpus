# review-pattern-corpus

`review-pattern-corpus` は、マージ済みPull Requestに残された人間のコードレビューを収集し、コードレビューエージェントが参照できる保守可能なパタンランゲージへ育てていくためのCLIツールです。

このプロジェクトは、特定のAIエージェントやAIモデルに依存しない設計にします。Codex、Claude Code、その他のコーディングエージェントそのものを実装するのではなく、それらが参照できるレビュー用パタンランゲージを運用・保守することを目的とします。

## ステータス

このリポジトリはOSS公開準備中です。タグつきリリースはまだ公開されていません。

リリース前に検証する場合は `@main` を指定してインストールできます。通常利用ではタグつきリリース公開後に `@v0.1.0` のような固定バージョンを指定してください。

## 目的

- リポジトリ内のマージ済みPull Requestからレビューコメントを収集する。
- 収集したレビューデータを、後続分析に使えるJSONLとして保存する。
- AIコーディングエージェントがコードレビュー用パタンランゲージを抽出・更新するためのプロンプトを生成する。
- 生成されたパタンランゲージをリポジトリ内のファイルとして保持し、レビューエージェントから参照できるようにする。
- 任意のローカルスクリプト、cron、CIからCLIを実行し、パタンランゲージを継続的にメンテナンスできるようにする。

## やらないこと

- MVP内で特定のAIモデルを実行する。
- MVP内で生成されたパタン更新を自動コミットする。
- CodexやClaude Codeなどに密結合したSkill実装を作る。
- レビューコーパスをデフォルトでリポジトリに保存する。
- 特定のCI/CDサービス専用のラッパーを提供する。

## インストール

タグつきリリース後は、Goの標準的なCLIインストールとして取得します。

```sh
go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@v0.1.0
```

公開前の検証では次を使えます。

```sh
go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@main
```

ソースツリーから実行する場合:

```sh
go run ./cmd/review-patterns --help
```

開発対象のGoバージョンは、初期設定時点の最新安定系列であるGo 1.26です。

## 基本ワークフロー

1. `review-patterns collect` で対象期間のレビューコーパスJSONLを生成する。
2. 必要に応じて `review-patterns filter` で、特定パス以下または特定authorのレビューコメントだけを抽出する。
3. `review-patterns prompt` で、今回のJSONLと既存の `.review-patterns/patterns/` から更新用プロンプトを生成する。
4. 人間が任意のAIコーディングエージェントに生成プロンプトを渡して実行する。
5. AIエージェントが `.review-patterns/patterns/catalog.yaml` とパタンMarkdownを更新する。
6. パタンランゲージの変更を通常のコード変更と同じようにレビューし、コミットする。

## デフォルトの対象期間

`since` と `until` が省略された場合、収集対象は前日UTCになります。

```text
since = 前日 00:00:00 UTC
until = 当日 00:00:00 UTC
```

Pull Requestの選択条件は以下です。

```text
since <= pull_request.merged_at < until
```

## CLI

```text
review-patterns collect
review-patterns filter
review-patterns prompt
```

### `review-patterns collect`

マージ済みPull Requestから人間によるレビューコメントとレビューサマリーを収集し、JSONLを出力します。

```sh
review-patterns collect \
  --repo owner/repo \
  --since 2026-06-21T00:00:00Z \
  --until 2026-06-22T00:00:00Z \
  --output tmp/reviews.jsonl
```

GitHub tokenは `GITHUB_TOKEN` または `GH_TOKEN` から読みます。必要に応じて `--token` でも指定できます。環境変数や `--token` がなければ、`gh auth token` を使ってGitHub CLIの認証情報を参照します。

`--repo` を省略した場合は、現在のディレクトリで `gh repo view` を実行して `owner/repo` を推測します。事前に `gh auth login` で対象アカウントを認証してください。

`--since` と `--until` は両方指定するか、両方省略してください。両方省略した場合は前日UTCの24時間を収集します。`--output -` または未指定の場合は標準出力へJSONLを書きます。

実行中は検索中の期間、検索済みPull Request候補数、見つかったPull Request数、Pull Requestごとの収集状況、書き込み先を標準エラーへ表示します。JSONLを標準出力に出す場合でも、進捗表示はJSONLに混ざりません。

レート制限を受けた場合、CLIは自動リトライせずに非ゼロ終了し、GitHub APIが返すリセット時刻または `Retry-After` をエラーメッセージに含めます。GitHub APIから30秒間応答がない場合も、無期限に待たずにエラーにします。

### `review-patterns filter`

収集済みJSONLから、指定したパスやauthorに関係するコメントだけを抽出して新しいJSONLを作成します。マッチした行は再エンコードせず、入力JSONLの1行をそのまま出力します。

```sh
review-patterns filter \
  --input tmp/reviews.jsonl \
  --output tmp/app-reviews.jsonl \
  --path /app/controllers \
  --author alice
```

`--path` は各JSONオブジェクトの `path` 値に対して検索します。先頭の `/` は任意で、`/app` は `app/...` や `packages/backend/app/...` のようにパス区切り単位で `app` を含むファイルに一致します。`/app/controllers` を指定すると、その配下のファイルに対するレビューコメントだけを出力します。`path` がないレビューサマリーやissue commentは出力しません。

`--author` は各JSONオブジェクトの `author` 値に対して完全一致で検索します。`--path` と `--author` を同時に指定した場合は積集合になり、両方の条件を満たす行だけを出力します。少なくとも `--path` または `--author` のどちらかを指定してください。

### `review-patterns prompt`

収集済みJSONLと既存のパタンファイルから、人間が任意のAIコーディングエージェントに渡すためのプロンプトを生成します。MVPではAIエージェント自体は実行しません。

```sh
review-patterns prompt \
  --input tmp/reviews.jsonl \
  --patterns-dir .review-patterns/patterns \
  --output tmp/prompt.md \
  --mode auto
```

`--mode auto` は、`--patterns-dir` に既存の `.md`、`.yaml`、`.yml` ファイルがある場合は差分更新用のプロンプトを、ない場合は初回抽出用のプロンプトを生成します。明示的に `--mode extract` または `--mode update` も指定できます。`--output -` または未指定の場合は標準出力へMarkdownを書きます。

生成プロンプトは、レビューJSONLファイルと既存パタンファイルのパスを根拠データとして示しつつ、Pull Request内の議論を復元し、個々の指摘を文脈、問題、フォース、解決の核へ分解してからパタン候補を抽出するようAIエージェントへ指示します。コミットされるパタンファイルには生のソースコード、長いdiff hunk、長いレビューコメント、不要な個人情報を残さないよう指示します。JSONL本文と既存パタン本文はプロンプト本文に埋め込みません。

## 保存方針

JSONLは生のレビューコメント、diff hunk、author、Pull Requestの文脈を含みます。デフォルトでは標準出力へ出し、ファイル保存は `--output` で明示します。

このリポジトリの `.gitignore` は `.review-patterns/corpus/` と `*.jsonl` を無視します。生のレビューコーパスをコミットする場合は、内容を確認し、意図的に除外設定を調整してください。通常は `tmp/` やCIの一時領域など、コミットされない場所への保存を推奨します。

パタンランゲージファイルは `.review-patterns/patterns/` 配下にコミットします。パタンファイルでは生データではなく、抽象化されたレビュー知識を保存してください。

## ドキュメント

- [インストールガイド](docs/install.md)
- [セキュリティとプライバシー](docs/security-and-privacy.md)
- [リリース手順](docs/release.md)
- [プロダクト要件](docs/product-requirements.md)
- [データとパタン形式](docs/data-and-pattern-formats.md)
- [マイルストーン](docs/milestones.md)
- [コントリビューションガイド](CONTRIBUTING.md)
- [セキュリティポリシー](SECURITY.md)

## ライセンス

MIT
