# KVS sector raft snapshot の設計

## 背景と動機

KVS のデータプレーンは「書き込み = raft ログエントリ」というモデルであり、
1 回の Set で value 全体が raft ログに載る。ログは `raft.MemoryStorage` に
オンメモリで追記され、compact されるのは snapshot 作成時のみ。snapshot が
無い現状では、消費メモリが「これまで書き込んだ全 value の総和 × レプリカ数」
となり、生存データ量が一定でも時間とともに青天井で増える。

想定用途（大きなデータ・高頻度の書き換え）ではこれは成立しないため、
raft snapshot + log compaction によりメモリを有界化する。

### スコープ

- **対象**: raft ログのメモリ有界化、および compact 後の follower 追従
  （InstallSnapshot）。
- **対象外**: プロセス再起動をまたぐ永続化・crash recovery。store も
  raft storage もオンメモリのままであり、再起動でレプリカが消えるのは
  従来どおり membership 側のリカバリ（fresh sectorNo での再追加）で扱う。

## snapshot に含める状態の決定

raft snapshot は「ログ index 1..I をすべて適用した後のステートマシンの完全な
状態」でなければならない。sector のステートマシン
（`Sector.ConsensusApplyProposal`）が変更する状態のうち、**全レプリカで一致
すべき複製状態**だけを含め、各ノードが自分の役割から再構築する**ローカルな
意図**は含めない。

| 状態 | 保持場所 | 判定 | 理由 |
|---|---|---|---|
| KV records | store (operator 経由) | 含める | データ本体 |
| `tail`（activation + range） | `Sector.tail` / operator range | 含める | activate/extend/split/merge の apply 結果 |
| `mergeBy`（merge lock） | `Sector.mergeBy` | 含める | prepare/release merge の CAS 状態。欠くと復帰レプリカだけ lock 状態が乖離し KvsSectorMergeLock の安全性が崩れる |
| `terminated` | `Sector.terminated` | 含める | terminate apply の結果 |
| members (sectorNo→nodeID) | `Consensus.members` | 含める | conf change の replay でしか構築されない routing 表。compact 後に snapshot で参加するメンバーは replay できないため、snapshot で運ぶしかない（raft の `ConfState` は raft ID しか持たない） |
| `splittingAddress` | operator | 含めない | `Migrate`（ローカルの split 起動）でのみ設定され apply では変更されない → ローカル意図 |
| `proposal*` フラグ | `Sector` | 含めない | 「このノードが提案したいこと」であり apply で clear される。復帰後は各ノードの retry ループが再構築する |
| `keys` | operator | 含めない | records から導出可能 |
| `confState` / index 類 | raft | 含めない | raft 自身が `Snapshot.Metadata`（ConfState/Index/Term）で管理する。Data には入れない |

### tail の乖離について

`Sector.tail` と operator の range はすべての apply ハンドラでペアで更新される
ため通常は常に一致する。唯一の例外は terminate で、`terminateLocked` は
operator 側だけ `ClearRange` して `Sector.tail` を残す。ただし
`terminated=true` が以後の全 proposal をゲートするため tail はもはや意味を
持たない。したがって snapshot では tail を 1 フィールドで扱い、
**`terminated=true` のときは tail を適用しない**（terminate の復元手順に従う）
と定めることで乖離は問題にならない。

### inactive sector の records

split の手順上、Import は未 activate の sector（tail == nil）にも適用される
（frontward sector へ records を入れてから CommitSplit で activate する）。
そのため「tail == nil だが records を持つ」snapshot は正当であり、
records の有無を tail と結び付けてはならない。records のエクスポートも
範囲フィルタ付きの `ExportRecords`（tail 必須）ではなく全件エクスポートを
使う。

## payload スキーマ（2 層構成）

members は consensus 層の所有物であり、tail/mergeBy/terminated/records は
sector・operator 層の所有物である。所有層を跨いだ 1 メッセージにはせず、
**consensus 層が sector 層の payload（bytes）を包む** 2 層構成とする。
consensus 層は sector payload をデシリアライズしない（素通し）。

```proto
// sector 層（Handler.ConsensusGetSnapshot が返し、
// Handler.ConsensusApplySnapshot が受け取る payload）
message SectorSnapshot {
  repeated Import.Record records    = 1; // KV 全レコード
  colonio.v1alpha.NodeID tail       = 2; // nil = 未 activate。terminated=true のとき無視
  colonio.v1alpha.NodeID merge_by   = 3; // nil = merge lock なし
  bool                   terminated = 4;
}

// consensus 層（raftpb.Snapshot.Data に入る外側メッセージ）
message ConsensusSnapshot {
  bytes sector_state = 1; // SectorSnapshot をシリアライズしたもの
  map<uint64, colonio.v1alpha.NodeID> members = 2; // sectorNo → nodeID
}
```

## 生成（leader 側）

`maybeTriggerSnapshot`（consensus ループ goroutine、`appliedIndex -
snapshotIndex > snapCount` で発火）:

1. `handler.ConsensusGetSnapshot()` → sector が `SectorSnapshot` を組み立てる
   - `s.mtx.RLock` の下で tail/mergeBy/terminated を読む
   - records は `operator.ExportAllRecords()`（範囲フィルタなし全件）
2. consensus が `members`（`n.mtx.RLock` の下で clone）と合成して
   `ConsensusSnapshot` を作り、`raftStorage.CreateSnapshot(appliedIndex,
   confState, data)` に渡す
3. `snapshotCatchUpEntriesN` 分を残して `Compact`

ConsensusGetSnapshot は consensus ループ goroutine から呼ばれ、
`ConsensusApplyProposal` と直列（Ready ループ内で apply の後）なので
sector の lock と衝突しない。送信 API を呼ばないため Transferer の
「mtx 下で送信 API を呼ばない」規約とも無関係。

## 適用（follower 側）

Ready ループの snapshot 分岐（`!raft.IsEmptySnap(rd.Snapshot)`）:

1. `raftStorage.ApplySnapshot` / `confState`・`snapshotIndex`・
   `appliedIndex` の更新（既存処理）
2. `ConsensusSnapshot` をデシリアライズし、`n.members` を **置換**
   （`n.mtx.Lock`）
3. `handler.ConsensusApplySnapshot(sector_state)` → sector が復元:
   - `terminated=true` なら `terminateLocked` 相当の手順
     （ClearRange・ReleaseSector・terminated/stopped セット・handler 通知）
   - それ以外は、store を `ReleaseSector`→`AllocateSector` でリセットした上で
     `operator.ReplaceRecords(records)`（keys 置換 + store 書き込み）、
     tail があれば `s.tail` セット + `operator.SetRange`、なければ
     `ClearRange`、`mergeBy` を復元

適用は **merge ではなく置換**である点が重要: 遅れた既存レプリカが snapshot を
受ける場合、ローカルに残る古い key（snapshot に無い key）は消えなければ
ならない。`ImportRecords`（merge）ではなく store リセット + 全置換で行う。

復元手順は apply ハンドラ同様、冪等でなければならない（重複適用・
再適用に耐える）。

## members 復元の必要性（補足）

`Consensus.members` は raft メッセージの宛先解決
（sectorNo → nodeID → transferer 送信）に使う routing 表で、conf change
エントリの apply でのみ更新される。ログが compact されると、snapshot で
参加するメンバーは compact 済み区間の conf change を replay できないため、
snapshot Data に members を含めないと「raft 上はメンバーだが誰にも
メッセージを送れない」レプリカができる。raft の
`Snapshot.Metadata.ConfState` は raft ID (= sectorNo) の集合しか持たず
nodeID との対応を運べないため、Data 側で運ぶ。

## 実装状況

- [x] Stage 1: 本設計
- [x] Stage 2: payload proto + Export/Apply 実装（2026-07-11）
- [x] Stage 3: trigger の有効化（2026-07-11）。`publishEntries` が適用済み
      バッチの最終 index まで `appliedIndex` を前進させ、`snapCount`(=1000)
      エントリごとに snapshot 生成 + compact が走る。同関数に重複適用
      フィルタも追加（同一 Ready 内で snapshot と重なる committed entries
      をスキップ。etcd raftexample と同型）。
- [x] Stage 4: follower 追従経路（2026-07-11）。`snapshotGuardStorage` は
      **撤去せず恒久化**した: 初回 snapshot が存在しない間だけ
      `ErrSnapshotTemporarilyUnavailable` で送信をスキップし（この間は
      ログ未 compact なので index 1 から replay 可能）、snapshot 生成後は
      素通しで InstallSnapshot が送られる。送信経路は既存の
      `sendMessages`（`msg.Marshal()` が snapshot data を含む）で成立。
      compaction 後に join した member が InstallSnapshot で追いつき、
      members 復元・voter 昇格・後続 proposal 適用まで到達することを
      結合テストで検証済み
      (`TestConsensus_joinAfterCompaction_catchesUpViaSnapshot`)。
- [ ] Stage 5 以降: 下記 TODO

### snapshot と ConfState のずれ（既知の許容事項）

`sendMessages` は MsgSnap 送信時に `Metadata.ConfState` を最新へ差し替えるが、
`Data` 内の members マップは snapshot 作成時点のまま。作成後に conf change が
あった場合、受信者は一時的に「ConfState にはいるが members にいない」peer を
持ち得る。ただし snapshot 以降のログ（conf change エントリ、nodeID context
付き）を直後に replay して members が追いつくため一時的な送信スキップ
（"Unknown node sectorNo" warning + raft リトライ）で収束する。etcd
raftexample と同型の挙動。

## Stage 5 シミュレーション検証の手順（準備済み）

データプレーン未実装のため、raft エントリの供給源は activation プロトコル
（management proposal + conf change + 選挙時 empty entry）のみ。閾値を
下げることで、この churn だけで snapshot 経路を頻繁に発火させて検証する。

**検証できること**: snapshot/compaction/InstallSnapshot が churn 下で正しく
動くこと（activation liveness の非退行、実 transferer 経由の MsgSnap 送受信、
compact 後の join/promotion）。
**検証できないこと**: メモリ有界化の定量評価と MsgSnap のサイズ限界
（records が空のため。Stage 6 でデータプレーン実装後に行う）。

1. `simulator/deploy/node/random/node.yaml` のコメントアウトされた env を
   有効化する:
   - `COLONIO_KVS_SNAP_COUNT=20`（デフォルト 1000）
   - `COLONIO_KVS_SNAP_CATCHUP=10`（デフォルト 100）
2. いつもの手順で実行: `make simulate-random` → 数時間 → `make export`
3. ログの観測点（simulator のログ収集は "==" / "@@" 行のみ保持）:
   - `@@ snapshot export`（leader が snapshot 生成、sector 層）
   - `@@ snapshot compact`（compact 実施、applied/compact index 付き）
   - `@@ snapshot apply`（follower が InstallSnapshot を適用）
4. 判定基準:
   - `export`/`compact` が出続け、`apply` が compaction 後の join で観測される
   - never-active / 赤・黄 率が run 16 の水準（never-active 0.3% 相当）から
     悪化しない
   - `Failed to apply snapshot` / `need non-empty snapshot` panic /
     watchdog 連鎖が出ない

### Stage 5 検証結果（2026-07-12 run, 4.1h, 200 node / 8 pod, SNAP_COUNT=20 / CATCHUP=10）

- snapshot 生成 + compact **65,551 回**、InstallSnapshot 適用 **30,186 回**。
  発火周期は applied 21/42/63/84... と snapCount どおり繰り返し。
- 失敗ゼロ: `Failed to apply snapshot` 0、`need non-empty snapshot` panic 0、
  publishEntries の gap エラー 0、watchdog 0。
- snapshot ループなし: 1 sector member あたりの apply は最大 2 回。
- terminated snapshot は 887 回生成されたが適用は 0 回
  （terminate 直後にグループが解体されるため。問題なし）。
- 健全性: yellow（inactive host）は各 5 分バケットで 0〜4、毎回別 node で
  恒久 stall なし。**最終状態 yellow=0**、never-active（10 分以上生存かつ
  非 stop）**0 体**（run 15 の 0.3% から改善維持）。red は churn による
  一時的な観測値のみ（1〜28 で振動、蓄積なし）。
- `Unknown node sectorNo` warn は 345k 件あるが、全 pod に均等・定常で
  加速なし（歴代の divergence storm の兆候である加速 + force terminate
  連鎖は不在。force terminate は 1,649 回で時間とともに減少）。churn 起因の
  背景ノイズと snapshot members の一時ずれ（上記「既知の許容事項」）の
  合算とみられる。snapshot 導入前の run との定量比較は未実施。
- 副次観測: management churn だけでは 1 sector group の生涯エントリ数は
  概ね 20〜100。本番値 snapCount=1000 では管理系 churn で snapshot が
  発火することはほぼなく、データプレーン書き込みが実質的な発火源になる。

## TODO

- **Stage 5: パラメータ調整と検証**
  - [x] churn 下の非退行をシミュレーションで確認（2026-07-12、上記）。
  - `snapCount`(1000) / `snapshotCatchUpEntriesN`(100) の本番値の実測調整
    → データプレーン実装後の書き込み負荷で行う。
  - `Unknown node sectorNo` warn のベースライン比較（snapshot 導入前 run が
    残っていないため未実施。気になる場合は main 相当で再 run して比較）。
  - snapshot を含む MsgSnap のメッセージサイズと transferer/network 層の
    パケットサイズ上限の関係を確認（records が大きい sector の snapshot は
    1 メッセージで送られる）→ データプレーン実装後（Stage 6）に実測。
- **Stage 6: データプレーンとの結合検証**（データプレーン実装後）
  - `operator.Set/Patch/Delete/ApplyProposal`（現状 panic スタブ）実装後、
    実書き込み負荷で snapshot/compaction を検証。
  - `Operation.operation_id` の重複適用防止（dedup 状態）を複製状態に
    加える場合、それも snapshot に含める必要がある（SectorSnapshot に
    フィールド追加）。
  - 必要なら TLA+ モデル（KvsSectorRaft）へ snapshot 適用の遷移を追加して
    安全性を再検証。
