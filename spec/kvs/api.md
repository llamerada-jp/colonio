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

### Stage B: revision / CAS(lock.md の層 1)

1. proto: `KvsRecord` エンベロープ、`Operation.cas_revision`、
   `SectorSnapshot` / Import / Export への counter 追加、
   `KvsOperationResponse` への revision と ERROR_CONFLICT 追加。
2. operator: エンベロープ encode/decode、sector counter(apply 増加・
   import で max-merge・snapshot 載せ替え)、CAS 判定。
3. 公開面: `GetResponse.Revision` / `SetResponse.Revision`、
   `WithRevision` / `WithAbsent`、`ErrConflict`。
   CAS 付き操作の UNKNOWN 自動再送もここで入れる。
4. simulator: kvsload に CAS 負荷(read-modify-write ループ)を追加、
   `@@ kvs verify` に「CAS 競合下でも lost update がない」検証を追加。

### Stage C: Patch(pluggable Patcher)

Stage B に依存(envelope、revision 返却、`WithRevision` の at-most-once)。

1. proto: `Operation` に patcher 名と patch bytes(COMMAND_PATCH の再定義)。
2. operator: apply での「store.Get → Patcher.Apply → store.Set」、未登録
   patcher / patch 失敗の waiterErr 化。host 側の propose 前ゲート
   (未登録名の即エラー)。
3. 公開面: `Client.Patch`、`node.WithKvsPatcher`、`Patcher` インタフェース、
   `WithoutValue`(GetOption)。`patchertest.AssertDeterministic` ヘルパ。
4. simulator: 複数 node から同一 key への並行 patch 後に**全 replica の
   value が一致する**こと(決定性の実地検証)を `@@ kvs verify` に追加。
   参照 Patcher(JSON Patch 実装)を負荷生成器用に用意する。

### Stage D: lease lock(lock.md の層 2)

1. proto: `KvsLock`、LOCK_* コマンド、`Operation.lock_owner/lock_generation`、
   ERROR_LOCKED。
2. operator / sector: lock apply(acquire=renewal 兼用・release/revoke の
   CAS)、guarded write 検証、lock index と失効スキャン(subRoutine tick)。
3. 公開面: `Client.Lock` と managed `Lock`(renewal ループ、self-fencing、
   `Done()`)。低レベル API は公開しない。
4. simulator: 二重起動防止シナリオ(複数 node が同一 key の Lock を奪い合い、
   guarded write の交錯が store を壊さないこと、`Done()` 後の書き込みが
   ErrConflict になること)を検証。

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
