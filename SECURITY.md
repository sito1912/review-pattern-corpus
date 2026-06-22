# セキュリティポリシー

`review-pattern-corpus` は、人間のレビューコメント、diff文脈、author metadata、Pull Requestリンクを収集します。生成されるJSONL Artifactは、内容を確認するまで機密性のある開発データとして扱ってください。

運用上の注意は [セキュリティとプライバシー](docs/security-and-privacy.md) を参照してください。

## サポート対象

最初の公開リリース後は、最新のタグ付きリリース系列を対象にセキュリティ修正を準備します。MVP期間中も、タグが利用できる場合は `main` ではなく固定タグを参照してください。

## 脆弱性の報告

このリポジトリでGitHub private vulnerability reportingが有効な場合、機微な報告にはそれを使ってください。

private reportingが利用できない場合は、公開issueに最小限の説明だけを書き、攻撃手順、private token、プロプライエタリなコード、生のコーパスデータは含めないでください。メンテナーがより安全な連絡経路を調整します。
