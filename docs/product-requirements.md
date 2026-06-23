# プロダクト要件

## 目的

`review-pattern-corpus` は、リポジトリ固有のコードレビュー文化をパタンランゲージとして言語化するためのCLIプロダクトです。

多くのチームには、繰り返し指摘される観点、好まれる修正方針、許容されるトレードオフ、プロダクト固有の判断基準があります。このプロジェクトでは、それらの暗黙知をAIコードレビューエージェントが参照できる永続的なファイルとして管理します。

このプロジェクトのスコープは、レビューエージェント本体ではなく、パタンランゲージの運用と保守です。

## 対象ユーザー

主なユーザーは、以下を実現したいリポジトリメンテナーです。

- マージ済みPull Requestから過去の人間によるレビュー指摘を収集する。
- 任意のAIコーディングエージェントを使って、繰り返し現れるレビュー観点を抽出する。
- 抽出されたコードレビュー用パタンランゲージをリポジトリにコミットする。
- パタンランゲージを継続的に更新する。

## 導入モデル

このプロダクトは、自身のレビューコーパスを管理したい各リポジトリにCLIとして導入されます。

MVPでは、中央サービスや複数リポジトリ横断のデータウェアハウスは想定しません。収集とパタン生成の入力は、CLIを実行する利用者またはジョブが明示したリポジトリ、JSONL、パタンディレクトリに閉じます。

## アーキテクチャ

想定する実装は以下です。

- `review-patterns` という名前のGo CLI。
- Go moduleは `github.com/sito1912/review-pattern-corpus` とし、開発対象は初期設定時点の最新安定系列であるGo 1.26とする。
- `go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@<version>` で導入できる配布形態。
- `.review-patterns/` 配下のファイルベースのパタンランゲージ管理。
- JSONLコーパスは標準出力または `--output` で指定されたファイルへ出力する。

設計では依存を最小限にし、枯れた技術を優先します。特定のCI/CDサービス専用のラッパーや、特定AIエージェントへの結合は中核機能に含めません。

## CLIコマンド

### `collect`

マージ済みPull Requestからレビューデータを収集します。

入力:

```text
--repo owner/repo
--since 2026-06-01T00:00:00Z
--until 2026-06-22T00:00:00Z
--output reviews.jsonl
--token
--include-issue-comments=false
```

挙動:

- `merged_at` が指定されたUTC期間に含まれるPull Requestを選択する。
- `--repo` が省略された場合は、GitHub CLIの `gh repo view` で現在のリポジトリを推測する。推測できない場合はエラーにする。
- GitHub tokenは `GITHUB_TOKEN` または `GH_TOKEN` から読み、必要に応じて `--token` で明示できる。
- 環境変数や `--token` がなければ `gh auth token` を使ってGitHub CLIの認証情報を参照する。
- `--since` と `--until` は両方指定するか、両方省略する。片方だけ指定された場合はエラーにする。
- 収集対象1件につき1つのJSONオブジェクトを出力する。
- JSONL形式で出力する。
- `--output` が省略された場合、または `--output -` の場合は標準出力へJSONLを書く。
- 実行中は検索中の期間、検索済みPull Request候補数、見つかったPull Request数、Pull Requestごとの収集状況、書き込み先を標準エラーへ表示する。JSONLを標準出力へ出す場合でも進捗表示を混ぜない。
- botコメントや生成コメントはデフォルトで除外する。
- GitHubのレート制限を尊重し、不要なAPI呼び出しを避ける。
- GitHub APIから30秒間応答がない場合、無期限に待たずにエラーにする。
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
--reviewer-patterns
```

挙動:

- 今回の実行で得られたJSONLを、検証と概要生成のために読み込む。
- 生成プロンプトにはJSONL本文を埋め込まず、入力コーパスとして読むべきファイルパスを含める。
- 既存の `.md`、`.yaml`、`.yml` パタンファイルがあれば検出し、生成プロンプトには既存パタン本文を埋め込まず、読むべきファイルパスを含める。
- `--mode auto` の場合、既存パタンファイルがなければ初回抽出用、あれば差分更新用のプロンプトを生成する。
- `--reviewer-patterns` を指定した場合、特定レビュワーの思考の癖に特化したパタン候補を抽出するため、「パタン候補の見つけ方」と「採用基準」を差し替える。
- `--reviewer-patterns` を指定した場合、入力JSONLの全レコードの `author` が同じ非空文字列であることを事前に検証する。異なるauthor、欠落、`null`、空文字があればエラーにし、`filter --author` で絞り込むことを提案する。
- `--output` が省略された場合、または `--output -` の場合は標準出力へMarkdownを書く。
- 人間が任意のAIコーディングエージェントに渡すためのプロンプトを生成する。
- MVPではAIエージェント自体を実行しない。
- 生成プロンプトは、AIエージェントがPull Request内の議論を復元し、個々の指摘を文脈、問題、フォース、解決の核へ分解してからパタン候補を抽出するよう指示する。
- パタン更新では、生のコメントやコードをコピーするのではなく、レビュー知識を要約・抽象化することを優先する。

### `filter`

収集済みJSONLから、指定したパスやauthorに関係するコメントだけを抽出して新しいJSONLを生成します。

入力:

```text
--input reviews.jsonl
--output app-reviews.jsonl
--path /app/controllers
--author alice
```

挙動:

- `--input` のJSONLを1行ずつ読み込む。
- `--path` または `--author` の少なくとも一方を指定する。
- 各JSONオブジェクトの `path` 値を、`--path` の値で検索する。
- パスは `/` 区切りへ正規化し、先頭の `/` や `./` の有無に左右されないようにする。
- `--path /app` は、`app/...` や `packages/backend/app/...` のようにパス区切り単位で `app` を含むファイルに一致する。
- `--path /app/controllers` は、その配下のファイルに一致する。
- `path` がない、`path` が `null`、または空文字のJSONオブジェクトは出力しない。
- `--author` は各JSONオブジェクトの `author` 値に対して完全一致で検索する。
- `--path` と `--author` を同時に指定した場合は、両方を満たすJSONオブジェクトだけを出力する。
- マッチした行は再エンコードせず、入力JSONLの1行をそのまま `--output` へ書き込む。
- 不正なJSONL行は行番号つきでエラーにする。

### `validate`

JSONLとパタンランゲージファイルを検証するコマンドはMVP後の候補です。公開時点の必須フローには含めません。

想定入力:

```text
--input reviews.jsonl
--patterns-dir .review-patterns/patterns
--schema-dir .review-patterns/schema
```

想定挙動:

- レビューJSONLの形を検証する。
- `catalog.yaml` を検証する。
- パタンMarkdownファイルに必須見出しがあることを検証する。
- 可能な限り、ファイルパスと行番号つきで修正可能なエラーを報告する。

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

- JSONLレビューコーパス: 標準出力。
- パタンランゲージ: `.review-patterns/patterns/` 配下のコミット済みファイル。

オプション:

- `--output <path>`: JSONLまたはプロンプトを指定したファイルへ保存する。
- `--include-issue-comments`: Pull Request conversation issue commentも収集する。

デフォルトでは、生のレビューコメントやコード断片をリポジトリにコミットしてはいけません。JSONLをファイルに保存する場合は、`tmp/`、CIの一時領域、または `.review-patterns/corpus/` など、コミット対象外の場所を使うことを推奨します。

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

MVPではJSONLとプロンプトファイルだけを生成します。

MVPでやらないこと:

- Codex、Claude Code、その他のAIコーディングエージェントを実行する。
- パタン変更をコミットする。
- Pull Requestを作成する。
- 中央ストレージサービスを提供する。
- 特定のCI/CDサービス専用ラッパーを提供する。

プロンプト生成では、MVPは以下を使います。

```text
今回の実行で得られたJSONL + 既存の .review-patterns/patterns/
```

過去全期間の再処理が必要な場合は、長い期間を指定して再収集するか、保存済みJSONLを任意の方法で結合してから `prompt` に渡します。
