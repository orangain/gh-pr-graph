# 開発の進め方

このファイルの指示は、このリポジトリ内のすべての作業に適用する。振る舞いやアーキテクチャを変更する前に `DESIGN.md` を読み、重要な設計判断を反映して最新の状態に保つこと。

## 作業方針

- ユーザーが別の言語を使わない限り、日本語でやり取りする。
- 安全で元に戻せる範囲では合理的に判断し、不要な確認で作業を止めず、依頼された内容を完了させる。
- ワークツリーにある無関係なユーザーの変更を保持する。編集前とコミット前に `git status` を確認する。
- ユーザーからの一まとまりの依頼が完了したら、自動的にコミットする。必要な確認事項が解決していない間はコミットしない。
- `feat:`、`fix:`、`style:`、`docs:`、`test:` などを使い、対象を絞ったConventional Commits形式のメッセージにする。
- 現在の依頼が、直前かつ未公開のコミットの不適切な変更を直すものなら、そのコミットをamendする。それより前のコミットは書き換えず、新しいコミットを積む。
- ユーザーから明示的な依頼がない限り、push、タグ作成、リリースは行わない。「push & release」のような依頼は、この3つすべての実行を許可するものとして扱う。
- 無関係なユーザー作成のコミットやファイルがリリースに含まれる場合は、黙って含めずユーザーへ伝える。

## 実装

- このプロジェクトは、Go 1.23で作られたGitHub CLI extensionであり、依存ライブラリを使わないHTML/CSS/JavaScriptのUIをバイナリへ埋め込んでいる。
- 既存のアーキテクチャと素のWeb技術を優先する。明確な理由がない限り、フレームワークや依存ライブラリを追加しない。
- 手作業のファイル編集には `apply_patch` を使い、Goコードの整形には `gofmt` を使う。
- GitHub APIとの通信効率を保つ。安全な範囲でGraphQLリクエストをまとめる一方、GitHubのquery node上限、既存の並行数制限、rate limit対策を守る。
- GitHubの認証情報、private API response、機密性のあるリポジトリ情報を、ブラウザ、永続キャッシュ、通常ログへ漏らさない。
- ユーザーが明示的に要件を変えない限り、サーバーはローカル（`127.0.0.1`）だけでlistenする。
- 振る舞い、データフロー、UIの意味、上限、キャッシュ、トレース、リリース設計を変更したら `DESIGN.md` を更新する。`README.md` はユーザー向けまたは開発者向けの説明が変わる場合だけ更新する。
- UI状態を視覚的に再現する必要がある場合は、デモデータも更新する。デモモードはCLIオプションやhelpに追加せず、`GH_PR_GRAPH_DEMO` だけで有効にする。
- 開発用トレースはCLIオプションにせず、`GH_PR_GRAPH_TRACE_OTEL` でのみ設定する。

## UIの原則

- PR同士の関係と、次に必要なアクションを素早く把握できることを優先する。特にstacked PRを重視する。
- ロードやリロード中のレイアウトシフトを可能な範囲で避け、自動リロードではスクロール位置を保持する。
- 状態の意味を混ぜない。ノード背景はviewerとの関係、枠線の太さはdraftかreadyか、アイコンやラベルはレビュー、CI、conflict、pending review、再レビュー状態を表す。
- 色を変更するときは、ライト・ダーク両テーマの可読性とコントラストを確認する。
- GitHubの概念に適切なアイコンがある場合は、Primer Octiconsを使う。
- PRカードをコンパクトに保ち、グラフのエッジや周辺情報から明らかな内容を重複して追加しない。

## 検証

変更内容に応じた検証を行う。コード変更をコミットする前には、通常は以下を実行する。

```sh
git diff --check
node --check internal/server/web/app.js
GOCACHE=/tmp/gh-pr-graph-go-cache go test ./...
GOCACHE=/tmp/gh-pr-graph-go-cache go vet ./...
```

ドキュメントだけの変更では `git diff --check` で十分とする。振る舞いの変更や不具合修正には、対応するテストを追加または更新する。

## Gitコマンドの許可

実行環境から許可を求められた場合は、コマンド全体に対する一度限りの許可ではなく、用途を絞った再利用可能なprefix ruleを申請する。このリポジトリで有用なprefixは次のとおり。

- `git add`
- `git commit`
- `git push`
- `git tag`
- `gh release view`
- `gh pr create`
- `gh pr view`
- `gh run list`
- `gh run view`
- `gh run watch`

許可状態は実行環境が管理するため、このファイルだけでは許可済みの状態を保証できない。上記は、必要になった際に申請すべき限定的な権限を記録したものである。

## リリース

- ユーザーが1.0リリースを明示的に認めるまで、バージョンを `1.0.0` 以上にしない。
- 変更内容から次のSemantic Versioningを決める。不具合修正、ドキュメント、小さな見た目の改善はpatch、新しいユーザー向け機能や意味のある振る舞いの変更はminorとする。
- `CHANGELOG.md`の各項目はGitHub Release上で不自然に改行されないよう1項目を物理的な1行で書く。対応Issueがあれば報告者とIssueを、対応PRがあればPRをリンクする。PR authorがrepository ownerでない場合はauthorも記載し、ownerの場合は省略する。
- 明示的にリリースを依頼されたら、最初にワークツリー、最新リリース、`Unreleased`の内容を確認して対象versionを決める。`Unreleased`の項目を日付付きversion sectionへ移し、比較linkを更新する変更は、`main`へ直接commitせず、リリース準備専用branchのcommitとしてpushし、PRを作成する。
- リリース準備branchは必ず`release/vX.Y.Z`とし、PRのbaseは`main`にする。PR本文には対象version、変更概要、検証結果を記載する。PR作成後はURLを報告し、人間が内容を確認してmergeするまで、agent自身はtag作成、tagのpush、GitHub Releaseの作成を行わない。
- リリース準備PRがmergeされると、Release workflowがmerge commitへannotated tagを作成し、同じrun内でGitHub Releaseを自動公開する。最初のリリース依頼は、この自動処理を含む一連のリリースを許可するものとし、merge後に追加指示を要求しない。agentがmerge後も実行中なら、通常CIとRelease workflowを監視し、公開状態を確認する。
- `vX.Y.Z` 形式のannotated tagを使い、メッセージは `gh-pr-graph X.Y.Z` とする。
- release tagの作成とpushはRelease workflowだけが行う。手動でtagをpushしてもreleaseは開始されない。agentはrelease tagを作成・pushせず、一度作成されたrelease tagも付け替えない。
- GitHub Releaseがdraftやprereleaseではなく公開済みであることを確認し、リンクとともに報告する。`main` が `origin/main` と同期していることも確認する。
- CIまたはリリースが失敗した場合は、成功したと報告せず、失敗ログを確認して依頼の範囲内で修正する。
