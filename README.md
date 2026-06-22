# review-pattern-corpus

`review-pattern-corpus` は、マージ済みPull Requestに残された人間のコードレビューを収集し、コードレビューエージェントが参照できる保守可能なパタンランゲージへ育てていくためのGitHub Actionsプロダクトです。

このプロジェクトは、特定のAIエージェントやAIモデルに依存しない設計にします。Codex、Claude Code、その他のコーディングエージェントそのものを実装するのではなく、それらが参照できるレビュー用パタンランゲージを運用・保守することを目的とします。

## 目的

- リポジトリ内のマージ済みPull Requestからレビューコメントを収集する。
- 収集したレビューデータを、後続分析に使えるJSONLとして保存する。
- AIコーディングエージェントがコードレビュー用パタンランゲージを抽出・更新するためのプロンプトを生成する。
- 生成されたパタンランゲージをリポジトリ内のファイルとして保持し、レビューエージェントから参照できるようにする。
- 収集とプロンプト生成を定期実行し、パタンランゲージを継続的にメンテナンスできるようにする。

## やらないこと

- MVP内で特定のAIモデルを実行する。
- MVP内で生成されたパタン更新を自動コミットする。
- CodexやClaude Codeなどに密結合したSkill実装を作る。
- レビューコーパスをデフォルトでリポジトリに保存する。

## プロダクト構成

想定する構成は、小さなGo製CLIを薄いComposite Actionで包む形です。

```text
review-patterns collect
review-patterns filter
review-patterns prompt
review-patterns validate
```

CLI名は `review-patterns` です。

Go module名は以下です。

```text
github.com/sito1912/review-pattern-corpus
```

開発対象のGoバージョンは、初期設定時点の最新安定系列であるGo 1.26です。

## デフォルトのワークフロー

1. 利用リポジトリにGitHub Actionを導入する。
2. ActionがUTCの対象期間に対して `review-patterns collect` を実行する。
3. 収集されたJSONLは、デフォルトではGitHub Actions Artifactとしてアップロードされる。
4. 必要に応じて `review-patterns filter` で、特定パス以下のレビューコメントだけを抽出する。
5. Actionが今回のJSONLと既存の `.review-patterns/patterns/` を使って `review-patterns prompt` を実行する。
6. 生成されたプロンプトはArtifactとしてアップロードされる。
7. 人間が任意のAIコーディングエージェントに生成プロンプトを渡して実行する。
8. AIエージェントが `.review-patterns/patterns/catalog.yaml` とパタンMarkdownを更新する。
9. パタンランゲージの変更を通常のコード変更と同じようにレビューし、コミットする。

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

### `review-patterns collect`

マージ済みPull Requestから人間によるレビューコメントとレビューサマリーを収集し、JSONLを出力します。

```sh
go run ./cmd/review-patterns collect \
  --repo owner/repo \
  --since 2026-06-21T00:00:00Z \
  --until 2026-06-22T00:00:00Z \
  --output reviews.jsonl
```

GitHub tokenは `GITHUB_TOKEN` または `GH_TOKEN` から読みます。必要に応じて `--token` でも指定できます。`--repo` を省略した場合は、GitHub Actions標準の `GITHUB_REPOSITORY` を使います。

ローカルPCで実行する場合、環境変数や `--token` がなければ `gh auth token` を使ってGitHub CLIの認証情報を参照します。事前に `gh auth login` で対象アカウントを認証してください。

`--since` と `--until` は両方指定するか、両方省略してください。両方省略した場合は前日UTCの24時間を収集します。`--output -` または未指定の場合は標準出力へJSONLを書きます。

実行中は検索中の期間、検索済みPull Request候補数、見つかったPull Request数、Pull Requestごとの収集状況、書き込み先を標準エラーへ表示します。JSONLを標準出力に出す場合でも、進捗表示はJSONLに混ざりません。

レート制限を受けた場合、CLIは自動リトライせずに非ゼロ終了し、GitHub APIが返すリセット時刻または `Retry-After` をエラーメッセージに含めます。GitHub APIから30秒間応答がない場合も、無期限に待たずにエラーにします。

### `review-patterns filter`

収集済みJSONLから、指定したパスに関係するコメントだけを抽出して新しいJSONLを作成します。マッチした行は再エンコードせず、入力JSONLの1行をそのまま出力します。

```sh
go run ./cmd/review-patterns filter \
  --input reviews.jsonl \
  --output app-reviews.jsonl \
  --path /app/controllers
```

`--path` は各JSONオブジェクトの `path` 値に対して検索します。先頭の `/` は任意で、`/app` は `app/...` や `packages/backend/app/...` のようにパス区切り単位で `app` を含むファイルに一致します。`/app/controllers` を指定すると、その配下のファイルに対するレビューコメントだけを出力します。`path` がないレビューサマリーやissue commentは出力しません。

### `review-patterns prompt`

収集済みJSONLと既存のパタンファイルから、人間が任意のAIコーディングエージェントに渡すためのプロンプトを生成します。MVPではAIエージェント自体は実行しません。

```sh
go run ./cmd/review-patterns prompt \
  --input reviews.jsonl \
  --patterns-dir .review-patterns/patterns \
  --output prompt.md \
  --mode auto
```

`--mode auto` は、`--patterns-dir` に既存の `.md`、`.yaml`、`.yml` ファイルがある場合は差分更新用のプロンプトを、ない場合は初回抽出用のプロンプトを生成します。明示的に `--mode extract` または `--mode update` も指定できます。`--output -` または未指定の場合は標準出力へMarkdownを書きます。

生成プロンプトは、レビューJSONLファイルと既存パタンファイルのパスを根拠データとして示しつつ、Pull Request内の議論を復元し、個々の指摘を文脈、問題、フォース、解決の核へ分解してからパタン候補を抽出するようAIエージェントへ指示します。コミットされるパタンファイルには生のソースコード、長いdiff hunk、長いレビューコメント、不要な個人情報を残さないよう指示します。JSONL本文と既存パタン本文はプロンプト本文に埋め込みません。

## GitHub Action

Composite Actionとして、GitHub Actions上で `review-patterns collect` と `review-patterns prompt` を実行できます。`since` と `until` を省略した場合は、前日UTCの24時間を収集します。

最小構成の手動実行workflow例:

```yaml
name: Collect review pattern corpus

on:
  workflow_dispatch:
    inputs:
      since:
        description: "Inclusive UTC start timestamp"
        required: false
      until:
        description: "Exclusive UTC end timestamp"
        required: false

jobs:
  collect:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      issues: read
    steps:
      - uses: actions/checkout@v4
      - uses: sito1912/review-pattern-corpus@main
        with:
          since: ${{ inputs.since }}
          until: ${{ inputs.until }}
```

デフォルトでは、収集されたJSONLは `review-patterns-corpus-YYYY-MM-DD` というArtifactとしてアップロードされます。生成されたプロンプトは `review-patterns-prompt-YYYY-MM-DD` というArtifactとしてアップロードされます。Artifactの保持期間は `retention-days` で指定でき、デフォルトは30日です。

`storage: repo` を指定した場合、JSONLは `.review-patterns/corpus/reviews-YYYY-MM-DD.jsonl` に書き込まれます。このモードではJSONLのArtifactアップロードは行いませんが、生成プロンプトのArtifactはアップロードします。workflowでJSONLをリポジトリに保存したい場合は、利用側のworkflowでcheckoutやcommit処理を明示してください。

Action入力の `redact` と `anonymize` はM3時点では予約入力です。`true` を指定すると、MVP後のredaction/anonymizationルール確定後に対応予定であることを示して明示的に失敗します。GitHub tokenはデフォルトで `github.token` を使い、Action内部でマスクします。別tokenが必要な場合は `github-token` 入力で指定できます。

## ドキュメント

- [プロダクト要件](docs/product-requirements.md)
- [データとパタン形式](docs/data-and-pattern-formats.md)
- [マイルストーン](docs/milestones.md)

## ライセンス

MIT
