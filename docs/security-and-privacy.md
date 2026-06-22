# セキュリティとプライバシー

`review-pattern-corpus` は、リポジトリ内の人間によるレビュー指摘を収集します。レビューコメントはプロダクトや組織の設計判断、未公開コードの文脈、レビュアー名を含むことがあります。導入時は、生成されるJSONLとパタンファイルを機密性のある開発データとして扱ってください。

## JSONLに含まれる情報

JSONLには、収集できる範囲で以下が含まれます。

- Pull Request番号、タイトル、merge時刻。
- review comment、review comment reply、review summary body。
- author、author type、author association。
- ファイルパス、行番号、diff hunk。
- Pull RequestやコメントへのURL。
- `include-issue-comments: true` の場合はPull Request conversation issue comment。

CI/check annotation、commit comment、botコメント、bodyが空のapprove、system/generated eventは収集対象外です。

## 保存方針

デフォルトではJSONLはGitHub Actions Artifactに保存され、リポジトリにはコミットされません。Artifactの保持期間は `retention-days` で指定できます。

`storage: repo` を明示した場合だけ、JSONLは `.review-patterns/corpus/reviews-YYYY-MM-DD.jsonl` に出力されます。このリポジトリの `.gitignore` は `.review-patterns/corpus/` と `*.jsonl` を無視するため、コミットする場合は利用側で明示的な判断が必要です。

生のレビューコーパスをコミットする前に、次を確認してください。

- private repositoryの情報を公開リポジトリへ持ち出していないか。
- 長いソースコード断片や機密仕様が含まれていないか。
- 個人情報や不要な個人名が含まれていないか。
- Artifact保存で足りる用途なのにrepo storageを選んでいないか。

## redactとanonymize

`redact` と `anonymize` はMVPでは予約入力です。

```yaml
with:
  redact: "true"
```

または

```yaml
with:
  anonymize: "true"
```

を指定すると、Actionは明示的に失敗します。現時点では、これらを指定しても機密情報の保護は行われません。

## GitHub token

通常は `github.token` を使います。workflowには次の最小権限を設定してください。

```yaml
permissions:
  contents: read
  pull-requests: read
  issues: read
```

別tokenを `github-token` で渡す場合は、必要最小限のスコープを持つsecretを使い、ログへ値を出さないでください。Actionは受け取ったtokenをGitHub Actionsのmask commandでマスクします。

## パタンファイルの公開基準

`.review-patterns/patterns/` にコミットする内容は、生データではなく抽象化されたレビュー知識にしてください。

残すべきもの:

- パタンが現れる文脈。
- 繰り返し起きる問題。
- 問題を難しくするフォースやトレードオフ。
- 中心的な解決。
- 結果として生じる文脈。
- レビュー時の観察ポイント、誤用、例外、関連パタン。

避けるべきもの:

- 長い生のレビューコメント。
- 長いプロプライエタリなコード断片。
- 不要な個人情報。
- 個人を責める表現。
- 根拠の弱い一回限りの好みを恒久ルールにすること。

## 公開リポジトリでの運用

公開リポジトリでこのActionを使う場合、Artifactやパタンファイルが第三者に見られる前提で運用してください。

private repositoryで使う場合でも、フォーク、外部コントリビューター、Artifactのアクセス範囲、保存期間を確認してください。特に `include-issue-comments` と `storage: repo` は、収集範囲と保存範囲を広げる入力です。
