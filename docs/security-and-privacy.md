# セキュリティとプライバシー

`review-pattern-corpus` は、リポジトリ内の人間によるレビュー指摘を収集します。レビューコメントはプロダクトや組織の設計判断、未公開コードの文脈、レビュアー名を含むことがあります。導入時は、生成されるJSONLとパタンファイルを機密性のある開発データとして扱ってください。

## JSONLに含まれる情報

JSONLには、収集できる範囲で以下が含まれます。

- Pull Request番号、タイトル、merge時刻。
- review comment、review comment reply、review summary body。
- author、author type、author association。
- ファイルパス、行番号、diff hunk。
- Pull RequestやコメントへのURL。
- `--include-issue-comments` を指定した場合はPull Request conversation issue comment。

CI/check annotation、commit comment、botコメント、bodyが空のapprove、system/generated eventは収集対象外です。

## 保存方針

`review-patterns collect` は、デフォルトではJSONLを標準出力へ書きます。ファイルに保存する場合は `--output <path>` を明示します。

このリポジトリの `.gitignore` は `.review-patterns/corpus/` と `*.jsonl` を無視します。生のレビューコーパスを保存する場合でも、通常は `tmp/`、CIの一時領域、または `.review-patterns/corpus/` など、コミットされない場所を使ってください。

生のレビューコーパスをコミットする前に、次を確認してください。

- private repositoryの情報を公開リポジトリへ持ち出していないか。
- 長いソースコード断片や機密仕様が含まれていないか。
- 個人情報や不要な個人名が含まれていないか。
- コミットせずに一時ファイルとして扱えば足りる用途ではないか。

## redactionとanonymization

MVPでは、安定したredactionやanonymizationは実装しません。

JSONLを共有、保存、または公開リポジトリに持ち込む前に、利用者が内容を確認してください。機密情報の削除や著者名の匿名化が必要な場合は、生成されたJSONLを別ツールで処理してから `review-patterns prompt` に渡します。

## GitHub token

GitHub APIを読むためのtokenは、次の順に参照します。

1. `--token`
2. `GITHUB_TOKEN`
3. `GH_TOKEN`
4. `gh auth token`

tokenには、対象リポジトリのPull Request、review comment、必要に応じてissue commentを読むための最小権限だけを付与してください。CLIのログや生成ファイルにtokenを含めないでください。

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

公開リポジトリで使う場合、保存されたJSONLやパタンファイルが第三者に見られる前提で運用してください。

private repositoryで使う場合でも、フォーク、外部コントリビューター、一時ファイルの保存先、ログの保管期間を確認してください。特に `--include-issue-comments` は収集範囲を広げるため、必要なリポジトリだけで有効化してください。
