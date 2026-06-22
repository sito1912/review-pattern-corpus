# リリース手順

この手順は、`review-pattern-corpus` のComposite Actionを他リポジトリから固定タグで参照できる状態にするためのものです。

## バージョン方針

- タグ名は `vMAJOR.MINOR.PATCH` 形式にします。
- `v0` のような移動タグを用意し、利用者が互換範囲内の更新を受け取れるようにします。
- 破壊的変更、Action入力の削除、JSONL形式の非互換変更は、リリースノートで明示します。
- MVP中の初回リリースは、メンテナーが内容を確認したうえで `v0.1.0` などの0系タグとして作成します。

## 事前確認

リリース前に次を確認します。

```sh
git status --short
go test ./...
```

確認観点:

- `README.md`、`docs/install.md`、`action.yml` の入力説明が一致している。
- `docs/security-and-privacy.md` に保存方針と `redact` / `anonymize` の制限が書かれている。
- `CONTRIBUTING.md` にメンテナンス方針が書かれている。
- 実装されていないCLIを利用者向け手順で必須にしていない。
- サンプルworkflowがタグ参照になっている。

## タグ作成

次の例では `v0.1.0` を作成します。実際のリリース番号は、差分の内容に応じて決めてください。

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

0系の互換タグを更新する場合:

```sh
git tag -f v0 v0.1.0
git push origin v0 --force
```

移動タグを更新した場合は、リリースノートにその旨を記録します。

## GitHub Release作成

GitHub上でタグに対応するReleaseを作成し、次を記載します。

- 追加、変更、修正の概要。
- Action入力やArtifact名の変更。
- JSONL、catalog、パタンMarkdown形式の変更。
- セキュリティやプライバシー上の注意。
- 既知の制限。MVPでは `redact` と `anonymize` が未実装であることを明記します。

## リリース後確認

新しい一時リポジトリ、または検証用リポジトリで、タグを参照するworkflowを実行します。

```yaml
- uses: sito1912/review-pattern-corpus@v0.1.0
```

確認すること:

- `review-patterns-corpus-YYYY-MM-DD` Artifactが生成される。
- `review-patterns-prompt-YYYY-MM-DD` Artifactが生成される。
- `since` と `until` を省略した場合、前日UTCが使われる。
- `redact: "true"` または `anonymize: "true"` を指定した場合、明示的に失敗する。

問題があれば、修正後にPATCHバージョンを上げて再リリースします。
