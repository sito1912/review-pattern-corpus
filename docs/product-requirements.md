# プロダクト要件

## 目的

`review-pattern-corpus` は、リポジトリ固有のコードレビュー文化をパタンランゲージとして言語化するためのプロダクトです。

多くのチームには、繰り返し指摘される観点、好まれる修正方針、許容されるトレードオフ、プロダクト固有の判断基準があります。このプロジェクトでは、それらの暗黙知をAIコードレビューエージェントが参照できる永続的なファイルとして管理します。

このプロジェクトのスコープは、レビューエージェント本体ではなく、パタンランゲージの運用と保守です。

## 対象ユーザー

主なユーザーは、以下を実現したいリポジトリメンテナーです。

- マージ済みPull Requestから過去の人間によるレビュー指摘を収集する。
- 任意のAIコーディングエージェントを使って、繰り返し現れるレビュー観点を抽出する。
- 抽出されたコードレビュー用パタンランゲージをリポジトリにコミットする。
- パタンランゲージを継続的に更新する。

## 導入モデル

このプロダクトは、自身のレビューコーパスを管理したい各リポジトリに導入されます。

MVPでは、中央サービスや複数リポジトリ横断のデータウェアハウスは想定しません。収集とパタン生成の入力は、Actionが実行されるリポジトリ内に閉じます。

## アーキテクチャ

想定する実装は以下です。

- `review-patterns` という名前のGo CLI。
- Go moduleは `github.com/sito1912/review-pattern-corpus` とし、開発対象は初期設定時点の最新安定系列であるGo 1.26とする。
- CLIを包むComposite GitHub Action。
- `.review-patterns/` 配下のファイルベースのパタンランゲージ管理。
- GitHub Actions Artifactをデフォルト保存先とするJSONLコーパス出力。

設計では依存を最小限にし、枯れた技術を優先します。

## CLIコマンド

### `collect`

マージ済みPull Requestからレビューデータを収集します。

M2で実装する入力:

```text
--repo owner/repo
--since 2026-06-01T00:00:00Z
--until 2026-06-22T00:00:00Z
--output reviews.jsonl
--token
--include-issue-comments=false
```

MVP内で追加する入力:

```text
--storage artifact|repo
--redact=false
--anonymize=false
```

挙動:

- `merged_at` が指定されたUTC期間に含まれるPull Requestを選択する。
- `--repo` が省略された場合は、GitHub Actions標準の `GITHUB_REPOSITORY` を使う。
- GitHub tokenは `GITHUB_TOKEN` または `GH_TOKEN` から読み、必要に応じて `--token` で明示できる。
- ローカルPCで実行する場合、環境変数や `--token` がなければ `gh auth token` を使ってGitHub CLIの認証情報を参照する。
- `--since` と `--until` は両方指定するか、両方省略する。片方だけ指定された場合はエラーにする。
- 収集対象1件につき1つのJSONオブジェクトを出力する。
- JSONL形式で出力する。
- `--output` が省略された場合、または `--output -` の場合は標準出力へJSONLを書く。
- 実行中は検索中の期間、見つかったPull Request数、Pull Requestごとの収集状況、書き込み先を標準エラーへ表示する。JSONLを標準出力へ出す場合でも進捗表示を混ぜない。
- botコメントや生成コメントはデフォルトで除外する。
- GitHubのレート制限を尊重し、不要なAPI呼び出しを避ける。
- レート制限を受けた場合、CLIは自動リトライせず非ゼロ終了する。GitHub APIが返すリセット時刻または `Retry-After` がある場合はエラーメッセージに含める。

`since` と `until` が省略された場合:

```text
since = 前日 00:00:00 UTC
until = 当日 00:00:00 UTC
```

### `prompt`

収集済みJSONLと既存パタンから、AIエージェント向けプロンプトを生成します。

入力:

```text
--input reviews.jsonl
--patterns-dir .review-patterns/patterns
--output prompt.md
--mode auto|extract|update
```

挙動:

- 今回の実行で得られたJSONLを読み込む。
- 既存のパタンファイルがあれば読み込む。
- 人間が任意のAIコーディングエージェントに渡すためのプロンプトを生成する。
- MVPではAIエージェント自体を実行しない。
- パタン更新では、生のコメントやコードをコピーするのではなく、レビュー知識を要約・抽象化することを優先する。

### `validate`

JSONLとパタンランゲージファイルを検証します。

入力:

```text
--input reviews.jsonl
--patterns-dir .review-patterns/patterns
--schema-dir .review-patterns/schema
```

挙動:

- レビューJSONLの形を検証する。
- `catalog.yaml` を検証する。
- パタンMarkdownファイルに必須見出しがあることを検証する。
- 可能な限り、ファイルパスと行番号つきで修正可能なエラーを報告する。

## GitHub Action入力

```yaml
inputs:
  since:
    required: false
  until:
    required: false
  include-issue-comments:
    default: "false"
  storage:
    default: "artifact"
  retention-days:
    default: "30"
  redact:
    default: "false"
  anonymize:
    default: "false"
```

## レビューデータ収集

### 必須で収集するもの

人間が書いた以下を収集します。

- Pull Request review comment。
- review commentの返信。
- Pull Request review summary body。
- bodyが空ではない `APPROVED` review body。

### オプションで収集するもの

有効化された場合のみ収集します。

- Pull Request conversation issue comment。

### 常に除外するもの

以下は常に除外します。

- botコメント。
- CI/check annotation。
- commit comment。
- bodyが空のapprove。
- system/generated event。

## 保存方針

デフォルト:

- JSONLレビューコーパス: GitHub Actions Artifact。
- パタンランゲージ: `.review-patterns/patterns/` 配下のコミット済みファイル。

オプション:

- `--storage=repo`: JSONLを `.review-patterns/corpus/` 配下に保存する。
- `--storage=artifact`: JSONLをAction Artifactとして保存する。
- `--redact`: 対応している範囲で機密情報をマスクする。
- `--anonymize`: 対応している範囲でauthor情報を匿名化する。
- `--retention-days`: Artifactの保持期間を指定する。

デフォルトでは、生のレビューコメントやコード断片をリポジトリにコミットしてはいけません。

## プライバシーと安全性

コミットされるパタンランゲージでは、生のコードや生のレビューコメント本文をできるだけ避けます。

パタンファイルに残すべきもの:

- パタンが現れる文脈。
- 繰り返し起きる問題。
- 問題を難しくしているフォースやトレードオフ。
- フォースを調停する中心的な解決。
- 解決の結果として生じる文脈。
- レビューエージェントが使うときの観察ポイント。
- 誤用や誤検知を避けるためのガイド。

パタンファイルで避けるべきもの:

- 不要な個人情報。
- 長い生のレビューコメント引用。
- プロプライエタリなソースコードの長い断片。
- 個人を責めるような表現。

## MVPの境界

MVPではArtifactとプロンプトファイルだけを生成します。

MVPでやらないこと:

- Codex、Claude Code、その他のAIコーディングエージェントを実行する。
- パタン変更をコミットする。
- Pull Requestを作成する。
- 過去Artifactを取得・管理する。
- 中央ストレージサービスを提供する。

プロンプト生成では、MVPは以下を使います。

```text
今回の実行で得られたJSONL + 既存の .review-patterns/patterns/
```

過去全期間の再処理が必要な場合は、長い期間を指定して再収集するか、`--storage=repo` を使います。
