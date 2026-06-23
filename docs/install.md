# インストールガイド

このガイドは、`review-patterns` CLIを導入し、マージ済みPull RequestからレビューコーパスJSONLとパタン更新用プロンプトを生成するための手順です。

MVPではAIエージェントを直接実行しません。CLIはJSONLとプロンプトを生成し、人間が任意のAIコーディングエージェントにプロンプトを渡して `.review-patterns/patterns/` を更新します。

## 前提

- Go 1.26以降。
- Pull Request、review comment、必要に応じてissue commentを読むためのGitHub権限。
- GitHub tokenを `GITHUB_TOKEN` または `GH_TOKEN` に設定するか、GitHub CLIで認証していること。

GitHub CLIを使う場合:

```sh
gh auth login
```

## インストール

タグつきリリース後は、固定バージョンを指定してインストールします。

```sh
go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@v0.1.0
```

公開前の検証では次を使えます。

```sh
go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@main
```

インストール先の `$(go env GOPATH)/bin` が `PATH` に入っていることを確認してください。

```sh
review-patterns --help
```

## ソースツリーから実行する

リポジトリをcloneして検証する場合は、インストールせずに実行できます。

```sh
go run ./cmd/review-patterns --help
```

## 最小実行

対象リポジトリと期間を指定してJSONLを生成します。

```sh
review-patterns collect \
  --repo owner/repo \
  --since 2026-06-21T00:00:00Z \
  --until 2026-06-22T00:00:00Z \
  --output tmp/reviews.jsonl
```

`since` と `until` を両方省略すると、前日UTCの24時間が対象になります。

```text
since <= pull_request.merged_at < until
```

`--repo` を省略した場合は、現在のディレクトリで `gh repo view` を実行して `owner/repo` を推測します。推測できない環境では `--repo owner/repo` を明示してください。

## 生成される成果物

`collect` はJSONLを標準出力、または `--output` で指定したファイルへ書きます。

JSONLには生のレビューコメント、diff hunk、author、Pull Requestの文脈が含まれます。デフォルトではリポジトリにコミットされません。このリポジトリの `.gitignore` は `.review-patterns/corpus/` と `*.jsonl` を無視します。

通常は次のように、コミットされない一時領域に保存してください。

```sh
mkdir -p tmp

review-patterns collect \
  --repo owner/repo \
  --output tmp/reviews.jsonl
```

## パタンファイルの初回作成

初回は `.review-patterns/patterns/` が空、または存在しなくても `prompt` を実行できます。その場合、初回抽出用のプロンプトを生成します。

```sh
review-patterns prompt \
  --input tmp/reviews.jsonl \
  --patterns-dir .review-patterns/patterns \
  --output tmp/prompt.md \
  --mode auto
```

生成プロンプトを任意のAIコーディングエージェントに渡し、以下のようなファイルを作成します。

```text
.review-patterns/
  patterns/
    catalog.yaml
    P001-example.md
```

パタンファイルには、生のレビューコメントや長いコード断片を保存せず、文脈、問題、フォース、解決、結果として生じる文脈を中心に抽象化して書きます。

特定レビュワーの思考の癖に特化したパタンランゲージを作る場合は、先に `filter` でauthorを1人に絞り込んでから `--reviewer-patterns` を指定します。

```sh
review-patterns filter \
  --input tmp/reviews.jsonl \
  --output tmp/alice-reviews.jsonl \
  --author alice

review-patterns prompt \
  --input tmp/alice-reviews.jsonl \
  --patterns-dir .review-patterns/patterns \
  --output tmp/alice-prompt.md \
  --mode auto \
  --reviewer-patterns
```

`--reviewer-patterns` は入力JSONLの全レコードの `author` が同じであることを検証します。異なるauthor、欠落、`null`、空文字が含まれる場合はエラーになります。

## 既存パタンの更新

`.review-patterns/patterns/` に既存の `.md`、`.yaml`、`.yml` ファイルがある場合、`prompt` は差分更新用のプロンプトを生成します。

人間は生成されたプロンプトを確認し、任意のAIエージェントへ渡します。AIエージェントが作成した変更は、通常のコード変更と同じようにPull Requestでレビューしてからコミットしてください。

## issue commentも収集する

Pull Request conversation issue commentを収集したい場合は、`--include-issue-comments` を指定します。

```sh
review-patterns collect \
  --repo owner/repo \
  --include-issue-comments \
  --output tmp/reviews.jsonl
```

issue commentにはレビュー以外の議論も含まれやすいため、必要なリポジトリだけで有効化してください。

## 特定範囲へ絞り込む

特定パスやauthorのコメントだけに絞る場合は `filter` を使います。

```sh
review-patterns filter \
  --input tmp/reviews.jsonl \
  --output tmp/app-reviews.jsonl \
  --path /app/controllers \
  --author alice
```

`--path` と `--author` を同時に指定した場合は、両方を満たす行だけが出力されます。

## 自動実行

CLIは標準入力や専用サービスに依存しません。定期実行したい場合は、cron、任意のCI、または社内ジョブ基盤から次の2段階を呼び出してください。

```sh
review-patterns collect --repo owner/repo --output tmp/reviews.jsonl
review-patterns prompt --input tmp/reviews.jsonl --patterns-dir .review-patterns/patterns --output tmp/prompt.md
```

生成された `tmp/prompt.md` を人間が確認し、任意のAIコーディングエージェントへ渡します。MVPではAIエージェントの実行、パタン変更のコミット、Pull Request作成は行いません。
