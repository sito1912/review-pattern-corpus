# AGENTS.md

## プロジェクト概要

このリポジトリは `review-pattern-corpus` を開発します。これは、人間によるコードレビュー指摘を収集し、リポジトリローカルなコードレビュー用パタンランゲージを保守するためのOSS CLIプロダクトです。

このプロダクトはAIエージェント非依存に保ちます。中核機能をCodex、Claude Code、その他の特定モデルに結合しないでください。CLIは人間がAIコーディングエージェントに渡すプロンプトを生成してよいですが、MVPではAIエージェントを直接実行してはいけません。

## 決定済み事項

- CLI名: `review-patterns`
- Go module: `github.com/sito1912/review-pattern-corpus`
- ライセンス: MIT
- 主な実装言語: Go
- 配布形態: `go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@<version>` で導入できるGo CLI
- デフォルトJSONL出力先: 標準出力。ファイル保存は `--output` で明示する
- デフォルトパタン保存先: `.review-patterns/patterns/` 配下のコミット済みファイル
- デフォルト対象期間: 前日UTC。当日0時UTCの24時間前から当日0時UTCまで

## MVPスコープ

以下のコマンドを実装します。

```text
review-patterns collect
review-patterns filter
review-patterns prompt
```

MVPでやること:

- UTC対象期間内にマージされたPull Requestからレビューデータを収集する。
- JSONLを出力する。
- 必要に応じて特定パスまたはauthorのレビューコメントだけに絞り込む。
- 今回のJSONLと既存パタンファイルからプロンプトを生成する。

MVPでやらないこと:

- AIコーディングエージェントを実行する。
- 生成されたパタン変更をコミットする。
- Pull Requestを作成する。
- 特定のCI/CDサービス専用ラッパーを提供する。
- 中央ストレージや複数リポジトリ横断ストレージを提供する。
- コーパスとパタンファイルを検証する `validate` CLIはMVP後の候補とする。

## 収集ルール

対象Pull Request:

```text
since <= merged_at < until
```

人間が書いた以下を収集します。

- Pull Request review comment。
- review commentの返信。
- Pull Request review summary body。
- bodyが空ではない `APPROVED` review body。

任意で収集します。

- Pull Request conversation issue comment。

以下は常に除外します。

- botコメント。
- CI/check annotation。
- commit comment。
- bodyが空のapprove。
- system/generated event。

## 保存ルール

デフォルトでは、生のレビューコーパスデータをコミットしないでください。

コミットされるパタンランゲージファイルは以下に置きます。

```text
.review-patterns/patterns/
```

JSONLを以下に書き込むのは、利用者が `--output` で明示した場合だけです。

```text
.review-patterns/corpus/
```

## パタンランゲージのルール

コミットされるパタンファイルでは、レビュー知識を抽象化・要約してください。形式はアレグザンダーのパタンランゲージに寄せ、「文脈、問題、フォース、解決、結果として生じる文脈」を中心にします。

コミットを避けるもの:

- 長い生のレビューコメント。
- 長い生のプロプライエタリなコード断片。
- 不要な個人情報。
- 個人を責めるような表現。

優先して残すもの:

- パタンが現れる文脈。
- 繰り返し起きる問題。
- 問題を難しくしているフォースやトレードオフ。
- フォースを調停する中心的な解決。
- 解決の結果として生じる文脈。
- レビューエージェントが使うときの観察ポイント。
- 誤用、例外、よくある誤検知。
- 関連パタン。

## 想定レイアウト

```text
.review-patterns/
  patterns/
    catalog.yaml
    P001-example.md
  prompts/
    extract.md
    update.md
  schema/
    review-comment.schema.json
    catalog.schema.json
    pattern.schema.json
```

## 開発ガイドライン

- 依存は最小限にする。
- 実用上問題がなければ標準ライブラリを優先する。
- GitHub APIの利用は保守的にし、レート制限を尊重する。
- ローカル環境と任意のCIで扱いやすいCLIとして動くコードを書く。
- コマンド出力はスクリプトから扱いやすくする。
- エラーは修正につながる内容にする。
- 永続化する時刻と期間比較にはUTCを使う。
- 生成プロンプトはプロダクトの表面として扱い、明示的で安定した内容にする。

## ドキュメント

挙動を変更する前に、関連するドキュメントを更新してください。

- プロダクト挙動やスコープの変更: `docs/product-requirements.md`
- JSONL、catalog、パタン形式の変更: `docs/data-and-pattern-formats.md`
- マイルストーンやロードマップの変更: `docs/milestones.md`
- ユーザー向け概要の変更: `README.md`

## レビュー観点

このリポジトリの変更をレビューするときは、以下を優先して確認してください。

- MVPを超えたスコープ拡大。
- 特定AIエージェントへの結合。
- 生のレビューコーパスデータの意図しないコミット。
- UTC対象期間の誤り。
- botや生成コメントのコーパス混入。
- 不要な負荷を生むGitHub API利用。
- パタンファイルが抽象ガイダンスではなく、生のコードやレビューコメントを保存していないか。
