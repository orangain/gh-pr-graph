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

既定の検索集合は以下 3 検索の和集合（PR ID で重複排除）とする。画面では各条件を独立したcheckboxとして表示し、GitHub 検索構文における複雑な OR の解釈差を避けるため、API 内部でも別々に実行する。

```text
is:open author:@me
is:open assignee:@me
is:open review-requested:@me
```

画面上部の検索欄では GitHub PR 検索構文を受け付け、選択中のcheckboxに追加する第4のOR条件として扱う。3つのcheckboxをすべて外せば、入力した検索条件だけを実行できる。`Show bots`はグラフ右上に浮かせる独立したclient-side表示filterとし、PRの見落としを避けるため既定はONにする。状態はURLへ保存せず、ページを開くたびにONへ戻す。切り替え時にAPIを再取得せず、保持した元graphから即座に再描画する。OFFの場合はメイングラフ上のBot authorノードだけを隠し、stack途中のBotノードについては前後のedgeを接続して関係を維持する。Included PR一覧内のBot PRは表示したままにする。

## 3. 画面設計

```text
┌──────────────────────────────────────────────────────────────────────┐
│ PR Graph [✓Authored][✓Assigned][✓Review requested] [OR query] [検索] │
│                                                           ↻         │
│           [Open ✓] [Draft ✓]  Repository: All   Updated 12:34 (自動)│
├──────────────────────────────────────────────────────────────────────┤
│                                               [✓ Show bots]          │
│ repo-a/main ──▶ #120 feature A ──▶ #124 feature A-2                  │
│                  approved           changes requested                 │
│                  Assignees: @a      Reviews 1/2 approved              │
│                  ▸ 2 included PRs                                      │
│                                                                      │
│ repo-b/main ──▶ #88 bug fix                                          │
└──────────────────────────────────────────────────────────────────────┘
```

### ノード

リポジトリノードは `owner/repo` と default branch を表示し、各リポジトリの最左列に固定する。上流を辿ってもdefault branchへ到達しない場合は、そのbase branchを独立したブランチノードとして表示し、ブランチ名からGitHubのbranch pageを開けるようにする。

PR ノードには以下を表示する。

- `#番号`、タイトル、draft 状態
- author の avatar と login
- assignees
- reviewer 個人名の代わりに承認数 / reviewer 数（例: `Reviews 2/3 approved`）
- review decision、CI、conflict を表す icon と短い label
- CI の集約状態、merge conflict（取得可能な場合）
- 最終更新時刻
- タイトルと PR URL を Markdown リンクとしてコピーする copy ボタン

タイトルは GitHub の PR URL を新しいタブで開く。ノード全体はキーボードで focus 可能とし、Enter でも詳細を開ける。

タイトル行の末尾にPrimer Octicons `copy` のボタンを置き、クリックでタイトルとPR URLをクリップボードへ書き込む。レビュー依頼や作業メモをissue、PR、chatへ貼るときに、タイトルとURLを手作業で組み立てる必要をなくすことを目的とする。

貼り付け先の種類が異なるため、1回のコピーで2つのformatを同時に書き込む。`text/plain` は `[#番号 タイトル](PR URL)` のMarkdownリンクとし、GitHubのコメント欄やeditorで使えるようにする。`text/html` は同じラベルとURLの `<a>` 要素とし、Microsoft TeamsやOutlookのようなrich text editorでハイパーリンクとして貼れるようにする。貼り付け先はどちらか適した形式を選ぶ。

Markdown側はタイトルに含まれる `[`、`]`、`\` をエスケープしてリンクが壊れないようにする。HTML側は `<a>` 要素をDOMで組み立てて `outerHTML` を取ることで、属性値と文字参照のescapeを自前で持たない。

`ClipboardItem` を使える場合は `navigator.clipboard.write` で両formatを書き込み、使えない場合は `navigator.clipboard.writeText`、さらに古い環境では一時的な `textarea` と `document.execCommand('copy')` へfallbackし、Markdown側だけを書き込む。いずれも失敗した場合はエラー表示へ回す。

コピー成功のフィードバックは、押したボタンのiconをPrimer Octicons `check` へ2秒間差し替えて示す。視線をカードから移動させずに済み、GitHubのcopy buttonと同じ挙動になる。別のボタンを押した場合は、直前のボタンを元のiconへ戻してから新しいボタンを`check`にする。iconの変化を知覚できない利用者向けに、視覚的に隠した `role="status"` のlive regionへコピー完了を通知する。ボタンは16pxに収めてカード寸法とタイトル幅へ影響させず、既定はmuted色、hoverで通常のテキスト色にする。

### 背景色、枠線、状態表示

背景色はレビューや CI の結果ではなく、viewer と PR の関係だけを表す。判定優先順位は次のとおり。

| 優先度 | 背景 | 意味 |
|---:|---|---|
| 1 | 青 | viewer が author の PR（「自分の PR」） |
| 2 | シアン | viewer が assignee の PR |
| 3 | 緑 | viewer 個人または viewer が所属する team にレビュー依頼されている PR |
| 4 | 灰 | 上記以外の検索結果や、stack を構成するため補完取得した PR |

たとえば viewer が author かつ assignee の場合は青、assignee かつ reviewer の場合はシアンを採用する。背景色には `My PR`、`Assigned`、`Review requested`、`Other` の accessible label を対応させ、色だけに依存しない。dark mode と WCAG AA を考慮した CSS custom properties を使う。

枠線は draft 状態を表す。ready for review の PR は太い実線、draft は細い実線とする。さらにタイトル先頭へPrimer Octiconsの`git-pull-request`または`git-pull-request-draft`を表示し、状態を統一した視覚表現で判別できるようにする。

`reviews(states:[PENDING])`にauthorがviewerのreviewがあるPRは、viewerがreview commentをまだ投稿していない状態と判定する。`viewerLatestReview`はこの未投稿reviewを返さないため判定には使わない。カード右上の枠へ重なる円形badgeにPrimer Octicons `comment-discussion`を表示し、`Pending review — submit your comments`のtooltipとaccessible labelを付ける。

現在viewerへレビュー依頼されており、viewerの直近レビューより後に同じviewerへの`ReviewRequestedEvent`があるPRは再レビュー待ちと判定する。カード右上の枠へ重なる円形badgeにPrimer Octicons `sync`を表示し、`Re-review requested`のtooltipとaccessible labelを付ける。再レビューとpending reviewのbadgeは同じ注意色を使う。両方が同時に成立する場合は、未投稿コメントを忘れるリスクを優先してpending review badgeだけを表示する。badgeは絶対配置してタイトル幅とカード寸法へ影響させない。背景色と枠線の意味は変更しない。timelineは検索結果について直近50件を取得し、上流・下流探索のbranch queryには含めない。上限外の古い履歴は再レビュー判定の対象外とする。

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
- header左上のbrandをprojectのGitHub pageへのlinkとし、hoverまたはkeyboard focus時だけbuild versionを表示する。browser tab titleはスクリーンショットやタブ一覧で簡潔に見えるようversionを含めない。headerには最終更新時刻、次回更新までの時間、自動更新の on/off toggle を表示する。既定は on。
- 初回取得中だけheader直下にprogress barと`Loading pull requests…`を表示する。初回表示後の自動・手動更新では、progress領域の出現によるviewportの縦ずれを避けるため表示しない。
- browser tab が非表示または端末が offline の間は polling を止め、再表示／online 復帰時に前回更新から 5 分以上経っていれば即座に更新する。
- 更新中も現在のgraphをそのまま残し、成功後にnode/edgeの差分を反映してviewportを維持する。
- 自動更新の失敗時は既存 graph を保持し、非遮断型の警告と再試行ボタンを表示する。失敗直後の自動再試行は exponential backoff とし、手動更新は常に可能にする。
- 検索条件が変更された場合は polling timer をリセットする。同じ query に対する同時リクエストは 1 件にまとめる。

### 折りたたみ

現在の PR の最新100 commitに取り込まれているmerged PRがある場合、status rowの下にOcticonsの`chevron-right`と`Included PRs (N)`を表示する。100件より古いcommitがある場合は件数を下限として`Included PRs (N+)`、候補を1件も検出できなければ`Included PRs (?)`とする。`(?)`は展開不可とする。`N`と`N+`はクリックで`chevron-down`へ変化し、検出した候補を最新merge commit順に表示する。

検索文字列と関係scopeは URL query parameter に保存し、再読み込みと URL 共有に耐えるようにする。`Show bots`、折りたたみ、viewportはセッション中だけの状態とする。ただしサーバーはローカルなので URL 自体を他端末から開く用途は想定しない。

## 4. グラフの意味

グラフは左から右へ流れる有向非巡回グラフとして描画する。

### ノードとエッジ

- リポジトリノード: `owner/repo@defaultBranch`
- ブランチノード: 上流の親PRがgraph内にないnon-default base branch。`(repository ID, ref name)`で一意化
- PR ノード: GitHub GraphQL の global node ID で一意化
- root edge: PR の `baseRefName == repository.defaultBranchRef.name`
- stack edge: PR A の `headRefName` と PR B の `baseRefName` が一致し、head repository も一致するとき `A -> B`

fork 由来ブランチの衝突を避けるため、ブランチの identity は単なる名前ではなく `(repository ID, ref name)` とする。

baseがdefault branchではなく、同じbaseをheadに持つ親PRもgraph内にない場合は、repository nodeの右にbase branch nodeを作る。repository nodeからbranch nodeへ破線edgeを、branch nodeからそのbranchをbaseにするPRへ通常のedgeを引く。同じrepository・base branchを持つ複数PRは1つのbranch nodeを共有し、PRとその下流stackはbranch nodeの分だけ右へ配置する。

同じ head branch を持つ複数 PR、削除済み branch、循環的に見える不正データは例外として扱う。edge の確度を `exact | inferred` で保持し、exact edge を優先する。循環を検出した場合は更新日時の古い inferred edge を切り、警告 badge を付ける。

### 上流 stack の再帰探索

検索に直接一致した各PRのbase branchがdefault branchでない場合、そのbase branchをheadに持つopen PRを親として取得する。見つかった親PRについても同じ探索を繰り返し、default branchまたは安全上限へ到達するまで上流のstackを補完する。

```text
head: feature-a の #118
       └─ 検索に一致した base: feature-a の #120
```

同じ深さのbranchはGraphQL aliasを使った1回のqueryへまとめる。branch identityは`(repository ID, ref name)`で照合し、forkにある同名branchのPRは親として採用しない。上流探索のseedは検索に直接一致したPRだけとし、上流で見つけた親から別の下流PRへ探索範囲を広げない。

### 下流 stack の再帰探索

既定検索またはユーザー検索に直接一致した PR だけではなく、その後続タスクも表示する。検索結果を seed PR とし、各 seed の head branch を base にする open PR を取得する。見つかった PR の head branch について同じ探索を繰り返し、下流の stack を再帰的に補完する。

```text
検索に一致した #120
  head: feature-a
       └─ base: feature-a の #124
             head: feature-a-2
                  └─ base: feature-a-2 の #129
```

この例では #124 や #129 がまだ viewer へのレビュー依頼前で、元の検索条件に一致しなくても表示される。各補完 PR にも通常と同じ関与判定を行うため、viewer が author なら青、assignee ならシアン、それ以外は灰色になる。

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

`GET /api/v1/graph`はIncluded PR未判定のメイングラフと各PRのhead commit SHA、base先端SHAを返す。初回ロード中のprogress textには現在の処理phaseと、目安のpercentに代えて重複排除後に収集済みのPR数を表示する。PR一覧が確定してgraphを描画した後は件数を外し、処理phaseだけを表示する。progress barの長さは従来どおりpercentを使う。browserはPR一覧が確定した時点でprogressを65%のままgraphを先に描画し、親PR単位で`POST /api/v1/inspect`を呼ぶ。commit inspectionは`commits(last:100)`の1requestだけを使い、最新commitから古いcommitの順に`Merge pull request #123`や`Merged ... #123`を走査してIncluded PR番号を重複排除する。`hasPreviousPage`がtrueでもpaginationせず、検出数を下限として扱う。全PRの候補数が確定した時点でgraphを更新してprogressを100%にする。この更新による多少のレイアウトシフトは許容する。検査結果は親PR IDごとに`(head commit SHA, base先端SHA)`をversionとして、順序付き候補番号とtruncated状態をメモリキャッシュし、両SHAが同じなら再走査しない。リロード時は新しい候補数が確定するまで前回のIncluded PR候補と詳細を同じPR IDへ引き継ぎ、トグルが一時的に消えることを防ぐ。候補setとtruncated状態が変わらなければ取得済みの詳細も維持する。

commit inspection responseには番号だけのIncluded PR候補を含め、browserの初期描画完了後に`POST /api/v1/included`を包含する親PR単位で自動実行して詳細を反映する。タイトル、URL、作者などの詳細は順序付き候補の先頭3件と末尾3件だけ取得し、候補が6件以下なら全件取得する。中間候補は`… N more found`のクリック後に番号リンクだけを表示し、追加requestを発生させない。truncatedの場合は検出済み候補の末尾に`and more…`を表示する。browserは親PR IDごとに順序付き番号とtruncated状態をsignatureとして詳細をメモリキャッシュし、次回更新でsignatureが同じなら通信せずキャッシュを反映する。変化した親PRだけを最大6並列で問い合わせ、完了した結果から画面へ反映する。標準的なmerge commit messageを残さないsquash/rebase mergeは検出対象外とする。

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
│   └── NDJSON stream (initial graph / included updates / progress)
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

GraphQL `search(type: ISSUE)` で node ID の集合を取得し、PR fragment で表示項目を hydrate する。選択された関係条件と任意の第4検索条件をそれぞれ並列実行し、global node ID で重複排除する。

hydrate 後、各 seed PR の `(repository ID, baseRefName)` から上流PRを、`(headRepository ID, headRefName)` から下流PRをbreadth-firstで再帰取得する。同じ深さのbranchはGraphQL aliasを使ってまとめるが、GitHub GraphQLの500,000 nodes上限に十分な余裕を持たせるため1 queryあたり20 branchまでとする。branch identityが重複する場合も問い合わせを1つにする。次の深さは直前の応答で判明するため、request数は概ねstackの深さに比例する。取得した PR は検索への直接一致か補完取得かを `source: search | upstream | downstream` として保持する。

主な取得フィールド:

```text
id, number, title, url, state, isDraft, createdAt, updatedAt
author { login avatarUrl }
repository { id nameWithOwner url defaultBranchRef { name target { oid } } }
baseRefName, baseRefOid, headRefName, headRefOid
headRepository { id nameWithOwner }
assignees, reviewRequests, reviews, latestReviews
timelineItems (ReviewRequestedEvent, PullRequestReview)
reviewDecision, mergeable, statusCheckRollup
```

GraphQL cost を抑えるため connection は小さい上限から開始し、assignee/reviewer が多い場合のみ追加ページを取る。検索結果は 100 件単位でページングし、UI へ進捗を通知する。

### キャッシュ

- memory cache: query と viewer ごとに 60 秒
- `Refresh` は query cache を無効化する
- 5 分 polling は query cache を再検証する
- 初期版では disk cache を持たず、token や private repository 情報を残さない
- rate limit 到達時は古い結果を保持したまま reset time を表示する

## 8. ローカル API

```text
GET  /api/v1/session
GET  /api/v1/graph?q=...&state=open&cursor=...
POST /api/v1/inspect
POST /api/v1/included
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
- stacked PR の探索失敗: 画面には対象 branch と console の確認案内だけを表示し、起動元の console には探索方向、対象 repository/branch、`gh api graphql` のエラー詳細を記録する。OTEL が有効な場合は対応する探索 span も error とする

## 11. テスト戦略

- graph builder: table-driven test（単純、分岐、stack、fork、branch 欠落、cycle）
- downstream discovery: 複数階層、分岐、seed 合流、循環、重複排除、上限到達のテスト
- included resolver: merge commit message、重複番号、現在のPR番号除外のテスト
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
- repository / branch / PR node、base/head による stack edge と下流 stack の再帰探索
- 番号、タイトル、author、assignee、reviewDecision の表示
- 青 / シアン / 緑 / 灰の関与別背景、draft / ready の枠線、review / CI / conflict label
- reviewer の承認数集約表示
- pan/zoom、手動 refresh、5 分ごとの自動 refresh、loading/error/empty state

### Phase 2: 状態の充実

- CI、mergeability、review request/team 集約の精度向上
- dark mode、keyboard navigation
- pagination、rate-limit UI、SSE progress

### Phase 3: Included PR

- commit message走査と候補PRの条件付き取得
- Included PRがあるノードだけの折りたたみ表示
- 大規模履歴のperformance test

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
6. 検索に直接一致した PR の base branch を head にする親PRも再帰探索され、default branchまたは安全上限まで表示される。
7. 上流を辿ってもdefault branchへ到達しないPRはbase branch nodeへ接続され、そのbranch nodeとrepository nodeの間が破線で表示される。
8. PR node から GitHub PR を開け、番号、タイトル、author、review、assign 状態を確認できる。
9. author の PR は青、assignee の PR はシアン、レビュー依頼された PR は緑、その他は灰色の背景で表示される。複数該当時はこの順に優先する。
10. ready for review は太枠、draft は細枠と `Draft` badge で識別できる。
11. reviewer は個人一覧ではなく `承認数 / reviewer 数` で表示され、レビュー、CI、conflict は icon と label で識別できる。
12. viewerのレビュー後に再度レビュー依頼されたPRは、カード右上の円形`sync` badgeで識別できる。
13. 5 分ごとに自動更新し、更新前にviewport中央付近の表示ノードと画面内offsetを記録して、更新後も同じノードが同じ位置に見えるようscrollを補正する。Included PR更新などで1frame内にrenderが連続する場合は、最初のanchorを保持して古い復元予約をcancelし、最後のrender後に一度だけ復元する。ノードが消えた場合は従来のscroll座標へfallbackする。非表示 tab と offline 中は polling しない。
14. メイングラフ取得後、browserはPR一覧が確定したgraphをprogress 65%の時点で描画する。commit inspectionのキャッシュを適用し、missした親PRだけ`POST /api/v1/inspect`で最新100 commitを走査する。全候補番号が揃ったらgraphを更新し、progress barを100%にして閉じる。古いcommitが残る場合は件数を`N+`または`?`で表し、paginationしない。リロード中は前回のIncluded PR情報を表示し続ける。100%到達後に先頭3件と末尾3件だけのPR詳細取得を自動開始するが、その進捗は表示せず、トグル操作も通信を発生させない。
15. GitHub token が frontend、ログ、disk cache に露出しない。

## 14. 先に固定する設計判断

- 初期版は読み取り専用とする。
- stacked PR edge はブランチ identity による exact match を基本とする。
- 検索結果を seed とし、head branch を base にする open PR を再帰的に補完する。
- Included PRは標準的なmerge commit messageから候補を検出し、merged PR情報を確認して確定する。
- backend は Go、frontend は React、単一バイナリへ埋め込む。
- 複数 repository を一画面に表示し、repository ごとに独立した root を持つ forest とする。

将来候補として、ノードからの approve/assign/merge、saved searches、通知、team dashboard があるが、token scope、CSRF、誤操作防止の設計が増えるため初期版からは外す。
