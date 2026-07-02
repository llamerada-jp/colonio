# リファクタリング計画 (優先度順)

長年の蓄積を整理するための計画。各項目に効果と予想難易度を付け、
「効果の大きさ ÷ 難易度」と依存関係を加味した優先度順に並べている。
KVS は開発中のため末尾の別節にまとめる (優先度 0〜3 の作業では
`node/internal/kvs/` 以下と `types/kvs/` には原則手を入れない)。

**前提条件**: 現在 kvs ブランチに未コミットの WIP (`node/internal/kvs/kvs.go`,
`simulator/base/*`, `spec/kvs/README.md`) がある。リファクタリング開始前にこれを
コミットするか退避し、main から作業ブランチを切ること。

**各項目共通の完了条件**: `make test` (unit + native e2e + WASM e2e) green、
`./ci.sh` 相当のチェック (lint / generate / format-code の差分なし、ライセンスヘッダ) が通ること。
1 項目 = 1 PR を原則とし、挙動を変えうる項目は着手前に該当箇所のテストを厚くする。

**凡例**

- 難易度 **低**: 半日〜1 日。局所的な変更で、既存テストでほぼ担保できる
- 難易度 **中**: 数日。複数ファイルに波及し、テストの追加が必要
- 難易度 **高**: 1 週間以上。設計判断・ワイヤ形式変更・広範な波及を含む
- 角括弧の ID (`N*` / `S*` / `P*` / `T*` / 旧 Phase) は旧構成の番号 (相互参照のため維持)

---

## 優先度 0 — 安全網と致命的リスク (最初にやる)

### 0-1. リモート入力での panic を除去 [N5 の一部] — 効果: 堅牢性 (致命バグ) / 難易度: 低

- [ ] `Transferer.Receive()` は未登録の content type を受け取ると panic する
      (transferer.go:284)。異バージョン・悪意ノードからのパケット 1 つでノードが落ちる。
      log + drop に変更する。全項目中で最優先

### 0-2. 静的解析の導入と機械的な近代化 [旧 Phase 1] — 効果: 保守性・安全網 / 難易度: 低 (量は多いが機械的)

挙動を変えない置き換えのみ。以降の作業の安全網を先に作る。

- [ ] `golangci-lint` (staticcheck, unused, govet 含む) を導入し、`ci.sh` と Makefile に組み込む。
      現状 lint は buf (protobuf) のみで Go コードの lint がない
- [ ] `golang.org/x/exp` への依存を除去
  - [ ] `simulator/datastore/mongodb.go` — `golang.org/x/exp/slog` → 標準の `log/slog` (明らかな旧 API の残骸)
  - [ ] `node/internal/network/routing/routing_2d.go` — `golang.org/x/exp/maps` → 標準 `maps` / `slices`
  - [ ] go.mod から `golang.org/x/exp` を削除
- [ ] `math/rand` → `math/rand/v2` へ移行 (`types/node_id.go`, `node/internal/geometry/*`,
      `node/internal/spread/spread.go`, `node/internal/network/{transferer,node_accessor}`,
      `seed/misc/logger.go`, `simulator/{base,utils}` の計 10 ファイル)
- [ ] `interface{}` → `any` (`node/internal/kvs/logger_*.go` は KVS 節で扱う。それ以外:
      `simulator/datastore/mongodb.go`, `node/internal/network/node_accessor/webrtc_link_wasm.go`,
      `test/cmd/luncher/main.go`)
- [ ] deadcode (`golang.org/x/tools/cmd/deadcode`) の指摘の精査と除去。
      公開 API (`node/node.go` / `seed/seed.go` の `WithX` オプション) はライブラリの入口なので残す。
      内部の真の死コード候補:
  - [ ] `node/internal/network/node_accessor/webrtc_link_native.go` の `isActive` / `isOnline` —
        インターフェース `webRTCLink` 経由でも呼ばれていない。インターフェース定義ごと削除を検討
  - [ ] `simulator/canvas/canvas.go` の `Canvas.Destroy` / `Canvas.Clear`
  - [ ] `simulator/datastore/reader.go` の `Reader.Close` (呼び忘れならむしろ呼ぶよう修正)
  - [ ] `test/util/util.go` のヘルパー群のうち実際に未使用のもの
- [ ] TODO コメントの棚卸し: 全 11 箇所 (KVS 内 6 箇所を除くと 5 箇所) を Issue 化するか
      本計画の該当項目に割り当て、コード上のコメントは Issue 番号付きに整理

### 0-3. 計測基盤の整備 [P0] — 効果: 単独では無し (以降の全性能項目の前提) / 難易度: 中

性能系の項目 (P1〜P12, T1〜T6) は必ず before/after を測って適用する。その土台。

- [ ] ベンチマークの整備: 2 ノード間のスループット / レイテンシ、多段中継 (hop 数を変えて)、
      近隣 multicast、の 3 シナリオを `go test -bench` か simulator 上で再現可能にする
- [ ] `observation` にメトリクスを追加: リンクごとの送受信バイト数・パケット数、
      送信キュー深度、断片化率 (PacketBaseBytes 超過の割合)、transferer の再送回数、
      routing 交換パケットのサイズと頻度
- [ ] simulator に性能測定シナリオを追加し、リファクタリング前のベースラインを記録する

---

## 優先度 1 — 効果大 (速度・堅牢性・利用者体験に直接効く)

### 1-1. モジュール分割と依存の整理 [旧 Phase 2] — 効果: 利用者体験◎ (依存の大幅削減) / 難易度: 中

ライブラリ利用者の go.sum に simulator / test 専用の重い依存
(go-sdl2, mongo-driver, k8s.io/api, chromedp, cobra, delaunay) が混入している。

- [ ] `simulator/` を独立した Go モジュールにする (`simulator/go.mod`)。
      simulator は既に独立した Makefile / Dockerfile を持つ別プログラムであり、自然な分割点
  - [ ] simulator → 本体は `github.com/llamerada-jp/colonio` を require (ローカルは go.work で解決)
  - [ ] simulator が `test/util` を import している (`simulator/base/node.go`) —
        必要なヘルパーを simulator 側へ複製するか、本体側の公開可能な場所へ移す
- [ ] `test/cmd/luncher` (chromedp 依存) の扱いを決める: test 用モジュールに分離するか、
      WASM テストランナーとして本体に残すか。残す場合でもルート go.mod から
      sdl2 / mongo / k8s が消えるだけで利用者の負担は大幅に減る
- [ ] 分割後にルート `go.mod` を `go mod tidy` し、ライブラリとして最小の依存
      (connect, pion/webrtc, raft, protobuf, uuid, gorilla/sessions, xxh3 程度) になることを確認
- [ ] `go.work` を用意し、ルートでの開発体験 (make test 一発) を維持する。CI も更新
- [ ] ディレクトリ名 typo の修正: `test/cmd/luncher` → `test/cmd/launcher` (難: 低)

### 1-2. PacketBaseBytes のデフォルト引き上げ [P3 の一部] — 効果: 速度 (断片化の大幅減) / 難易度: 低

- [ ] デフォルト 4096 は小さすぎる。現行ブラウザ/pion は 64KiB のメッセージを問題なく
      扱える (SDP の max-message-size で確認可能)。デフォルトを引き上げて断片化
      (`stack` による再結合, node_link.go:464-509) の発生頻度を大きく下げる。
      値の変更だけなので低コスト、P0 のベンチで効果を確認して確定する

### 1-3. 送信バッファの O(n²) 解消と back-pressure [P4] — 効果: 速度・メモリ堅牢性 / 難易度: 低〜中

- [ ] `sendPacket` が呼ばれるたびにキュー全体を走査して合計バイト数を再計算している
      (node_link.go:224-229)。送信が積まれるほど O(n²) になるので累積カウンタにする (難: 低)
- [ ] bufferedAmount を一切監視していない (native / wasm とも)。受信側が遅いと
      pion・ブラウザの送信バッファに無制限に積まれ、メモリ膨張や送信失敗になる。
      `BufferedAmountLowThreshold` / `onbufferedamountlow` による back-pressure と、
      キュー上限 + ドロップポリシー (制御 > 応答 > one-way の優先度) を入れる (難: 中)
- [ ] `BufferInterval` (10ms) / `PacketBaseBytes` が `NewNode` 内にハードコード
      (N9 と同件) — 性能チューニングのために Config へ昇格する (難: 低)

### 1-4. Delaunay 再三角形分割の抑制 [P9] — 効果: 速度◎ (2D 使用時の常時 CPU 負荷) / 難易度: 低〜中

- [ ] `neighborNodeIDChanged()` は呼ばれるたびに全ノードの点集合を作り直して
      `delaunay.Triangulate` を実行する (routing_2d.go:267-350)。これが
      updateNodeConnections / updateLocalPosition / recvRoutingPacket の
      3 経路すべてから呼ばれるため、移動中のノードや隣接ノードからの routing パケット
      受信のたびに O(n log n) の再計算 + 大量のアロケーションが走る
- [ ] 対策案 (P0 で頻度と所要時間を測ってから):
      位置変化が `geometry.GetPrecision()` 未満なら skip、
      複数の更新契機を 1 秒 tick に集約して 1 回だけ再計算 (debounce)、
      点集合スライスの再利用でアロケーション削減
- [ ] `getNextStep` (routing_2d.go:82) はパケット転送のホットパスで routeInfos を
      線形走査する。隣接ノード数が小さい前提なら問題ないが、その前提をコメントで明示する

### 1-5. パケット転送パスの一本化と lazy decode [N6 + P1 + P2] — 効果: 速度◎ (中継コスト) + 可読性 / 難易度: 高

構造改善 (N6) と性能改善 (P1) がちょうど噛み合うので 1 つの設計でまとめて行う。

- [ ] 現状、パケットは受信のたびに `PacketContent` 全体を Unmarshal し (node_link.go:518)、
      送信のたびに Marshal している (node_link.go:203)。中継ノードは content の中身を
      解釈せず、経路決定はヘッダ (dst/src/mode) だけでできるのに、hop ごとに
      ペイロード全体のデコード/エンコードが走る [P1]
- [ ] `Packet.Content` をワイヤ形式 (bytes) のまま保持し、最終宛先で初めて Unmarshal する
      (lazy decode)。中継はヘッダの書き換えのみになり、hop あたりのコストが
      ペイロードサイズに依存しなくなる [P1]
- [ ] `NodeNeighborhoods` 宛の多重送信はリンクごとに同一 Content を隣接ノード数だけ
      Marshal し直している (node_accessor.go:465-473 → node_link.go:203)。
      lazy decode 化でシリアライズ済み bytes の共有として自然に解決する [P2]
- [ ] hop 制限 (`checkHopCount`)、`classifyPacket`、relay 失敗処理が `Network` に、
      経路決定が routing に分散している。受信 → hop チェック → 経路決定 →
      配送/中継/エラー応答の一連を 1 つの forwarder コンポーネントにまとめ、
      `Network` は配線だけにする [N6]
- [ ] `Packet` の変異問題: `checkHopCount` が受信パケットを直接インクリメントし
      (network.go:240)、近隣多重送信は shallow copy で `Content` ポインタを共有したまま
      `DstNodeID` を差し替える (node_accessor.go:466-467)。Packet を「作成後は不変、
      転送時はヘッダのみ差し替えた新インスタンス」というモデルに整理する [N6]

### 1-6. NodeID の値セマンティクス化と「宛先」概念の分離 [N1] — 効果: 可読性◎ + panic 除去 / 難易度: 中〜高 (波及は広いが機械的)

- [ ] `*types.NodeID` のポインタ渡しをやめ、値渡し (`types.NodeID`) に統一する。
      NodeID は 20 バイトの comparable struct であり、現状のポインタ運用が
      `Copy()` の頻発、`Equal` の nil 三値ロジック、
      「DO NOT USE NodeID pointer for map key」(node_id.go:41) という footgun コメント、
      `nodeID2link` 系 map での値/ポインタ混在を生んでいる。値化ですべて消える
- [ ] 「ノードの識別子」と「パケットの宛先」の分離。`NodeLocal` / `NodeNeighborhoods` という
      センチネル値が NodeID 型に同居しているため、`Proto()` / `Raw()` / `Add()` が
      型フィールドを見て panic する設計になっている (node_id.go:104,129,174)。
      宛先を `Destination` (Normal(NodeID) | Local | Neighborhoods) として別型にすれば、
      `classifyPacket` (network.go:245) の分岐が型で表現でき、panic 箇所が消える。
      1-5 の forwarder 設計と相性が良いので先行または同時に行う

### 1-7. 受信側の重複排除 (二重実行の防止) [T2] — 効果: 正しさ (副作用の二重実行) / 難易度: 中

- [ ] 現在の再送は at-least-once で、受信側に重複検出がない: 応答だけが遅延・喪失した場合、
      再送された要求は受信側でもう一度ハンドラ実行される (transferer.go:278-287 は
      無条件 dispatch)。副作用のある操作 (messaging ハンドラ等) が二重実行される
- [ ] (SrcNodeID, packet ID) の短期 seen キャッシュを受信側に置き、重複要求には
      ハンドラを再実行せず前回の応答を再送する (応答キャッシュ)。保持期間は
      再送ウィンドウ (T1 の RTO × 再送回数) から導出する。spread が既に
      cache による重複抑制を持っている (spread.go) ので、モデルを揃える
- [ ] 上記を入れないと決めた場合は「ハンドラはべき等であるべき」を公開 API の契約として
      明文化する (この場合は難: 低)

### 1-8. 適応的再送タイムアウト [T1, P7 を吸収] — 効果: 速度 (失敗時レイテンシ最大 ~60 秒 → 秒オーダ) / 難易度: 中

- [ ] transferer の再送は固定 10 秒 × 5 回 (transferer.go:34-35) で 1 秒粒度のスキャン駆動。
      TCP (RFC 6298) に倣い、応答時間から SRTT / RTTVAR を推定して RTO を計算し、
      再送のたびに指数バックオフする
- [ ] Karn のアルゴリズムを踏襲する: 再送したパケットへの応答は RTT サンプルに使わない
- [ ] `Request` に context を渡せるようにし、呼び出し側が要求単位の deadline を
      制御できるようにする [N5 の一部]

### 1-9. 接続要求・シグナリングの資源保護 [T4] — 効果: 堅牢性 (悪意・過負荷耐性) / 難易度: 中

- [ ] node 側: `offerID2state` が無制限 (node_accessor.go:72)。リモートからの offer
      連打で connectingState + PeerConnection がタイムアウト (30 秒) まで無制限に積まれ、
      メモリ・fd を食い潰せる。同時 connecting 数の上限 (全体 + ピア単位) を設け、
      超過時は即 reject の answer を返す
- [ ] seed 側: server にレート制限が一切ない (seed/server/server.go)。AssignNode 連打で
      NodeID 空間とノード台帳を汚染でき、SendSignal 連打で任意ノードへ offer を
      流し込める。接続元 (session / IP) 単位のレート制限と、1 セッションあたりの
      割当ノード数上限を入れる
- [ ] signalChannels のバッファ (controller.go:317 の 100) があふれた場合の挙動を確認し、
      過負荷時に古いシグナルから捨てる等のポリシーを明示する

### 1-10. seed ホットパスの O(n) 解消と isAlone 見直し [旧 Phase 5 + S3] — 効果: 速度 (seed のスケール上限) / 難易度: 中

- [ ] `GetNextByMap` の O(n) 全走査 (seed/misc/map_helper.go, TODO 明記済み) の解消:
      gateway のノード管理をソート済み構造 (ソート済みスライス + 二分探索で十分) にし、
      `SimpleGateway` の `GetNodesByRange` 等も同じ構造に乗せる
- [ ] `isAlone` が AssignNode / Keepalive / ReconcileNextNodes / SendSignal の各所で
      毎回 `GetNodeCount` + `GetNodesByRange` の 2 往復で計算される (controller.go:370-394)。
      判定と利用の間にレース窓 (TOCTOU) もある。gateway 操作の戻り値に含めるか、
      ノード数変化イベントで保持するモデルに変え、往復と競合を減らす。
      分散バックエンドでは厳密な isAlone は原理的に近似になるので、その旨も仕様化する [S3]

---

## 優先度 2 — 構造・可読性の本丸と中規模改善

### 2-1. Handler コールバック網の解体 [N2] — 効果: 可読性◎ (中核の見通し) / 難易度: 高

- [ ] 各サブコンポーネントが親を `Handler` インターフェースで逆参照し、`Network` が
      seed_accessor / node_accessor / transferer / routing の 4 つの Handler を
      1 つの型で束ねる「神仲介者」になっている (network.go:160-233 はほぼ全行が転送コード)。
      メソッド名のプレフィックス (`NodeAccessorRecvPacket`, `RoutingUpdateConnection`, ...)
      で衝突回避しているのがその症状。
      → 各コンポーネントに「必要な操作だけ」を関数フィールドか小インターフェースで注入する。
      `routing1DConfig.reconcileNextNodes` (routing_1d.go) が既にこの形なので、それに揃える
- [ ] spread ⇔ network の相互参照が `colonioImpl` を経由して往復している
      (`NetworkUpdateNextNodePosition` → spread, `SpreadGetRelayNodeID` → network,
      node.go:367-373)。spread の Config に network 側の必要インターフェースを直接渡し、
      colonioImpl から仲介メソッドを消す
- [ ] `observation.Caller` の nil チェックが network / routing に散在
      (`if n.observation != nil`) — no-op 実装をデフォルトにして分岐を消す

### 2-2. node_accessor の接続状態モデル [N7 + 旧 Phase 4 の一部] — 効果: 可読性・堅牢性 / 難易度: 中〜高

- [ ] 4 つの map (`nodeID2link` / `link2nodeID` / `offerID2state` / `link2offerID`) の
      整合性を手動で保っている (node_accessor.go:69-73)。link 自身に
      nodeID・offerID・接続状態を持たせ、map を 1〜2 個に減らして不変条件を単純化する
- [ ] `disconnectLink(link, lock bool)` のロックフラグ引数 (node_accessor.go:404) を廃止し、
      locked 版 / unlocked 版に分離する (難: 低)
- [ ] `nodeLinkChangeState` / `nodeLinkUpdateICE` がイベントごとに goroutine を起こして
      ロックを取り直す ad-hoc な非同期化 (node_accessor.go:503-565) — イベントを
      単一の処理 goroutine に流す (シリアル化) か、スレッディングモデルを文書化する
- [ ] 同時接続の衝突解決 (両ノードが同時に offer した際の label 比較ロジック、
      node_accessor.go:514-531) を純粋関数に抽出して単体テストを書く (難: 低)
- [ ] node_link の状態機械 (active/online/エラー) を明文化し、native / wasm 実装の
      共通部分 (状態遷移、SDP/ICE ハンドリングの段取り) を抽出してプラットフォーム
      差分を最小化する (node_link.go 550 行 + webrtc_link 3 ファイル 720 行)

### 2-3. 「deadlock 回避のための go」パターンの解消 [N11] — 効果: 堅牢性 (イベント順序) / 難易度: 中

- [ ] `NodeAccessorChangeConnections` が「use go routine to avoid deadlock」コメント付きで
      `go n.routing.UpdateNodeConnections(...)` している (network.go:186-187)。
      ロック順序の問題を goroutine 起動で回避するパターンで、イベントの順序保証が失われ、
      接続変化の適用順が不定になる。node_accessor 側の同種パターン (2-2) と合わせて、
      「コンポーネント間のイベントはロックを持たずに発火する」規約を決めて解消する。
      2-1 / 2-2 とセットで行うと効率が良い

### 2-4. ライフサイクルモデルの統一 [N3] — 効果: 可読性・堅牢性 / 難易度: 中

- [ ] 全コンポーネントが `NewX(config)` + `Start(ctx, localNodeID)` の 2 段階初期化。
      localNodeID が seed 接続後にしか決まらない制約のせいで、routing の `r1d` / `r2d` が
      コンストラクタでなく `Start()` 内で生成される (routing.go:88-100) など、
      「Start 前は不完全な状態」が広く存在する。localNodeID 依存を明示した初期化順序に整理する
- [ ] `Stop()` が `cancel()` を呼ぶだけで goroutine の終了を待たない
      (node.go:271, transferer.go:132)。graceful shutdown (WaitGroup / errgroup) にするか、
      「Stop 後の再利用不可・非同期終了」を API 契約として明文化する。
      `Start()` 二重呼び出しチェック (node.go:255) はあるが Stop→Start の再開はできない
- [ ] seed 側は `Run(ctx) error` (ブロッキング) 形式。node 側 (`Start`/`Stop`) と
      スタイルが割れているので、どちらかに統一を検討

### 2-5. transferer の残り整理 [N5 の残り] — 効果: 可読性・堅牢性 / 難易度: 中

- [ ] reflection ベースの dispatch (`reflect.TypeOf`) を proto oneof の型 switch に置き換え、
      `SetRequestHandler` の TODO (T の制約チェック, transferer.go:103) ごと解消する
- [ ] 応答/エラーハンドラが毎回 `go handler(...)` で野良 goroutine 化される
      (transferer.go:148,267,287) — 順序保証・back-pressure がないことを契約として明文化するか、
      worker 化する (受信パスの goroutine 積み上がり [P6 の一部] もこれで解決)

### 2-6. panic ポリシーと sentinel error [N8] — 効果: 利用者体験・堅牢性 / 難易度: 低〜中

- [ ] ライブラリ内 panic の棚卸し: リモート入力起因 (0-1) は除去済みの前提で、
      プログラミングエラー起因 (`NodeID.Proto`, `Transferer.Request` のモード検査,
      `routing.GetNextStep2D`) は残すなら doc コメントで契約を明示する
- [ ] 公開 API に sentinel error がない (すべて `fmt.Errorf`)。`ErrTimeout` /
      `ErrNoOneReceive` 等を公開パッケージに定義し、内部の `PacketErrorCode` から
      マップする。呼び出し側が「タイムアウト」と「宛先不在」を区別してリトライ判断
      できるようにする (現状は文字列比較しかない)

### 2-7. 公開 API の型一貫性 [N9] — 効果: 可読性・利用者体験 / 難易度: 低

- [ ] NodeID の表現が公開境界で `string`、内部で `types.NodeID` と二重になっている
      (`MessagingPost(dst string, ...)`, `GetLocalNodeID() string` vs 公開済みの
      `types.NodeID`)。`types.NodeID` に統一するか、string で通すか方針を決めて揃える
      (1-6 の値化とセットで行うと二度手間がない)
- [ ] WebRTC 関連の期間パラメータ (SessionTimeout 5min / KeepaliveInterval 1min /
      BufferInterval 10ms / PacketBaseBytes 4096) が `NewNode` 内にコメント付きで
      ハードコードされている (node.go:203-229)。Config に昇格するか、意図的に
      非公開なら定数として `node_accessor` 側へ移す

### 2-8. observation のイベント配送モデル [N10] — 効果: 正しさ (観測順序)・速度 (小) / 難易度: 低

- [ ] イベントごとに `go handler(...)` を起こしており (observation.go:37,43,49)、
      配送順序の保証がない。状態スナップショット系イベントは古い状態が後から届くと
      観測側 (simulator の record) が誤った最終状態を記録する。
      1 本のディスパッチ goroutine + 順序付きキューに変更する
- [ ] Handler は設定されているが個別コールバックが nil の場合でも、呼び出し側で
      `ConvertNodeIDSetToStringMap` の変換が先に走る (network.go:190, routing.go:173,181)。
      変換を Caller 実装側に移し、コールバック未設定時はゼロコストにする
- [ ] コールバック引数が `map[string]struct{}` (NodeID の hex 文字列)。observation を
      使うのは simulator / テストだけなので、`types.NodeID` のまま渡して文字列化は
      受け手に任せる (P10 とセット)

### 2-9. 明示的エラー通知 (ICMP 相当) [T3] — 効果: 障害時レイテンシ・デバッグ性 / 難易度: 中

- [ ] hop limit 超過が現在サイレントドロップ (network.go:236-243)。ICMP Time Exceeded に
      相当するエラーパケットを送信元へ返し、送信元が RTO 満了を待たずに失敗を検知・
      ルーティングループを観測できるようにする
- [ ] 未知の content type の受信 (0-1 で log+drop 化) に ICMP Port Unreachable 相当の
      `PacketError` 応答を返し、旧バージョンのノードに要求を送った送信元が再送せず
      即座に失敗できるようにする。`PacketErrorCode` に UnknownContent / HopLimitExceeded を追加

### 2-10. 再接続の指数バックオフ + ジッタ [T5] — 効果: 堅牢性 (復帰時の殺到防止) / 難易度: 低

- [ ] seed への poll 失敗時のリトライが固定 10 秒 (seed_accessor.go:125)、keepalive も
      固定 10 秒 (seed_accessor.go:144)。seed 再起動やネットワーク断からの復帰時に
      全ノードが同期して殺到する。指数バックオフ + ランダムジッタ (フルジッタ方式) にする
- [ ] node 間接続も同様: 最初のリンク確立の再試行は 1 秒 tick に同期しており
      (node_accessor.go:153)、失敗時のバックオフがない。接続失敗回数に応じた
      バックオフを connectingState に持たせる

### 2-11. routing 交換トラフィックの削減 [P5] — 効果: 速度 (スケール時の帯域) / 難易度: 中 (ワイヤ形式変更)

- [ ] routing パケットが `map<string, RoutingNodeRecord>` で、node ID を
      32 文字の hex 文字列キーとして送っている (node.proto:62, routing.go:198)。
      `repeated` + バイナリ NodeID (uint64×2 = 16 バイト) にすれば
      エントリあたり約半分になる。ワイヤ形式の変更なので proto の版管理に注意
- [ ] 変化があるたびに routing テーブル全体を全隣接ノードへ送っている
      (routing.go:196-210)。ノード数・隣接数が増えると O(N×M) で膨らむので、
      前回送信分との差分送信を検討 (P0 でサイズと頻度を測ってから)

### 2-12. NodeID の文字列化・変換コスト [P10] — 効果: 速度 (小〜中) / 難易度: 低

- [ ] `NodeID.String()` が `fmt.Sprintf("%016x%016x", ...)` (node_id.go:116)。
      ログ・observation・routing パケットで頻繁に呼ばれるので、
      `strconv.AppendUint` / `hex.Encode` ベースに置き換える
- [ ] slog の属性に NodeID を渡す箇所は `slog.LogValuer` を実装して遅延評価にし、
      ログレベルで無効なときに文字列化コストを払わないようにする
- [ ] `ConvertNodeIDSetToStringMap` (node_id.go:247) は 2-8 の「NodeID のまま渡す」で
      呼び出し自体をなくす

### 2-13. spread cache の資源上限 [P11] — 効果: 堅牢性 (リモート起因のメモリ成長) / 難易度: 低

- [ ] spread の重複抑制 cache は受信パケットでもエントリを作る (spread.go:266) ため、
      リモートノードが uid を変えて送り続けると TTL (cacheLifetime) 内は無制限に成長する。
      エントリ数上限 + 超過時のドロップポリシーを入れる (1-9 と同じ動機)
- [ ] cleanup が `cacheLifetime/2` の ticker (spread.go:135) なので、実際の保持期間が
      設定値の最大 1.5 倍になる。契約としてドキュメント化するか、期限ベースの削除にする

### 2-14. ポーリング型 subRoutine の整理 [N4] — 効果: 可読性・省リソース / 難易度: 中

- [ ] network / node_accessor / transferer / routing がそれぞれ 1 秒 ticker +
      `subRoutine()` を持ち、計 4 本以上の常駐ポーリング goroutine が走る。
      `isAlone` の伝搬も 1 秒ポーリング (network.go:123-135)。
      イベント駆動 (状態変化時に通知) に寄せられるものから移行し、
      残すものは interval を Config で調整可能にする
- [ ] routing の `triggeredAction` が生の int ビットマスク
      (`requireUpdateConnections = 0x1`, routing.go:35)。型付き flag にして
      1D/2D の戻り値契約を明示する (難: 低)

### 2-15. 小粒ユーティリティパッケージの統廃合 [旧 Phase 3] — 効果: 可読性 / 難易度: 低

- [ ] `node/internal/constants/` (定数 3 つのみ、2 ファイル) を解体し、
      `ONE_SIDE_NEXT_COUNT` は routing パッケージへ、`PacketErrorCode` は
      `types/network` か transferer へ移動。あわせて `ONE_SIDE_NEXT_COUNT` を Go 命名規約に修正
- [ ] `node/internal/wait_any/` — 利用箇所は spread のみ。spread 内部へ移すか、
      `sync.WaitGroup` + チャネル / `errgroup` での置き換えを検討。
      `Done()` 内の panic 条件も見直す
- [ ] `seed/misc/` の解体 (名前が責務を表していない)
  - [ ] `map_helper.go` の `GetNextByMap` — 1-10 でソート済み構造に置き換えるため、
        まず gateway/controller 側へ移動
  - [ ] `channel.go` の `Channel[T]` — close 後送信を守るラッパー。利用箇所
        (server / controller / gateway) ごとに本当に必要か精査し、
        context ベースの停止に統一できるなら削除
  - [ ] `logger.go` — seed 用ロガーヘルパー。`seed/internal/` 配下へ
- [ ] `seed/` 内部実装 (`controller`, `gateway`, `misc` 後継) を `seed/internal/` に移して
      公開 API 面を意図したものだけに絞れるか検討 (Gateway はカスタム実装の差し替え点なので
      インターフェースは公開のまま維持する)

### 2-16. node パッケージの残り整理 [旧 Phase 4 の残り] — 効果: 可読性 / 難易度: 低〜中

- [ ] `routing.go:164` 「should be optimized」TODO — P0 のベンチで計測してから着手
- [ ] `routing_1d.go:185` — 線形走査を二分探索に (ソート済み前提の明確化とテスト追加)
- [ ] `geometry/plane.go` — 内部からは未到達 (e2e は sphere のみ)。
      `WithPlaneGeometry` を公開 API として維持するなら plane の単体テスト/e2e を追加、
      維持しないなら deprecate を検討
- [ ] `node/node.go` (451 行) — API 定義とオプション、実装が同居。
      実装 (`colonioImpl`) を `node/internal/` へ下ろし、node.go を API 面だけにする

### 2-17. seed Controller の責務分割 [S1] — 効果: 可読性 / 難易度: 中

- [ ] 1 つの Controller (439 行) に 3 責務が同居している:
      (a) ノード割当 (AssignNode/UnassignNode)、
      (b) keepalive・lifespan 管理と cleanup ループ、
      (c) シグナリング中継 (SendSignal/PollSignal)。
      それぞれ独立した型に分割し、Controller は組み立てだけにする
- [ ] `signalChannels` / `keepaliveChannels` + `misc.Channel` + mutex という同型の
      購読管理が 2 セット重複している。汎用の subscription registry 型
      (Subscribe/Unsubscribe/Publish) を 1 つ作って両方を載せる (2-15 の
      `misc.Channel` 整理とセット)

### 2-18. Gateway インターフェースの分割 [S2] — 効果: 保守性 (カスタム実装の負担減) / 難易度: 低〜中

- [ ] 12 メソッドの太いインターフェースを役割で分割する:
      ノード台帳 (Assign/Unassign/Count/Lifespan/GetNodesByRange/GetNodes)、
      keepalive pub/sub、signal pub/sub。カスタムバックエンド (Redis 等) 実装時に
      必要な部分だけ実装・テストできるようにする (KVS 向け拡張は KVS 節で扱う)
- [ ] `ErrKvsFirstActiveCandidateAlreadySet` が非 KVS の gateway パッケージに定義されている
      (gateway.go:28) — KVS 側パッケージへ移動 (難: 低)
- [ ] `GetNodesByRange(ctx, nil, nil)` が「全件取得」を意味する暗黙のオーバーロードに
      なっている (controller.go:382) — 専用メソッドにするか doc コメントで契約を明示 (難: 低)

### 2-19. seed の初期化構造 [S4] — 効果: 可読性・組み込みやすさ / 難易度: 低

- [ ] `Seed` が `gateway.Handler` を実装して controller へ転送する 3 メソッド
      (seed.go:120-130) は、gateway → handler → controller の初期化循環を断つためだけの
      glue。gateway に `SetHandler` (late-bind) を用意して転送コードを消す
- [ ] `NewSeed` が `COLONIO_COOKIE_SECRET_KEY_PAIR` 未設定で panic する (seed.go:89)。
      ライブラリコードでの env 参照と panic をやめ、`NewSeed` を error 返却にして
      env の読み取りは呼び出し側 (cmd / テストヘルパー) に移す
- [ ] `Seed` が `options` 構造体を embed している (seed.go:70) — 通常フィールドにして
      公開面から隠す

### 2-20. テストヘルパーの委譲コード削減 [旧 Phase 5 の一部] — 効果: 保守性 / 難易度: 低

- [ ] `test/util/helper/gateway.go` (189 行) — Gateway インターフェースの全メソッドを
      手書き委譲している。埋め込み (`gateway.Gateway` を embed して必要なメソッドだけ
      オーバーライド) に書き換え、インターフェース変更のたびの追従コストをなくす

---

## 優先度 3 — 低優先 (手が空いたとき / 大掛かりで効果が不確実なもの)

### 3-1. ビルド・テスト基盤の整理 [旧 Phase 7] — 効果: 開発体験のみ / 難易度: 低 (quick win 集)

- [ ] Makefile の TODO 解消: `build-js` が毎回走る問題 (`output/colonio.js` の依存関係が
      webpack の出力タイムスタンプと噛み合っていない)。stamp ファイル方式に変更
- [ ] `make clean` の `git checkout $(OUTPUT_PATH)` ハック (output/.gitkeep 復元) を
      `mkdir -p` 方式に変更
- [ ] WASM テストビルドのシェルループ (`test/dist/tests.txt` ターゲット) を整理。
      対象パッケージの列挙を `go list` ベースにする
- [ ] カバレッジプロファイル (`*.covprofile`) の出力先をリポジトリルートから
      専用ディレクトリ (例 `coverage/`) へ移動
- [ ] e2e テストの所要時間見直し: `3*time.Minute` の Eventually が多用されている。
      失敗時のフィードバックを早める (タイムアウトの根拠をコメント化、短縮可能なら短縮)
- [ ] `src/webrtc.ts` (399 行) — WASM グルーコード。Go 側 (`webrtc_link_wasm.go`) との
      コールバック契約を doc コメント化し、命名を揃える

### 3-2. simulator の整理 [旧 Phase 6] — 効果: 可読性 (simulator のみ) / 難易度: 低〜中

(1-1 のモジュール分割後に実施)

- [ ] `circle/` と `sphere/` の重複整理: render.go (137 行 / 153 行) と node.go の構造がほぼ同型。
      story 差分 (座標系・描画) だけを差し替え可能にして共通部分を `base/` へ寄せる
- [ ] `cmd/` のサブコマンド群 (helper.go, seed.go, node.go, render.go, ...) の共通フラグ処理を整理
- [ ] `datastore/` — writer/reader/types の関係を見直し。`Reader.Close` 未呼び出し問題 (0-2) の本修正
- [ ] リポジトリ内に転がっている生成物 (`simulator/colonio-simulator.tar`, `dump.json`,
      `out.mp4`, `render/`, `work/`) は gitignore 済みだが、`make clean` で確実に消えることを確認

### 3-3. 受信パスの割り当て削減 [P6 の残り] — 効果: 速度 (小) / 難易度: 低

- [ ] 断片再結合の `bytes.Join` (node_link.go:516) が全断片を再コピーする —
      合計長を先に計算して 1 回の確保にする (1-2 で断片化自体が減れば優先度はさらに下がる)

### 3-4. DataChannel の分離・詳細チューニング [P3 の残り] — 効果: 速度 (HoL blocking 解消) / 難易度: 高 (プロトコル変更)

- [ ] 全トラフィックが ordered/reliable な単一 DataChannel に載っており
      (src/webrtc.ts:86, native はデフォルト設定)、大きなパケットの断片列が
      keepalive や routing 制御パケットを head-of-line blocking する。
      制御用と bulk 用の channel 分離、または unordered channel + パケット単位の
      再結合への移行を検討。ただし現在の再結合はチャネルの順序保証に依存している
      (node_link.go:468 の stackID 連続性チェック) ので、プロトコル変更とセット
- [ ] native (pion) では `SettingEngine.DetachDataChannels()` による
      受信バッファのコピー削減を検討 (wasm はブラウザ実装なので対象外)

### 3-5. 中継キューの管理 (AQM) と送信元ペーシング [T6] — 効果: 堅牢性 (過負荷時) / 難易度: 高

- [ ] 1-3 のキュー上限 + ドロップポリシーの具体化: tail drop は同期的な全滅を招くという
      TCP/IP の教訓 (RED の動機) に倣い、キュー長がしきい値を超えたら one-way パケットから
      確率的に早期ドロップする。制御 (routing) > 応答 > 要求 > one-way の優先度で保護する
- [ ] 送信元ペーシング: messaging は request/response で自然に自己制限されるが、
      one-way / spread は無制限に送出できる。リンクごとの送出レート上限 (トークンバケット)
      を設け、超過分はローカルでブロック/エラーにして中継網に流さない

### 3-6. seed リンクの効率化 [P8] — 効果: 速度 (接続確立時のみ) / 難易度: 低

- [ ] keepalive ループの固定 `time.Sleep(10s)` (seed_accessor.go:144) が
      サーバ側 long-poll と噛み合っておらず、切断検知・再接続の遅延源になる。
      間隔を Config 化し、long-poll の契約 (サーバは normalLifespan/2 まで保持) を
      ドキュメント化する
- [ ] PollSignal (server streaming) / Keepalive / SendSignal が同一 HTTP クライアント上で
      多重化されているか確認 (特に WASM の fetch 実装)。HTTP/1.1 に落ちている場合、
      接続数制限がシグナリング遅延 → 接続確立遅延につながる

### 3-7. WASM の syscall/js 境界コスト [P12] — 効果: 速度 (WASM のみ、要計測) / 難易度: 中

- [ ] WASM では送受信パケット 1 つごとに `js.Value.Call` + `CopyBytesToGo/JS` の
      境界越えが発生する (webrtc_link_wasm.go)。`BufferInterval` (10ms) の送信バッファリングと
      噛み合わせて複数パケットを 1 回の Call にまとめられるか検討する
- [ ] `jsOnUpdateLinkState` がイベントのたびに `getLabel` を Call し直している
      (webrtc_link_wasm.go:73) — link 生成時に一度取得して保持する (難: 低)

### 3-8. end-to-end の完全性と真正性 [T7] — 効果: セキュリティ (脅威モデル次第) / 難易度: 高 (設計ドキュメントから)

- [ ] リンク単位は DTLS で守られるが、**中継ノードはパケットを平文で読めて書き換えられる**。
      また `SrcNodeID` は自己申告で、直接接続の隣接ノードが任意の送信元を名乗れる
      (node_link.go:530 は head をそのまま信用)。TCP checksum が「経路上の破損は
      end-to-end でしか守れない」と示した教訓と同型
- [ ] 段階案: (1) まず脅威モデルを doc に明文化する (中継ノードは信頼できる前提なのか)。
      (2) 改ざん検知だけなら e2e チェックサム/MAC。 (3) なりすまし対策まで踏み込むなら
      NodeID を公開鍵から導出 (NodeID = hash(pubkey)) して署名で検証する設計になり、
      影響範囲が大きいので独立した設計ドキュメントを書いてから着手する

---

## KVS (開発中 — 安定化と並行して行う整理、優先順は機能開発の進捗に従う)

KVS は機能が未完成 (一部動いていない) ため、上記の優先度とは独立に、機能開発の流れの中で行う。
**TLA+ 仕様との対応表 (`spec/kvs/README.md`) があるため、関数名・パッケージ名の変更時は必ず両方を更新すること。**

### 構造の整理

- [ ] `kvs.go` (1,194 行) の分割: 公開操作 (Get/Set/Patch/Delete)、セクター制御フロー
      (Activate/Split/Extend/Merge/Terminate)、状態管理を別ファイル/型に分ける。
      TLA+ 対応表が関数単位なので、分割は対応表の更新とセットで (難: 中)
- [ ] `sector/sector.go` (873 行) も同様に責務単位で分割を検討 (難: 中)
- [ ] `inbound.go` / `outbound.go` パターンの統一: kvs / hosting は両方持つが
      activation は outbound + resolver。命名規約 (inbound = メッセージハンドラ、
      outbound = 送信、resolver = ?) を README に明文化し、構成を揃える (難: 低)
- [ ] raft 用ロガー (`logger_empty.go` / `logger_wrapper.go`) — `raft.Logger` の
      ボイラープレート。`logger_wrapper` に統一し empty は slog の discard ハンドラで
      代替できないか確認。`interface{}` → `any` もここで (難: 低)

### コード内 TODO の解消 (実装安定後)

- [ ] `sector/operator/operator.go:124` — splittingTail より前方の proposed operations の適用待ち (難: 中)
- [ ] `sector/consensus/consensus.go:186` — ConfChangeV2 による append/remove のバッチ化 (難: 中)
- [ ] `sector/consensus/consensus.go:197,212` — エラーハンドリング欠落 (現状握りつぶし) (難: 低〜中)
- [ ] `sector/consensus/consensus.go:421` — AddNode/RemoveNode で Context を共用している問題。
      ConfChangeV2 化 (上項) とあわせて分離 (難: 中)

### API とテスト

- [ ] 公開 API の確定: `KvsGet` 等が `chan` を返す設計 (`node/node.go:356-371`) を
      context 付き同期 API にするか確定してから e2e を書く (難: 低〜中、判断が主)
- [ ] `test/e2e/e2e.go:121` のコメントアウト済み KVS e2e ブロックの復活
      (復活時の最低限チェック項目はコメントに明記済み: 別ノード間の Set/Get、上書き挙動、
      ローカルデータ整合) (難: 中、実装の安定が前提)
- [ ] `test/e2e/e2e.go:185` の `TestGetSetInCB` (コールバック内 KVS 操作) —
      デッドロックリスクのため無効化中。コールバック契約を明文化してから判断 (難: 中)
- [ ] `simple_store.go` — `types/kvs.Store` インターフェースとデフォルト実装の API を
      外部実装可能な形に固める (seed/gateway と同じパターン) (難: 中)
- [ ] deadcode 指摘の `WithKvsStore` / `WithRaftLogging` は KVS 安定後に公開 API として
      ドキュメント化 (現状未使用なのは開発中ゆえ) (難: 低)

### ドキュメント同期

- [ ] `spec/kvs/README.md` の TLA+ ↔ Go 対応表をリファクタリング後の関数名に追従させる (難: 低、都度)
- [ ] `spec/kvs/design.md` と実装の乖離チェックをフェーズ完了ごとに行う (難: 低、都度)
