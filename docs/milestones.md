# マイルストーン

## M1: リポジトリ雛形と仕様

ゴール: 実装前にプロジェクトの形を確立する。

成果物:

- `README.md`
- `AGENTS.md`
- プロダクト要件ドキュメント
- データとパタン形式ドキュメント
- マイルストーン計画
- MITライセンス
- 初期Go module設定

完了条件:

- 開発者が元の計画議論を読まなくてもMVPの境界を理解できる。
- CLI名、module名、保存方針、デフォルト対象期間が文書化されている。

## M2: `collect` CLI

ゴール: マージ済みPull Requestからレビューデータを収集し、JSONLを出力する。

成果物:

- `review-patterns collect`
- GitHub token対応
- `--repo`、`--since`、`--until`、`--output`
- 前日UTCを使うデフォルト対象期間
- 人間によるreview comment収集
- review summary body収集
- bodyが空のapprove除外
- bot除外
- JSONL出力処理

完了条件:

- コマンドがリポジトリのレビューデータを収集し、有効なJSONLを生成できる。
- 同じ期間に対する繰り返し実行では、実用上安定した出力順になる。
- レート制限時の挙動が文書化され、過剰なリトライを行わない。

## M3: Composite GitHub Action

ゴール: GitHub Actions上で収集を実行する。

成果物:

- Composite Actionのラッパー
- 期間、保存先、保持期間、匿名化、マスク処理、issue comment収集のAction入力
- JSONLのArtifactアップロード
- `redact` と `anonymize` は予約入力として提供し、`true` 指定時はMVP後対応予定として明示的に失敗する
- M4完了後、生成プロンプトのArtifactアップロード

完了条件:

- 利用リポジトリがworkflowを追加し、手動で収集を実行できる。
- 時刻入力がない場合、前日UTCを収集する。
- JSONLがデフォルトでArtifactとしてアップロードされる。

## M4: `prompt` CLI

ゴール: JSONLと既存パタンからAIエージェント向けプロンプトを生成する。

成果物:

- `review-patterns prompt`
- `--input`
- `--patterns-dir`
- `--output`
- `--mode auto|extract|update`
- 初回抽出用プロンプトテンプレート
- 差分更新用プロンプトテンプレート

完了条件:

- JSONLがあり既存パタンがない場合、抽出用プロンプトを生成できる。
- JSONLと既存パタンがある場合、更新用プロンプトを生成できる。
- プロンプトが、生のコードや生のレビューコメントを不要に残さないようAIエージェントへ明確に指示している。

## M5: `validate` CLI

ゴール: コーパスとパタンランゲージファイルを検証する。

成果物:

- `review-patterns validate`
- JSONL検証
- `catalog.yaml` 検証
- パタンMarkdownの見出し検証
- 有用なエラーメッセージ

完了条件:

- 不正なJSONL行が行番号つきで報告される。
- catalog entryとファイルの対応が検証される。
- パタンファイルに必須見出しがあることが検証される。

## M6: OSS公開準備

ゴール: 他のリポジトリが利用できる状態にする。

成果物:

- インストールガイド: `docs/install.md`
- GitHub Actions workflow例: `docs/examples/collect-review-pattern-corpus.yml`
- セキュリティとプライバシーに関する注意: `docs/security-and-privacy.md`、`SECURITY.md`
- コーパス保存方針の説明: `docs/install.md`、`docs/security-and-privacy.md`
- コントリビューションガイド: `CONTRIBUTING.md`
- リリース手順: `docs/release.md`

進捗:

- 着手済み。公開前に必要な導入、保存方針、セキュリティ、コントリビューション、リリース手順のドキュメントを追加する。
- `validate` CLIは開発をスキップしているため、公開用導入手順では必須手順に含めない。
- 残タスクは、タグつきリリースの実行と、リリースタグを参照した検証用リポジトリでのAction実行確認。

完了条件:

- 新しいリポジトリがドキュメントだけを見てActionを導入できる。
- メンテナンスモデルが明確である。
- タグつきリリースを公開できる。

## MVP後の候補

- 安定した匿名化マッピング。
- secretや明らかなPIIのredactionルール。
- リポジトリ保存コーパスの圧縮。
- 過去Artifactの取得。
- GitHub GraphQL最適化。
- 任意のissue comment収集。
- 任意のAIエージェントコマンド実行。
- パタン更新PRの生成。
- 複数リポジトリ横断の集約モード。
