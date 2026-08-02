# gh pr-graph 設計

## 1. 目的

`gh pr-graph` は、自分が関与する Pull Request（以下 PR）を、base/head ブランチの関係に基づく有向グラフとしてローカル Web UI に表示する GitHub CLI extension とする。

主な利用目的は次のとおり。

- 自分が作成、担当、レビュー依頼されている open PR を一覧する
- stacked PR の順序と依存関係を把握する
- レビュー、アサイン、CI、マージ可否を一画面で把握する
- ある PR のブランチに既に取り込まれた PR を、親 PR の中で確認する

GitHub 上のデータは参照専用とし、初期版ではレビュー、アサイン、マージ等の更新操作を提供しない。

## 2. ユーザー体験

```text
$ gh pr-graph
Loading pull requests...
Opened http://127.0.0.1:43781
Press Ctrl-C to stop.
```

- コマンドは loopback interface の空きポートで HTTP サーバーを起動する。
- 初回データ取得後、既定ブラウザを自動で開く。
- `Ctrl-C` で graceful shutdown する。
- `--no-open` ではブラウザを開かず URL のみ表示する。
- `--port 8080` でポートを固定できる。listen address は安全のため常に `127.0.0.1` を既定とする。
- `--repo OWNER/REPO` は対象を限定する。未指定時は複数リポジトリを横断する。

既定の検索集合は以下 3 検索の和集合（PR ID で重複排除）とする。GitHub 検索構文における複雑な OR の解釈差を避けるため、API 内部では別々に実行する。

```text
is:open author:@me
is:open assignee:@me
is:open review-requested:@me
```

画面上部の検索欄では GitHub PR 検索構文を受け付ける。ユーザーが検索を実行した後は、その文字列を単一の検索条件として扱う。空欄に戻すと既定の 3 検索へ戻る。

## 3. 画面設計

```text
┌──────────────────────────────────────────────────────────────────────┐
│ PR Graph  [ is:open team-review-requested:org/team       ] [検索] ↻ │
│           [Open ✓] [Draft ✓]  Repository: All   Updated 12:34 (自動)│
├──────────────────────────────────────────────────────────────────────┤
│ repo-a/main ──▶ #120 feature A ──▶ #124 feature A-2                  │
│                  approved           changes requested                 │
│                  Assignees: @a      Reviews 1/2 approved              │
│                  ▸ 2 included PRs                                      │
│                                                                      │
│ repo-b/main ──▶ #88 bug fix                                          │
└──────────────────────────────────────────────────────────────────────┘
```

### ノード

リポジトリノードは `owner/repo` と default branch を表示し、各リポジトリの最左列に固定する。

PR ノードには以下を表示する。

- `#番号`、タイトル、draft 状態
- author の avatar と login
- assignees
- reviewer 個人名の代わりに承認数 / reviewer 数（例: `Reviews 2/3 approved`）
- review decision、CI、conflict を表す icon と短い label
- CI の集約状態、merge conflict（取得可能な場合）
- 最終更新時刻

タイトルは GitHub の PR URL を新しいタブで開く。ノード全体はキーボードで focus 可能とし、Enter でも詳細を開ける。

### 背景色、枠線、状態表示

背景色はレビューや CI の結果ではなく、viewer と PR の関係だけを表す。判定優先順位は次のとおり。

| 優先度 | 背景 | 意味 |
|---:|---|---|
| 1 | 青 | viewer が author、または viewer が assignee の PR（「自分の PR」） |
| 2 | 緑 | viewer 個人または viewer が所属する team にレビュー依頼されている PR |
| 3 | 灰 | 上記以外の検索結果や、stack を構成するため補完取得した PR |

たとえば viewer が assignee かつ reviewer の場合は青を採用する。背景色には `My PR`、`Review requested`、`Other` の accessible label を対応させ、色だけに依存しない。dark mode と WCAG AA を考慮した CSS custom properties を使う。

枠線は draft 状態を表す。ready for review の PR は太い実線、draft は細い実線とし、draft には `Draft` badge も表示する。これにより背景色と直交した二つの軸として判別できる。

レビュー状態、CI、conflict は背景色や枠線を変更せず、ノード下部の status row に icon と label で表示する。

```text
✓ Reviews 2/3 approved   ● CI passing   ! Conflict
```

- Reviews: 緑の check / 黄の clock / 赤の change-requested icon
- CI: success / pending / failed / skipped / unknown の icon と label
- Conflict: conflict がある場合のみ赤い warning icon と `Conflict` label

reviewer の avatar や login は通常表示しない。review summary の分子は、dismissed でない最新レビューが `APPROVED` の reviewer 数とする。分母はレビューを提出した reviewer と未回答の individual review request を login で重複排除した人数とする。team review request は team 全体を 1 枠として分母に数え、所属メンバーの approved review があれば承認済みとみなす。GitHub から team membership を確認できず集約不能な場合は、誤った数値を出さず `Reviews 2 approved · team pending` のように分けて表示する。

### 自動更新

- graph 表示完了から 5 分ごとに現在の検索条件を再取得する。
- header に最終更新時刻、次回更新までの時間、自動更新の on/off toggle を表示する。既定は on。
- 初回取得と更新の間は header 直下に progress bar と `Loading pull requests…` を表示し、処理中であることを常に確認できるようにする。
- browser tab が非表示または端末が offline の間は polling を止め、再表示／online 復帰時に前回更新から 5 分以上経っていれば即座に更新する。
- 更新中も現在の graph を残し、header の小さな progress 表示だけを出す。成功後は node/edge の差分を反映し、pan、zoom、選択、折りたたみ状態を維持する。
- 自動更新の失敗時は既存 graph を保持し、非遮断型の警告と再試行ボタンを表示する。失敗直後の自動再試行は exponential backoff とし、手動更新は常に可能にする。
- 検索条件が変更された場合は polling timer をリセットする。同じ query に対する同時リクエストは 1 件にまとめる。

### 折りたたみ

現在の PR の head 履歴に取り込まれている merged PR を、ノード下部の `Included PRs (N)` に折りたたんで表示する。展開行には番号、タイトル、author、mergedAt、リンクを表示する。

表示状態（検索文字列、filter、折りたたみ、viewport）は URL query parameter に保存し、再読み込みと URL 共有に耐えるようにする。ただしサーバーはローカルなので URL 自体を他端末から開く用途は想定しない。

## 4. グラフの意味

グラフは左から右へ流れる有向非巡回グラフとして描画する。

### ノードとエッジ

- リポジトリノード: `owner/repo@defaultBranch`
- PR ノード: GitHub GraphQL の global node ID で一意化
- root edge: PR の `baseRefName == repository.defaultBranchRef.name`
- stack edge: PR A の `headRefName` と PR B の `baseRefName` が一致し、head repository も一致するとき `A -> B`

fork 由来ブランチの衝突を避けるため、ブランチの identity は単なる名前ではなく `(repository ID, ref name)` とする。base にしている PR が検索結果に含まれない場合は、軽量な「外部ブランチ」ノードを補い、孤立して見えないようにする。

同じ head branch を持つ複数 PR、削除済み branch、循環的に見える不正データは例外として扱う。edge の確度を `exact | inferred` で保持し、exact edge を優先する。循環を検出した場合は更新日時の古い inferred edge を切り、警告 badge を付ける。

### 下流 stack の再帰探索

既定検索またはユーザー検索に直接一致した PR だけではなく、その後続タスクも表示する。検索結果を seed PR とし、各 seed の head branch を base にする open PR を取得する。見つかった PR の head branch について同じ探索を繰り返し、下流の stack を再帰的に補完する。

```text
検索に一致した #120
  head: feature-a
       └─ base: feature-a の #124
             head: feature-a-2
                  └─ base: feature-a-2 の #129
```

この例では #124 や #129 がまだ viewer へのレビュー依頼前で、元の検索条件に一致しなくても表示される。各補完 PR にも通常と同じ関与判定を行うため、viewer が author / assignee なら青、それ以外は灰色になる。

探索規則は以下とする。

1. queue を seed PR で初期化し、breadth-first で探索する。
2. PR A の head identity `(headRepository ID, headRefName)` と一致する base identity を持つ open PR B を取得し、`A -> B` を張る。
3. B が未訪問なら queue に追加して B の後続も探索する。
4. GraphQL global node ID の visited set で重複と循環を防ぐ。
5. 1 ブランチから複数の PR が派生していれば、すべて表示して分岐させる。
6. 削除済み head repository / branch は探索を終了し、警告 badge を付ける。

GitHub API の暴走と巨大 graph を防ぐため、初期上限を seed を含め 500 PR、深さ 20、1 branch あたり 100 PR とする。いずれかを超えた場合は途中結果を表示し、切り詰めた branch に `More downstream PRs` ノードと warning を出す。検索を更新するたびに seed から再探索し、自動更新でも新しく作成された後続 PR を発見する。

### レイアウト

- rank 0 に repository root、依存を 1 rank ずつ右へ置く
- repository ごとに独立した横方向の lane を作り、その repository node と PR node を同じ lane 内へ配置する
- すべての階層を tidy-tree として再帰配置し、単一の親子は同じY座標、分岐した親は子サブツリー全体の中央に置く
- DOM描画後に各カードの実寸を測り、サブツリーの高さをbottom-upで求めて重なりを防ぐ
- DAGで複数の親を持つPRは主エッジを1本選んで配置し、その他のエッジも描画する
- repository node はrank 1の全サブツリーに対して中央配置する
- 同一 repository / rank 内は stack root、PR number の順で安定ソートする
- edge crossing を減らすため barycenter ordering を適用する
- 初期版は `@xyflow/react` の custom node + `dagre` を推奨する
- 500 ノードを超える場合は描画を仮想化するか、検索条件の絞り込みを促す

## 5. Included PR の判定

「PR 内にマージされた」の定義は曖昧になりやすいため、次の厳密な意味に固定する。

> merged PR の merge commit が、表示対象 PR の head commit から到達可能であり、同 PR の base commit からは到達できない。

つまり、対象ブランチが今回追加する履歴の中に merge commit がある場合だけ included とする。既に default branch に入っている過去 PR は除外する。

GitHub API だけで全候補に compare API を実行すると rate limit が厳しいため、段階的に取得する。

1. GraphQL で対象 PR の base/head OID と、head 側 commit history をページング取得する（初期上限 250 commits）。
2. commit の `associatedPullRequests` と merge commit OID から merged PR 候補を集める。
3. base 側にも存在する commit は除外する。
4. squash/rebase merge は merge commit だけでは完全判定できないため、初期版では `exact` と断定せず `possibly included` とする。
5. 250 commits を超えた場合は結果を `truncated: true` とし、UI に「一部のみ」と表示する。

この機能は高コストなので、グラフ本体の表示後に `/api/pr/:id/included` から lazy load する。結果は head/base OID を key にキャッシュする。

## 6. アーキテクチャ

単一バイナリ配布を重視し、以下を推奨する。

| 層 | 技術 | 理由 |
|---|---|---|
| CLI / server | Go | gh extension として単一バイナリ配布しやすい |
| GitHub client | `github.com/cli/go-gh/v2` | gh の host、認証、REST/GraphQL 設定を再利用 |
| Frontend | TypeScript + React + Vite | graph UI と状態管理に適する |
| Graph UI | `@xyflow/react` + dagre | custom node、pan/zoom、DAG layout |
| frontend 配布 | Go `embed.FS` | 実行時に Node.js 不要 |

```text
gh-pr-graph (Go process)
├── CLI flags / signal handling / browser launcher
├── HTTP server (127.0.0.1:random)
│   ├── embedded SPA
│   ├── JSON API
│   └── Server-Sent Events (progress / refresh)
├── GitHub service
│   ├── search + hydration
│   ├── included-PR resolver
│   └── rate-limit aware cache
└── Graph builder (pure functions, testable without GitHub)
```

frontend に GitHub token を渡さない。すべての GitHub 通信を Go process 経由にし、API は loopback のみで公開する。起動時にランダムな session secret を生成し、変更系 API が加わる場合に備えて custom header と Origin check を行う。

## 7. GitHub API 取得方針

### 認証と host

- `go-gh` を通して現在の `gh auth` を利用する
- `--hostname` または `GH_HOST` を尊重し、GitHub Enterprise Server に対応する
- token や API response 全体をログへ出さない
- 必要 scope が不足した場合、必要な `gh auth refresh` コマンドを具体的に案内する

### 検索と hydration

GraphQL `search(type: ISSUE)` で node ID の集合を取得し、PR fragment で表示項目を hydrate する。既定条件は 3 query を並列実行し、global node ID で重複排除する。

hydrate 後、各 PR の `(headRepository ID, headRefName)` に対して対象 repository の `pullRequests(baseRefName: ..., states: OPEN)` を問い合わせ、下流 PR を breadth-first で再帰取得する。branch identity ごとに request をまとめ、同じ branch を複数 seed が参照しても API call を重複させない。取得した PR は検索への直接一致か補完取得かを `source: search | downstream` として保持する。

主な取得フィールド:

```text
id, number, title, url, state, isDraft, createdAt, updatedAt
author { login avatarUrl }
repository { id nameWithOwner url defaultBranchRef { name target { oid } } }
baseRefName, baseRefOid, headRefName, headRefOid
headRepository { id nameWithOwner }
assignees, reviewRequests, reviews, latestReviews
reviewDecision, mergeable, statusCheckRollup
```

GraphQL cost を抑えるため connection は小さい上限から開始し、assignee/reviewer が多い場合のみ追加ページを取る。検索結果は 100 件単位でページングし、UI へ進捗を通知する。

### キャッシュ

- memory cache: query と viewer ごとに 60 秒
- included PR cache: `(host, prID, baseOID, headOID)` ごとにプロセス存続中保持
- `Refresh` は query cache を無効化する
- 5 分 polling は query cache を再検証し、前回と同じ head OID の included PR cache は再利用する
- 初期版では disk cache を持たず、token や private repository 情報を残さない
- rate limit 到達時は古い結果を保持したまま reset time を表示する

## 8. ローカル API

```text
GET  /api/v1/session
GET  /api/v1/graph?q=...&state=open&cursor=...
GET  /api/v1/pr/{node-id}/included
POST /api/v1/refresh
GET  /api/v1/events                 # SSE
GET  /healthz
```

`GET /api/v1/graph` の概略 response:

```json
{
  "nodes": [
    {"id":"repo:R_1","kind":"repository","rank":0,"data":{}},
    {"id":"pr:PR_1","kind":"pullRequest","rank":1,"data":{}}
  ],
  "edges": [
    {"id":"e1","source":"repo:R_1","target":"pr:PR_1","confidence":"exact"}
  ],
  "pageInfo":{"hasNextPage":false,"cursor":null},
  "rateLimit":{"remaining":4200,"resetAt":"..."},
  "warnings":[]
}
```

domain model と API DTO は分離する。API は `/v1` で versioning し、frontend と backend の build version が違う場合は再読み込みを促す。

## 9. パッケージ構成

```text
cmd/gh-pr-graph/main.go
internal/
  app/              # 起動、終了、依存組み立て
  github/           # GraphQL/REST client と query
  graph/            # node/edge 構築、cycle 処理、rank 計算
  included/         # commit 到達性と included PR 判定
  server/           # HTTP handler、SSE、embed assets
  cache/
web/
  src/components/
  src/features/search/
  src/features/graph/
  src/api/
  src/theme/
  package.json
docs/
  api.md
```

実行ファイル名は gh extension の規約に合わせて `gh-pr-graph` とする。release artifact は主要 OS/architecture 向けに生成し、extension repository の release から `gh extension install OWNER/gh-pr-graph` で導入できる形にする。

## 10. エラーと edge case

- 未認証: `gh auth login` を案内して終了
- repository 権限消失: 該当 PR だけ warning として除外
- head branch 削除済み: PR は表示し、stack edge を張らず badge を付ける
- GitHub API 一時障害: exponential backoff、既存画面を保持
- secondary rate limit: 自動連打せず reset/retry 時刻を表示
- PR が 0 件: 検索例を含む empty state
- 同名 branch: repository ID を含めて判定
- レビュー状態不明: 灰色。レビュー一覧から独自に required approval を推測しない
- private repository: browser console、URL、永続 cache に内容を漏らさない

## 11. テスト戦略

- graph builder: table-driven test（単純、分岐、stack、fork、branch 欠落、cycle）
- downstream discovery: 複数階層、分岐、seed 合流、循環、重複排除、上限到達のテスト
- included resolver: commit DAG fixture による到達性テスト
- GitHub client: `httptest.Server` と固定 GraphQL response による pagination/cost/error テスト
- HTTP API: handler contract と Origin/session validation
- frontend: component test、keyboard navigation、色以外の状態表現
- auto refresh: fake timer による 5 分間隔、tab visibility、offline、失敗時 backoff、UI state 維持のテスト
- E2E: mock GitHub API を使い、起動から検索、展開、リンクまで Playwright で検証
- release: Linux/macOS/Windows の smoke test と `gh extension install` 検証

実リポジトリへアクセスするテストは opt-in にし、通常 CI では token を必要としない。

## 12. 実装フェーズ

### Phase 1: 最小価値

- gh extension scaffold、認証、ローカル server、browser open
- 既定 3 検索と任意検索
- repository / PR node、base/head による stack edge と下流 stack の再帰探索
- 番号、タイトル、author、assignee、reviewDecision の表示
- 青 / 緑 / 灰の関与別背景、draft / ready の枠線、review / CI / conflict label
- reviewer の承認数集約表示
- pan/zoom、手動 refresh、5 分ごとの自動 refresh、loading/error/empty state

### Phase 2: 状態の充実

- CI、mergeability、review request/team 集約の精度向上
- dark mode、keyboard navigation
- pagination、rate-limit UI、SSE progress

### Phase 3: Included PR

- commit history 取得と lazy resolver
- exact / possibly included / truncated 表現
- fixture と大規模履歴の performance test

### Phase 4: 配布品質

- multi-platform release、checksum、installation docs
- GitHub Enterprise Server の compatibility test
- telemetry なしで診断可能な `--debug` ログ

## 13. 初期版の受け入れ条件

1. `gh pr-graph` で 5 秒以内（API 応答時間を除く）に browser が開く。
2. 既定状態で author / assignee / review-requested の open PR の和集合を表示する。
3. default branch を base にする PR が repository node の右に位置する。
4. 表示対象 PR の head branch を base にする PR が、さらに右に接続される。
5. 検索に直接一致しない後続 PR も、各 head branch を起点に再帰探索され、stack の末端または安全上限まで表示される。
6. PR node から GitHub PR を開け、番号、タイトル、author、review、assign 状態を確認できる。
7. author または assignee の PR は青、レビュー依頼された PR は緑、その他は灰色の背景で表示される。複数該当時はこの順に優先する。
8. ready for review は太枠、draft は細枠と `Draft` badge で識別できる。
9. reviewer は個人一覧ではなく `承認数 / reviewer 数` で表示され、レビュー、CI、conflict は icon と label で識別できる。
10. 5 分ごとに自動更新し、graph の viewport と展開状態を維持する。非表示 tab と offline 中は polling しない。
11. included PR の取得失敗や truncation が graph 全体の表示を妨げない。
12. GitHub token が frontend、ログ、disk cache に露出しない。

## 14. 先に固定する設計判断

- 初期版は読み取り専用とする。
- stacked PR edge はブランチ identity による exact match を基本とする。
- 検索結果を seed とし、head branch を base にする open PR を再帰的に補完する。
- included PR は commit 到達性で定義し、squash/rebase は「可能性あり」と区別する。
- backend は Go、frontend は React、単一バイナリへ埋め込む。
- 複数 repository を一画面に表示し、repository ごとに独立した root を持つ forest とする。

将来候補として、ノードからの approve/assign/merge、saved searches、通知、team dashboard があるが、token scope、CSRF、誤操作防止の設計が増えるため初期版からは外す。
