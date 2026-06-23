# リリース手順

この手順は、`review-patterns` CLIを固定バージョンで `go install` できる状態にするためのものです。

## バージョン方針

- タグ名は `vMAJOR.MINOR.PATCH` 形式にします。
- 破壊的変更、CLIフラグの削除、JSONL形式の非互換変更は、リリースノートで明示します。
- MVP中の初回リリースは、メンテナーが内容を確認したうえで `v0.1.0` などの0系タグとして作成します。
- 利用者には `@latest` ではなく、必要に応じて `@v0.1.0` のような固定タグを推奨します。

## 事前確認

リリース前に次を確認します。

```sh
git status --short
go test ./...
go install ./cmd/review-patterns
```

確認観点:

- `README.md`、`docs/install.md`、`docs/product-requirements.md` のCLI説明が一致している。
- `docs/security-and-privacy.md` にJSONL保存方針と公開時の注意が書かれている。
- `CONTRIBUTING.md` にメンテナンス方針が書かれている。
- 実装されていないCLIを利用者向け手順で必須にしていない。
- `go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@<tag>` で導入する前提の説明になっている。

## タグ作成

次の例では `v0.1.0` を作成します。実際のリリース番号は、差分の内容に応じて決めてください。

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

## GitHub Release作成

GitHub上でタグに対応するReleaseを作成し、次を記載します。

- 追加、変更、修正の概要。
- CLIコマンドやフラグの変更。
- JSONL、catalog、パタンMarkdown形式の変更。
- セキュリティやプライバシー上の注意。
- 既知の制限。MVPではAIエージェント実行、パタン変更コミット、Pull Request作成を行わないことを明記します。

## リリース後確認

一時ディレクトリで、公開タグからCLIをインストールできることを確認します。

```sh
GOBIN="$(pwd)/bin" go install github.com/sito1912/review-pattern-corpus/cmd/review-patterns@v0.1.0
./bin/review-patterns --help
```

必要に応じて、読み取り可能な検証用リポジトリに対して次を確認します。

- `review-patterns collect --repo owner/repo --output tmp/reviews.jsonl` がJSONLを生成する。
- `since` と `until` を省略した場合、前日UTCが使われる。
- `review-patterns prompt --input tmp/reviews.jsonl --patterns-dir .review-patterns/patterns --output tmp/prompt.md` がMarkdownプロンプトを生成する。
- JSONLやプロンプトが意図せずコミット対象になっていない。

問題があれば、修正後にPATCHバージョンを上げて再リリースします。
