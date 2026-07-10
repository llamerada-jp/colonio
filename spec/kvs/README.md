# KVS セクター活性化プロトコル仕様

## 概要

KVS の ring 区間管理・活性化プロトコルを TLA+ でモデル化し、
安全性（safety）と活性（liveness）を形式的に検証する。

5 種類のモデルを用意:

| モデル | メンバーシップ | ネットワーク | 検証対象アクション |
|--------|---------------|------------|--------------------|
| **KvsSector**        | 固定              | 遅延なし | ActivateFirst, ActivateFrontward |
| **KvsSectorDyn**     | 動的（Join/Leave） | 遅延なし | + Split, Extend |
| **KvsSectorDelay**   | 動的             | 遅延あり（1 ビュー） | + RefreshView, Merge, Terminate |
| **KvsSectorSepView** | 動的             | 遅延あり（2 ビュー） | Merge が実際に発火する拡張版 |
| **KvsSectorRaft**    | 動的             | 遅延 + Raft 非原子性 | Activate/Split/Merge を非原子化、Terminate が発火 |

## 用語

- **スタル**: 古いビューや未反映情報により、本来進むべき操作が進まない状態（進行停滞）。
- **チャーン (churn)**: ノードの Join/Leave が短時間に繰り返し発生すること。
- **インターリーブ (interleaving)**: 複数ノード/複数操作の手順が途中で割り込んで交互に実行されること。
- **safety（安全性）**: 「悪い状態が一度も起きない」性質（例: NoOverlap が常に成り立つ）。
- **liveness（活性）**: 「いつか良い状態に到達する」性質（例: EventuallyAllActive）。

## ファイル構成

```
spec/kvs/
  KvsSector.tla            # 固定メンバー版の仕様
  KvsSectorMC.tla          # 固定版モデルパラメータ
  KvsSector.cfg            # 固定版 TLC 設定

  KvsSectorDyn.tla         # 動的メンバー版の仕様（遅延なし）
  KvsSectorDynMC.tla       # 動的版モデルパラメータ
  KvsSectorDyn.cfg         # 動的版 TLC 設定

  KvsSectorDelay.tla       # ネットワーク遅延あり版の仕様　（rView 1 つ）
  KvsSectorDelayMC.tla     # 遅延版モデルパラメータ
  KvsSectorDelay.cfg       # 遅延版 TLC 設定

  KvsSectorSepView.tla     # 遅延 + 2 ビュー分離版 (Merge が発火する)
  KvsSectorSepViewMC.tla   # 2 ビュー版モデルパラメータ
  KvsSectorSepView.cfg     # 2 ビュー版 TLC 設定

  KvsSectorRaft.tla        # Raft 非原子性版 (Terminate が発火する)
  KvsSectorRaftMC.tla      # Raft 版モデルパラメータ
  KvsSectorRaft.cfg        # Raft 版 TLC 設定

  README.md                # このファイル
```

## 対応する Go 実装

| TLA+ | Go 実装 |
|------|---------|
| `ActivateFirst(n)`     | `activateHostingSector(checkEntireState=true)` |
| `ActivateFrontward(n)` | `operateSectors` の Activate ケース |
| `Split(n)`             | `operateSectors` の Split ケース |
| `Extend(n)`            | `operateSectors` の Extend ケース |
| `Merge(n)`             | `operateSectors` の Merge ケース |
| `Terminate(n)`         | `operateSectors` の Terminate ケース |
| `ProposeActivate(n)`   | `activateFrontwardSector()` の Raft.Propose (Raft 版のみ) |
| `CommitActivate(n)`    | Raft commit → `processActivateProposal()` (Raft 版のみ) |
| `ProposeSplit(n)`      | `splitSector()` の PreCommitSplit (Raft 版のみ) |
| `CommitSplit(n)`       | `splitSector()` の CommitSplit (Raft 版のみ) |
| `ProposeMerge(n)`      | `mergeSector()` の ProposeMerge (Raft 版のみ) |
| `CommitMerge(n)`       | `mergeSector()` の CommitMerge (Raft 版のみ) |
| `AbortProposal(n)`     | 提案対象が離脱した場合のキャンセル (Raft 版のみ) |
| `Join(n)`              | `hostingManager.ManageMember` がメンバー追加 |
| `Leave(n)`             | `hostingManager.ManageMember` がメンバー削除 |
| `RefreshRView(n)`      | routing / gossip の近傍情報伝播 |
| `LearnSActive(n)`      | 新規 sector 起動の Raft 通知 (SepView 版のみ) |
| `ForgetSActive(n)`     | sector 停止の Raft 通知 (SepView 版のみ) |
| `rView[n]`             | `KvsGetStability()` 返値 (routing 由来の近傍ノード集合) |
| `sActives[n]`          | `k.sectors` のうち active な head の集合 (SepView 版のみ) |
| `NextInView(n)`        | `frontwardNextNodeID` (routing 由来) |
| `FrontwardSector(n)`   | `getFrontwardCondition` が返す frontwardNextSector の head (SepView) |
| `IsBetween(x, from, to)` | `types.NodeID.IsBetween(back, front)` |
| `anyActive`            | seed の `ResolveKvsActivation` / `EntireState` |
| `state[n]`             | `hostingSector.GetTailAddress() != nil` |
| `tail[n]`              | `hostingSector.GetTailAddress()` |

> **Delay 版と SepView 版の違い**:
> Delay 版は routing と sector-store の両方を `rView` 1 つに約しているため、
> Go 実装では起きる「f を store 経由で見えているが routing 上では f
> より外側のノードが近傍と見える」という Merge トリガ状態が表現できない。
> SepView 版は `rView` (routing) と `sActives` (sector-store) を別変数に分け、
> Go と同じく 2 つの情報源が独立に遅延されるものとして Merge を再現した。

## 抽象化（モデルで省略しているもの）

- **Raft 合意**:
  - 固定版 / 動的版 / 遅延版 / SepView 版: 即時反映と仮定。
  - Raft 版: ActivateFrontward, Split, Merge を各 Propose → Commit の 2 ステップに
    分割。Propose と Commit の間に他ノードのアクションが介入できるため、
    一時的なセクター重複が発生し Terminate が発火する。
    CommitMerge は吸収対象が既に inactive の場合を冪等に処理。
    Raft リーダー選出・ログ複製・過半数合意は省略。
  - **全モデル共通の暗黙の前提: 提案は必ず最終的に commit される**。
    Raft 版でも Commit 系アクションに WF を付与しており、「グループが quorum を
    失って提案が永久にペンディングになる」故障モードは表現できない
    (さらに `Leave(n)` は `proposing[n] = "none"` を前提とするため、提案中の
    離脱も起きない)。シミュレーション (2026-07-04) でこの前提が実装では
    成立しないことが確認された。→ [TODO-1](#todo-1)
- **パケット損失 / メッセージ順序入れ替え**: 省略
- **sector メンバーシップ管理** (`hosting.Manager`): 省略（Members 集合で抽象化）
- **ネットワーク遅延**:
  - 固定版 / 動的版: 省略
  - 遅延版: 「各ノードのローカルビュー」として抽象化。
  - SepView 版: routing と sector-store を 2 つの独立したビューとしてモデル化。
- **情報源の統合 (Delay 版の制限)**:
  Delay 版は routing と sector-store を `rView` 1 つに畳んでいるため、
  Go で Merge をトリガする「store より routing が先に進んだ」状態が表現できない。
  この制限を取り除いたのが SepView 版。
- **Terminate の発火条件 (Raft 版での発見)**:
  Go の `operateSectors` には 2 つの Terminate-both パスがある:
  - (A) `!frontwardNodeMatch` かつ `frontwardNextSector` が自分のセクター内
  - (B) `frontwardNodeMatch` だが `frontwardNextSector` が自分のセクター内

  SepView 版以下の原子的モデルでは NoOverlap が常に保たれるため両方とも
  発火しない。Raft 版では Propose→Commit 間のインターリービングにより
  セクター重複が一時的に発生し、(B) パスの TerminateB が発火することを確認。
- **anyActive のリセット**: Go では一度 TRUE で持続するが、
  動的モデルでは最後の active が抜けた時に FALSE に戻す（liveness 確保）

## 検証する性質

### Safety（全状態で成立）

- **TypeOK**: 変数の型整合
- **NoOverlap**: active なセクター同士の区間 `[head, tail)` が重ならない
  - Raft 版では一時的に破れる（Propose→Commit 間の非原子性）ため、
    INVARIANT ではなく、代わりに EventuallyNoOverlap を PROPERTY で検証
- **ValidRange**: active セクターの tail が ring 上で妥当な位置にある
- **ActiveFlagConsistent**（動的版以降）: `anyActive = (active メンバーが居る)`

### Liveness（最終的に成立）

- **AllActive / EventuallyAllActive**: 全（現メンバー）が active になる
- **FullCoverage / EventuallyFullCoverage**: ring がギャップなくカバーされる
- **EventuallyNoOverlap** (Raft 版のみ): 一時的な重複が解消される

## 実行方法

```bash
# tla2tools.jar が必要
cd spec/kvs

# 固定メンバー版
java -jar tla2tools.jar -workers auto -config KvsSector.cfg KvsSectorMC.tla

# 動的メンバー版
java -jar tla2tools.jar -workers auto -config KvsSectorDyn.cfg KvsSectorDynMC.tla

# ネットワーク遅延あり版 (1 ビュー)
java -jar tla2tools.jar -workers auto -config KvsSectorDelay.cfg KvsSectorDelayMC.tla

# 遅延 + 2 ビュー分離版 (Merge が発火)
java -jar tla2tools.jar -workers auto -config KvsSectorSepView.cfg KvsSectorSepViewMC.tla

# Raft 非原子性版 (Terminate が発火)
java -jar tla2tools.jar -workers auto -config KvsSectorRaft.cfg KvsSectorRaftMC.tla
```

## パラメータ調整

### 固定版 `KvsSectorMC.tla`

```tla
MC_Nodes == 0..2   \* N=3（高速、数秒）
MC_Nodes == 0..3   \* N=4
MC_Nodes == 0..4   \* N=5
```

### 動的版 `KvsSectorDynMC.tla`

```tla
MC_Nodes          == 0..2     \* N=3
MC_InitialMembers == {0, 1}   \* 最初のメンバー（⊆ Nodes、非空）
MC_MaxChurn       == 2        \* Join + Leave の総回数上限
```

### 遅延版 `KvsSectorDelayMC.tla`

rView の状態空間が加わるためサイズは控えめにすること。

```tla
MC_Nodes          == 0..2     \* N=3
MC_InitialMembers == {0, 1}
MC_MaxChurn       == 1        \* 0、1、2 が現実的
```

### SepView 版 `KvsSectorSepViewMC.tla`

rView と sActives の 2 つの遅延状態が換え算で増えるため、
Delay 版よりさらに控えめにする。Merge を見てたいなら N=4 が必要。

```tla
MC_Nodes          == 0..3     \* N=4
MC_InitialMembers == {0, 3}   \* 初期メンバーをソースに 1、2 は遅れて Join
MC_MaxChurn       == 1
```

### Raft 版 `KvsSectorRaftMC.tla`

proposing 状態が加わるため SepView 版より状態空間が大きい。

```tla
MC_Nodes          == 0..2     \* N=3
MC_InitialMembers == {0, 1}
MC_MaxChurn       == 1
```

組み合わせ目安（8 コアでの実測）:

| モデル    | N | InitialMembers | MaxChurn | 状態数  | 所要時間 | Merge 発火 |
|---------|---|----------------|----------|----------|---------|------------|
| Dyn     | 3 | {0,1}          | 2        | 43       | < 1 秒  | -          |
| Dyn     | 3 | {0,1}          | 3        | 71       | 1 秒    | -          |
| Dyn     | 4 | {0,1}          | 3        | 191      | 2 秒    | -          |
| Dyn     | 4 | {0,1}          | 4        | 306      | 3 秒    | -          |
| Delay   | 3 | {0,1}          | 1        | 46       | 1 秒    | 0 (到達不可) |
| Delay   | 3 | {0,1}          | 2        | 153      | 1 秒    | 0 (到達不可) |
| Delay   | 4 | {0,1}          | 2        | 731      | 4 秒    | 0 (到達不可) |
| SepView | 3 | {0,1,2}        | 1        | 645      | 5 秒    | 0 (N=3 では起こらず) |
| SepView | 4 | {0,3}          | 1        | 884      | 5 秒    | 24         |
| SepView | 4 | {0,1,2,3}      | 1        | 35,489   | 42 秒   | 0 (初期全員参加だと rView スタートが完全、スタルが起きず) |

| モデル | N | InitialMembers | MaxChurn | 状態数 | 所要時間 | Terminate 発火 |
|--------|---|----------------|----------|--------|---------|---------------|
| Raft   | 3 | {0,1}          | 1        | 1,208  | 1 秒    | TerminateB: 8, ProposeMerge: 30, CommitMerge: 27  |
| Raft   | 3 | {0,1}          | 2        | 3,206  | 3 秒    | TerminateB: 8, ProposeMerge: 45, CommitMerge: 29  |
| Raft   | 3 | {0,1}          | 3        | 32,131 | 7 秒    | TerminateA: 20, TerminateB: 165 |
| Raft   | 3 | {0,1}          | 4        | 49,371 | 8 秒    | 複数チャーンレース検証 |
| Raft   | 4 | {0,3}          | 1        | 2,380  | 2 秒    | TerminateB: 14, ProposeMerge: 28, CommitMerge: 40 |

## 検証で見つけたバグ・改善点

| バグ | 場所 | 修正 |
|------|------|------|
| `ValidRange` の `IsBetween` の否定が逆 | `KvsSector.tla` | `~` を削除 |
| `Extend` が inactive ノードを飛ばし liveness 破壊 | `KvsSector.tla` | 固定版では Extend 自体不要と判明し削除 |
| 全 active 到達後の deadlock 誤検出 | `KvsSector.cfg` | `CHECK_DEADLOCK FALSE` |
| `Split` が全周 sector で発火しない | `KvsSectorDyn.tla` | `tail[n] # n` ガードを削除 |
| 古いビューで ActivateFrontward が重複 sector を作る | `KvsSectorDelay.tla` | 重なり検査ガードを追加 |
| 2 ビュー分離で Extend が古い rView と sActives の不一致で sector 重なり | `KvsSectorSepView.tla` | Extend に重なり検査ガードを追加 |
| Raft 非原子性で CommitActivate 後に提案対象が離脱 → liveness 破壊 | `KvsSectorRaft.tla` | AbortProposal アクションを追加 |
| frontwardNodeMatch=true でもセクター重複が発生する (Go L418-424) | `KvsSectorRaft.tla` | TerminateB アクションを追加 |
| Merge で吸収される側が提案中でも proposing がクリアされない | `KvsSectorRaft.tla` | Merge で吸収側の proposing をリセット |
| CommitActivate/CommitSplit が anyActive を更新しない (MaxChurn≥2) | `KvsSectorRaft.tla` | commit 時に anyActive = TRUE を設定 |
| TerminateB で inactive にされるノードの proposing がクリアされない | `KvsSectorRaft.tla` | TerminateB で proposing/propTarget/propTail もリセット |
| CommitMerge で fs を active→inactive にした際に anyActive を更新しない | `KvsSectorRaft.tla` | CommitMerge の THEN 節で anyActive を条件更新 |

## モデルのスコープ外で見つかった実装バグ（2026-06）

「ほとんどの Sector が active にならない」症状の原因は、モデルの抽象度より
**下のレイヤー（Go のロック実装）** にあった。全モデルの liveness が成立していても、
以下のバグ群はアクション内部の並行性に起因するため TLC では原理的に検出できない。

| バグ | 内容 | 影響 |
|------|------|------|
| `sectorActivate` の自己デッドロック | `k.mtx.Lock()` を保持したまま `activateHostingSector` を呼び、その末尾の `markSectorUpdated()` が再度 `k.mtx.Lock()` を取得（Go の mutex は非再入） | **ActivateFrontward の受信側が必ずフリーズ**。`k.mtx` が永久に保持され、Raft 受信 (`processConsensusMessage`) も停止。最初に ActivateFirst した 1 セクターのみ active になり、チェーン活性化が全停止 |
| 単独ノードの活性化不能 | `activateHostingSector` の「他セクター不存在」チェックが hosting sector 自身を数えていた | ActivateFirst (tail=self) が一度も発火しない |
| `SectorAppendNode` ⇄ `ManageMember` の ABBA | `k.mtx → m.mtx` と `m.mtx → k.mtx`（hosting.Manager 経由）の取得順序逆転 | 確率的な相互デッドロック |
| `takeObservation`/`countActiveSectors` の k→s エッジ | `k.mtx` 保持中に `GetTailAddress()`（s.mtx）を取得 | Terminate commit (s→k) と循環 |
| `processTerminateProposal` の s→m/k エッジ | `s.mtx` 保持中に `SectorTerminated` ハンドラ（m.mtx, k.mtx）を呼ぶ | `applyMemberSectors` (m→s) と循環 |
| 活性化の重なりガードが inactive レプリカも対象 | モデルの `ActivateFrontward` ガードは `\A other \in Actives` だが、Go の `activateHostingSector` は `k.sectors` の**全**セクター（inactive 含む）で `IsBetween` 判定していた | 停止したノードの inactive レプリカは誰も掃除しないため、`[local, frontward)` に死んだレプリカの head が残ると活性化が**永久にスキップ**され、チェーンがその点で停止（シミュレータ 231 ノード・ランダム停止で再現: active が 31 で停滞）。モデルでは Leave が Members/Actives から即座に除去するため「離脱ノードの inactive レプリカ残留」が表現されず検出不能だった |
| `applyProposals` が `s.mtx.RLock` 保持中にブロックする `raftNode.Propose` を呼ぶ (2026-07-04 発見) | etcd raft の `Propose` はリーダー不在の間ブロックし続けるため、quorum 喪失グループでは retry ループが RLock を握ったまま停止する | 後続の write lock（timeout 後の proposal クリア、強制破棄、`Terminate` 等）が全て永久に待たされ、**timeout を実装しても効かない**。修正: 提案を RLock 下で収集しロック解放後に Propose + `consensus.Propose` に有界 context (2s) を導入 |
| inactive セクターへの Terminate の apply が完了不能 (2026-07-04 発見) | `processTerminateProposal` が `ReleaseSector` のエラーで `terminated` を立てずに return。inactive セクターは `AllocateSector` 未実行のため必ず失敗する | Terminate が「commit → apply 失敗 → 再提案」を永久に繰り返し、**健全なグループが quorum 喪失と同一の症状**（毎秒空振り）を示し活性化チェーンを恒久停止。commit が進み続けるため pending バックストップも発火しない。修正: terminate の apply を必ず完了させる + activate/import の AllocateSector を冪等化 |
| explicit パケットの非宛先受理 → ゴーストレプリカ (2026-07-04 run3 発見) | `classifyPacket` はルートテーブルが自ノードを返すと explicit でも `Receive` していた | 死亡ノード宛の SectorManageMember / raft メッセージを別ノードが受理し、**同じ raft メンバー ID を複数の物理ノードが名乗る**。誤ノードへのデータ複製・本来メンバーの永久非同期・二重投票リスク。修正: explicit は宛先一致時のみ受理 |
| apply エラーで committed entries のバッチが中断 (2026-07-04 run3 発見) | `publishEntries` が apply エラーで即 return するが `Advance()` は実行される | 同一バッチの残り committed entries が**適用されないまま消費**され、そのメンバーだけ状態が乖離。非冪等な `processCommitSplitProposal`（エラー返却 + proposal 未クリア → 3 秒ごと再提案）が毒エントリー化して恒常的にこれを誘発。修正: ログして継続 + CommitSplit apply の冪等化 |
| 破棄済みレプリカの同一キー復活 → raft panic (2026-07-04 run5 発見) | 非 Normal メンバーへの setting message 毎秒再送 × ローカル強制破棄の組み合わせで、ack 済みレプリカが**同じ {sectorID, sectorNo} で空ログ再作成**される（learner-first で再送窓が拡大し顕在化） | グループはその raft ID の Match・投票を記憶しており、「メンバーはログを失わない」前提が破れて **etcd raft 内部で panic（プロセス停止・recover 不能）**。復活→再破棄のループが破棄数も増幅。修正: sector tombstone で同一キー再作成を拒否 + 停滞メンバーを reap して新 sectorNo で再追加 |
| snapshot 未実装のまま raft が snapshot 送信を要求 → panic (2026-07-04 run6 発見) | `operator.ExportSnapshot`/`ImportSnapshot` がスタブな上、`appliedIndex` が通常エントリーで更新されず snapshot 作成トリガが不活性。raft はフォロワーの Next がログ範囲外になると `Storage.Snapshot()` を要求し、空だと panic する | churn でフォロワー進捗とリーダーのログがずれた瞬間 **`need non-empty snapshot` でプロセス停止**。修正: `snapshotGuardStorage` が空 snapshot を `ErrSnapshotTemporarilyUnavailable` に変換（送信スキップ → reap による新 sectorNo 再作成で index 1 から追いつく）。snapshot 本実装は TODO |

**教訓**: TLA+ のアクションは原子的にモデル化されるため、アクション「内部」の
ロック取得順序はモデルの検証対象外。実装側は以下のロック規約で防ぐ
(`node/internal/kvs/kvs.go` の `KVS.mtx` コメント参照):

- 取得順序は `s.mtx (Sector) / m.mtx (hosting.Manager) → k.mtx (KVS)` の一方向のみ
- `k.mtx` 保持中に hostingManager・Sector のロックを取るメソッドを呼ばない
- `s.mtx` 保持中に KVS/Manager へコールバックしない（Terminate はロック解放後に通知）

回帰テスト: `node/internal/kvs/kvs_test.go` の
`TestKVS_sectorActivate_completes` / `TestKVS_activateHostingSector_singleNode` /
`TestKVS_sectorActivate_ignoresInactiveSectorBetween`。

### シミュレータ解析からの追加知見（231 ノード・ランダム停止、simulator/dump.json）

- tail が「停止した active ノード」を指すケースは、既存の Merge 修復経路が
  約 1 分で解消することをログ上で確認（恒久停止ではない）。
- `is_stable`（seed の reconcile と routing ビューの一致）は、停止ノードが
  seed の lifespan 失効（約 3 分）で除去されるまで多数のノードでフラップし、
  一部ノードは hosting sector の作成自体が 3 分遅延した。KVS の進行が
  seed 側の失効タイマに律速される構造は将来の改善候補。
- Extend の重なりガード未実装（モデル修正 #6 の Go 側未反映、kvs.go の NOTE 参照）
  に起因するとみられる active セクターの重複が複数残存していた。
  TerminateB による修復は重複相手のレプリカを持たないと発火しないため、
  非隣接ノード間の重複は解消されない。
  → その後 `hasActiveSectorHeadInRange` (kvs.go) として実装済み。

### シミュレーション解析からの追加知見（100 ノード・ランダム停止、2026-07-04）

`simulator/logs.txt`（2 回の実行、2 回目はタイムスタンプ付き）の解析で、
活性化チェーンの**恒久停止**を 2 クラス確認した。いずれも
「**quorum を失った Raft グループは何も commit できない**」ことに起因し、
上記のモデル前提（提案は必ず commit される）の外側で起きている。

| クラス | 症状 | 機構 |
|--------|------|------|
| A: splitSector ハング（1 回目 9 ペア、2 回目 5 件） | hosting 側が `Migrate` から戻らず `mtxOperateSectors` を握ったまま、当該ノードのセクター管理が全停止。frontward 側は `proposedSplitting` を保持したまま待機 | `Migrate` 内の Import 提案が frontward 側グループの quorum 喪失で commit されない。グループが治癒して 23 秒後に回復した例もあるが、過半数喪失時は治癒に必要な ConfChange 自体が commit 不能で永久化 |
| B: stale active レプリカ（1 回目のみ 2 件） | 離脱ノードを head とする **active な**レプリカが活性化の重なりガード (skip 1) を永久発動させ、チェーンがその点で停止 | レプリカの掃除 (`Terminate` / `SectorRemoveNode`) 自体が死んだグループの raft commit を要するため誰にも消せない。掃除経路は backward の hosting が active になった後にしか走らないという鶏卵もある |

付随する観測:

- **Terminate 自体が raft commit を要する**ため、quorum 喪失グループは自分自身を
  終了することすらできない。「proposer 離脱を検知して Terminate」「frontward の
  不一致レプリカを Terminate」がどちらも毎秒空振りし続けるケースを観測。
- 2026-06 に修正した「重なりガードが inactive レプリカも対象」問題（前節の表参照）は、
  active なレプリカが残留するケース（クラス B）では不十分だったことが判明。
- 観測の詳細は `node/internal/kvs/kvs.go` / `node/internal/kvs/sector/sector.go` の
  NOTE コメント (2026-07-04 付) に記録。`sector.go` の `applyProposals` に
  リトライ時の Raft ステータス出力（`## retry proposals ... state/lead/term`）を
  追加済みで、次回実行でリーダー不在を直接確認できる。

## アルゴリズム改善案（Go 実装側）

形式検証で見つかったバグの多くは「複数の独立した状態を整合させる責務がアプリケーション側に
散らばっている」ことに起因する。以下は **Go 実装の構造的な改善案** で、形式モデルを楽に
するためではなく、実装側の堅牢性を上げるためのもの。

### A. proposing 状態をセクター自身が持つ（バグの構造的予防）

**問題**:
TerminateB / Merge で「あるノードを inactive にする」時、そのノードが提案中
(`activateFrontwardSector` / `splitSector` / `mergeSector` の途中) だと、
あとから到着する commit が古い前提で実行されて不整合を起こす。
TLA+ で見つけたバグ 3 件 (#7, #10, #11) は全てこのパターン。

現状の Go 実装では `HasManagementProposal()` で「提案中フラグ」を持つが、
**inactive 化する側 (Terminate/Merge) がこのフラグをクリアする責務を持っていない**。
個別の commit ハンドラ側で「自分が inactive ならスキップ」と防御する必要があり、
パスが増えるほど漏れやすい。

**改善案**:
`sector.Sector` に「外部から inactive 化された時のキャンセル通知」を組み込む:

```go
// sector/sector.go
type Sector struct {
    ...
    cancelPending context.CancelFunc  // 提案中操作のキャンセル
}

func (s *Sector) Terminate() {
    if s.cancelPending != nil {
        s.cancelPending()  // 提案中のものを全部キャンセル
        s.cancelPending = nil
    }
    // ...既存の Terminate 処理
}

func (s *Sector) ProposeActivate(...) error {
    ctx, cancel := context.WithCancel(s.ctx)
    s.cancelPending = cancel
    return s.raft.Propose(ctx, ...)
}
```

commit ハンドラは `ctx.Err()` をチェックして無視するだけになる。
「外側 (kvs.go) で都度クリアを忘れない」のではなく、
「セクター自身が自分のライフサイクルを守る」設計。

### B. CommitMerge を冪等にする（既に Go 実装で必要）

> **おおむね実装済み (2026-07-04)** — apply ハンドラの「冪等かつ必ず完了」規約として
> 実装した（Terminate の必ず完了・CommitSplit の no-op 化・AllocateSector の許容。
> run2/run3 の修正、design.md の規約参照）。本節の CommitMerge の
> extendTailOnly 分岐のみ未実装。

TLA+ で発見した CommitMerge の二重実行パスは、Go でも以下のシナリオで起こりうる:

1. ノード n が fs に対して `mergeSector(hosting, fs)` を開始
2. `PrepareMerge` → `Merge` まで commit 済み
3. ここで TerminateB が走って fs が独立に inactive 化
4. `frontwardNextSector.Terminate()` がエラーを返す可能性

現在の Go 実装はこの分岐を考慮していない（`Terminate()` の戻り値を見ていない
箇所がある）。**「既に inactive なら no-op」をセクター操作の規約として明文化** すべき。

```go
// sector/sector.go
func (s *Sector) Terminate() error {
    if s.state == StateInactive {
        return nil  // 冪等
    }
    // ...
}

func (s *Sector) CommitMerge(newTail *types.NodeID) error {
    if s.state == StateInactive {
        // 既に他経路で吸収された場合、tail のみ拡張すれば良い
        return s.extendTailOnly(newTail)
    }
    // ...
}
```

### C. anyActive をローカル計算に切り替える

**問題**:
`anyActive` (seed の `ResolveKvsActivation`) はグローバル状態として保持され、
ローカル commit 時に手動で更新する必要がある。これが「更新漏れバグ」を 2 件発生させた
(`CommitActivate/CommitSplit`, `CommitMerge`)。

**改善案**:
`anyActive` は **派生情報** として、必要時に
`len(k.activeSectors()) > 0` から計算する。
- 安全性: 更新漏れバグが構造的に発生しない
- コスト: ローカル `k.sectors` の走査だけなのでホットパスでない
- 互換性: seed 経由の `anyActive` は「分散初期化フラグ」として残し、
  「最初の 1 つを作る」プロトコルだけに使う

```go
// kvs.go
func (k *KVS) hasAnyActiveSector() bool {
    for _, s := range k.sectors {
        if s.IsActive() {
            return true
        }
    }
    return false
}
```

### D. TerminateA パスは本当に必要か再検討

TLA+ では MaxChurn=3 (N=3) で初めて TerminateA が 20 回発火した。
発火頻度が低いことは「特殊条件でしか到達しない」ことを示しており、
**バグの巣窟になりやすい**。

`!frontwardNodeMatch` (routing 上の next ≠ 自セクター tail)
かつ `frontwardNextSector` が自セクター範囲内、というのは
具体的にどんなレース条件で発生するのか? 反例トレースを精査した上で:

- **(案 D-1)** TerminateB に統合できるなら、コードパスを 1 本にする
- **(案 D-2)** TerminateA が本当に必要なら、テストで再現可能にして
  unit test を追加する。現状は到達するのに 3 回以上のチャーンが必要で、
  e2e テストでは安定して再現できない

### E. Split を単一 Raft グループでアトミック化

**問題**:
`splitSector` は `hostingSector.PreCommitSplit()` → `frontwardNextSector.CommitSplit()`
と **2 つの異なる Raft グループ** を順に commit するため、中間状態でカバレッジに
ギャップが生じる。TLA+ で `CommitSplit` 直前に `Leave` が割り込むと
liveness が破れるケースを観測 (#5 AbortProposal で対処済み)。

**改善案 (大きな設計変更)**:
親セクターの Raft ログに「Split イベント」を 1 つ書き、その commit 通知を受けた
新しい hosting 担当が **新規 Raft グループを起動する** (子は親の commit ログから
状態を継承)。
- メリット: 中間状態がなくなる → AbortProposal や複雑な commit 順序制御が消える
- デメリット: 新規 Raft グループ起動の overhead, 子の合意までの遅延
- 実装難易度: 高 (etcd raft の snapshot + 新グループ初期化)

短期的には現状維持で良いが、**スケール時 (数百ノード規模) のスループットが
問題になった場合の構造的な選択肢** として記録しておく。

### F. Merge の 4 コミットを 2 コミットに削減

現状: `PrepareMerge` → `Merge` → `Terminate` → `CommitMerge`
これは TLA+ の `ProposeMerge` → `CommitMerge` 2 コミットで等価にモデル化できた
(検証で見つかったバグはあっても、ステップ数自体は問題なかった)。

実装上 4 コミットになっているのは「異なる Raft グループにまたがる」ためだが、
**Prepare と Merge は同じグループ (吸収される側 fs)** で連続実行可能。
Terminate は CommitMerge の副作用として扱えば、実質 **2 コミット (fs 側 1, 自側 1)**
に減らせる:

```go
// 改善後の流れ
func (k *KVS) mergeSector(hosting, fs *sector.Sector) error {
    // 1. fs 側に "Prepare+Merge" を提案 (1 Raft commit)
    //    成功時、fs は inactive かつ「マージ済み」マークを持つ
    if err := fs.ProposeMergeAndPrepare(k.localNodeID); err != nil {
        return err
    }
    // 2. 自側に CommitMerge (1 Raft commit)
    //    tail 拡張 + fs.Terminate() を副作用で実行
    return hosting.CommitMerge(fs.GetTailAddress())
}
```

メリット: 中間状態の窓が半分になり、レース面が狭まる。

### G. 重複検出の即時化（TerminateB の起動条件）

現在の TerminateB は `operateSectors()` の周期実行に依存しているため、
重複が生じてから検出までに最大「1 routing 更新 + 1 周期」のラグがある。
TLA+ では即時実行に近いモデルでも安全だったため、**Commit 完了通知時に
インラインで NoOverlap チェックを走らせる** ことで修復遅延を短縮できる:

```go
// commit ハンドラ末尾で
func (k *KVS) onAnyCommit() {
    k.checkAndRepairOverlap()  // 即時 Terminate 判定
}
```

副作用: commit ハンドラ内の処理時間が増えるが、O(セクター数) なので
小規模クラスタでは無視できる。

### 優先度サマリ

| 案 | 効果 | 実装コスト | 推奨度 | 状況 (2026-07-04) |
|---|------|----------|--------|-------------------|
| A: proposing をセクター内蔵 | バグ予防 (大) | 中 | ★★★ | 未実装。ただし動機の多く（提案スタックの解消）は TODO-3 の timeout+abort と強制破棄で代替済み |
| B: 操作の冪等化 | バグ予防 (中) | 小 | ★★★ | **おおむね実装済み**: Terminate は必ず完了・CommitSplit は既活性化で no-op・Activate/Import の AllocateSector は割り当て済みを許容（run2/run3 の修正）。CommitMerge の「fs が先に inactive 化された場合の extendTailOnly」は未実装 |
| C: anyActive のローカル化 | バグ予防 (中) | 小 | ★★★ | 未実装 |
| D: TerminateA 再検討 | 保守性 | 小 (調査のみ) | ★★ | 未着手 |
| E: Split のアトミック化 | 構造改善 | 大 | ★ (将来) | 未着手 |
| F: Merge 2 コミット化 | レース面縮小 | 中 | ★★ | 未着手 |
| G: TerminateB 即時化 | 修復遅延短縮 | 小 | ★★ | 未着手 |

## 今後の TODO

### 完了済み

モデル:

- [x] **routing ビューと sector-store を分離** → `KvsSectorSepView.tla` で実装済
- [x] **Raft 合意の非原子性をモデル化** → `KvsSectorRaft.tla` で実装済
      - ActivateFrontward / Split を Propose → Commit の 2 ステップに分割
      - TerminateB が発火することを確認 (N=3: 8回, N=4: 14回)
- [x] **複数の Join/Leave 同時発生のレース検証** → N=3 MaxChurn=4 まで検証済
      - MaxChurn=2 で Merge 吸収側の proposing 未クリア / CommitActivate の anyActive 未更新を発見・修正
- [x] **Merge も非原子化** → `KvsSectorRaft.tla` で ProposeMerge/CommitMerge に分割
      - MaxChurn=3 で TerminateB→CommitMerge 間のインターリーブバグ 2 件を発見・修正
      - TerminateA が MaxChurn=3 で初めて発火 (20回)

Go 実装（いずれも 2026-07-04、詳細は各 run のセクション参照）:

- [x] **TODO-3: セクター操作の timeout + abort**（proposalWaitTimeout=15s、
      Propose の有界化、`applyProposals` のロック外 Propose）
- [x] **TODO-4: quorum 喪失セクターのローカル強制破棄**（leaderless 30s /
      pending 停滞 45s の 2 系統 + CheckQuorum 有効化）
- [x] **apply ハンドラの冪等・必ず完了規約**（terminate 完了保証、CommitSplit
      no-op 化、AllocateSector 許容、publishEntries のバッチ継続 = 改善案 B の主要部）
- [x] **learner-first メンバーシップ**（未同期 voter による quorum 毀損の根絶。
      run7 で「active セクターの破壊 0 件」を確認）
- [x] **raft メンバー ID の使い捨て化**（sector tombstone + 停滞メンバーの
      reap/新 slot 再追加）
- [x] **explicit パケットの非宛先受理ガード**（ゴーストレプリカ対策）
- [x] **snapshot 未実装対策のガード**（`ErrSnapshotTemporarilyUnavailable` 変換で
      `need non-empty snapshot` panic を根絶。本実装は未完了 TODO 側）
- [x] **ManageMember の localNodeID panic ガード** / **sectorPrepareSplit の
      nil ガード**（クラッシュ系の穴埋め）

Go 実装（いずれも 2026-07-06、詳細は run 8〜10 のセクション参照）:

- [x] **Import / CommitSplit の activation ゲート通過**（split が構造的に
      不成立だった規約違反の解消。run 8）
- [x] **bootstrap conf change への nodeID context 付与 + publishEntries の
      conf change 適用エラー継続 + nodeID 不明 learner の promote 抑止**
      （join レプリカの恒久乖離 → 強制破棄ストームの正帰還を解消。run 9）
- [x] **メンバー除去の out-of-band 通知**（COMMAND_REMOVE。除去済みメンバーの
      stale レプリカが強制破棄まで 30〜60 秒残留する赤の主因を解消。run 10）

### 未完了（2026-07-09 run 11 反映）

| 項目 | 種別 | 参照 |
|------|------|------|
| **prepare_merge (mergeBy) の解放経路**（preparer 死亡で merge 永久拒否 → activation チェーン恒久停止。run 11 の最重要） | 設計 + Go + モデル | run 11 (A) |
| **disconnect 経路の na.mtx 保持解消**（nodeLinkChangeState / houseKeeping が pion Close 越しにロック保持 → zombie 残穴） | Go 実装 (node) | run 11 (C) |
| TODO-1: quorum 喪失の拡張モデル（LocalDestroy の safety 検証） | モデル | 下表 |
| TODO-2: stale active レプリカのガード緩和検証 | モデル | 下表 |
| snapshot の本実装（operator serialize + appliedIndex + トリガ。ログ無限成長対策と表裏一体） | Go 実装 | run6 の残課題 |
| is_stable ゲートの緩和（不安定時の修復凍結 = カバレッジ漸減の律速。run 11 で「接続不良 node 1 つで隣接の activation が skip 2 凍結」を確認、sectorActivate の失敗も観測不能） | 設計 + Go | run4 改善候補 2 / run7 考察 / run 11 (B) |
| ManageMember のヒステリシス | Go 実装 | run4 改善候補 3 |
| 改善案 A / C / D / E / F / G（B の残り: CommitMerge extendTailOnly を含む） | Go 実装 | アルゴリズム改善案 |
| 同一 term 二重リーダー疑いの系譜特定（run6 の未特定事項） | 調査 | run6 |
| Col.Stop() 後のセクター raft goroutine 残留（解析ノイズ） | Go 実装 (node/simulator) | run 11 その他 |

### 状況（2026-07-04 のシミュレーション解析より）

TODO-3 / TODO-4 の Go 実装は 2026-07-04 に先行実装した（下記
「quorum 喪失対策の実装」参照）。TODO-1 のモデル検証は未着手のため、
強制破棄の誤発動時の安全性はモデルでは未確認（実装は「誤発動しても既存の
重複修復経路で収束し、データ喪失は design.md が許容済み」という設計判断に依る）。

| # | 内容 | 種別 | 優先度 | 状況 |
|---|------|------|--------|------|
| [TODO-1](#todo-1) | quorum 喪失の故障モードを含む拡張モデル | モデル | 高 | 未着手 |
| [TODO-2](#todo-2) | stale active レプリカの掃除とガード緩和の検証 | モデル | 高 | 未着手（TODO-1 と独立に着手可）。クラス B' は run4/run7 でも継続観測（短時間で解消し恒久化はしていない） |
| [TODO-3](#todo-3) | セクター操作の timeout + abort | Go 実装 | 高 | **実装済み (2026-07-04)**、モデル検証は TODO-1 待ち |
| [TODO-4](#todo-4) | quorum 喪失セクターのローカル強制破棄 | 設計 + Go 実装 | 高 | **実装済み (2026-07-04)**、モデル検証は TODO-1 待ち |

#### quorum 喪失対策の実装（2026-07-04, Go 実装側）

`sector.go` / `consensus.go` に以下の脱出経路を実装した:

1. **提案の有界化** (`consensus.Propose`, proposeTimeout=2s):
   etcd raft の `raftNode.Propose` はリーダー不在の間ブロックし続けるため、
   有界 context を付与した。失敗した提案は既存のリトライループ
   (`applyProposals`, 3 秒間隔) が再提案する。
2. **ブロッキング操作の timeout + abort** (TODO-3, `waitProposal`,
   proposalWaitTimeout=15s): `Extend` / `Import` / `PreCommitSplit` /
   `CommitSplit` / `PrepareMerge` / `CommitMerge` は raft 適用まで待つが、
   タイムアウトで `ErrProposalTimeout` を返し pending proposal をクリアする。
   これにより splitSector が `mtxOperateSectors` を握ったままハングする
   クラス A の全停止が解消される（呼び出し側は既存の abort パスで Terminate）。
   また Stop / 強制破棄で起こされた場合は成功と区別するため
   `ErrSectorStopped` を返す。
3. **ローカル強制破棄** (TODO-4, `checkQuorumLoss`): 「commit できない
   グループ」をローカル判定し、raft を経由せずセクターを破棄する
   (TLA+ の `LocalDestroy` に相当)。判定は 2 系統 (いずれかで発火):
   - リーダー不在 (`Status().Lead == 0`) が forceTerminateDuration=30s 継続。
     leader-without-quorum を検出するため raft の `CheckQuorum` を有効化
     (下記「シミュレーション再実行での発見」参照)。
   - management proposal が pending のまま commit index が
     forcePendingDuration=45s 進まない。リーダー状態の如何によらず
     「commit できない」症状そのものを見るバックストップ。

   破棄は `SectorTerminated` 経由で通常の再作成フローに入る。これにより
   「Terminate 自体が commit できない」クラス A/B の恒久停止が解消される。
   誤発動（実際は生きているグループの破棄）はメンバー離脱と等価で、
   生じた重複は quorum を持つ側の TerminateB / Merge 経路で修復される。

しきい値 (2s / 15s / 30s / 45s) は `## retry proposals` ログの実測に基づく
再調整を想定した暫定値。回帰テスト:
`node/internal/kvs/sector/sector_test.go` の
`TestSector_forceTerminate_onQuorumLoss` /
`TestSector_forceTerminate_notFiredWithLeader` /
`TestSector_forceTerminate_leaderWithoutQuorum` /
`TestSector_import_timeoutOnQuorumLoss` /
`TestSector_import_unblockedByForceTerminate`。

#### シミュレーション再実行での発見（2026-07-04, simulator/node.log 2 回）

**run 1（timeout+abort + リーダー不在検知のみ）**: timeout+abort は機能した
（import timeout 123 件、mtxOperateSectors の恒久ハングは消滅）が、
「proposer 死亡後の Terminate 空振り」型の停滞が残存し、強制破棄による回復も
観測されなかった。当初これを **leader-without-quorum**（etcd raft は
`CheckQuorum` なしでは quorum を失ったリーダーも降格せず、`Status().Lead ≠ 0`
のままになる）が原因と推定し、`CheckQuorum: true` と pending バックストップを
追加した（この検出ギャップ自体は単体テスト
`TestSector_forceTerminate_leaderWithoutQuorum` で実在を実証済み。対処は妥当として維持）。

**run 2（CheckQuorum + バックストップ入り）**: 同型の停滞が残存
（`proposedSplitRoutine: proposer left, terminate hosting sector` を最長 138 秒
毎秒繰り返し、新 backward からの prepareSplit を拒否し続ける）。dump.json の
セクター推移と突き合わせて真因を特定した:

> **Terminate は commit されていたが、apply が完了できていなかった。**
> `processTerminateProposal` は `store.ReleaseSector` のエラーで `terminated` を
> 立てずに return する。**inactive なセクターは `AllocateSector` を一度も呼んで
> いない**（割り当ては activate/import 時のみ）ため ReleaseSector は必ず失敗し、
> Terminate は「commit → apply 失敗 → 3 秒後に再提案 → 再 commit → 再失敗」を
> 永久に繰り返す。グループは健全なので (a) リーダー不在検知は（正しく）発火せず、
> (b) 再提案のたびに commit index が進むため pending バックストップも
> リセットされ続ける。**quorum 喪失と同一の症状を健全なグループが示していた。**

修正（実装済み・回帰テスト `TestSector_terminate_inactiveSector`。テスト用
store は SimpleStore と同じ「未割り当ての解放はエラー」セマンティクスを模倣）:

- `terminateLocked`: ReleaseSector の失敗を「解放済み」として許容し、必ず完了する
- `processActivateProposal` / `processImportProposal`: AllocateSector の
  「既に割り当て済み」も許容（apply ハンドラの冪等化。apply がエラーを返すと
  publishEntries が同一バッチの残り committed entries の適用まで中断する問題も回避）

教訓:

1. **apply ハンドラは冪等かつ必ず完了することを規約とする**（アルゴリズム改善案 B
   の実証）。apply 失敗ループは quorum 喪失と区別できない症状を作り、しかも
   commit が進み続けるため「commit 停滞」ベースの検知の盲点に入る。
2. **観測経路自体を検証してから結論を出す**。node.log の収集は `==` / `@@` を
   含む行のみを残すフィルタがかかっており、`## force terminate` /
   `## retry proposals` は 2 回の実行とも全て欠落していた。run 1 の
   「強制破棄が発火しなかった」という判断はこのアーティファクトに部分的に
   依存していた。診断ログは `@@ force terminate` / `== retry proposals` に
   改名してフィルタを通るようにした（state/lead/term/commit 付き）。
3. 2026-06〜07 に quorum 喪失へ帰属させた「Terminate 毎秒空振り」観測の一部は、
   実際にはこの apply バグだった可能性が高い（inactive レプリカへの Terminate は
   常にこのバグを踏む）。純粋な quorum 喪失（import timeout が併発する形態）も
   併存するため、修正後の再実行で残存停滞を再分類する必要がある。

#### シミュレーション run 3（terminate apply 修正後、2026-07-04）

修正の効果を定量確認した:

- terminate 空振りループは消滅（`terminate: release sector skipped` 594 件 =
  旧コードで無限ループしていたケースが全て完了）
- 強制破棄が 135 回発火、**全て leaderless 経路**（CheckQuorum によるリーダー降格が
  機能。pending バックストップの出番なし）
- 診断ログ（`== retry proposals` / `@@ force terminate`）は収集フィルタを通過

それでも活性化は序盤（1 分で 37 activate）以降ほぼ停止（以後 7 分で 6）。
残存停滞は 2 クラス:

| クラス | 症状 | 分類 |
|--------|------|------|
| C: already-activated ループ（**新規**） | backward ノードが sectorActivate を毎秒送り続け、受信側は「already activated」を返し続ける（最長 128 秒 / 114 回）。受信側の hosting グループは健全（StateLeader・commit 前進中）だが、**送信側が持つ同グループのレプリカに tail が複製されず**、送信側の operateSectors が永遠に Activate ケースに留まる。受信側グループでは appendNodes/removeNodes の ConfChange が pending ⇄ 解消を振動しており、routing 不安定（`skip subRoutine: node is not stable` が毎分 1000 件超）によるメンバー追加/削除の往復で、送信側レプリカが スナップショット同期前に削除→再作成 を繰り返している疑い | 新規・要調査 |
| B': stale active レプリカの重なりガード | 死亡ノードを head とする active レプリカが skip 1 を発動し続ける。ただし当該グループは**残存メンバーで quorum を維持**しており（リーダー健在）、強制破棄は（正しく）発火しない。head 不在のまま active であり続けるセクターを誰が畳むかという問題で、まさに TODO-2 のスコープ | TODO-2 |

考察: クラス C は「ローカルレプリカの tail を frontward の活性状態の情報源にする」
現設計の弱点（レプリカ同期がメンバーシップ churn に負ける）。sectorActivate 応答は
成功を返しているため、応答を情報源として使う・レプリカ同期を待つ間の再送を抑制する等の
選択肢があるが、tail 値自体は後続判定（extend/split）に必要なため応答だけでは足りない。
メンバー ConfChange の振動（追加→削除→追加）自体の抑制も含めて要設計判断。

#### run 3 深掘り: ゴーストレプリカと apply バッチ中断（2026-07-04, dump.json 解析）

クラス C の root cause 調査（dump.json でグループ全メンバーのレプリカ推移を追跡）で、
さらに 2 つの実装バグを発見・修正した。

**1. ゴーストレプリカ（explicit パケットの非宛先受理）**

dump.json 上で、**同じ sectorNo を複数の物理ノードが同時期に保持する**事例を多数観測
（例: slot 5 を 8f31f085 と e6289d10、slot 12 を 80c35cb0 と 8eaacb67）。さらに
死亡メンバーの slot が**リング上の遠いノードに tail 付きで出現**（= raft メンバーとして
snapshot/log を受領しデータ複製まで受けた）する事例も確認（slot 2/3/6/9）。

機構: `classifyPacket` は `GetNextStep1D` が自ノードを返すと explicit パケットでも
`transferer.Receive` していた。ルートテーブルが一時的に他宛先を自ノードへ解決すると、
死亡ノード宛に毎秒再送される `SectorManageMember` や raft メッセージを別ノードが
受理し、**同じ raft メンバー ID を複数ノードが名乗る**。raft の前提（メンバー ID と
プロセスの 1:1 対応）が破れ、誤ったデータ複製・応答の横取り（本来のノードが
永久に同期しない）・二重投票のリスクを生む。
→ 修正: explicit パケットは宛先一致時のみ受理（不一致は
`== drop explicit packet` を出力して破棄）。

**2. apply エラーによる committed entries のバッチ中断**

`publishEntries` は apply エラーで即 return していたが、`Advance()` は実行されるため
**同一 Ready バッチの残りの committed entries が適用されないまま消費**され、
そのメンバーだけグループ状態から永久に乖離する。毒エントリー源として
`processCommitSplitProposal` が非冪等（tail 設定済みでエラーを返し、かつ
pending proposal をクリアしない → 3 秒ごと再提案 → 毎回 commit → 毎回 apply 失敗）
であることも特定した。
→ 修正: (a) publishEntries は apply エラーをログして継続（committed entry の適用は
必ず完了する、の徹底）、(b) CommitSplit の apply を冪等化（既活性化は no-op +
proposal クリア）。

**検証**: クリーン条件（死亡メンバー入りグループへの空ログメンバー join）は
単体テスト `TestConsensus_joinCatchesUpWithDeadMember` で正常動作を確認済み。
クラス C の完全な再現（churn による孤児レプリカ + 非対称 routing ビュー）は
再現条件が複雑なため、次回シミュレーションで `== drop explicit packet` の発火と
already-activated ループの消長を観測して判定する。クラス B'（TODO-2）は未着手。

#### シミュレーション run 4（全修正後、2026-07-04）

**恒久停止クラスは全て解消した**:

- proposer-left ループ: 0 件（run 2: 最長 138 秒＋無限 → 消滅）
- already-activated ループ（クラス C）: 2 件のみ、いずれも **40〜70 秒で自己解消**
  （孤児レプリカがリーダー不在 30 秒の強制破棄で掃除され再作成される）
- skip 1（クラス B'）: 1 件・25 秒継続で run 終了。恒久化の証拠なし
- `== drop explicit packet` は 0 件（ゴースト再現なし。ガードは保険として妥当）

一方で**新しい支配的問題**を確認: 活性化カバレッジは t=130s に 69/100 まで到達後、
**単調減少に転じ t=250s には 46 まで低下**した。減少は 2 つの位相からなる。

**位相 1 (t=130〜180s): is_stable ゲートによる修復凍結 + churn の自然減**

active 集合の差分では、この 50 秒間の喪失 7 は**全てノード自体のランダム停止**
（強制破棄ではない）で、**新規獲得は 0**。獲得ゼロの理由:

- 活性化チェーンの前進イベント（`Activate hosting sector`）は t=138 を最後に沈黙。
- kill された active ノードの穴を塞ぐ修復も凍結（この 70 秒間で **Extend 0 件**、
  Terminate frontward 2 件のみ）。原因は `node is not stable` によるスキップ
  （10 秒あたり 78〜186 件 ≒ 大半のノードが不安定判定）。`subRoutine` は
  is_stable でないと ManageMember にも operateSectors にも到達しないため、
  churn 中は修復を担うノードが何もできない。
- 231 ノード解析（2026-06）で記録した「KVS の進行が seed の reconcile /
  is_stable フラップに律速される」構造問題が、クラッシュ系バグの解消後の
  律速要因として表面化した形。

**位相 2 (t≈240s〜): 強制破棄ストーム**

大量 join（+20 ノード超）に伴い強制破棄が集中（5 分間で 498 回・151 グループ、
うち 84 回は直前まで active なセクター。ほぼ全て leaderless 判定、破棄セクターの
年齢中央値 244 秒 = 序盤にできた古参セクター）。

機構: join 波でルーティング近傍が入れ替わり、hostingManager がメンバーを
追加/削除し続ける。**新 voter は同期完了前から quorum 計算に入る**（learner 段階が
ない）ため、未同期 voter の増加と旧メンバーの離脱が重なるとグループが本物の
leaderless に落ち、30 秒後に破棄される。破棄はグループ単位で並列に起こるが、
再活性化はチェーン伝播で直列にしか進まないため、churn が続く間はカバレッジが
純減する。**脱出経路（強制破棄）は正しく機能しており、ボトルネックは
「churn 下で quorum を守れないメンバーシップ管理」に移った。**

**訂正 (2026-07-04): 「自己メンバーシップの疑い」は誤認だった**

当初、e24f5072 を head とするレプリカが slot 5 / slot 10 で force terminate された
ログから「hostingManager が自ノードを重複 append した」と推定したが、これは
**`@@ force terminate` の出力がレプリカ保持ノードではなく sector の head を
表示する**ことによる読み違い。実際は「e24f のグループの通常レプリカ (slot 5, 10)
を、それを保持する別々のメンバーノードが各自破棄した」正常な動作だった。

また routing 側を精査した結果、`recvRoutingPacket` は localNodeID を
secondNeighborhoods から明示的に除外しており、`neighborhoodInfos`（接続済みピア）
にも自分は入り得ないため、**「nextNodeIDs に自分が現れない」は能動的に維持された
不変条件**である。`initHostSector` の panic はこの不変条件の破れ = ロジックエラーの
検出器として妥当（タイミング起因で正常系に発生する状態ではない）。
`ManageMember` 側の toAppend にはガード自体がなく、不変条件が破れた場合は
panic せず黙って自己 append してしまうため、initHostSector と同じ panic を
置くのが一貫する（ログ格下げではなく検出の追加）。

改善候補（優先順）:

1. ~~learner-first メンバーシップ~~ → **実装済み (2026-07-04)**。下記
   「learner-first メンバーシップの実装」参照。
2. **is_stable ゲートの緩和 / seed 律速の解消**（設計判断・中〜大): 不安定時も
   修復系操作（Extend / Terminate frontward）だけは許可する、あるいは
   is_stable の判定自体を安定化する。位相 1 対策。
3. **ManageMember のヒステリシス**: routing ビューが N tick 連続で同一の場合のみ
   メンバー変更を発行し、view flap の追従で ConfChange を浪費しない。
4. ~~ManageMember に localNodeID の panic ガードを追加~~ → **実装済み (2026-07-04)**。
   チェックは入力 nextNodeIDs に対して行う（toAppend 導出内では自ノードが
   memberMap（host slot）に吸収され検出できないため）。回帰テスト:
   `TestManager_ManageMember_panicsOnLocalNodeID`。
5. しきい値調整は対症療法にしかならない（destroy を遅らせても quorum 喪失自体は
   解消しない）。

#### learner-first メンバーシップの実装（2026-07-04, consensus.go）

- **追加は learner から**: `consensus.AppendNode` は `ConfChangeAddLearnerNode` を
  提案する。learner は quorum に入らないため、**未同期/死亡ノードの append が
  グループの commit 能力を毀損しない**（run4 の破棄ストームの根本対策）。
- **昇格はリーダーが自動判定**: `maybePromoteLearners`（1 秒周期）が、リーダー上で
  `Progress[id].Match >= Commit` に達した learner を `ConfChangeAddNode` で
  voter へ昇格する。死亡/停滞 learner は昇格されないまま残り、routing から
  消えた時点で通常の RemoveNode 経路で除去される。
- **冪等性ガード**: `AppendNode` は confState を確認し、既に learner/voter の
  id には何も提案しない。リトライループが 3 秒ごとに再呼び出しする前提のため、
  **voter に learner-add を再提案すると降格してしまう**ことへの防御でもある。
- **hostingManager への通知は昇格時のみ**: `ConsensusAppendNode` は voter 昇格の
  apply で発火する（learner 追加では発火しない）。メンバー状態機械にとって
  「メンバーになった」= quorum に参加した、で意味が揃う。learner の間は
  `proposalAppendingNodes` が pending のまま残るため、**強制破棄のバックストップ
  からは ConfChange 系 pending を除外**した（健全なグループが learner の
  追随待ちで破棄されないように）。
- **スコープ**: append 経路のみ。グループ新規作成時の初期メンバーは従来どおり
  voter で bootstrap する（`StartNode` の peers）。初期メンバーに死者が混ざる
  「stillborn グループ」は従来どおり 30 秒の leaderless 破棄で掃除される。
- 回帰テスト: `TestConsensus_learnerFirst_deadAppendKeepsQuorum`（dead append
  でも commit 継続 = learner-first なしでは失敗する）/
  `TestConsensus_learnerFirst_liveAppendPromoted`（追随後に昇格し通知が届く）/
  `TestConsensus_checkQuorum_leaderStepsDown`（leader-without-quorum の降格）/
  `TestSector_appendDeadNode_learnerKeepsQuorum`（sector 層での同性質。旧
  `TestSector_forceTerminate_leaderWithoutQuorum` は「dead append で quorum が
  壊れる」前提自体が learner-first で成立しなくなったため置き換え）。

#### シミュレーション run 5（learner-first 導入後、2026-07-04）: raft panic

activate の恒久停止は発生しなかったが、t≈180s から不安定化
（`node is not stable` が min3: 1504 → min5: 3733/分、強制破棄 990 回）し、
最終的に **etcd raft 内部で panic してプロセスごと停止**した
（`slice bounds out of range` at `nextCommittedEnts` / `unstable.slice`、
`RestartNode` で生成されたレプリカの run goroutine）。

**機構（レプリカ復活による raft メンバーの記憶喪失）**:

1. learner-first により、メンバーが Normal になるまで hostingManager の
   `sendSettingMessage` が **APPEND を毎秒再送し続ける**ようになった
   （従来は昇格が即時だったため再送窓が短かった）。
2. 追随中のレプリカ（entries を ack 済み = リーダーの Progress に Match>0 が残る）
   が、churn によるリーダー不在 30 秒でローカル強制破棄される。
3. 再送 APPEND が**同じ {sectorID, sectorNo} を空ログで再作成**する
   （`RestartNode` — panic スタックと一致）。
4. グループはその raft ID の Match・投票を記憶しているため、
   **「メンバーは自分のログを失わない」という raft の大前提が破れ**、
   復活インスタンスとの整合が取れず内部状態矛盾で panic する。
   panic は raft 内部の goroutine で起きるため recover 不能
   （1 プロセス 100 ノードのシミュレータでは全ノード即死）。

なお「復活レプリカ → 30 秒後にまた leaderless 破棄 → また復活」のループが
破棄数を増幅する（990 回）ため、t≈180s 以降の不安定化自体もこの復活サイクルが
一因とみられる（run4 の同時間帯は 498 回だった）。run3 で観測した
「slot 12 を 2 ノードが保持」も、再配送 APPEND による複製インスタンスという
同族の問題。

**修正（実装済み）**:

- **sector tombstone**（kvs.go）: ローカル破棄（SectorTerminated）または
  remove 適用（SectorRemoveNode）された {sectorID, sectorNo} を記録し、
  同じキーの SectorManageMember CREATE/APPEND を拒否する。
  **raft メンバー ID は使い捨て**とし、同一 ID の空ログ再作成を構造的に禁止。
- **停滞メンバーの reap**（hosting.go `reapStaleMembers`,
  memberSetupTimeout=30s）: 非 Normal のまま一定時間経過したメンバーを
  Removing にし、routing に残っていれば**新しい sectorNo で追加し直す**。
  tombstone 拒否からの回復経路であり、死んだ/停滞 learner の掃除も
  routing の失効（約 3 分）を待たず 30 秒に短縮される。

回帰テスト: `TestKVS_sectorManageMember_rejectsTombstonedKey` /
`TestManager_ManageMember_reapsStaleMember`。

#### シミュレーション run 6（tombstone 導入後、2026-07-04）: `need non-empty snapshot` panic

m=+161 で `panic: need non-empty snapshot`（`raft.maybeSendSnapshot`、リーダーの
run goroutine）によりプロセス停止。161 秒の短命 run のため tombstone / reap の
効果評価は持ち越し（tombstone 拒否 0 件・force terminate 14 件のみ）。

**確認結果: snapshot 機能は実質未実装だった。**

1. `operator.ExportSnapshot` / `ImportSnapshot` は `panic("not implemented")` の
   スタブ。
2. さらに `consensus.appliedIndex` は snapshot 適用時にしか代入されず、通常
   エントリーの適用で**一切更新されない**ため、`maybeTriggerSnapshot`
   （1000 エントリーごとの snapshot 作成 + compaction）は永遠に発火しない
   不活性コード。つまり**どのノードも snapshot を作らず、ログは compaction
   されないまま無限に伸びる**。
3. 一方 etcd raft は、フォロワーの Next がリーダーのログ範囲外になると
   （`term(Next-1)` が ErrCompacted/ErrUnavailable）snapshot 送信に落ち、
   `Storage.Snapshot()` が空だと panic する。churn でこの状況に入った瞬間、
   プロセスごと死ぬ（recover 不能）。

**修正（実装済み）**: `snapshotGuardStorage` — MemoryStorage をラップし、空の
snapshot 要求を etcd raft 公式のエスケープ **`ErrSnapshotTemporarilyUnavailable`**
に変換する。raft は panic せず送信をスキップし、当該フォロワーは同期不能の
ままになるが、membership manager の reap (30s) が**新しい sectorNo で作り直し、
ログは compaction されていないので index 1 から追いつける** — snapshot 本実装なしで
整合する。回帰テスト: `TestSnapshotGuardStorage`。

#### シミュレーション run 7（snapshot ガード後・70 ノード、2026-07-04）

100 ノードは負荷起因とみられる不安定（30 秒〜）のため 70 ノードに変更。
activate の恒久停止なし。m=+413 で **自前コードの nil 参照 panic**:
`sectorPrepareSplit` が `GetHostingSectorKey()` の戻りを nil チェックせず
`.SectorID` を参照していた。hosting sector の（強制）破棄から ManageMember に
よる再作成までの窓では key が nil になり、そこへ prepareSplit RPC が着信すると
落ちる。破棄→再作成が高頻度になったことで顕在化した潜在バグ
（他の 3 箇所の呼び出しは nil チェック済みだった）。
→ 修正: nil なら reject。回帰テスト `TestKVS_sectorPrepareSplit_noHostingSector`。

健全性の指標は大きく改善:

- カバレッジは t=80s に **66/71 (94%)** 到達（run4 のピークは 69/100）。
- 強制破棄 511 回のうち、**直前まで active だったセクターの破棄は 0 回**
  （run4: 84 回）— learner-first により生きているグループの quorum が
  churn で毀損されなくなった効果が確認できた。
- reap は 39 回発動。tombstone 拒否は 0 回（再配送の発生前に reap が
  取り除いている、または復活サイクル自体が消えたため機会なし）。

一方、ピーク後のカバレッジは単調減少（t=340s で 36/70）。active セクターの
破壊はもう起きていないため、減少の機構は「ランダム停止が active ノードを
削る速度 > チェーン再活性化の速度」。再活性化側の律速は既記録の改善候補
2（is_stable ゲートによる修復凍結）と 3（ヒステリシス）の領域で、
run 終盤（min6: 破棄 353 回）は kill の累積により inactive レプリカの掃除が
集中したことも負荷に寄与した。

**残課題（TODO として記録）**: snapshot の本実装
（operator の store serialize + appliedIndex の更新 + トリガの有効化）。
現状はログ無限成長（MemoryStorage のメモリ増加）とも表裏一体で、長時間運用・
大量書き込みでは必須になる。実装時は「learner の catch-up が snapshot 経由に
なる」ため、`ConsensusApplySnapshot` の store 反映と冪等性もセットで設計する。
なお「リーダーの Next がログ範囲外に出た正確な系譜」（同一 term での二重リーダー
疑い = 空ログ再作成による votedFor 忘却の残存経路の可能性）は未特定で、
次回 run の観測対象。

#### シミュレーション run 8（100 ノード、2026-07-06）: split が構造的に不成立

「一度 active になった位置が inactive のまま戻らない」症状を解析。
終了時 active 22 / inactive 69。dump 解析で「active → inactive に戻った
セクター」は 0 件で、黄色の実体は terminate → 新 sectorID で再作成された
別セクターが activate できないループと判明。ログ集計で
**split 試行 784 回 / migrate 失敗 779 回 / `split: done` 0 回**
（初回失敗は開始 69 秒後）— split は run 開始から一度も成功していなかった。

> **真因**: `ConsensusApplyProposal` の `if s.tail == nil { return nil }`
> （activation ゲート）が、**commit 済みの Import / CommitSplit 提案を
> inactive セクターで黙って捨てていた**。split の import 先 frontward
> セクターは定義上 inactive なので、この実装では split は構造的に成功不能。
> commit は成功し続けるため pending バックストップも発火しない —
> run 2 の terminate と同型の **「apply ハンドラは必ず完了する」規約違反**
> （commit → apply 黙殺 → 15s timeout → abort → 再作成 → 無限ループ）。

→ 修正: Import / CommitSplit の適用を activation ゲートの前に移動
（両ハンドラは冪等実装済み）。回帰テスト
`TestSector_import_commitSplit_onInactiveSector`（修正前コードで失敗を確認）。

#### シミュレーション run 9（100 ノード、2026-07-06）: join レプリカの恒久乖離

split 修正の効果確認: `split: done` 119 回、**t≈105s で 97/97 全 active 達成**
（プロトコル自体の活性が初めて全域で成立）。しかし churn 開始後
13:26 から単調劣化（inactive 0→14、強制破棄 18→95 件/分、
`Unknown node sectorNo` 5.1 万件）。

> **真因**: bootstrap の `raft.Peer{ID}` に nodeID context を付けていな
> かった。ログは未圧縮（snapshot 未実装）のため後から join するメンバーは
> entry 1 から全履歴を replay するが、離脱済み bootstrap メンバーの
> AddNode entry を手持ちのメンバー表（append 時点の現在ビュー）で解決できず
> `missing node ID in conf change`（729 件）→ publishEntries がバッチを
> エラー中断（Advance は進む）→ **catch-up 中の初回バッチ ≒ 全履歴を恒久
> 喪失**。乖離レプリカは raft レベルでは健全（ack・voter 昇格可）だが
> メンバー表が壊れており、candidate/leader になるとメッセージを送れず
> グループが実質 leaderless 化 → 強制破棄ストーム → 補充メンバーが同じ毒
> entry を replay する正帰還。

→ 修正 (3 点): (1) bootstrap peer に `Context: nodeID` 付与（replay の
自己記述化）、(2) publishEntries の conf change 適用エラーを
log-and-continue に（normal entry と同方針。ApplyConfChange は適用済みの
ため raft 内部状態は整合）、(3) nodeID 不明 learner の promote を skip
（context 空の AddNode を新規にログへ入れない）。回帰テスト
`TestConsensus_joinResolvesRemovedBootstrapMember`（修正前コードで失敗を確認）。

#### シミュレーション run 10（100 ノード・15 分、2026-07-06）: 残存赤の分析

run 9 の 2 修正の効果確認: inactive は全期間 0〜3 の transient のみで劣化なし、
publish 失敗 729→0、`Unknown node sectorNo` 5.1 万→3 千、強制破棄は定常
~20 件/分（全て leaderless 判定）で加速なし。**恒久的な赤（レプリカ状態
不一致 / メンバー数 <5）は存在しない**（最長でも連続 55 秒、累計 90 秒/585 秒）。

残存する赤の内訳:

1. **主因（tail 不一致 838 sector-sample）**: 近傍変化によるメンバー rotation
   で remove されたメンバーの stale レプリカ。**除去されたメンバーは自分の
   除去 commit をグループから学習できない**（除去適用後リーダーは送信を
   止める = raft の既知の性質）ため、leaderless 30s の強制破棄で刈られる
   まで古い tail のまま残留する。実例（セクター …4a2ec0cc0f90）では
   online のままの /2 /3 /4 が順に「ちょうど 30 秒 leaderless」で自壊、
   /4 は最新 tail に同期済みでも rotation により除去されていた。
   常時 ~5 セクターがこの状態のため動画では恒久的な赤に見える。
2. 副因: 新規セクターのメンバー充足待ち（レンダラは memberFullCount=5
   未満を赤描画）。仕様通りの transient。

→ 修正 (2026-07-06、再 run 未実施): `SectorManageMember COMMAND_REMOVE` に
よる**除去の out-of-band 通知**。除去 conf change の適用時に host が removed
node へ直接通知し、受信側は tombstone + `Sector.TerminateLocally()`
（強制破棄と同じローカル破棄経路）で即時破棄。通知は best-effort の
one-shot で、パケット喪失時は従来の強制破棄が backstop として残る。
hosting sector 宛の REMOVE は拒否（host は自グループから除去されない仕様の
ため、受理すると無認証パケット 1 つで active セクターを破棄できてしまう）。
回帰テスト: `TestManager_OnSectorRemoveNode_notifiesRemovedMember` /
`TestKVS_sectorManageMember_removeDestroysReplica` /
`TestKVS_sectorManageMember_removeRejectsHostingSector`。

#### run 10 後の node レイヤー修正（2026-07-06〜07-07、モデルのスコープ外）

run 10 後の再解析で、残存する赤/黄の律速が KVS プロトコルから node レイヤーに
移ったことを確認し、2 つの修正を実装した:

1. **リンク死活検知の短縮**（commit a35c67b）: `SessionTimeout` 5min → 30s、
   `KeepaliveInterval` 1min → 10s。死んだ node が他 node の routing ビューに
   実測 151〜245 秒残留し、KVS の修復機構（memberSetupTimeout /
   forceTerminateDuration = 30s）と時間スケールが合っていなかった。
2. **network-zombie 対策**（commit fcee2eb）: `RelayPacket` / `webRTCLinkNative.send`
   が na.mtx / w.mtx を pion の blocking call 越しに保持しない形に変更 +
   simulator watchdog 強化（ループ 5s 停滞で warn、60s で goroutine dump と
   `Col.Stop()` による強制停止）。send 中のリンク死亡で na.mtx が凍結し、
   リンク keepalive だけ生き残る「半死 node」が 12 件発生していた。

#### シミュレーション run 11（100 ノード・15 分、2026-07-09）: 非 active セクターの分析

上記 2 修正の効果確認と「active（緑）にならないセクター」の原因調査
（simulator/node.log + dump.json、23:36〜23:51）。

**修正の効果**: 全体の約 8 割が緑を維持。dead node の検知→修復開始は
約 50 秒（run 10 以前の 151〜245 秒残留は解消）。zombie は 12 件 → 1 件に
減少し、watchdog は設計通り動作（warn → gdump → 60s で強制停止）。

**残存問題**: 90 秒以上停滞するチェーンが 2 系統あり、いずれも未修正の
別バグ。加えて zombie 1 件の凍結フレームが gdump で確定した。

**(A) stale prepare_merge（mergeBy）デッドロック — 恒久停止、最重要**

セクターチェーン c13fd132 → c1afd62e → c2e01337 が 200 秒以上 yellow の
まま run 終了まで復旧しなかった。経緯:

1. c063b759 が active セクター 019f4942-ec2f を host したまま 23:48:35 に
   churn で停止。レプリカ群は quorum を保ったまま「host 不在の active
   leftover」として残存。
2. backward の bfa7be50 が設計通り merge で掃除を開始し、prepare_merge を
   commit（mergeBy=bfa7be50）した直後の 23:49:26 に**自分も churn で停止**。
3. 後任 bb89dd23 の merge は `merge is prepared by bfa7be50, not bb89dd23`
   で毎秒失敗（27 回）。**`mergeBy` をクリアするコードはどこにもない**
   （`sector.go` `processPrepareMergeProposal`: 一度セットしたら commit_merge
   後も Terminate 時も preparer 死亡時も残る）。
4. leftover が active なまま残るため、frontward の inactive セクター群は
   `activateHostingSector` の overlap ガード（ログ「skip 1」）で activate
   できない。leftover のグループは健全なので `checkQuorumLoss` の強制破棄も
   発動しない → 恒久停止。

> **設計ギャップ**: design.md の merge 手順は「prepare_merge は同時に 1
> node」の排他だけで**解放条件が未定義**。split には「node[i] 消失を監視して
> terminate」があるが、merge の preparer 死亡には対応するものがない。
> kvs.go の既存 TODO「mergeSector: ターゲット離脱時の abort」とも別で、
> 今回はターゲットではなく **merge する側**の死。

なお run 中に同型の leftover ブロック（019f493d-8d77、skip 1 発火 118 回、
被害 140 秒）が「たまたま間に別 node が join して frontward が変わった」
ことで解消した例があり、**leftover の掃除経路は自力では機能していない**。

→ 修正方針: preparer が routing から消えたら mergeBy をクリアする raft
commit（lease/timeout でも可）を追加。design.md に解放条件を明記し、
TLA+ モデル（PrepareMerge を持つ Raft 版以降）にも反映する。

**(B) is_stable 永久 false → hosting sector 未作成 → 隣接の「skip 2」**

run 終端で 8 セクターのチェーン（459964ea〜599d0676）が yellow。

- 473531f4 は 23:46:25 の起動から 5 分間**一度も is_stable にならず**
  （required 1d の 4ae30a39 が接続不良 conn=3 で繋がらない）、subRoutine が
  走らないため hosting sector を一度も作らなかった。
- backward の 459964ea は sectorActivate を毎秒受信するが、
  `activateHostingSector` の candidate 探索が「frontward node の sector
  レプリカが手元にあること」を要求するため「skip 2」で毎回断念。
- **sectorActivate は skip しても err なしを返す**ので、送信側（42be922c）は
  失敗を検知できず無限リトライ。

design.md 既知課題「is_stable ゲートによる修復凍結」の新しい現れ方:
接続不良 node が 1 つあるだけで隣の node の安定化が止まり、その先の
activation チェーン全体が凍結する。対策候補: (a) sectorActivate 受信側が
tail を frontwardNodeID そのもので決められるようにする、(b) is_stable の
要件から到達不能 node を除外する、(c) 最低限 sectorActivate に失敗理由を
返させ観測可能にする。

**(C) zombie 残穴の凍結フレーム確定（fcee2eb の補完が必要）**

残った zombie 1 件（bb89dd23、A のチェーンの起点でもある）の gdump より:
`nodeLinkChangeState.func1`（node_accessor.go）が `na.mtx.Lock()` を保持した
まま `disconnectLink(link, false)` → `link.disconnect()` → pion
`PeerConnection.Close()` でブロック。fcee2eb は send 経路のロックは外したが、
**disconnect 経路が na.mtx を握ったまま**だった（`houseKeeping` の
`disconnectLink(link, false)` も同じ穴）。修正方針: RelayPacket と同様
「ロック下で対象 link を収集 → 解放後に disconnect」。`disconnectLink` の
`lock=true` 経路は既に disconnect をロック外で呼ぶ正しい形。

**その他の観測**: `Col.Stop()` 後もセクターの raft ループ goroutine が
生き残り、停止済み node が force terminate ログを出し続ける（c063b759 で
停止 95 秒後まで確認）。シミュレーションの解析ノイズになるため、Stop での
goroutine 終了を確認する余地あり。

3 つの問題は独立しており、(C) を直しても (A)(B) は解決しない。

<a id="todo-1"></a>
#### TODO-1: quorum 喪失の故障モードを含む拡張モデルの追加

- **背景**: 全モデルは「提案は必ず commit される」(Commit 系アクションに WF) を
  前提としており、`Leave(n)` も `proposing[n] = "none"` を要求するため、
  「グループメンバーの死亡により commit が永久に来ない」故障モードを表現できない。
  シミュレーションでこの故障が活性化チェーン恒久停止の主因と確定した
  （前述「シミュレーション解析 2026-07-04」クラス A）。
- **内容**: `KvsSectorRaft.tla` をベースに拡張モデル（例: `KvsSectorFail.tla`）を追加する。
  - `proposing[n]` が非決定的に "stuck" 状態へ遷移するアクションを追加し、
    stuck した提案の Commit から公平性を外す。quorum やレプリカを精密に
    モデル化する必要はなく、「commit が永久に来ないことがある」の抽象で十分。
  - 回復アクションを追加する: `TimeoutAbort(n)`（提案を諦めて提案前状態に戻す）、
    `LocalDestroy(n)`（raft を経由せず sector を破棄して inactive/absent 相当へ）。
- **検証項目**:
  1. 回復アクションを入れても safety（NoOverlapCommitted, ActiveFlagConsistent,
     ValidRange）が保たれること。
  2. 回復アクションに WF を付与すると EventuallyAllActive が復活すること。
  3. `LocalDestroy` の誤発動（実際には commit 可能なグループを破棄）を許した
     場合に safety が破れるか。破れるなら発動条件に何が必要かを特定する。
- **位置づけ**: TODO-3/4 の実装 (2026-07-04) は本検証を待たずに先行したため、
  これは「実装をブロックするタスク」ではなく **設計判断を恒久化する前に払うべき
  検証負債** である。実施しない場合に負うリスク:
  1. **誤発動時の安全性論証の穴を見逃す**。実装の安全性は「誤発動しても
     メンバー離脱と等価で、生じた重複は TerminateB / Merge が修復する」という
     非形式的論証に依るが、修復経路には既知の制限がある
     （TerminateB は重複相手のレプリカを自ノードが持たないと発火せず、
     **非隣接ノード間の重複は解消されない**。231 ノードシミュレーションで観測済み）。
     「破棄 → 再作成 → 再活性化」と生き残った旧グループの遅延 commit
     （古い tail の Extend/CommitSplit 適用）のインターリーブが、修復されない
     重複を作らないかは網羅されていない。本プロジェクトではこの種の
     インターリーブバグをモデル検証で 11 件発見しており、目視レビューでは
     見つからなかった実績がある。
  2. **回復動作自体が新しい liveness バグを持ち込む可能性**。例えば、破棄後の
     再作成は routing ビューからメンバーを選ぶため、死亡ノードが seed の
     lifespan 失効（約 3 分）まで routing に残る間は「再作成グループが再び
     quorum-less → 30 秒後にまた破棄」の振動が起こりうる（最終的な収束条件は
     未検証）。TimeoutAbort 後の再試行での受諾状態残留 (`sectorPrepareSplit`)
     も同様。この種の「回復が回復しない」パターンの列挙が経験頼みになる。
  3. **発見手段が後払いになる**。モデルなしで問題が出た場合、発見手段は
     シミュレータ（非網羅・タイミング依存）と本番ログの逆算になり、TLC なら
     数分で反例トレースが出る問題に日単位のコストがかかる。
     `KvsSectorRaft.tla` という土台があるため追加コストは小さい
     （stuck 状態 1 つ + 回復アクション 2 つ）。
  また、しきい値（forceTerminateDuration=30s）をどこまで詰められるかは
  誤発動時の安全性の境界に依存するため、本検証はしきい値調整の前提でもある。
- **追記 (2026-07-06)**: 実装の LocalDestroy 相当は発火経路が 2 系統になった:
  (a) タイマー判定（leaderless 30s / pending 停滞 45s、TODO-4）、
  (b) **除去の out-of-band 通知**（COMMAND_REMOVE、run 10 参照）。
  モデル化する際は両者を**単一の抽象アクション `LocalDestroy(n)` の
  発火条件違い**として扱えば十分で、別アクションにする必要はない。
  (b) は「グループが除去を commit 済み」が送信の前提なので構成上
  誤発動ではない（= メンバー離脱そのもの）が、遅延・重複パケットと
  再作成のインターリーブは (a) の誤発動と同じ検証枠（検証項目 3）に入る。
  なお sectorNo は使い捨て（tombstone）のため、遅延 REMOVE が別の実体を
  指すエイリアシングは実装上排除済み。
- **関連**: `kvs.go` の `splitSector` / `mergeSector` / `proposedSplitRoutine` の
  NOTE (2026-07-04)、`sector.go` の `Terminate` / `TerminateLocally` の NOTE。

<a id="todo-2"></a>
#### TODO-2: stale active レプリカの掃除とガード緩和の検証（クラス B）

- **背景**: 離脱ノードを head とする active レプリカが `k.sectors` に残留すると、
  活性化の重なりガード (Go の skip 1 / モデルの
  `\A other \in Actives : ~IsBetween(other, f, ft)`) が永久発動する。
  モデルでは `ForgetSActive` が「タダの」ローカルビュー更新 (WF 付き) だが、
  Go では掃除自体が死んだグループの raft commit を要するためこの抽象が成立しない。
  2026-06 の「inactive レプリカを除外」修正は active レプリカ残留には無力だった。
- **内容**: SepView 版 / Raft 版で次のいずれか（または両方）を検証する。
  - `ForgetSActive` を raft ゲート付き（stuck グループでは発火しない）に変えた場合、
    liveness がどう壊れるかを確認し、問題を再現する。
  - 修正案「head が rView に存在しない active エントリはガード対象外とする、
    または強制 Forget できる」をモデル化し、safety が保たれるか検証する。
- **検証項目**: NoOverlap 系不変式。特に「head が rView にない」はローカル観測に
  すぎない（ルーティングの遅延で一時的に見えないだけ）ケースで、生きている
  active セクターと重なる activate を許してしまわないか。
- **関連**: `kvs.go` `activateHostingSector` の Overlap guard NOTE。
  シミュレーション 1 回目で 2 件観測（再現はタイミング依存: 「activate 済み
  ノードの死亡 + backward 隣接ノードが未 activate」の一致が必要）。

<a id="todo-3"></a>
#### TODO-3: セクター操作の timeout + abort（Go 実装）

> **実装済み (2026-07-04)** — 上記「quorum 喪失対策の実装」参照。
> モデル検証 (TODO-1 の TimeoutAbort) は未実施。

- **背景**: `sector.Sector` の `Import` / `PreCommitSplit` / `CommitSplit` /
  `PrepareMerge` / `CommitMerge` / `Extend` は raft 適用まで `cond.Wait()` で
  無期限ブロックする。`splitSector` がこの構造で `Migrate` 中にハングし、
  `mtxOperateSectors` を握ったまま当該ノードのセクター管理を全停止させた。
  モデルには `AbortProposal` があるが Go 実装に対応物がない。
- **内容**:
  - 上記のブロッキング操作にタイムアウトを導入し、期限超過でエラー復帰する。
  - `splitSector` のタイムアウト時に frontward 側へ abort を伝え、
    `proposedSplitting` 状態を解消する（相手側は proposer 監視だけでは不十分:
    proposer が「生きているがハング」の場合に検知できないことを観測済み）。
  - タイムアウト値は `applyProposals` の再提案間隔 (3 秒) との整合を考慮する。
- **検証項目**: TODO-1 の `TimeoutAbort` の検証結果に従うこと。abort 後の再試行で
  重複 split 提案（`sectorPrepareSplit` の受諾状態残留）が起きないこと。
- **関連**: 「アルゴリズム改善案 A（proposing をセクター内蔵）」と親和性が高く、
  同時に実施すると設計が単純になる可能性がある。

<a id="todo-4"></a>
#### TODO-4: quorum 喪失セクターのローカル強制破棄（設計 + Go 実装）

> **実装済み (2026-07-04)** — 上記「quorum 喪失対策の実装」参照。死亡判定は
> 「リーダー不在の継続」+「pending proposal の commit 停滞」の 2 系統
> （routing 上の消失との組み合わせは未実装、しきい値実測後に再検討）。
> 初版のリーダー不在のみの判定は leader-without-quorum（CheckQuorum なしでは
> quorum を失ったリーダーが降格しない）を検出できないことを単体テストで実証し、
> CheckQuorum 有効化とバックストップを追加した。なおシミュレーションで残存した
> 停滞の真因は Terminate の apply バグだった（上記「シミュレーション再実行での
> 発見」参照）。誤判定時の安全性のモデル検証 (TODO-1 検証項目 3) は未実施。

- **背景**: 終了 (Terminate) 自体が raft commit を要する現構造では、quorum を
  失ったグループは**自分自身を終了することもできず**、誰にも消せない
  （シミュレーションで「Terminate 毎秒空振り」を複数経路で観測）。
  raft を経由しない破棄の脱出経路がない限り、クラス A/B とも恒久停止は解消しない。
- **内容**: 以下の設計判断と実装。
  1. **死亡判定の基準**: リーダー不在 (`raft.Status().Lead == 0`) の継続時間か、
     メンバーの routing 上の消失か、その組み合わせか。
     `sector.go` の `## retry proposals` ログ（Raft ステータス付き）で実測してから
     閾値を決める。
  2. **誤判定時の安全性**: ネットワーク的に見えないだけで生きている quorum が
     存在するケースで破棄しても safety が保たれるか → TODO-1 の検証項目 3 で確認。
  3. **破棄後の再作成経路**: 破棄された hosting sector が hostingManager の
     create/append で再作成され、通常の活性化フローに復帰すること。
- **データ喪失について**: `design.md`「その他の性質」は過半数同時 offline による
  データ喪失を既に許容しているため、強制破棄は設計方針と矛盾しない。
  ただし現モデルはレコードを扱わないため、データ喪失の頻度・範囲の評価は
  モデルでは答えが出ない（必要ならシミュレータで測定する）。
- **関連**: `sector.go` `Terminate` の NOTE、`consensus.go` に `Status()` 追加済み。

## 再検証クイックリファレンス

```bash
cd spec/kvs

# Raft 版 (デフォルト: N=3, MaxChurn=1)
java -XX:+UseParallelGC \
  -cp <path-to-tla2tools.jar> tlc2.TLC \
  -workers auto -coverage 1 -config KvsSectorRaft.cfg KvsSectorRaftMC.tla

# MaxChurn を変えるには KvsSectorRaftMC.tla の MC_MaxChurn を書き換える
# N=4 にするには MC_Nodes == 0..3, MC_InitialMembers == {0, 3}
```

**注意点**:
- MaxChurn=1,2 だけでは TerminateA / 非原子 Merge のバグは再現しない。
  MaxChurn≥3 で初めて到達する。
- N=4 MaxChurn≥2 は状態爆発の可能性あり（未検証）。
- InitialMembers を全ノードにすると Merge が発火しない（rView が最初から完全なため）。
- トレースファイル (`*_TTrace_*.tla`, `*_TTrace_*.bin`) は `spec/.gitignore` で除外済み。