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
review-patterns prompt
review-patterns validate
```

CLI名は `review-patterns` です。

Go module名は以下です。

```text
github.com/sito1912/review-pattern-corpus
```

## デフォルトのワークフロー

1. 利用リポジトリにGitHub Actionを導入する。
2. ActionがUTCの対象期間に対して `review-patterns collect` を実行する。
3. 収集されたJSONLは、デフォルトではGitHub Actions Artifactとしてアップロードされる。
4. Actionが今回のJSONLと既存の `.review-patterns/patterns/` を使って `review-patterns prompt` を実行する。
5. 生成されたプロンプトはArtifactとしてアップロードされる。
6. 人間が任意のAIコーディングエージェントに生成プロンプトを渡して実行する。
7. AIエージェントが `.review-patterns/patterns/catalog.yaml` とパタンMarkdownを更新する。
8. パタンランゲージの変更を通常のコード変更と同じようにレビューし、コミットする。

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

## ドキュメント

- [プロダクト要件](docs/product-requirements.md)
- [データとパタン形式](docs/data-and-pattern-formats.md)
- [マイルストーン](docs/milestones.md)

## ライセンス

MIT
