# インストールガイド

このガイドは、別リポジトリに `review-pattern-corpus` のComposite Actionを導入し、マージ済みPull RequestからレビューコーパスJSONLとパタン更新用プロンプトを生成するための手順です。

MVPではAIエージェントを直接実行しません。ActionはJSONLとプロンプトを生成し、人間が任意のAIコーディングエージェントにプロンプトを渡して `.review-patterns/patterns/` を更新します。

## 前提

- GitHub Actionsが利用できるリポジトリ。
- Pull Request、review comment、必要に応じてissue commentを読むための権限。
- Actionを固定タグで参照できる `review-pattern-corpus` のリリース。公開後は `@v0` または `@v0.1.0` のようなタグを使います。

## 最小workflow

次の内容を利用リポジトリの `.github/workflows/review-pattern-corpus.yml` に置きます。

```yaml
name: Collect review pattern corpus

on:
  workflow_dispatch:
    inputs:
      since:
        description: "Inclusive UTC start timestamp, for example 2026-06-21T00:00:00Z"
        required: false
      until:
        description: "Exclusive UTC end timestamp, for example 2026-06-22T00:00:00Z"
        required: false
  schedule:
    - cron: "17 1 * * *"

jobs:
  collect:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      issues: read
    steps:
      - uses: actions/checkout@v4
      - uses: sito1912/review-pattern-corpus@v0
        with:
          since: ${{ github.event.inputs.since }}
          until: ${{ github.event.inputs.until }}
          retention-days: "30"
```

`since` と `until` を両方省略すると、前日UTCの24時間が対象になります。

```text
since <= pull_request.merged_at < until
```

## 生成される成果物

デフォルトでは、JSONLコーパスと生成プロンプトはGitHub Actions Artifactとして保存されます。

- `review-patterns-corpus-YYYY-MM-DD`: 収集されたJSONL。
- `review-patterns-prompt-YYYY-MM-DD`: AIエージェントに渡すMarkdownプロンプト。

JSONLには生のレビューコメント、diff hunk、author、Pull Requestの文脈が含まれます。デフォルトではリポジトリにコミットされません。

## パタンファイルの初回作成

初回は `.review-patterns/patterns/` が空、または存在しなくてもActionを実行できます。その場合、`prompt` は初回抽出用のプロンプトを生成します。

生成プロンプトを任意のAIコーディングエージェントに渡し、以下のようなファイルを作成します。

```text
.review-patterns/
  patterns/
    catalog.yaml
    P001-example.md
```

パタンファイルには、生のレビューコメントや長いコード断片を保存せず、文脈、問題、フォース、解決、結果として生じる文脈を中心に抽象化して書きます。

## 既存パタンの更新

`.review-patterns/patterns/` に既存の `.md`、`.yaml`、`.yml` ファイルがある場合、`prompt` は差分更新用のプロンプトを生成します。

人間は生成されたプロンプトを確認し、任意のAIエージェントへ渡します。AIエージェントが作成した変更は、通常のコード変更と同じようにPull Requestでレビューしてからコミットしてください。

## issue commentも収集する

Pull Request conversation issue commentを収集したい場合は、次の入力を追加します。

```yaml
      - uses: sito1912/review-pattern-corpus@v0
        with:
          include-issue-comments: "true"
```

issue commentにはレビュー以外の議論も含まれやすいため、必要なリポジトリだけで有効化してください。

## JSONLをリポジトリに保存する

JSONLをArtifactではなくリポジトリ内に書きたい場合は、明示的に `storage: repo` を指定します。

```yaml
      - uses: sito1912/review-pattern-corpus@v0
        with:
          storage: repo
```

この場合、JSONLは `.review-patterns/corpus/reviews-YYYY-MM-DD.jsonl` に出力されます。Actionは自動コミットしません。保存する場合は、利用リポジトリ側のworkflowで内容を確認し、コミット処理を明示してください。

デフォルトでは生のレビューコーパスをコミットしない運用を推奨します。

## 入力一覧

| 入力 | 既定値 | 説明 |
| --- | --- | --- |
| `since` | 空 | UTC対象期間の開始。`until` とセットで指定します。 |
| `until` | 空 | UTC対象期間の終了。`since` とセットで指定します。 |
| `include-issue-comments` | `false` | Pull Request conversation issue commentを収集します。 |
| `storage` | `artifact` | JSONL保存先。`artifact` または `repo`。 |
| `retention-days` | `30` | Artifact保持日数。 |
| `redact` | `false` | 予約入力。MVPでは `true` を指定すると失敗します。 |
| `anonymize` | `false` | 予約入力。MVPでは `true` を指定すると失敗します。 |
| `github-token` | `github.token` | GitHub APIを読むためのtoken。通常は指定不要です。 |

## ローカルで確認する

CLIをローカルで実行する場合は、GitHub tokenを `GITHUB_TOKEN` または `GH_TOKEN` に設定するか、GitHub CLIで認証します。

```sh
go run ./cmd/review-patterns collect \
  --repo owner/repo \
  --since 2026-06-21T00:00:00Z \
  --until 2026-06-22T00:00:00Z \
  --output reviews.jsonl

go run ./cmd/review-patterns prompt \
  --input reviews.jsonl \
  --patterns-dir .review-patterns/patterns \
  --output prompt.md \
  --mode auto
```

特定パスのコメントだけに絞る場合は `filter` を使います。

```sh
go run ./cmd/review-patterns filter \
  --input reviews.jsonl \
  --output app-reviews.jsonl \
  --path /app/controllers
```
