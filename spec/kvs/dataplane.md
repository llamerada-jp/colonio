# KVS データプレーン（Get/Set/Patch/Delete）の設計

2026-07-12 実装。raft snapshot（spec/kvs/snapshot.md）の Stage 6 前提となる
データプレーン本体。

## 経路の全体像

```
client: KVS.Set(key, value)
  → outbound.sendKvsOperation          … key のハッシュ宛にルーティング
  → (network) → 対象 node の inbound.recvKvsOperation
      … transferer は packet ごとに goroutine を起動するため
        以降のブロッキングは安全
  → KVS.kvsOperate → hosting sector の operator
  → Operator.Set = proposeOperation
      1. ゲート検査 + waiter 登録（同一クリティカルセクション）
      2. handler.OperatorProposeOperation → consensus.Propose(Operation)
      3. waiter を operationTimeout (10s) まで待つ
  → (raft commit) → publishEntries → Sector.ConsensusApplyProposal
  → Operator.ApplyProposal
      … store へ反映し、提案元 replica なら waiter を解決
  → proposeOperation が返る → KvsOperationResponse
```

- **書き込み ack = ローカル apply 完了**。commit だけでなく提案元 replica の
  ステートマシン反映まで待つので、直後の Get が自分の書き込みを見る
  （read-your-writes）。
- **読み取り（Get）はホストのローカル store から**。raft を通さない。
  `Operation.COMMAND_GET` は enum に存在するが未使用。

## 一貫性の意味論（意図的な緩和）

- 書き込みは線形化される（raft ログ順）。
- 読み取りはホスト replica の適用済み状態を返す。ホストが leader でない
  瞬間や apply 遅延があるため、**commit 済み・未 apply の書き込みは見えない
  ことがある**（他クライアントから見た stale read）。自分の ack 済み書き込みは
  必ず見える。
- タイムアウト時の結果は**不定**（raft は負の応答を返さない。proposal が後で
  commit される可能性がある）。`ErrOperationTimeout` → `ERROR_UNKNOWN` で
  返し、リトライ判断はクライアントに委ねる。

## 操作の待ち合わせ（operation_id / waiters）

- `operation_id` は **operator インスタンス内のみで一意**（uint32 連番）。
  1 raft グループの Operation 提案者はホスト（sectorNo=1 の hosting sector）
  の operator 1 体だけなので、apply された id は「自分の waiters にある」か
  「waiter なし（replica / 途中参加の replay / 旧インスタンス）」の
  どちらかにしかならない。
- タイムアウト時は waiter を登録解除する。解除とシグナルの競合は
  バッファ付き channel + 解除後の non-blocking 受信で吸収。

## ゲート（ErrorSectorNotReady → ERROR_PREPARING、リトライ可能）

`writableLocked` / Get 冒頭で検査:

| 条件 | 理由 |
|---|---|
| 未 activate（tail == nil） | 従来の PREPARING と同じ扱い |
| key が範囲 [head, tail) 外 | stale routing または split/merge 直後。リトライで正しいホストに届く |
| split 移譲範囲 [splittingAddress, tail) への書き込み | export 後に apply された書き込みは PreCommitSplit で消える（ack 済みロスト）ため事前拒否 |
| mergeBy fence 中の全書き込み | merge で範囲ごと吸収される間の同種のロスト防止。読み取りは許可 |

- head == tail は「リング全体」（IsBetween が同値で panic するため特別扱い）。
- **apply 時にも範囲を再検査**する: propose 後に split/merge が commit されて
  範囲が動いた場合、store への反映を決定的にスキップし、waiter にだけ
  NotReady を返す（クライアントがリトライ）。tail は複製状態なので全 replica
  で同じ位置の log entry に対して同じ判定になる。

## 書き込みロスト窓の閉鎖

1. **split**: `SetSplitting(address)` がフェンスを張った上で、フェンス前に
   受理済みの pending 操作が全て apply されるまでドレインする
   （operationTimeout で有界。旧 TODO の解消）。その後に `Migrate` が
   export するので、ack 済み書き込みは必ず export に含まれる。
2. **merge**: prepare_merge の apply で `SetMergeFence(true)`、release_merge
   の CAS 成功で解除。snapshot 復元時も mergeBy に同期。
   fence は sector の apply ハンドラ駆動なので replica 間で決定的。

### 残存する既知の窓（TODO）

- merge のデータ移送は吸収側 node の**ローカル replica** の records を読む
  （`Sector.Merge(from)`）。fence により新規書き込みは止まるが、fence 前に
  commit 済みで**吸収側 replica にまだ apply されていない**操作は export から
  漏れ得る。閉じるには「from グループの applied index が fence 時点の commit
  index に追いつくまで待つ」等が必要。
- release_merge が誤発火（保持者が生きているのに 30s 超過）した場合、
  fence が外れて上記の窓が再度開く。頻度は低い（healthy merge は 1 秒未満）。

## エラーマッピング（kvsOperate）

| operator の返り値 | KvsOperationResponse |
|---|---|
| nil | ERROR_NONE |
| ErrorStoreKeyNotFound | ERROR_NOT_FOUND（GET / DELETE） |
| ErrorSectorNotReady | ERROR_PREPARING（リトライ可能） |
| ErrOperationTimeout / その他 | ERROR_UNKNOWN |

## TODO

- **Patch の意味論が未定義**。経路（propose → apply → store.Patch）は実装
  済みだが、`SimpleStore.Patch` は「未サポート」エラーを返す（apply 経路での
  panic は全 replica の consensus loop を落とすため error 化した）。部分更新
  フォーマットを決めたら store 実装と合わせて定義する。
- **クライアント側リトライ**: ERROR_PREPARING を受けた `KVS.Set` 等は現状
  そのままエラーを返す。ルーティング収束を待って再送する層を入れるなら、
  **Operation の dedup**（提案の再送で二重 apply しない仕組み）が先に必要。
  dedup 状態を複製状態に加える場合は SectorSnapshot にも載せること
  （snapshot.md Stage 6 の注意）。
- **読み取り一貫性の強化**（必要になれば）: ReadIndex / lease read、または
  Get も raft を通す。現状の用途では stale read 許容とする。
- merge データ移送の replica 遅延窓（上記）。
- 書き込み負荷での snapshot/compaction 検証と `snapCount` 本番値調整
  （snapshot.md Stage 5/6 の残項目。データプレーンが入ったので実施可能）。
