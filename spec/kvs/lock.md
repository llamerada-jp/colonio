# KVS lock 機構(revision/CAS + lease)の設計

2026-07-12 設計。未実装。データプレーン(spec/kvs/dataplane.md)の上に積む
2 層構成の排他機構。アプリケーション層(何を lock で守るか)はこの文書の
スコープ外とし、KVS が提供するプリミティブのみを定義する。
公開 API の形(module client / ctx / managed Lock / Watch)と実装ステージの
全体順序は spec/kvs/api.md を正典とする。

## 目的と全体像

提供するのは次の 2 層。下層が上層の実装基盤になるため、実装順序もこの順。

1. **revision / CAS(楽観的並行性制御)**: 全レコードに単調増加の revision を
   持たせ、`SetIf` / `DeleteIf`(expected revision 一致時のみ適用)を提供する。
   クライアント死亡時に解放すべき状態がなく、atomic な read-modify-write は
   これだけで足りる。
2. **lease lock**: key 単位の `{owner, generation, deadline}` を複製状態として
   持ち、owner が renewal し続ける限り保持され、renewal が絶えると host が
   失効させる。generation は fencing token を兼ね、lock 付きレコードへの
   書き込みは generation 検証で保護される。

設計全体を貫く原則(design.md の apply 規約に従う):

- **lock/revision 状態はすべて複製状態**。変更は必ず raft Operation の
  apply で行い、apply は決定的・冪等。
- **apply の中で時計を読まない**。時間判定(lease 失効)は host が自分の
  時計で観測し、失効を raft に提案する(ReleaseMerge / mergeBy と同じ、
  `KvsSectorMergeLock.tla` で検証済みのパターン)。
- **相互排除は絶対保証しない**。過半数同時喪失によるデータ消失
  (design.md「その他の性質」)で lock レコード自体が消え得る。正しさが
  必要な用途は generation(fencing)で書き込み側を守る前提とする。

## レコードエンベロープ

revision と lock は store の value にエンベロープとして埋め込む:

```proto
message KvsRecord {
  bytes    value    = 1;
  uint64   revision = 2;  // このレコードへの最終書き込み時の sector counter 値
  KvsLock  lock     = 3;  // unset = unlocked
}

message KvsLock {
  colonio.v1alpha.NodeID owner       = 1;
  uint64                 generation  = 2;  // fencing token。sector counter から採番
  int64                  deadline_ms = 3;  // 提案時に host が計算した unix ms
}
```

エンベロープ方式を選ぶ理由: snapshot(`ExportAllRecords` / `ReplaceRecords`)と
split/merge のデータ移送(`ExportRecords` / `ImportRecords`)はいずれも
opaque な key→value bytes を運ぶだけなので、**revision / lock 状態が既存の
全経路に自動的に乗る**。SectorSnapshot や Import メッセージに並行する
テーブルを足す方式だと、載せ忘れが即座に replica 乖離になる
(dataplane.md の dedup 状態に関する注意と同種の罠)。

- store 層(`kvsTypes.Store`)のインターフェースは変えない。encode/decode は
  operator 層で行い、store は従来どおり bytes を保持する。
- `Operator.keys` と同様に、lock 中 key の in-memory index
  (key → deadline)を operator が apply / snapshot 復元時に再構築する
  (導出状態。失効スキャンに使う)。

## revision counter(sector スコープ・max-merge)

per-key のカウンタではなく **sector スコープの単調カウンタ** を 1 本持ち、
書き込み成功のたびに `counter++` して当該レコードの revision に刻む
(etcd の global revision の sector 版)。

- counter は複製状態: apply でのみ増加し、`SectorSnapshot` に新フィールドで
  載せ、snapshot 復元で同期する。
- **移送時は max-merge する**: split の export / merge の export に元 sector の
  counter を含め、import 側は `counter = max(counter, imported)` する
  (Import / CommitSplit の適用内なので決定的)。

per-key でなく sector counter + max-merge にする理由は **単調性の維持**:

- key k への書き込みは常に k のレコードを保持する sector で起こり、その
  sector の counter はレコード到着時(import)の max-merge により
  k の現 revision 以上になっている。よって新 revision は必ず増加する。
- **Delete → 再作成の ABA も閉じる**: レコードが消えても counter は減らない
  ため、再作成された revision は削除前より必ず大きい。per-key カウンタだと
  tombstone を残さない限り再作成でリセットされ、CAS が古い観測に基づいて
  成功し得る。範囲が移送された後の再作成も、export に counter を含める
  ことでカバーされる。

限界: sector の複製ごと失われた場合(過半数喪失 → 空で再作成)は counter が
0 に戻り、単調性が破れる。エポック併用などの緩和は TODO 参照。

## CAS 操作

`Operation` に expected revision を追加する:

```proto
// Operation に追加
uint64 cas_revision = 5;  // 0 = 無条件(従来の Set/Delete と同じ)
```

- apply 時、`cas_revision != 0` かつ現 revision と不一致なら store に触れず
  waiter に conflict を返す(決定的: revision は複製状態)。
  「レコード不在」は revision 0 とみなし、`cas_revision == 0` の使い分けと
  衝突しないよう **「不在を期待する CAS」は専用フラグ**(または
  `cas_revision = math.MaxUint64` の番兵)で表す。実装時に proto 上の表現を
  確定する。
- `Get` は `(value, revision)` を返すように拡張し、`Set` も apply で採番された
  新 revision を返す(`KvsOperationResponse` に revision を追加。公開 API 側は
  api.md の `GetResponse` / `SetResponse`)。
- **UNKNOWN(タイムアウト)後の再送が安全になる**: 同じ expected revision で
  再送すると、初回が apply 済みなら conflict で止まる(二重適用しない)。
  つまり CAS は dedup 機構なしで at-most-once になる。ただし「実は成功して
  いたのに conflict が返る」偽陰性があるため、conflict 時は Get で現状を
  確認して判断する、をクライアント規約とする。dataplane.md TODO の
  「UNKNOWN 再送には dedup が先」は、CAS 経路については解消される
  (無条件 Set/Delete には引き続き当てはまる)。
- Patch は CAS の read-modify-write では**代替しない**: 大 value の部分更新は
  patch 文書だけを流すサーバ側 patch が帯域・raft ログサイズで構造的に有利。
  pluggable Patcher(利用者登録の決定的純関数)として再定義する
  (api.md「Patch」)。非冪等なので UNKNOWN 自動再送は `WithRevision`
  併用時に限る。

## lease lock

### 状態と操作

lock はレコードのエンベロープ(`KvsLock`)に載る。レコード不在の key への
acquire は空 value のレコードを作成して lock を付ける(k8s の Lease
オブジェクト相当の使い方を可能にする)。

新設する raft Operation コマンド:

| コマンド | 引数 | apply の意味論(すべて決定的) |
|---|---|---|
| LOCK_ACQUIRE | owner, ttl_ms | unlocked → `{owner, generation: ++counter, deadline}` を設定。**同一 owner が保持中 → deadline を延長し既存 generation を維持(renewal を兼ねる)**。他 owner が保持中 → conflict |
| LOCK_RELEASE | owner, generation | `(owner, generation)` 一致時のみ解除する CAS。不一致 → conflict。unlocked → 成功(no-op) |
| LOCK_REVOKE | owner, generation | LOCK_RELEASE と同じ CAS。host が失効時に提案する内部用 |

- deadline は **host が propose 時に自分の時計で計算**して Operation に
  詰める。log に載った値は全 replica で同一なので apply は決定的
  (apply 中に時計を読まない、の規約と両立する)。
- **acquire が renewal を兼ねる**ため、UNKNOWN 後の再送は owner 冪等:
  初回が apply 済みでも再送は「延長」になるだけで generation は変わらない。
  release も unlocked を成功扱いにすることで再送安全。
- generation の採番は revision counter を共用する(単調ソースを 1 本に保つ)。

### 失効(revoke)

- host の operator が lock index(key → deadline)を定期スキャンし、
  `now > deadline + margin` の lock について LOCK_REVOKE を propose する。
  margin は host 交代時の時計スキューを吸収するためのもの。
- apply は CAS なので、スキャンと renewal が競合しても「延長済みの lock を
  誤って剥がす」ことはない(generation が同じでも deadline 延長後に revoke が
  apply される順序はあり得るが、その場合 owner は次の renewal の conflict で
  喪失を知る。誤剥がしは相互排除を壊さない — 新 owner は新 generation を
  得るため、旧 owner の guarded write は拒否される)。
- host 死亡時: lock 状態は複製状態なので、sector の再 activate 後に新 host が
  index を復元してスキャンを引き継ぐ。activation が長引いた分だけ失効が
  遅れるだけで、安全側に倒れる。

### guarded write(fencing の強制)

`Operation` に fencing 情報を追加する:

```proto
// Operation に追加
colonio.v1alpha.NodeID lock_owner      = 6;
uint64                 lock_generation = 7;
```

- apply 時、対象レコードが locked なら `(lock_owner, lock_generation)` が
  現 lock と一致する場合のみ適用する。不一致・未指定 → conflict。
  unlocked なレコードへの guarded write は conflict(lock を失った証拠)。
- これにより、失効に気づいていない旧 owner(network-zombie 等の半死 node)の
  書き込みは状態を壊せない。**排他の「正しさ」はここで担保**し、lease は
  調停(誰が書き込み権を持つかの合意形成)に徹する。
- holder による Delete は lock ごとレコードを消す(release + delete の
  atomic 化)。CAS(`cas_revision`)と guarded write は併用可能。

### TTL の下限

churn 中は range が split/merge fence・再 activate 待ちで **十数秒 PREPARING に
留まる**ことが実測済み(dataplane.md「クライアント側リトライ層」)。renewal が
この窓を跨いで失敗しても失効しないよう、TTL は 30 秒以上を既定とする
(`mergeReleaseDuration = 30s` と同じオーダー)。renewal 間隔は TTL/3 程度。
owner 側は「renewal が TTL を超えて成功しない場合は保持喪失とみなす」ことを
規約とし、renewal ループと self-fencing(`Done()` channel)は公開ライブラリの
managed `Lock` が内蔵する(api.md「lock は managed な Lock オブジェクトだけを
公開」)。

## エラーマッピングの拡張

`KvsOperationResponse.Error` に追加:

| operator の返り値 | KvsOperationResponse | クライアントの扱い |
|---|---|---|
| ErrorCasConflict(新設) | ERROR_CONFLICT(新設) | Get で現状確認して再試行判断 |
| ErrorLockHeld(新設) | ERROR_LOCKED(新設) | 待って再試行 or 諦める |

既存の PREPARING / NOT_FOUND / UNKNOWN は従来どおり。conflict 系は
「受理前拒否」ではなく apply 結果なので、PREPARING と違い盲目的リトライは
不可(必ず再読み取りを挟む)。

## 実装ステップ

全体の実装順序は **api.md「実装タスク」を正典とする**(公開 API 再編
Stage A → revision/CAS Stage B → Patch Stage C → lease lock Stage D →
Watch Stage E。API 再編を先に行うことで、以降の機能追加が公開面の破壊的
変更を伴わない)。本書の範囲は Stage B/D の内部実装で、内訳は:

- **Stage B(revision/CAS)**: proto(`KvsRecord` エンベロープ、
  `Operation.cas_revision`、`SectorSnapshot` / Import/Export への counter
  追加、`KvsOperationResponse` への revision / ERROR_CONFLICT 追加)、
  operator(エンベロープ encode/decode、counter の apply 増加・import
  max-merge・snapshot 載せ替え、CAS 判定)。
- **Stage D(lease lock)**: proto(`KvsLock`、LOCK_* コマンド、
  `Operation.lock_owner/lock_generation`、ERROR_LOCKED)、operator
  (lock apply 3 コマンド + guarded write 検証、lock index と失効スキャン)、
  sector / hosting(失効スキャンの subRoutine tick 駆動、`KvsOperation` →
  新コマンドの配線)。
- **simulator 検証**: kvsload に CAS / lock の負荷パターンを追加し、
  `@@ kvs verify` に「generation 不一致書き込みが store に反映されない」
  「CAS 競合下でも lost update がない」検証を足す(各 Stage の詳細は api.md)。

各ステップで design.md の apply 規約(冪等・必ず完了・状態ゲートで
黙殺しない)を再確認すること。特に conflict は「waiter へのエラー返却」で
あって apply の失敗ではない(ApplyProposal の waiterErr / storeErr の
区別に従う)。

## 保証しないこと(明示的な限界)

- **絶対的な相互排除**: (a) 失効から旧 owner の自主停止までの窓、
  (b) 過半数喪失による lock レコード消失、では二重保持が起こり得る。
  guarded write により KVS 上の状態は壊れないが、KVS 外の副作用は
  守れない(守るなら副作用側で generation を検証する)。
- **generation の跨障害単調性**: sector の複製ごと消失すると counter が
  リセットされ、新 generation が過去より小さくなり得る(TODO 参照)。
- **merge データ移送の replica 遅延窓**(dataplane.md 既載)は lock 状態にも
  同様に当てはまる: fence 前に commit 済み・吸収側未 apply の lock 操作は
  移送から漏れ得る。

## TLA+ 検証の要否

**現時点では追加検証は不要**と判断する。根拠:

- lock/revision の状態遷移はすべて**単一 raft グループ内の複製ステート
  マシン**に閉じており、並行性は raft の線形化に還元される。モデル検証が
  価値を持つのは sector 間のインターリービング(activation / split / merge /
  leftover)だが、本設計はそこに**新しいアクションを足さない**(既存の
  移送・snapshot 経路にデータが乗るだけ)。
- revoke の CAS 解放は `KvsSectorMergeLock.tla` で検証済みの mergeBy /
  ReleaseMerge と同型で、誤発動時の safety は同モデルの Phase 3(無条件
  解放でも safety 成立)がそのまま当てはまる。
- lease の時間依存部分(TTL、スキュー)は TLA+ で自然にモデル化しにくく、
  かつ設計上「時間判定が誤っても generation が safety を守る」構造に
  してあるため、検証の投資対効果が低い。

再検討のトリガー: counter の max-merge を**移送経路に手を入れて**変更する
場合(per-key 単調性の不変条件が split/merge のインターリービングに依存する
ようになるため、`KvsSectorLeftover` 系にステートを足して検証する価値が出る)。

## TODO

- **generation の跨障害単調性**: counter リセット対策として、sector 再作成時に
  ランダムエポック(または粗い wall-clock 上位ビット)を counter 初期値に
  混ぜる案を検討。fencing を KVS 外の副作用に使う場合に必要になる。
- ~~クライアント側 lock ライブラリ~~ → **api.md で設計済み (2026-07-12)**:
  managed `Lock`(renewal ループ、self-fencing の `Done()`)と PREPARING
  リトライの内蔵。実装は api.md Stage A/D。
- **失効スキャンのコスト**: lock 数が多い場合の per-tick 全走査を避けるなら
  deadline heap。まずは全走査で実測してから。
- ~~COMMAND_PATCH の存廃~~ → **存置と決定 (2026-07-12)**: pluggable Patcher
  (利用者登録・決定性契約・クラスタ均質性規約付き)として再定義する。
  正典は api.md「Patch」、実装は Stage C。
- **revoke の margin 値**: host 間時計スキューの実測に基づいて決める
  (NTP 前提なら数百 ms + 余裕で十分のはず)。
- merge 移送の replica 遅延窓(dataplane.md TODO)の閉鎖。lock 状態が
  乗るようになると影響範囲が広がるため優先度が上がる。
