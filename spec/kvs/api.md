# KVS 公開 API の設計(module client / context / retry 内蔵 / Watch)

2026-07-12 設計。未実装。ライブラリは未リリースで互換性制約がないため、
現行の `chan` 返し API(`Colonio.KvsGet` 等)は**全面置き換え**とする。
lock 機構のプリミティブ設計は spec/kvs/lock.md、データプレーンの意味論は
spec/kvs/dataplane.md を正典とし、本書はそれらを呼び出す公開面だけを扱う。

## 全体像

```go
type Colonio interface {
    KVS() *kvs.Client
    // 他モジュール (messaging 等) も同型のアクセサに揃える余地がある (スコープ外)
    ...
}

// package kvs (公開型。internal の実装とは別パッケージ)
func (c *Client) Get(ctx context.Context, key string, opts ...GetOption) (*GetResponse, error)
func (c *Client) Set(ctx context.Context, key string, value []byte, opts ...WriteOption) (*SetResponse, error)
func (c *Client) Patch(ctx context.Context, key string, patcher string, patch []byte, opts ...WriteOption) (*SetResponse, error)
func (c *Client) Delete(ctx context.Context, key string, opts ...WriteOption) error
func (c *Client) Lock(ctx context.Context, key string, opts ...LockOption) (*Lock, error)
func (c *Client) Watch(ctx context.Context, key string, opts ...WatchOption) (*Watcher, error) // 将来拡張

type GetResponse struct {
    Value    []byte
    Revision uint64
}

type SetResponse struct {
    Revision uint64 // apply で採番された新 revision。CAS ループの再 Get を不要にする
}

// GetOption
func WithoutValue() GetOption              // revision のみ取得 (HEAD 相当。大 value の CAS 前確認用)

// WriteOption
func WithRevision(rev uint64) WriteOption  // CAS: 一致時のみ適用
func WithAbsent() WriteOption              // 「レコード不在」を期待する CAS (作成競合の防止)
func WithLockToken(gen uint64) WriteOption // guarded write (lock.md)
func WithoutRetry() WriteOption            // 内蔵リトライの無効化 (一発勝負)

// typed error (errors.Is で判定。実装は types/kvs の sentinel の alias)
var (
    ErrNotFound      = ... // Get/Delete: key 不在
    ErrPreparing     = ... // リトライ可能クラス。通常は内蔵リトライが消費し、
                           // deadline 超過時に ctx エラーと併せて wrap されて届く
    ErrResultUnknown = ... // 書き込み結果不定 (timeout)。CAS なしの盲目再送は不可
    ErrConflict      = ... // Stage B: revision / generation 不一致。再読み取りしてから再試行
    ErrLockHeld      = ... // Stage D: 他 owner が保持中
)
```

## 設計判断

### 1. `chan` 返しをやめて `context.Context` + 同期返し

現行の `KvsSet(key, value) chan error` は、キャンセルと deadline を呼び出し側が
制御できず(操作タイムアウトはサーバ側固定の operationTimeout=10s のみ)、
合成も手作業になる。非同期にしたい呼び出し側は `go func()` で包めば済むが、
逆変換は不格好。etcd clientv3・database/sql・各種クラウド SDK が収斂した
`(ctx) → (result, error)` に揃える。

### 2. PREPARING リトライをライブラリに内蔵

dataplane.md の TODO で定量確認済みのとおり、churn 中は range が split/merge
fence・再 activate 待ちで十数秒 PREPARING に留まり、素朴な呼び出しの ~9% が
失敗する。PREPARING はトポロジ実装の内部事情であり、呼び出し側全員に同じ
バックオフループを書かせない。

- `Get` / `Set` / `Delete` は `ErrorSectorNotReady`(PREPARING)を内部で
  バックオフ付き再送し、**ctx の deadline まで**粘る。呼び出し側には
  ctx 超過時のみ `context.DeadlineExceeded`(原因エラーを wrap)が見える。
  ctx に deadline がない場合は無期限に粘る(呼び出し側の責務)。
- UNKNOWN(タイムアウト = 結果不定)の自動再送は **CAS 付き操作
  (`WithRevision` / `WithAbsent`)に限る**。CAS は再送しても at-most-once
  (lock.md「CAS 操作」)。無条件 Set / Delete は再送せず typed error で返し、
  判断を呼び出し側に委ねる。**Patch は一般に非冪等**(後述)なので、
  `WithRevision` 併用時のみ自動再送の対象になる。
- `WithoutRetry()` で全リトライを無効化できる(simulator の検証など、
  生の失敗率を観測したい用途)。
- ERROR_CONFLICT / ERROR_LOCKED は apply 結果であり盲目再送不可
  (lock.md「エラーマッピングの拡張」)。リトライ対象にしない。

### 3. モジュール別クライアント + 具象型返し

`Colonio` インタフェースに `Kvs*` プレフィックスでメソッドを増やす形をやめ、
etcd clientv3 の `client.KV` / `client.Lease` / `client.Watch` と同様の
モジュール別アクセサ `cn.KVS()` にする。

- 戻り値はインタフェースでなく**具象型**(`*kvs.Client`, `*GetResponse` 等)。
  struct へのメソッド追加は非破壊だが、公開インタフェースへの追加は利用側
  実装を壊す。Watch / Txn 等の将来追加に備える(「interface を受け取り
  struct を返す」)。
- `Set` にも Revision を返させる(etcd の Put が header revision を返すのと
  同じ)。sector counter 方式なので apply 時に自明に得られる。

### 4. lock は managed な `Lock` オブジェクトだけを公開

低レベルの acquire/release(raft の LOCK_* コマンド 1 対 1 の API)は
**公開しない**。renewal と self-fencing を欠いた生 acquire は誤用
(renewal 忘れ → 意図せぬ失効)しかねず、正しく使うには結局ラッパが要る。
公開面を `Lock` に絞ることで、内部プロトコル(LOCK_* の形)を後から自由に
変えられる。一回きりの排他は `Lock` して即 `Release` すればよい。

```go
type Lock struct { ... }

func (l *Lock) Token() uint64          // generation (fencing token)。WithLockToken に渡す
func (l *Lock) Done() <-chan struct{}  // 保持喪失 (とみなすべき時点) で close
func (l *Lock) Err() error             // Done 後の喪失理由
func (l *Lock) Release(ctx context.Context) error
func (l *Lock) Set(ctx context.Context, value []byte, opts ...WriteOption) (*SetResponse, error)
    // locked key 自身への guarded write の糖衣 (WithLockToken を自動付与)

// LockOption
func WithTTL(d time.Duration) LockOption   // 既定 30s (lock.md「TTL の下限」)
func WithTryOnce() LockOption              // 取得待ちせず ErrLockHeld を即返す
```

内部の責務:

- **renewal ループ**: TTL/3 間隔で acquire を再送(owner 冪等なので UNKNOWN も
  盲目再送可)。PREPARING は内蔵リトライで吸収(churn の十数秒窓を跨ぐ)。
- **self-fencing 判定**: ローカル単調時計で「最後の renewal 成功から
  TTL(マージン込み)経過」したら、lease の実際の生死に関わらず `Done()` を
  close する。保守側に倒す。**`Done()` は advisory であり、本当の防御は
  guarded write が `ErrConflict` で落ちること**。
- **取得待ち**: `Lock` は `ErrLockHeld` をポーリングで待つ(ctx で打ち切り)。
  Watch 実装後は Watch ベースに置き換える。

lock が守るのは **locked な key 自身だけ**(lock.md「guarded write」。
key は別 sector にハッシュされ得るため、apply 時の cross-sector 検証は
構造的に不可能)。etcd の Lease のように 1 つのリースを複数 key に付ける
形には**寄せない**と決めておく。複数の値を 1 つの lock で守る場合は
1 レコードにまとめて CAS する。

### 5. owner は暗黙に呼び出し元 node

lock の owner を引数に取らず、常にローカル NodeID を使う。「実行中の node
だけが書ける」という想定用途にちょうど合い、他 node への成りすまし API を
最初から存在させない。

## Patch(pluggable Patcher)

大きな value の一部更新用。patch の適用形式(JSON Patch 等)はデータ形式に
依存するため KVS は規定せず、**利用者が Patcher として組み込む**。

CAS の read-modify-write では代替にならない: 大 value の RMW は全文 Get +
全文 Set が毎回ネットワークと raft ログを通るが、サーバ側 patch なら
**クライアント→host も raft ログも patch 文書だけが流れ**、各 replica が
手元の value に適用する。帯域・ログサイズ・(snapshot 間の)ログ再生コストの
すべてで patch が構造的に有利。

```go
// 利用者が実装する
type Patcher interface {
    // Apply は決定的な純関数でなければならない:
    // 同じ (current, patch) は全 replica で同じ結果を返すこと
    Apply(current []byte, patch []byte) ([]byte, error)
}

// node 構築時に名前付きで登録 (WithKvsStore と同列の ConfigSetter)
node.WithKvsPatcher("json-patch", myJSONPatcher{})

// クライアント側は patcher 名を指定して patch 文書を送る
res, err := kv.Patch(ctx, key, "json-patch", patchBytes)  // res.Revision が返る
```

- `Operation` に patcher 名と patch bytes が載る。apply 時に operator が
  envelope を decode → 登録済み Patcher の `Apply` → 結果を store に書き戻す。
  patch 後の value は**返さない**(大データ用途なので転送しない。必要なら Get)。
- patch の失敗(path 不在、test op 不成立、未登録の patcher 名)は
  waiterErr(クライアントへの typed error)であって apply の失敗ではない
  (lock.md「実装ステップ」の waiterErr / storeErr の区別に従う)。
- `kvsTypes.Store.Patch` は**削除する**。patch 適用は operator 層で
  「store.Get → Patcher.Apply → store.Set」と分解でき、store は dumb な
  byte store のままでいられる(カスタム store 実装者に patch 意味論を
  背負わせない)。

### 決定性契約(利用者への要求。最重要)

Patcher は raft apply の中で全 replica 上で走る**利用者コード**であり、
design.md の apply 決定性規約がそのまま適用される。非決定な Patcher は
普通のバグではなく **replica 乖離 = 状態破壊**を起こす。契約として明文化する:

- `Apply` は純関数であること。時計・乱数・環境・グローバル状態を参照しない。
- 再シリアライズの罠に注意: Go の `encoding/json` は map キーをソートする
  ので素朴な unmarshal→marshal は一応決定的だが、キー順保持系のライブラリや
  浮動小数の再フォーマットは replica 間で割れ得る。バイト列を直接操作する
  patch 形式が最も安全。
- 決定性の実地検証ヘルパ(`patchertest.AssertDeterministic` のような、
  同一入力の反復適用・並行適用で結果一致を確認するテストユーティリティ)を
  ライブラリとして提供する。

### クラスタ均質性の規約

patch は key の host とその replica(任意の node)で適用されるため、
**全 node が同じ Patcher 群(同じ名前・同じ挙動)を持つ**必要がある。
colonio はアプリに埋め込むライブラリで全 node が同一バイナリという前提なら
普段は自然に満たされるが、**ローリングアップグレード中は破れる**(新 patcher を
知らない replica と知っている replica で apply が割れる)。これは raft
ステートマシンのコード更新一般と同じ問題で完全には解決できないため、
「**patcher の追加・挙動変更は全 node の更新完了後に使い始める**」を運用規約と
する。host 側で propose 前に未登録名を即エラーにするゲートは入れる
(誤設定の早期検出)が、replica 間の差は防げないことを明記しておく。

### 再送制限と revision 併用

- patch は一般に**非冪等**(JSON Patch の `add` を配列に 2 回適用すると
  二重追加)。素の `Patch` は UNKNOWN 自動再送の対象外。
- `WithRevision` を併用すると CAS と同じ at-most-once になり自動再送可能。
  大 value で revision を知るためだけの全文 Get を避けるために
  `Get(ctx, key, WithoutValue())`(revision のみ返す HEAD 相当)を対で用意する。
- 楽観制御を patch 形式側の語彙(JSON Patch の `test` op 等)で行う道もあり、
  それは Patcher の実装内の話で KVS は関知しない。

## Watch(将来拡張。初期実装には含めない)

メタデータ用途はいずれ変更通知が欲しくなる(lock 取得待ち、プロセス状態の
監視)。revision が単調な今回の設計は「revision N 以降の変更を通知する」形の
Watch をそのまま支えられるため、API 面だけ先に確保しておく。

```go
func (c *Client) Watch(ctx context.Context, key string, opts ...WatchOption) (*Watcher, error)

type Watcher struct { ... }
func (w *Watcher) Events() <-chan WatchEvent
func (w *Watcher) Err() error // Events close 後の理由

type WatchEvent struct {
    Key      string
    Value    []byte // Deleted のとき nil
    Revision uint64
    Deleted  bool
}

func WithSinceRevision(rev uint64) WatchOption // これより後の変更から通知
```

### 意味論(設計スケッチ)

- **per-key watch のみ**。prefix / range watch は非目標: key はリング全体に
  ハッシュ分散されるため、範囲購読は全 sector への購読になり、この
  アーキテクチャでは自然に実装できない。
- **coalesced / at-least-once**: 全変更履歴の配送は保証しない。切断・host
  交代を挟んだ場合は「最新状態」1 イベントに合流し得る。revision は単調で、
  同一 revision の重複配送はあり得る(受信側は revision で冪等化)。
- 実装スケッチ: watcher は key の host に購読を登録し、host は apply フックで
  イベントを push する。PREPARING / host 交代 / 切断を検知したら
  `WithSinceRevision(最終受信 revision)` で再登録し、現 revision が
  それより大きければ現在状態を 1 イベントとして配送する(差分回復)。
  購読は host 側で lease 的に管理し、keepalive が絶えたら破棄する。
- **既知の限界**: tombstone を持たないため、再登録時に「レコードが不在」の
  場合、購読断の間に Delete があったのか元々不在だったのかを区別できない。
  不在は `Deleted` イベントとして配送する(冪等なので過剰通知は無害)。

## 実装タスク(interface 変更を含む見直し)

lock.md の実装ステップを置き換える全体順序。API 再編を先に行うことで、
以降の機能追加が公開面の破壊的変更を伴わなくなる。

### Stage A: 公開 API 再編(proto 変更なし・機能同等)— 実装済み (2026-07-12)

1. ~~`kvs.Client`(公開パッケージ)新設~~ → **`node/kvs` パッケージとして実装**。
   internal を import しない構造的 `Backend` インタフェース経由で
   internal/kvs.KVS を呼ぶ。typed error は types/kvs の sentinel の alias
   (`ErrNotFound` / `ErrPreparing` / `ErrResultUnknown`。
   `ErrorOperationResultUnknown` を types/kvs に追加し、従来 UNKNOWN で
   潰れていたコードを typed 化)。
2. ~~PREPARING の内蔵リトライ~~ → 実装済み(backoff 100ms→2 倍→上限 2s、
   ctx deadline まで。deadline 超過時は `ErrPreparing` と ctx エラーの
   多重 wrap で返し、in-flight timeout の `ErrResultUnknown` と区別できる)。
   `WithoutRetry` も実装。
3. ~~`Kvs*` メソッド削除 → `KVS()` アクセサ~~ → 実装済み。
   `KvsGetStability` は公開 `Node` インタフェースに元々含まれておらず
   (internal kvs.Handler の実装メソッド)、対応不要だった。
4. ~~呼び出し箇所の移行~~ → 実装済み。kvsload の自前リトライ層
   (kvsRetry/kvsAwait*)を削除し、per-op 15s deadline + 内蔵リトライに
   置き換え。`@@ kvs load` の出力形式が変更:
   旧 `prep(試行回数)/timeout/exhausted` → 新 `prep(deadline 時 preparing)/
   unk(結果不定)/err`。Stage 6 合格基準の読み替えに注意。
   test/e2e の KVS 利用は元々コメントアウト済みで変更なし。
5. ~~旧 `COMMAND_PATCH` 経路と `kvsTypes.Store.Patch` の削除~~ → 実装済み。
   apply に届いた COMMAND_PATCH は default 分岐で決定的な waiter エラーに
   落ちる(apply 失敗ではない)。

### Stage B: revision / CAS(lock.md の層 1)— 実装済み (2026-07-12)

1. ~~proto~~ → 実装済み: `KvsRecord`(deterministic marshal で store value に
   格納)、`Operation.cas_revision` + `cas_absent`(「不在を期待する CAS」は
   番兵でなく独立フラグにした)、`Import.revision_counter` /
   `SectorSnapshot.revision_counter`、`KvsOperationResponse` の revision と
   ERROR_CONFLICT。
2. ~~operator~~ → 実装済み: counter は apply でのみ増加(SET 成功時。
   DELETE は増やさない — 再作成の ABA は counter 単調性で既に閉じている)、
   Import は max-merge、snapshot 復元は verbatim 代入。CAS 判定は apply 内で
   決定的(conflict は waiterErr であって apply 失敗ではない)。
   waiter は `applyResult{err, revision}` を返す。
3. ~~公開面~~ → 実装済み: `WithRevision(0)` と `WithRevision`+`WithAbsent` の
   併用は呼び出し時に即エラー(0 は wire 上の「無条件」番兵のため黙って
   条件が消えるのを防ぐ)。CAS 付き書き込みは UNKNOWN を自動再送
   (ctx が生きている場合のみ。再送が conflict になる偽陰性は ErrConflict の
   doc に明記)。
4. ~~simulator~~ → 実装済み: 負荷 mix を Set 55% / CAS RMW 15% / Get 25% /
   Delete 5% に変更。CAS 成功時に `@@ kvs cas ok: <key> <baseRevision>` を
   出力し、**クラスタ全ログで (key, baseRevision>0) が重複しないこと**が
   lost update 不在の判定基準(base=0 = 不在条件は delete/再作成で正当に
   複数回勝てるため除外。過半数喪失の counter リセットは稀な偽陽性源 —
   force-terminate ストームと突合する)。`@@ kvs load` に
   `cas/conf/prep/unk/err` を追加。

   **run 検証済み (2026-07-12 run, ~30min, 激 churn)**:
   - corrupt 0(Set 79k / CAS 成功 18.7k / Get 34k)、watchdog 0、
     snapshot 発火継続。
   - **バグ発見→修正**: `responseErrorToError` に ERROR_CONFLICT の
     逆マッピングが欠けており、conflict が結果不定に化けて CAS の自動再送が
     deadline まで空回りしていた(conf=0 / unk=15.5% ≒ 実際の競合率が
     全部 unk に計上)。修正済み + 全コード網羅の回帰テスト追加
     (`TestResponseErrorToError`)。conf/unk の正常化は次回 run で確認する。
   - cas ok 重複 91/18.5k は**全て発生間隔 2 分以上**(85/94 は 5 分超)。
     RMW 窓(サブ秒)での重複は 0 件 = lost update の証拠なし。
     force terminate 134 回による counter リセット後の revision 再訪
     (文書化済みの偽陽性)と整合。ノイズ削減には lock.md TODO の
     lineage epoch が要る。

   **再 run 検証 (2026-07-12 run 2, ~19min)**:
   - **conflict 修正の効果を確認**: conf 3,077(CAS の 21.8%、無条件 Set 55%
     と共有 key 空間なら妥当な競合率)、cas unk 15.5%→**0.37%**(set の
     0.78% と同水準に正常化)。corrupt 0 / watchdog 0 は維持。
   - cas ok 重複 40 件のうち**短間隔(3〜56 秒)が 4 件**。counter リセット
     では説明できないためトレースした結果、原因は **merge/overlap 抗争
     ループ**: 生きている node (c5ea…) の active sector を、routing 視界の
     不一致から隣接 host (c3a…) が「死んだ host の leftover」と誤認 →
     tail 切り詰め activate → merge で吸収(この sector への CAS が成功)→
     生存側が再 activate → 重複検知で両方 Terminate(**ack 済み書き込みごと
     データ破棄** = design.md で許容済みのクラス)→ 再ループ。46 秒間に
     Terminate 27 回、同一 (key, base=488) の CAS が 3 回成功。
     **CAS 実装のバグではなく**、既知の ack 済み書き込みロスト窓が CAS 監査で
     初めて定量可視化されたもの(発生率 ~0.04% of CAS)。恒久対策をするなら
     「merge 前の leftover host 生存確認」等の activation 層の課題
     (design.md「churn 下のメンバーシップ管理の課題」ファミリー)。

### Stage C: Patch(pluggable Patcher)— 実装済み (2026-07-13)

Stage B に依存(envelope、revision 返却、`WithRevision` の at-most-once)。

1. ~~proto~~ → 実装済み: `Operation.patcher`(patch 文書は `value` に載せる)、
   `KvsOperation.patcher` / `without_value`、ERROR_PATCH_FAILED
   (定性的失敗: store 未変更)。
2. ~~operator~~ → 実装済み: `Patcher` registry は node 全体で 1 つの
   immutable map を全 operator が共有。propose 前ゲート(未登録名は
   `ErrorPatchFailed` 即返し)+ apply 内の決定的再検査。apply は
   store.Get → envelope decode → Patcher.Apply → counter++ で再 encode →
   store.Set。不在 key は NOT_FOUND(作成は Set の仕事)、patcher 拒否・
   未登録・decode 失敗はすべて waiterErr(apply 失敗ではない)。
   CAS 併用可(casConflictLocked が先に走る)。
3. ~~公開面~~ → 実装済み: `Client.Patch(ctx, key, patcher, patch, opts...)`
   (`WithAbsent` は misuse として即エラー、patcher 名必須)、
   `node.WithKvsPatcher(name, patcher)`、`kvsTypes.Patcher`(決定性・
   クラスタ均質性の契約を godoc に明文化)、`Get(..., WithoutValue())`
   (host 側で value を落とす HEAD 相当)、`ErrPatchFailed`。
   素の Patch は UNKNOWN を再送しない(非冪等)、`WithRevision` 併用時のみ
   自動再送 — Stage A/B の retry 機構にそのまま乗る。
   `patchertest.AssertDeterministic`(逐次+並行反復・入力非破壊の検査。
   純粋性の証明ではなくヒューリスティックである旨を明記)も追加、
   stateful な patcher を検出する self-test 付き。
4. ~~simulator~~ → 実装済み: JSON Patch でなく **counter increment
   patcher(`sim-inc`)** を参照実装とした(外部依存なし・バイト列直接操作で
   決定性の罠がない)。value 形式 `key|N|padding` の N を +1 する。
   負荷 mix は Set 50% / CAS 15% / **Patch 5%** / Get 25% / Delete 5%。
   patch 成功後は verify probe(key プレフィックス検査)を通すため、
   非決定 patcher や移送での乖離は `@@ kvs verify corrupt` に現れる。
   `@@ kvs load` に `patch/nf/prep/unk/err` を追加(err は
   ERROR_PATCH_FAILED を含む=0 が正常)。

   **run 検証済み (2026-07-12 run, ~5.4min)**:
   - patch 成功 523 / **err 0**(ERROR_PATCH_FAILED ゼロ = registry 均質・
     value 形式健全)、**corrupt 0**(非決定 patcher・移送乖離の兆候なし)。
   - patch nf 11% ≒ get nf 10%(Delete 5% 環境の正常系)、
     patch unk 2.6% ≒ set unk 1.7% と同オーダー。
   - 短時間 run のため snapshot 発火なし・prep 比率は立ち上がり支配で高め。
     cas ok 重複 2/2355 は既知クラス(counter リセット / merge 抗争)。
     長時間 run での snapshot 併用検証は次回以降の通常 run に相乗りで足りる。

   **長時間 run 検証済み (2026-07-12 run, 4.3h, 155 万操作)**:
   - **corrupt 0**(set 791k / cas 222k / patch 73k / get 386k)、
     **patch err 0**、watchdog 0 — snapshot 1,405 回発火・force terminate
     1,010 回の churn 下で patcher 決定性と envelope 移送が維持された。
   - **メモリ有界**: クラスタ平均 heapAlloc は 750〜920MiB で 4 時間以上振動、
     単調増加なし(envelope + patch 負荷でも snapshot/compaction が有効)。
   - unk は全操作 0.3〜0.6%、miss 0.42%(過去 run の 0.64〜0.77% より改善)。
   - cas ok 重複 1,957/217k (0.9%): 97% が 5 分超(counter リセット偽陽性、
     force terminate 1,010 回と整合)、10 秒未満は 13 件(~3/h、既知の
     merge/overlap 抗争クラス。design.md の未解決課題のまま)。
   **Stage A〜C はデータプレーン全体として長時間 churn 検証済み。**

### Stage D: lease lock(lock.md の層 2)— 実装済み (2026-07-13)

1. ~~proto~~ → 実装済み: `KvsRecord.lock`(envelope field 3。移送・snapshot に
   自動で乗る)、LOCK_ACQUIRE/RELEASE/REVOKE、`Operation.lock_owner /
   lock_generation / lock_deadline_ms`、`KvsOperation` の LOCK_* +
   `lock_ttl_ms` / `lock_generation`、応答の `lock_generation /
   lock_deadline_ms` + ERROR_LOCKED。**owner は packet source を host が
   採用**(偽装可能な owner フィールドを wire に置かない、設計どおり)。
2. ~~operator / sector~~ → 実装済み:
   - acquire は renewal 兼用(同一 owner は generation 維持で deadline 延長 =
     owner 冪等)、不在 key は lease 付き空レコードを作成。generation は
     revision counter から採番。
   - release/revoke は (owner, generation) の CAS。unlocked への release は
     no-op 成功(再送安全)、不一致は release→CONFLICT / revoke→沈黙
     (stale 失効が新 lease を壊さない)。
   - guarded write(lockGuard): unlocked+token→CONFLICT(lease 喪失)、
     locked+無/他者 token→LOCKED、locked+旧 generation→CONFLICT(fencing)。
     SET/PATCH で lease は ride along、holder の guarded DELETE は
     release+delete の atomic。読み取りは lease で阻まれない。
   - 失効スキャン: lock index は導出状態(apply/Import/Replace/SetRange で
     同期)、hosting sector の 3 秒 tick が deadline+margin(3s) 超過を検知して
     LOCK_REVOKE を propose(`@@ lock revoke` ログ)。apply は時計を読まない。
     TTL は host 側 clamp [5s, 1h]。
3. ~~公開面~~ → 実装済み: `Client.Lock`(`WithTTL` 既定 30s・クライアント側
   最小 10s、`WithTryOnce`)、managed `Lock`(renewal TTL/3、self-fencing =
   最終証明から TTL−interval で `Done()` close、`Token/Key/Done/Err/Release` +
   guarded `Set/Patch/Delete` 糖衣)。renewal が「失効後の再交付」を受けた
   場合は新 lease を release して喪失として報告(古い token は死んでいるため
   黙って続行しない)。`Release` の CONFLICT は「もう自分のものではない」=
   成功扱い。`WithLockToken` は guarded write 用で、**単独では
   at-most-once にしない**(再送で guard は再び通る。必要なら WithRevision
   併用)ことを明記。低レベル acquire/release は非公開。
4. ~~simulator~~ → 実装済み(`kvslockload.go`): 二重起動防止サイクル
   (共有 32 key の Lock を奪い合い、保持中のみ 2 秒間隔で guarded write、
   5〜15 秒保持 → release)。監査: `@@ kvs lock ok/end: <key> <generation>`
   の**同一 key の区間重複 = double grant**、(key, generation) の
   クラスタ一意性(counter リセットの偽陽性は CAS 監査と同じ)、
   `@@ kvs lock` 分計(acq/held/prep/unk/err, guard ok/conf/locked/err,
   lost/rel)。guard conf は「自分は保持中と思っているのに fence された」=
   lost と対で現れるのが正常。

   **run 検証済み (2026-07-12 run, 40min, 激 churn)**:
   - acquire 成功 4,820 / guarded write 23,159 / corrupt 0。
   - **相互排除監査**: 保持区間の重複 26/5,031 (0.5%)。全件が「後側の
     generation が極小(counter リセット後の新系譜での再交付)」=
     sector データ喪失クラス(force terminate 164 回・merge 抗争環境)で、
     **fencing が全件捕捉**(guard locked 22 + conf 6 ≒ 重複件数、黙って
     続行した holder は 0)。設計どおり「二重交付は起こり得るが guarded
     状態は壊れない」を実測確認。
   - 失効スキャン(`@@ lock revoke`)130 回 ≒ 死亡 holder 数(shutdown 82 +
     end なし 24 + fence 切断 31)と整合。
   - **課題発見→修正**: acquire の unk が定常 13〜20% と高かった。原因は
     「保持中でも acquire が毎回 raft propose になる」ため、待機 node の
     1 秒ポーリング(数百 node × 32 key)が lock key の host group への
     proposal 殺到になっていたこと。**host 側 fast-path 拒否**(未失効の
     他者 lease が applied 済みなら propose せず LOCKED 即返し。deadline+
     margin 超過後は素通しなので takeover はこのゲートに依存しない)を
     実装済み(回帰テストあり)。unk 正常化は次回 run で確認する。
   - lost=0 は正常(renewal 間隔 10s より guarded write 間隔 2s が先に
     fence を検知するため。lease 喪失は guard-locked/conf として現れた)。

   **再 run 検証 (2026-07-16 run, ~15min)**:
   - **fast-path の効果を確認**: acquire unk 13〜20% → **8.5%** に半減。
     区間重複は **1/1,863 (0.05%)**(0.6 秒、後側 generation=3 = counter
     リセット指紋、guard locked 1 で fencing 捕捉)。corrupt 0、データ
     プレーンの unk は全操作 0.5% 以下を維持。
   - 残る unk 8.5% の大半は**クライアント分類のアーティファクト**と特定:
     競合待ちの 45s deadline が「ポーリングの in-flight 中」に切れると、
     直前まで LOCKED を観測していても unk に分類されていた
     (unk/(held+unk)=19% ≒ RTT/(RTT+poll 1s) と整合)。
     → `Client.Lock` を修正済み: deadline 時に直近の確定観測が「保持中」
     なら ErrLockHeld として報告(回帰テストあり)。12:55〜58 の
     churn バンプ(15〜18%)は本物の混雑で、既知の churn 挙動の範囲。
   - **Stage D 完了**。unk 指標のベースライン確認は次回の通常 run に相乗り。

### Stage E: Watch(必要になったら)

上記スケッチの具体化。lock 取得待ちのポーリング置き換えもここで。

## TODO

- messaging 等、他モジュールのアクセサ同型化(スコープ外だが Stage A の
  ついでに形だけ決める価値あり)。
- `KvsGetStability` の置き場所(Stage A の 3)。
- Watch の購読管理(host 側 lease、split/merge 時の purge)の詳細設計は
  Stage E 着手時に。sector の移送・snapshot に**購読状態は載せない**
  (複製状態ではなく host ローカルの導出状態とし、host 交代はクライアント側
  再登録で回復する)方針だけ先に固定しておく。
- ctx deadline なしで PREPARING が永続するケース(range の恒久欠損)の
  観測性: 内蔵リトライが握りつぶさないよう、リトライ回数・待ち時間の
  メトリクス/ログ露出を Stage A で入れる。
