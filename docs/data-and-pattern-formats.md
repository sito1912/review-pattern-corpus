# データとパタン形式

## リポジトリ構成

コミットされるパタンランゲージファイルは `.review-patterns/` 配下に置きます。

```text
.review-patterns/
  patterns/
    catalog.yaml
    P001-error-handling.md
  prompts/
    extract.md
    update.md
  schema/
    review-comment.schema.json
    catalog.schema.json
    pattern.schema.json
```

JSONLコーパスファイルはデフォルトではコミットしません。`--storage=repo` が指定された場合のみ、以下のような場所に保存できます。

```text
.review-patterns/
  corpus/
    reviews-2026-06-21.jsonl
```

Composite Actionで `storage=repo` を指定した場合、出力ファイル名は対象期間の開始UTC日を使って `.review-patterns/corpus/reviews-YYYY-MM-DD.jsonl` にします。

## レビューJSONLスキーマ

収集処理は、収集対象1件につき1つのJSONオブジェクトを出力します。

例:

```json
{
  "schema_version": "1.0",
  "repo": "owner/repo",
  "pr_number": 123,
  "pr_title": "リクエストバリデーションを改善する",
  "pr_merged_at": "2026-06-21T00:00:00Z",
  "comment_type": "review_comment",
  "comment_id": 222,
  "review_id": 111,
  "in_reply_to_id": null,
  "review_state": "COMMENTED",
  "author": "alice",
  "author_type": "User",
  "author_association": "MEMBER",
  "path": "src/main.go",
  "language": "go",
  "line": 40,
  "start_line": null,
  "side": "RIGHT",
  "diff_hunk": "@@ -10,6 +10,7 @@",
  "body": "ここでは呼び出し元の文脈を残すことを検討してください。",
  "created_at": "2026-06-21T00:00:00Z",
  "updated_at": "2026-06-21T00:00:00Z",
  "metadata": {
    "base_ref": "main",
    "head_sha": "abc123",
    "html_url": "https://github.com/owner/repo/pull/123#discussion_r222"
  }
}
```

### 必須フィールド

- `schema_version`
- `repo`
- `pr_number`
- `pr_merged_at`
- `comment_type`
- `comment_id`
- `author`
- `author_type`
- `body`
- `created_at`
- `updated_at`

### 文脈フィールド

取得できる場合は以下を使います。

- `pr_title`
- `review_id`
- `in_reply_to_id`
- `review_state`
- `author_association`
- `path`
- `language`
- `line`
- `start_line`
- `side`
- `diff_hunk`
- `metadata.base_ref`
- `metadata.head_sha`
- `metadata.html_url`

### コメント種別

初期値:

```text
review_comment
review_comment_reply
review_summary
issue_comment
```

`issue_comment` は、Pull Request conversation issue commentの収集が有効な場合のみ出力します。

## パタンカタログ

`catalog.yaml` は、有効なパタンと過去のパタンを管理する索引です。

例:

```yaml
schema_version: "1.0"
patterns:
  - id: P001
    title: エラーでは呼び出し元の文脈を保つ
    file: P001-preserve-caller-context-in-errors.md
    status: active
    tags:
      - go
      - error-handling
    updated_at: "2026-06-22"
```

### パタンステータス

対応するステータス:

```text
active
deprecated
merged
draft
```

## パタンMarkdown形式

各パタンファイルは、アレグザンダーのパタンランゲージに寄せた構造を使います。

パタンは単なるチェックリストではなく、「ある文脈で繰り返し起きる問題に対して、そこで働く複数の力を調停する、再利用可能な解決」を記述します。

```md
# P001: エラーでは呼び出し元の文脈を保つ

## 要約

## 文脈

## 問題

## フォース

## 解決

## 結果として生じる文脈

## レビューでの使い方

## 具体化の方向

## 誤用と例外

## 信頼度

## 出典メモ

## 関連パタン

## 変更履歴
```

### 見出しの意味

- `要約`: パタンの意図を1〜3文で説明する。
- `文脈`: この問題が現れるプロダクト、設計、コード、運用上の状況を説明する。
- `問題`: その文脈で繰り返し起きるレビュー上の問題を説明する。
- `フォース`: 問題を単純に解けなくしている制約、緊張関係、トレードオフを列挙する。
- `解決`: フォースを調停する中心的な判断を説明する。レビューコメントの文面ではなく、再利用可能な方針として書く。
- `結果として生じる文脈`: 解決を適用した後に得られる状態と、新たに注意すべき副作用を説明する。
- `レビューでの使い方`: レビューエージェントがどのような兆候を見つけ、どの強さでコメントするかを説明する。
- `具体化の方向`: 実装者に示せる修正の方向性を書く。ただし、特定コードのコピーは避ける。
- `誤用と例外`: このパタンを適用しない方がよい条件や、誤検知しやすい条件を書く。
- `信頼度`: コーパス上の根拠の強さを `high`、`medium`、`low` などで示し、弱い根拠を硬いルールにしない。
- `出典メモ`: 生のコメントやコードを残さず、どのようなレビュー群から抽象化したかを短く記録する。
- `関連パタン`: 近いパタン、前提になるパタン、衝突しうるパタンを記録する。
- `変更履歴`: パタンの追加、統合、deprecated化、重要な更新を記録する。

## パタン記述ルール

パタンは、一般的なスタイルガイドやレビュー指摘テンプレートではなく、レビュー文化を生成的に再利用するための言語として書きます。

各パタンに含めるべきもの:

- そのパタンが適用される文脈。
- 繰り返し発生する問題。
- 問題を難しくしているフォース。
- フォースを調停する中心的な解決。
- 解決によって生じる良い結果と副作用。
- レビューエージェントが使うときの観察ポイント。
- 許容可能な具体化の方向性。
- 誤用、例外、誤検知。
- 必要に応じた関連パタンへのリンク。

各パタンで避けるべきもの:

- 生のプロプライエタリなコードのコピー。
- 長い生のレビューコメントのコピー。
- 必要がない個別レビュアー名の記録。
- 一回限りの好みを恒久的なルールにすること。
- コーパス上の根拠が弱いのに硬いルールにすること。
- 「必ず」「常に」のような断定を、フォースや例外の説明なしに使うこと。

## プロンプト出力への期待

`prompt` コマンドは、AIエージェントに以下を指示する必要があります。

- JSONLコーパスを読む。
- 新しいレビュー例と既存パタンを比較する。
- 繰り返し現れる問題とフォースがある場合だけ新規パタンを追加する。
- 新しい例によって適用範囲が明確になる場合は既存パタンを更新する。
- 重複するパタンを統合またはdeprecatedにする。
- パタン本文は「文脈、問題、フォース、解決、結果として生じる文脈」を中心に構成する。
- コミットされるパタンファイルは抽象的で再利用可能な内容に保つ。
- パタンファイルに生のソースコードや生のレビューコメントを残すことを避ける。
