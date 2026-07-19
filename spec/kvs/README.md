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

  KvsSectorMergeLock.tla        # merge 排他ロック (mergeBy) + ReleaseMerge 版
  KvsSectorMergeLockMC.tla      # 同モデルパラメータ (Phase 1/2 は定数を切替)
  KvsSectorMergeLock.cfg        # 同 TLC 設定 (liveness 含む)
  KvsSectorMergeLockSafetyMC.tla # Phase 3 (誤検知 safety) パラメータ
  KvsSectorMergeLockSafety.cfg  # Phase 3 TLC 設定 (safety のみ)

  KvsSectorLeftover.tla         # leftover セクター + tail 切り詰め activation 版
  KvsSectorLeftoverMC.tla       # 同モデルパラメータ (Phase L1/L2 は定数を切替)
  KvsSectorLeftover.cfg         # 同 TLC 設定 (liveness 含む)
  KvsSectorLeftoverSafetyMC.tla # Phase L3 (誤検知 safety) パラメータ
  KvsSectorLeftoverSafety.cfg   # Phase L3 TLC 設定 (safety のみ)

  KvsSectorFalseLeftover.tla    # merge/overlap 抗争 (偽 leftover 誤認 +
                                #   不死身セクター + データ喪失) 版
  KvsSectorFalseLeftoverMC.tla  # 同モデルパラメータ (Phase FL1〜FL3 は定数を切替)
  KvsSectorFalseLeftover.cfg    # 同 TLC 設定 (liveness 含む)
  KvsSectorFalseLeftoverSafety.cfg # Phase FL1/FL3 TLC 設定 (safety のみ)

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
| `mergeLock[fs]`        | `Sector.mergeBy` (MergeLock 版以降) |
| `ReleaseMergeLock(fs)` | `Sector.checkMergeRelease` → ReleaseMerge 提案 (MergeLock 版以降) |
| `state[n] = "leftover"` | host 死亡後もレプリカ群が保持する active セクター (Leftover 版のみ) |
| `CommitActivate` の blockers / 切り詰め | `activateHostingSector` の overlap ガードと clipped tail (Leftover 版のみ) |
| `LeftoverQuorumLoss(h)` | leftover レプリカの quorum 喪失 → `checkQuorumLoss` のローカル破棄 (Leftover 版のみ) |
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

# merge 排他ロック + ReleaseMerge 版 (run 11 対策の検証)
# Phase 2 (修正確認, デフォルト設定)。Phase 1 (バグ再現) は
# KvsSectorMergeLockMC.tla の MC_EnableRelease を FALSE にして実行
java -jar tla2tools.jar -workers auto -config KvsSectorMergeLock.cfg KvsSectorMergeLockMC.tla
# Phase 3 (誤検知 safety)
java -jar tla2tools.jar -workers auto -config KvsSectorMergeLockSafety.cfg KvsSectorMergeLockSafetyMC.tla

# leftover セクター + tail 切り詰め activation 版 (run 13 対策の検証)
# Phase L2 (修正確認, デフォルト設定)。Phase L1 (バグ再現) は
# KvsSectorLeftoverMC.tla の MC_ClipActivationTail を FALSE にして実行
java -jar tla2tools.jar -workers auto -config KvsSectorLeftover.cfg KvsSectorLeftoverMC.tla
# Phase L3 (誤検知 safety)
java -jar tla2tools.jar -workers auto -config KvsSectorLeftoverSafety.cfg KvsSectorLeftoverSafetyMC.tla

# merge/overlap 抗争 + データ喪失版 (run 2026-07-16 対策の検証)
# Phase FL2 liveness (デフォルト設定、churn 1)。Phase FL1 (バグ再現) は
# KvsSectorFalseLeftoverMC.tla の MC_Fix* 4 つを FALSE にして Safety.cfg で実行。
# safety は MC_MaxChurn = 2 で Safety.cfg を使う (liveness 込みだと状態爆発)
java -jar tla2tools.jar -workers auto -config KvsSectorFalseLeftover.cfg KvsSectorFalseLeftoverMC.tla
# Phase FL1 / FL3 / churn 2 safety (safety のみ)
java -jar tla2tools.jar -workers auto -config KvsSectorFalseLeftoverSafety.cfg KvsSectorFalseLeftoverMC.tla
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
| snapshot 未実装のまま raft が snapshot 送信を要求 → panic (2026-07-04 run6 発見) | `operator.ExportSnapshot`/`ImportSnapshot` がスタブな上、`appliedIndex` が通常エントリーで更新されず snapshot 作成トリガが不活性。raft はフォロワーの Next がログ範囲外になると `Storage.Snapshot()` を要求し、空だと panic する | churn でフォロワー進捗とリーダーのログがずれた瞬間 **`need non-empty snapshot` でプロセス停止**。修正: `snapshotGuardStorage` が空 snapshot を `ErrSnapshotTemporarilyUnavailable` に変換（送信スキップ → reap による新 sectorNo 再作成で index 1 から追いつく）。snapshot 本実装は 2026-07-12 に完了 ([snapshot.md](snapshot.md)) |

**教訓**: TLA+ のアクションは原子的にモデル化されるため、アクション「内部」の
ロック取得順序はモデルの検証対象外。実装側は以下のロック規約で防ぐ
(`node/internal/kvs/kvs.go` の `KVS.mtx` コメント参照):

- 取得順序は `s.mtx (Sector) / m.mtx (hosting.Manager) → k.mtx (KVS)` の一方向のみ
- `k.mtx` 保持中に hostingManager・Sector のロックを取るメソッドを呼ばない
- `s.mtx` 保持中に KVS/Manager へコールバックしない（Terminate はロック解放後に通知）

回帰テスト: `node/internal/kvs/kvs_test.go` の
`TestKVS_sectorActivate_completes` / `TestKVS_activateHostingSector_singleNode` /
`TestKVS_sectorActivate_ignoresInactiveSectorBetween`。

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

| 案 | 効果 | 実装コスト | 推奨度 | 状況 (2026-07-17 コード再確認) |
|---|------|----------|--------|-------------------|
| A: proposing をセクター内蔵 | バグ予防 (大) | 中 | ★★★ | 未実装。ただし動機の多く（提案スタックの解消）は TODO-3 の timeout+abort と強制破棄で代替済み |
| B: 操作の冪等化 | バグ予防 (中) | 小 | ★★★ | **おおむね実装済み**: Terminate は必ず完了・CommitSplit は既活性化で no-op・Activate/Import の AllocateSector は割り当て済みを許容（run2/run3 の修正）。CommitMerge の「fs が先に inactive 化された場合の extendTailOnly」は未実装 |
| C: anyActive のローカル化 | バグ予防 (中) | 小 | ★★★ | 未実装 |
| D: TerminateA 再検討 | 保守性 | 小 (調査のみ) | ★★ | 未着手 |
| E: Split のアトミック化 | 構造改善 | 大 | ★ (将来) | 未着手 |
| F: Merge 2 コミット化 | レース面縮小 | 中 | ★★ | 未着手 |
| G: TerminateB 即時化 | 修復遅延短縮 | 小 | ★★ | 未着手 |

## シミュレーション実行の記録

シミュレーション run とその解析・対策・実装の時系列記録。
ここで見つかった課題（隠れ TODO を含む）は「[今後の TODO](#今後の-todo)」の
一覧に集約している。

### シミュレータ解析からの追加知見（231 ノード・ランダム停止、simulator/dump.json）

- tail が「停止した active ノード」を指すケースは、既存の Merge 修復経路が
  約 1 分で解消することをログ上で確認（恒久停止ではない）。
- `is_stable`（seed の reconcile と routing ビューの一致）は、停止ノードが
  seed の lifespan 失効（約 3 分）で除去されるまで多数のノードでフラップし、
  一部ノードは hosting sector の作成自体が 3 分遅延した。KVS の進行が
  seed 側の失効タイマに律速される構造は将来の改善候補
  （→ TODO「is_stable ゲートの緩和」）。
- Extend の重なりガード未実装（モデル修正 #6 の Go 側未反映、kvs.go の NOTE 参照）
  に起因するとみられる active セクターの重複が複数残存していた。
  TerminateB による修復は重複相手のレプリカを持たないと発火しないため、
  非隣接ノード間の重複は解消されない。
  → その後 `hasActiveSectorHeadInRange` (kvs.go) として実装済み。

### シミュレーション解析からの追加知見（100 ノード・ランダム停止、2026-07-04）

`simulator/logs.txt`（2 回の実行、2 回目はタイムスタンプ付き）の解析で、
活性化チェーンの**恒久停止**を 2 クラス確認した。いずれも
「**quorum を失った Raft グループは何も commit できない**」ことに起因し、
前述のモデル前提（提案は必ず commit される）の外側で起きている。

| クラス | 症状 | 機構 |
|--------|------|------|
| A: splitSector ハング（1 回目 9 ペア、2 回目 5 件） | hosting 側が `Migrate` から戻らず `mtxOperateSectors` を握ったまま、当該ノードのセクター管理が全停止。frontward 側は `proposedSplitting` を保持したまま待機 | `Migrate` 内の Import 提案が frontward 側グループの quorum 喪失で commit されない。グループが治癒して 23 秒後に回復した例もあるが、過半数喪失時は治癒に必要な ConfChange 自体が commit 不能で永久化 |
| B: stale active レプリカ（1 回目のみ 2 件） | 離脱ノードを head とする **active な**レプリカが活性化の重なりガード (skip 1) を永久発動させ、チェーンがその点で停止 | レプリカの掃除 (`Terminate` / `SectorRemoveNode`) 自体が死んだグループの raft commit を要するため誰にも消せない。掃除経路は backward の hosting が active になった後にしか走らないという鶏卵もある |

付随する観測:

- **Terminate 自体が raft commit を要する**ため、quorum 喪失グループは自分自身を
  終了することすらできない。「proposer 離脱を検知して Terminate」「frontward の
  不一致レプリカを Terminate」がどちらも毎秒空振りし続けるケースを観測。
- 2026-06 に修正した「重なりガードが inactive レプリカも対象」問題
  （「モデルのスコープ外で見つかった実装バグ」の表参照）は、
  active なレプリカが残留するケース（クラス B）では不十分だったことが判明。
- 観測の詳細は `node/internal/kvs/kvs.go` / `node/internal/kvs/sector/sector.go` の
  NOTE コメント (2026-07-04 付) に記録。`sector.go` の `applyProposals` に
  リトライ時の Raft ステータス出力（`## retry proposals ... state/lead/term`）を
  追加済みで、次回実行でリーダー不在を直接確認できる。

### quorum 喪失対策の実装（2026-07-04, Go 実装側）

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

### シミュレーション再実行での発見（run 1・run 2、2026-07-04, simulator/node.log 2 回）

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

### シミュレーション run 3（terminate apply 修正後、2026-07-04）

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

### run 3 深掘り: ゴーストレプリカと apply バッチ中断（2026-07-04, dump.json 解析）

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

### シミュレーション run 4（全修正後、2026-07-04）

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

### learner-first メンバーシップの実装（run 4 の対策、2026-07-04, consensus.go）

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

### シミュレーション run 5（learner-first 導入後、2026-07-04）: raft panic

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

### シミュレーション run 6（tombstone 導入後、2026-07-04）: `need non-empty snapshot` panic

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

### シミュレーション run 7（snapshot ガード後・70 ノード、2026-07-04）

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
→ **その後 2026-07-12 に実装完了**（設計・検証記録は [snapshot.md](snapshot.md)）。
現状はログ無限成長（MemoryStorage のメモリ増加）とも表裏一体で、長時間運用・
大量書き込みでは必須になる。実装時は「learner の catch-up が snapshot 経由に
なる」ため、`ConsensusApplySnapshot` の store 反映と冪等性もセットで設計する。
なお「リーダーの Next がログ範囲外に出た正確な系譜」（同一 term での二重リーダー
疑い = 空ログ再作成による votedFor 忘却の残存経路の可能性）は未特定で、
次回 run の観測対象。

### シミュレーション run 8（100 ノード、2026-07-06）: split が構造的に不成立

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

### シミュレーション run 9（100 ノード、2026-07-06）: join レプリカの恒久乖離

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

### シミュレーション run 10（100 ノード・15 分、2026-07-06）: 残存赤の分析

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

### run 10 後の node レイヤー修正（2026-07-06〜07-07、モデルのスコープ外）

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

### シミュレーション run 11（100 ノード・15 分、2026-07-09）: 非 active セクターの分析

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

→ **対策済み (2026-07-10、モデル検証 → Go 実装の順で実施)**:
下記「mergeBy 解放のモデル検証と実装」を参照。

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

### mergeBy 解放のモデル検証と実装（run 11 (A) の対策、2026-07-10）

run 8 の教訓（apply 黙殺は実装先行では見つからない）を踏まえ、今回は
**モデル反映 → Go 実装**の順で実施した。

**モデル**: `KvsSectorMergeLock.tla`（Raft 版のコピー派生）。既存 Raft 版は
(1) mergeBy に相当する target 側ロックを持たず（ProposeMerge は overlap
ガードのみで排他）、(2) `Leave(n)` が `proposing[n] = "none"` を要求するため
「prepare を commit した後、merge 完了前に preparer が死ぬ」系列が到達不能で、
このバグを原理的に表現できなかった（TODO-1 の既知の制約の具体例）。差分:

- `mergeLock[fs]`（= Go の `Sector.mergeBy`）を追加。`ProposeMerge` は
  `mergeLock[fs] \in {-1, n}` を要求して取得、`ProposeSplit` は
  `mergeLock[n] = -1` を要求（= PreCommitSplit の拒否）。セクター破棄
  （CommitMerge 吸収 / Terminate / Leave）で当該セクターのロックは消えるが、
  **離脱ノードが他セクターに保持するロックは残る**（バグの忠実な写像）。
- `Leave` を `proposing \in {"none", "merge"}` に緩和（merge 中の死亡のみ。
  activate/split 中の離脱は TODO-1 のスコープ）。
- 修正本体 `ReleaseMergeLock(fs)`: 保持者が Members にいないとき生存メンバー
  が解放（WF 付き）。誤検知検証用 `ReleaseMergeLockAny(fs)`: 保持者の生死に
  関係なく解放できる（公平性なし）。

**検証結果**（N=4, InitialMembers={0,1}, MaxChurn=3, 24 workers）:

| Phase | 設定 | 結果 |
|-------|------|------|
| 1: バグ再現 | EnableRelease=FALSE | **EventuallyAllActive / EventuallyFullCoverage 違反**（72 秒、500 万状態生成）。counterexample は「1 が 2 のセクターに ProposeMerge（lock 取得）→ Leave(1) → Join(3) が (2,0) 内 → 2 の ProposeSplit が stale lock で永久ブロック → 0 と 3 が永遠に inactive」— run 11 と同型の系列 |
| 2: 修正確認 | EnableRelease=TRUE | **全 safety + 全 liveness 成立**（14 分、5,545 万状態生成 / 584 万 distinct、深さ 40） |
| 3: 誤検知 safety | +PermissiveRelease=TRUE (safety のみ) | **全 invariant 成立**（2 分、1 億 2,692 万状態生成 / 1,205 万 distinct、深さ 54）。解放ゲートがどれだけ誤発動しても safety は保たれる |

モデルの抽象度の限界: sector = node のため run 11 の「host 死亡後の leftover
への merge が塞がる」形そのものは表現できず（leftover が存在できない）、
stale lock が split を塞ぐ形で同一バグを再現している。leftover 形は下記の
Go 回帰テストで担保。

**Go 実装**（検知と解放を `Sector` 内に封じ込め、kvs.go は無変更）:

- proto: `ConsensusProposal` に `ReleaseMerge{handler}` を追加。apply は
  「mergeBy = handler のときだけ nil に戻す」**CAS で決定的・冪等**
  （新しい PrepareMerge と競合しても新しい claim を消さない）。activation
  ゲートの前に適用する（run 8 の「apply は必ず完了する」規約）。
- 解放ゲート: PrepareMerge / PreCommitSplit が同一保持者の mergeBy に
  `mergeReleaseDuration`（30s、他の修復系と同スケール）連続で拒否されたら
  ReleaseMerge を提案（`checkMergeRelease`、sector ループで毎 tick 判定）。
  健全な merge は 1 秒未満で完了するため、30 秒継続する競合は保持者の死亡
  または stall とみなせる。誤検知はロックなしのインターリービング（Phase 3
  で safety 検証済み）に戻るだけで、生じうる重複は既存 Terminate 系が修復。
- 付随修正: PrepareMerge / PreCommitSplit の mergeBy 競合チェックを
  **propose 前**に移動（モデルの enabling と同じ意味論）。従来は propose 後
  の waitProposal 内で検出しており、(a) 競合エラーで pending フラグが漏れて
  retry tick ごとに再 propose される（run 11 で "pending prepareMerge" の
  スパムとして観測）、(b) PreCommitSplit は apply 側にガードがないため
  競合中でも tail 縮小だけ commit され、呼び出し元の abort と食い違う、
  という 2 つの問題があった。
- 回帰テスト: `TestSector_prepareMerge_releasedAfterHolderStalls`（leftover
  形: 保持者死亡 → 後任の merge が release 後に成功、CAS の確認込み）/
  `TestSector_preCommitSplit_releasedAfterHolderStalls`（split 形: Phase 1
  counterexample と同じ被害経路、tail 非縮小の確認込み）。
  いずれも修正前コードで失敗することを確認済み。

### シミュレーション run 12（180 ノード・2 時間・生存最長 20 分、2026-07-10）: mergeBy 修正後の確認

mergeBy 解放実装後の run（02:26〜04:26。(B) is_stable と (C) disconnect 経路は
未修正のまま）。

**mergeBy 修正の結果**: **stale mergeBy による恒久停止は再発なし**。
2 時間で prepare_merge 競合は 1 インシデントのみ（03:40:19〜48、zombie 化で
強制停止された host の leftover を 2 つの backward が同時に merge しようとした
もの）。ブロックされた側は 29 秒リトライした後、対象セクターが強制破棄されて
自然解消 — release 発火のちょうど 1 秒前で、`ReleaseMerge` の提案は 2 時間で
0 回（**誤検知もゼロ**。30s ゲートは「本当に固着した場合のみ」の設計どおり）。
なお「preparer が lock 保持のまま死ぬ」系列そのものは今回の run では発生して
おらず、その修正効果はモデル (Phase 1→2) と回帰テストによる担保のまま。

**残存問題の主因は (C) zombie 連鎖**: 03:02〜03:38 に **10 ノードが zombie 化
し、リング上を後方（ID 降順: 6b35 → 6927 → 68fd → … → 5bf1）へ 2〜6 分間隔で
単調に連鎖**した。gdump で凍結フレームを確認: `nodeLinkChangeState.func1` が
na.mtx を保持したまま `disconnectLink` → pion `PeerConnection.Close()`
(webrtc_link_native.go:320) でブロック — run 11 (C) で特定した未修正の穴
そのもの。**死にかけの zombie へのリンクを切断しようとした隣接ノードが
Close() のハングで次の zombie になる**、という自己増殖メカニズムが実測された。

影響: 連鎖の進行域（ID 5b〜76 のアーク）で **361 セクターが 2 分以上非緑**
（最長 35 分）、全体の緑は 175/183 → 135 まで低下（03:01〜03:56）。連鎖が
止まった後は自己修復し、**04:01 以降は緑 165+/171・yellow ≤4 の健全状態で
run 終了まで安定** — zombie の発生源さえ止まれば修復機構は機能する。
修復の 20 分テールは再活性化の波が skip 1（2,324 件）/ skip 2（441 件、
うち 381 件が 03:50〜04:00 の回復期に集中）/ is_stable ゲート (B) に律速
されたもの。

→ 次の優先順位: **(C) disconnect 経路の na.mtx 保持解消**（連鎖の根源）、
次いで (B) is_stable ゲート（修復テールの律速）。

### (C) disconnect 経路の修正（run 12 の対策、2026-07-10、モデルのスコープ外）

`NodeAccessor.disconnectLink` を「map からの登録解除は同期（na.mtx 下）、
`link.disconnect()` は専用 goroutine」に分離した。従来は disconnect()（pion
`PeerConnection.Close()` に降りる。死にかけの association で無期限ハングする
ことを run 11/12 の gdump で実測）を先に実行してから map を掃除していたため、
na.mtx を保持する呼び出し元（`nodeLinkChangeState` / `houseKeeping` /
`ConnectLinks` / `SignalingAnswer` / shutdown）が Close() のハングごと
na.mtx を凍結させ、zombie 連鎖（run 12: 10 ノード）の根源になっていた。

- 登録解除が同期なので、以後のパケットが死んだリンクへルーティングされる
  ことはない。`nodeLink.disconnect()` は keepalive/buffer の ticker 停止と
  ルーチンの context cancel を Close() の**前**に行うため、ハングが残っても
  リークするのは goroutine 1 つだけで、リンクとしては即座に死ぬ。
- 回帰テスト `TestNodeAccessor_disconnectDoesNotBlockMutex`（disconnect が
  ブロックする stub webRTC link で、Close() ハング中も na.mtx が取得可能かつ
  リンクが登録解除済みであることを検証）。修正前コードで
  「na.mtx frozen (network zombie)」として失敗することを確認済み。
- 残る候補: `connect()`（subRoutine / ConnectLinks から na.mtx 下で呼ばれる）
  内の `newNodeLink` は pion の ICE agent / mDNS 初期化を含む。run 12 の
  gdump に該当スタックが見えたがブロックの確証はない。zombie が再発する
  場合はここを疑う。

### シミュレーション run 13（180 ノード・18 分・生存最長 20 分、2026-07-10）: leftover 循環待ち

mergeBy 解放 (run 11 対策) と disconnect 経路修正 (run 12 対策、上記) の後の
確認 run。**zombie は 0 件** (disconnect 修正が有効)、全体は緑 160〜177/182 で
run 12 のような劣化ウィンドウなし。merge 競合は 1 系統のみで run 終了 24 秒前
開始のため release ゲート (30s) 発火前に run 終了 (設計どおり)。skip 2 は
2 件に激減。

**残った未 activate (sector 019f4a85-b850, host b8901da5, 14 分 yellow)**:

1. host ba82438a が 05:36:29 に churn で死亡。その active セクター
   019f4a83-0766 [ba82438a, bc02dbb1) が **leftover** (レプリカ群は quorum
   健全) として約 13 分残留。生存時間を 20 分に延ばしたことで leftover の
   自然消滅 (メンバー全滅) も遅くなり顕在化しやすくなった。
2. backward の b8901da5 (inactive) は sectorActivate を 611 回受信したが、
   leftover が (自分, frontward) 内にあるため overlap ガード「skip 1」で
   569 回拒否。
3. **循環待ち**: leftover を merge で掃除できるのは隣接する active セクター
   だけだが、その位置にいる b8901da5 は inactive で merge 経路を実行できない。
   さらに後方の active node はより近い b8901da5 の inactive セクターしか
   見ない。「activate は leftover が邪魔で不可、leftover の掃除は active
   でないと不可」のデッドロック。分岐表に該当行が存在しなかった。
   run 11 でも同型を観測 (当時は偶然の join で解消)。

### leftover 循環待ちの対策: tail 切り詰め activation（run 13 の対策、2026-07-10、モデル検証 → Go 実装）

**モデル**: `KvsSectorLeftover.tla`（MergeLock 版のコピー派生）。従来モデルは
sector = node の抽象化のため「node は死んだが sector は残る」leftover を
表現できず、Go の skip 1 ガード自体もモデル化されていなかった。差分:

- `state[n]` に `"leftover"` を追加し、**Members（routing に見えるノード）/
  Actives（active な member = seed の EntireState の見え方）/
  ActiveSectors（leftover を含むセクター head、overlap 判定用）を分離**。
  leftover は rView から消えて sActives に残る = Go の「sector store には
  見えるが routing にはいない」状態の写像。
- `LeaveLeftover(n)`: active ノードがセクターを残して離脱（発火条件は
  「他に member が 2 つ以上 (うち active が 1 つ以上)」= レプリカ quorum が
  host 抜きで維持できる状況の抽象化）。leftover の自然消滅は
  `LeftoverQuorumLoss`（member が 1 つ以下に減った場合のみ、WF 付き =
  checkQuorumLoss の写像）だけに限定し、「quorum 健全な leftover は修復機構
  自身が吸収しなければならない」という liveness を検証対象にした。
- `CommitActivate` / `ActivateFirst` に Go の overlap ガード（skip 1）を
  忠実に追加し、修正本体を `ClipActivationTail` 定数でゲート:
  skip する代わりに **tail を範囲内最近傍の active head に切り詰めて
  activate** する。

モデル作成過程で 2 つの意味論バグを踏んで修正した点も記録しておく:
(1) 生存 member 1 つ + leftover の組は merge の幾何条件 (`v # n`) が
構造的に満たせない — 実機では quorum 喪失 → LocalDestroy の領域なので
`LeftoverQuorumLoss` として明示。(2) `anyActive` が leftover を数えると
「active member 全滅 + leftover 残存」で ActivateFirst が封じられ spurious
violation になる — Go の seed EntireState は生きているノードの hosting
セクター報告から作られるため leftover を含まない (`Actives` を追跡)。

**検証結果**（N=4, InitialMembers={0,1,2,3}, MaxChurn=3, 24 workers）:

| Phase | 設定 | 結果 |
|-------|------|------|
| L1: バグ再現 | ClipActivationTail=FALSE | **EventuallyAllActive / EventuallyFullCoverage / EventuallyNoLeftover 違反**（68 秒）。counterexample は「1 が active 化 → 1 が 2 を activate → LeaveLeftover で 1 が leftover 化 …」run 13 と同型の循環待ち |
| L2: 修正確認 | ClipActivationTail=TRUE | **全 safety + 全 liveness 成立**（6 分 53 秒、2,906 万状態生成 / 326 万 distinct、深さ 30）。leftover は必ず merge で吸収される (EventuallyNoLeftover) |
| L3: 誤検知 safety | +PermissiveRelease=TRUE (safety のみ) | **全 invariant 成立**（44 秒、3,365 万状態生成 / 368 万 distinct）。切り詰め activation と無条件 lock 解放が併発しても safety 維持 |

**Go 実装**（`activateHostingSector` の tail 決定のみ、新アクションなし）:

- skip 1 で nil を返す代わりに、(local, frontwardNodeID) 内の**最近傍の
  active head を blocker として tail に採用**する
  （ログ: `@@ Activate hosting sector clipped to X instead of Y`）。
  [local, blocker) は重複を生まず、active になったセクターが通常の merge
  （ReleaseMerge backstop 込み）で leftover を吸収して tail が伸びる。
  blocker がある場合は frontward セクターの replica candidate も不要に
  なるため、skip 2 の一部も同時に解消される。
- 回帰テスト `TestKVS_sectorActivate_clipsTailAtLeftover`（dead host の
  ACTIVE レプリカを ConsensusApplyProposal で再現し、activation の tail が
  leftover head に切り詰められることを検証）。修正前コードで失敗
  （activation が skip され続けて timeout）することを確認済み。

### シミュレーション run 14（180 ノード・114 分・生存最長 20 分、2026-07-10）: seed セッション喪失による「接続黒穴」

tail 切り詰め activation 導入後の確認 run。**最終フレームは yellow 0 で、
これまでの修正はすべて維持されている**:

- 最終フレーム (11:57): alive 174、sector 178 中 **green 172 / yellow 0** /
  red 2（churn 直後の member 補充中 3〜4/5、構造的でない）/ no-host 4
  （新鮮な leftover、修復途中）。
- 切り詰め activation は 221 回発火し、leftover 起因の恒久循環待ちは消滅。
- network zombie は 0（watchdog が stuck loop を 5 回検知して即 stop、
  連鎖なし）。stale mergeBy デッドロックも再発なし。

一方で run 途中には **120 秒以上 yellow のままの episode が 169 件**
（最長 1,005 秒）あり、**約 1,900 の node 寿命のうち 50 が一度も active に
ならないまま死亡**（例: 6c1f1294 は寿命 20 分をフルに ready 状態で待機、
`@@ Entire not inactive` を毎秒出力し続けた）。yellow は 6x / a0x / d4x /
1c-22 などの **リング領域単位のクラスタ**で発生し、领域内の全 node が
同時期に解消される。

**根本原因: seed セッションを失った「接続黒穴」node。**

1. 全 WebRTC シグナリング (offer/answer/ICE) は seed の PollSignal
   ストリーム経由。このセッションが壊れた node は**新規リンクを一切
   確立できなくなる**が、確立済みリンクはそのまま生きるため routing 上は
   健在に見え、自己判定 (`nextNodeMatched`) も stable のままになり得る。
2. dump 解析で「必須リンク未接続 + 新規接続ゼロが 240 秒以上」の node を
   **26 体**検出。主要 yellow クラスタは全て黒穴と 1:1 対応する
   （6x 領域 = 692dcc11、a 領域 = a092970f、d 領域 = d4f96032、
   1c-22 領域 = 1dbdf037）。
3. 黒穴の node.log シグネチャ: `failed to poll` / `failed to keepalive`
   (`internal: reqID: ...`) が発生開始から**死ぬまで 10 秒間隔で継続**
   （1dbdf037 は 19 分間）。回復例ゼロ。
4. 周辺への波及: 黒穴の隣に join した node は必須 1D リンクが張れず
   `is_stable=false` のまま → KVS subRoutine 全体が skip → hosting sector
   を作れない → backward active は `frontward next sector is not created
   yet` を毎秒 skip → **activation チェーンが hop 単位で堰き止められ、
   黒穴が寿命 (最長 20 分) で死ぬまで領域全体が yellow**。
   （例: 1cd7c7cc は寿命 950 秒間、必須リンク 1 本 (対 1dbdf037) だけが
   張れず一度も stable にならず、hosting sector を作れなかった。）

**seed 側の構造要因**（seed/controller/controller.go）:

- keepalive は逆方向の long-poll: 他 node が `ReconcileNextNodes` で
  disconnected を報告すると seed が対象 node に challenge を発行し、
  **lifespan を ShortLifespan=10 秒に短縮**する。ところが client は
  keepalive 応答後に **10 秒 sleep してから再購読**するため、challenge
  後の再購読 (= lifespan 復元) は構造的に約 10 秒 + RTT 後になり、
  eviction tick (5 秒間隔) との**際どい競合**になる。負荷や GC で
  数百 ms 遅れると healthy な node が evict される。
- evict 後は Keepalive / PollSignal / SubscribeSignal がすべて
  `CodeInternal` を返し続けるが、**client (SeedAccessor) は同じ失敗を
  10 秒おきに再試行するだけで AssignNode をやり直す経路がない** →
  セッションは永遠に回復しない。
- PollSignal / Keepalive の「already subscribed」ガード: 古い stream が
  server 側で終了を検知されないまま残ると、再購読が
  `already subscribed` で拒否され続ける（keepalive 側は
  normalLifespan/2 = 15 分のタイマーまで解放されない）。

**対策**（詳細と TODO は [spec/seed/README.md](../seed/README.md) に移管）:

- **対応済み (2026-07-11)**: challenge 競合の解消を両輪で実施 —
  ShortLifespan 10 秒 → 30 秒 (seed/seed.go) + client keepalive ループの
  sleep をエラー時のみに変更 (seed_accessor.go、成功時は即再購読)。
  未購読窓 ≒ RTT vs 猶予 30 秒となり競合は実質消える。
- **TODO (spec/seed 側)**: (1) client の AssignNode リトライ（黒穴の
  恒久解。run 15 で `failed to poll` の 10 秒間隔ストリークが残存したら
  着手）、(2) already subscribed の自己修復、(3) 周辺 node 側の防御
  (is_stable 緩和とセットで判断)。

### シミュレーション run 15（180 ノード・6.8 時間、2026-07-10〜11）: 黒穴修正の確認と残存 tail の分類

challenge 競合修正後の長時間 run。node.log は最初の 4 時間で途切れている
（dump は 6.8 時間分）。churn 率は run 14 と同等 (約 1,000 starts/h) で、
寿命 60 秒以上の node 寿命は 6,753 (run 14 の 3.6 倍のサンプル)。

**黒穴は完全に消滅し、修正の因果が確定**:

| 指標 | run 14 | run 15 | 変化 |
|------|--------|--------|------|
| `failed to poll` / `failed to keepalive` | 1,500 / 1,507 件 | **0 / 0 件** | 消滅 |
| 黒穴 (必須リンク未接続+新規接続ゼロ ≥240s) | 26 体 | **0 体** | 消滅 |
| time-to-first-stable p99 / max / never | 177s / 733s / 10 | **6s / 10s / 0** | 桁違いに改善 |
| 一度も active にならず死んだ寿命 | 50/1,851 (2.7%) | **20/6,753 (0.3%)** | 1/9 |
| time-to-active median / p90 / p99 | 6s / 95s / 467s | 5s / 9s / **152s** | p90 で 1/10 |
| yellow episode (≥120s) 発生率 | 88 件/h | **16 件/h** | 1/5.5 |
| yellow episode median / p90 / max | 195s / 600s / 1005s | 180s / 525s / 1035s | 同等〜微改善 |
| watchdog (loop stuck) | 2.6 回/h | 0.75 回/h | 1/3.5 |
| 最終フレーム | green 172 / y 0 / r 2 / nh 4 | green 162 / y 0 / r 3 / nh 4 | 同等 (健全) |

run 14 で黒穴が time-to-first-stable を桁で悪化させていた副作用
(evict された node は seed の `GetNodesByRange` からも消えるため、周辺の
`ReconcileNextNodes` が構造的に mismatch し続ける) も同時に解消された。

**TODO 残置ケースの悪化有無**: 悪化なし。

- leftover 残留 (nohost episode ≥120s): 発生率 56→35 件/h に減、
  median 255→285s / p90 555→645s と分布はわずかに長い側に寄ったが
  max は同等 (1155→1095s)。「quorum 健全な leftover は merge か
  checkQuorumLoss まで残る」という既知挙動の範囲内
  （黒穴消滅で node が寿命を全うするようになり、健全な leftover が
  増えた影響と整合）。
- ReleaseMerge backstop 10 回発火 / clip 241 回発火 — いずれも正常動作、
  恒久停止なし。
- is_stable フラップ: median 13→15 回/寿命 (max 47→167)。寿命が延びた
  分の増加で、停滞への寄与は観測されず。

**残存 tail event = 既知 TODO の領域** (6.8 時間で地域クラスタ 2 件のみ、
いずれも自然回復):

1. **f 領域 19:36〜20:09**: watchdog が「loop stuck」で f4c5e325 を stop
   (19:33:54) した直後から発生。merge 相手だった f433e0e1 → その後継
   f424cb86 が「hosting sector の force terminate → 再作成」を繰り返し
   (inactive leftover が 4〜6 世代堆積、うち 1 つは **replicas=8** と
   member 定員 5 を超過)、チェーン先端がそこで足踏み。
   → 既知 TODO: TODO-2 (stale レプリカ掃除)、quorum 喪失 terminate の連鎖。
2. **e 領域 21:51〜22:03**: active だった e0x〜e7x の広域が一斉に
   inactive 化する「地域崩壊」(旧 active sector は tails が divergent な
   leftover として残留)。再 activate は hop-by-hop なので回復に約 12 分。
   → 起点は下記「反応しない node の調査」で特定 (loop-stuck の連鎖死)。

### run 15 続報: 「反応しない node」の正体 — loop-stuck 12 体 (newNodeLink 穴は無実、2026-07-11 解析)

dump 全走査 (6,777 寿命) で無応答系のシグネチャを分類した結果:

- **na.mtx 型 network zombie はゼロ**: レコード途絶→復帰 (60 秒超の loop
  停止から生還) 0 件、長時間 offline (赤) 0 件、conn リスト凍結 0 件。
- **「stop レコードなしで消えた node」が 12 体** (0.18%)。うち 3 体は log
  で watchdog kill を確認済み (warn 5s / stop 60s)。残り 9 体は log 欠落窓
  (19:34 以降) だが同一シグネチャで、**時刻もリング領域もクラスタして
  おり、上記 2 つの yellow クラスタと一致** (19:53〜20:07 の 5 体 =
  f クラスタの後半、21:50〜21:56 の 4 体 = e クラスタの起点。消えた node の
  sector がそのまま divergent leftover になっている)。地域崩壊の正体は
  この連鎖死。
- **凍結機構 (gdump 16:37:20 の解析)**: DTLS GCM encrypt/decrypt 内の
  `sync.Pool.pinSlow` (Go runtime のグローバル `allPoolsMu`) に **2,109
  goroutine が滞留**する convoy → nodeLink send mutex → transferer.mtx の
  writer 待ちに変換され Receive RLock 293 本 + main loop (MessagingPost)
  が凍結 → watchdog 発火。**180 node を 1 プロセスに同居させた simulator
  アーティファクト** (~8,200 DTLS セッションが GC のたびに pool 再 pin で
  グローバルロックに殺到) であり、colonio プロトコルのバグではない。
  実運用 (1 node / 1 process) では発生しない。dying region の隣接 node は
  修復トラフィックで DTLS 負荷が上がるため、連鎖死が地域に固まるのも整合。
- **newNodeLink 候補穴は無実と確認**: gdump 上、重い ICE agent / mDNS
  生成は connect() の非同期 goroutine (getLocalSDP → CreateOffer 経由の
  遅延生成) で走っており、na.mtx 下の NewPeerConnection は軽量。na.mtx の
  待ちは一時的な RLock (n=1) のみ。
- 副作用: loop-stuck 死は runNode の defer (stop レコード + 通常 teardown)
  を通らないため sector が汚く残り、地域の修復コスト (force terminate /
  divergent leftover) を増やす。watchdog の Col.Stop 後も raft goroutine が
  数十秒残ることを log で確認 (既知 TODO: Col.Stop 残留)。
- 対策候補 (simulator 側): 1 プロセスあたりの node 数削減、GOGC/GOMAXPROCS
  調整、pion の mDNS 無効化 (MulticastDNSMode — mDNS socket bind の
  syscall 滞留 10 件も観測)。

### シミュレーション run 16（150 ノード・6 プロセス・57 分、2026-07-11）: Transferer 自己デッドロックの特定

simulator を 6 pod に分割した run。プロセス分割により run 15 の sync.Pool
convoy ノイズが消え、**loop-stuck の真因が colonio 本体のデッドロックだと
確定**した。

**30 秒以上 active にならない sector**: 142 episode (+ 終了時進行中 4)。
分類:

1. **起動ウェーブ (01:12〜01:15)**: 全ノード同時 join 後、activation
   チェーンが全域に届くまで最長 165 秒。1 秒周期の hop-by-hop 伝播の
   設計特性で、churn 由来ではない (寿命の短い 9 ノードはウェーブ到達前に
   死亡し never-active)。
2. **終端クラスタ (02:01〜run 終了、ID 08〜11 領域)**: watchdog kill 5 件が
   **4 つの異なるプロセス**で発生、全て同一リング領域。健全な新規 joiner
   7 体 (ready で `Entire not inactive` を待ち続ける) が never-active の
   まま run 終了。

**根本原因 (gdump で確定): Transferer の RWMutex 自己デッドロック**。

```
Transferer.subRoutine (transferer.go:137 で mtx.Lock を保持)
  → :162 再送 TransfererSendPacket           ← ロック保持のまま送信
  → Network.classifyPacket → 経路なし (network.go:254)
  → Transferer.Error → Response (宛先 = 元パケットの SrcNodeID = 自分)
  → classifyPacket → ローカル配送
  → Transferer.Receive (:257) → mtx.Lock      ← 同一 goroutine で再取得
  → 非再入 RWMutex のため永久デッドロック
```

- 発火条件は「retry 対象の宛先への経路が瞬間的に消える」こと。churn の
  波で routing に穴が開いた領域の (特に joining 直後の) ノードが踏む。
  初被害の 0e05fcd9 は join の約 23 秒後にデッドロック。
- デッドロックしたノードはリンク keepalive だけ生き残る半死状態
  (受信 goroutine 41 本が RLock 待ちで滞留) → 60 秒後に watchdog kill →
  defer を通らない汚い死で領域の churn がさらに進み、**連鎖的に隣接
  joiner が同じ罠を踏む** (5 件が 10 分間に同一領域で連鎖)。
- `Request()` (:194) はロックを外してから送信しており、subRoutine (:162)
  だけが「mtx 保持中に送信しない」規約に違反している。run 15 の
  loop-stuck 12 体にも同じシグネチャが含まれていた可能性が高い
  (当時は pool convoy と混在して切り分け不能だった)。
- 「might be stacked」警告 (5 秒) = 5 件が全て致死 (60 秒) に進行 =
  一時停止ではなく恒久デッドロックであることと整合。

**対策 (2026-07-11 修正済み)**: subRoutine をロック下で再送パケットの
収集のみ行い、ロック解放後に送信する形に変更 (Request と同じ「mtx 保持中に
送信しない」規約に統一)。回帰テスト
`TestRetry_noRouteErrorDoesNotDeadlock` (classifyPacket の「経路なし →
Error → 自ノードへ同期配送」を忠実に模倣) が修正前コードで両アサーション
失敗 (デッドロック検出 + mtx 保持継続) することを確認済み、修正後パス。
なお「送信 API が同一 goroutine で Receive に再入する」構造自体は残って
いるため、送信 API の新規呼び出し箇所では同じ規約 (ロック下で送信しない)
を守る必要がある。Receive のローカル配送を goroutine に逃がす構造的解消は
順序保証への影響評価が必要なため見送り (規約 + 回帰テストで担保)。

### merge/overlap 抗争の解析とモデル検証（run 2026-07-16 → KvsSectorFalseLeftover、2026-07-17〜19）

design.md「churn 下の課題」で未解決だった merge/overlap 抗争
（ack 済み書き込みの修復起因破棄）の対応。run 2026-07-16 (30 分・150 node)
のログでループ 1 周を完全にコード対応付けし（詳細は design.md の
「追加解析 2026-07-17」: 自壊ペア 103 回 / merge 総数 505 / 48 node、
CAS 重複 126 組・revision リセット実測）、モデル反映 → 修正検証を実施した。

**モデル**: `KvsSectorFalseLeftover.tla`（Leftover 版のコピー派生）。差分:

- **merge の Go 忠実な非原子化**: ProposeMerge (lock) → MergeMigrate
  (移送 + terminate 提案 = fire-and-forget) → CommitMergeExtend (tail 拡張)
  に分割し、victim の破棄を独立アクション ApplyVictimTerminate (WF) に分離。
- **stuck[h]**: グループの commit 不能 (quorum 喪失) を抽象化。terminate が
  効かない「不死身の active セクター」を表現。回復は LocalDestroy
  (checkQuorumLoss の写像、WF) のみ。TODO-1 の「commit が来ない」故障モードの
  部分実装でもある。
- **DropFromRView**: 生存 member の視界喪失（偽 leftover 誤認のトリガ）。
- **データ (追跡アーク 1 本)**: holder 変数で最新 ack 済み書き込みの保持
  セクターを追跡し、churn 起因の喪失 (LostChurn、design.md が許容) と
  修復起因の喪失 (LostRepair = バグ) を区別。検証ターゲットは
  **NoRepairLoss** (修復がデータを壊さない)。

**TLC が特定した欠陥** (修正を外すと反例が出ることを個別に確認):

| # | 欠陥 | 修正 |
|---|------|------|
| 1 | CommitMerge が victim の terminate 完了を確認せず tail 拡張 → 重複 → TerminateA が merger (データ保持側) を自壊。stuck 不要のレースでも発生、stuck だと無限ループ (run 実測形) | 修正 1: ConfirmVictim |
| 2 | migrate 後の victim への書き込み窓 | 既存 `mergeFenced` が対処済み (モデルを実装に整合) |
| 3 | merger 死亡後の孤児 terminate が後から生存セクターを破棄 | 修正 2: terminate の tenure スコープ |
| 4 | 同一保持者の re-prepare と旧 terminate の ABA 対合 | 修正 2': prepare 世代の携行・一致検査 |
| 5 | (誤検知) release 後の migrate が無世代 terminate を残す | 修正 2' に包含 (世代外は dead-on-arrival) |
| 6 | activation の背後カバー盲点 → 修復不能な恒久重複 (liveness 違反) | 修正 3: NoActivateUnderCover |
| 7 | 被 merge 中のセクターが吸収役になり、absorber の export に乗らないデータが消滅 (`mergeFenced` は Import を塞がない) | 修正 4: NoAbsorbWhileLocked |
| 8 | prepare 時に捕獲した propTail が victim の split 後に stale 化し、生きたセクターを飲み込む | 修正 1': CommitMerge 前の範囲再検証 (Extend と同じガード) |

併せて ReleaseMergeLock の enabling を Go の実トリガー（競合タイマー、
保持者の生死を見ない）に整合させた。

**検証結果** (N=4, InitialMembers={0,1,2,3}, stuck 1, drop 1, 24 workers):

| Phase | 設定 | 結果 |
|-------|------|------|
| FL1: バグ再現 | 修正 4 種 OFF, safety | **NoRepairLoss 違反**（深さ 10、1 秒） |
| FL2 safety | 修正 4 種 ON, churn 2 | **違反なし**（75 億状態生成 / 6.98 億 distinct / 深さ 48、2h59m） |
| FL2 liveness | 同, churn 1 | **全 safety + 全 liveness 成立**（4.35 億状態生成 / 4,769 万 distinct / 深さ 39、2h04m） |
| FL3: 誤検知 safety | + PermissiveRelease, churn 2 | **違反なし**（117 億状態生成 / 10.4 億 distinct / 深さ 45、4h36m）。この規模は fingerprint 衝突推定が高い点に注意（MC のコメント参照） |

**Go 実装は同日完了 (2026-07-19)**: 4 修正をモデルの Fix 定数と 1:1 対応で
実装（proto に Terminate の merge スコープと SectorSnapshot の
merge_generation を追加）。回帰テスト 5 本はいずれも修正前コードで失敗する
ことを確認済み。詳細は TODO の同名項目を参照。

### run 17: merge/overlap 修正の定量確認（2026-07-19、22 分・延べ 501 node）

修正 4 種を入れた初回 run。04:25:58〜04:48:13 の約 22 分、延べ 501 node
（churn あり）、終盤時点の生存 210 node 中 208 が active sector を host
（normal/online/stable 183）。**判定: merge/overlap 抗争は解消**。

- **CAS 監査**: cas ok 15,542 件中、同一 (key, revision) の重複成功は
  **1 組**（run 2026-07-16: 126 組）。しかもこの 1 組は下記 ep1 の
  revision 空間再利用（リセット後に同じ番号を再通過）であり、重複 active
  セクターによる二重 ack ではない。
- **自壊 ping-pong は 0**（run 2026-07-16: 自壊ペア 103 回、46 秒に 27 周）。
  同一 (node, tail) の activate は最多 4 回/22 分で正常範囲。
- **新ガードの発火**: `WaitTerminated` abort 15 回・範囲再検証
  「abort commit」18 回。後者は従来なら「重複 → TerminateA 自壊」に進んだ
  事象がそのまま阻止された回数に相当する。abort はいずれも 15〜20 秒間隔の
  再試行 1〜2 回で自己解消（同一 merger の 3 連続 abort なし）。skip 3・
  stale tenure・import fence の発火は 0（発火機会が生じなかった）。
- **revision 後退は 3 エピソード**（watcher の `watch back` 23 件が 3 時点に
  集中。各エピソードはリング上で隣接するキー群 = 単一セクターに対応）:
  - **ep1 (04:31:10)**: sector [372904af, 3bc76609) が leaderless →
    checkQuorumLoss の force terminate → 空で再 activate。rev 約 180 →
    1 からやり直し（キー 3 個全損）。**quorum 喪失の設計上の損失経路**
    （design.md が許容する churn 起因喪失）であり抗争ではない。
  - **ep2 (04:39:49) / ep3 (04:41:30)**: 末尾 1〜11 rev の小幅後退。
    force terminate 後の復旧（leftover import）元 replica の applied 状態が
    commit より僅かに遅れていたことによる ack 済み末尾の喪失。改善候補として
    TODO に追加（[詳細](#todo-stale-leftover-import)）。
- **nohost 非退行**: dump.json から 256 キー位置の active 被覆を 1 秒粒度で
  解析（warmup 300 秒除外）。uncovered episode は 12 件、median 5s /
  max 11s — run 15 の nohost episode（median 285s / p90 645s）から大幅改善。
  `WaitTerminated` の 15s タイムアウトは修復律速になっていない。
- **データプレーン健全**: corrupt 0（load・watch とも）、lock lost 0 /
  conf 0 / unk 33 (1.3%)。負荷総量: set 64,586 / cas 15,563 / get 30,800 /
  patch 5,921 / del 6,053。merge は preparing 397 → done 302
  （差分は prepare 失敗 56 + abort 33 + migrate 失敗等）。

合格ライン「CAS 監査 0 件・revision リセット 0 件」は字義通りには
1 組 / 3 エピソード残ったが、全て force terminate（quorum 喪失）系に帰着し、
修正対象だった「生存 host の leftover 誤認 → merge → 自壊ループ」の兆候は
ゼロ。TODO の merge/overlap 項目は完了とする。

## 今後の TODO

KVS 関連の TODO はこの章に一元化する。マークの読み方: `[x]` = 対応済み /
`[ ]` = 未対応。対応済み項目の経緯・実測データは
「シミュレーション実行の記録」の各節、未対応項目の背景は「詳細」の各節を参照。
関連文書側にも TODO がある: snapshot のパラメータ調整は
[snapshot.md](snapshot.md)、データプレーンは [dataplane.md](dataplane.md)、
公開 API は [api.md](api.md)、lock は [lock.md](lock.md)、seed は
[spec/seed/README.md](../seed/README.md)。

### 一覧

#### モデル (TLA+)

- [x] **routing ビューと sector-store の分離** → `KvsSectorSepView.tla` で実装済
- [x] **Raft 合意の非原子性のモデル化** → `KvsSectorRaft.tla` で実装済
      （ActivateFrontward / Split を Propose → Commit に分割、TerminateB の
      発火を確認: N=3: 8 回, N=4: 14 回）
- [x] **複数の Join/Leave 同時発生のレース検証** → N=3 MaxChurn=4 まで検証済
      （MaxChurn=2 で Merge 吸収側の proposing 未クリア / CommitActivate の
      anyActive 未更新を発見・修正）
- [x] **Merge の非原子化** → `KvsSectorRaft.tla` で ProposeMerge/CommitMerge に
      分割（MaxChurn=3 でインターリーブバグ 2 件を発見・修正、TerminateA が
      初めて発火: 20 回）
- [x] **prepare_merge (mergeBy) の解放経路の検証** (2026-07-10) →
      `KvsSectorMergeLock.tla`（バグ再現 → ReleaseMerge で回復 → 誤検知
      safety の 3 phase。run 11 (A) の対策）
- [x] **leftover セクターと tail 切り詰め activation の検証** (2026-07-10) →
      `KvsSectorLeftover.tla`（Phase L1〜L3、EventuallyNoLeftover 含む。
      run 13 の対策）
- [x] [**TODO-1: quorum 喪失の故障モードを含む拡張モデル**](#todo-1) —
      `KvsSectorFail.tla` で stuck/TimeoutAbort/LocalDestroy をモデル化し
      検証完了 (2026-07-25)。誤発動 (misfire) を許しても safety + 全 liveness
      が成立することを N=3/N=4 の複数規模で確認（詳細は「TODO-1 の検証結果」の
      節）。TODO-3/4 (2026-07-04 実装) の設計判断の検証負債を解消
- [ ] [**TODO-2: stale active レプリカの掃除とガード緩和の検証**](#todo-2) —
      優先度: 高。TODO-1 と独立に着手可。クラス B' は run 4 / run 7 でも
      継続観測（短時間で解消し恒久化はしていない）

#### Go 実装（KVS プロトコル）

- [x] [**TODO-3: セクター操作の timeout + abort**](#todo-3) (2026-07-04) —
      proposalWaitTimeout=15s、Propose の有界化、`applyProposals` のロック外
      Propose。モデル検証は TODO-1 待ち
- [x] [**TODO-4: quorum 喪失セクターのローカル強制破棄**](#todo-4)
      (2026-07-04) — leaderless 30s / pending 停滞 45s の 2 系統 +
      CheckQuorum 有効化。モデル検証は TODO-1 待ち
- [x] **apply ハンドラの冪等・必ず完了規約** (2026-07-04) — terminate 完了保証、
      CommitSplit no-op 化、AllocateSector 許容、publishEntries のバッチ継続
      （= 改善案 B の主要部。run 2 / run 3 の対策）
- [x] **learner-first メンバーシップ** (2026-07-04) — 未同期 voter による
      quorum 毀損の根絶（run 4 の破棄ストーム対策、run 7 で「active セクターの
      破壊 0 件」を確認）
- [x] **raft メンバー ID の使い捨て化** (2026-07-04) — sector tombstone +
      停滞メンバーの reap/新 slot 再追加（run 5 の対策）
- [x] **explicit パケットの非宛先受理ガード** (2026-07-04) — ゴーストレプリカ
      対策（run 3 深掘りの対策）
- [x] **snapshot 未実装時の送信要求ガード** (2026-07-04) —
      `ErrSnapshotTemporarilyUnavailable` 変換で `need non-empty snapshot`
      panic を根絶（run 6 の対策）
- [x] **ManageMember の localNodeID panic ガード / sectorPrepareSplit の
      nil ガード** (2026-07-04) — クラッシュ系の穴埋め
- [x] **Import / CommitSplit の activation ゲート通過** (2026-07-06) —
      split が構造的に不成立だった規約違反の解消（run 8 の対策）
- [x] **bootstrap conf change への nodeID context 付与 + conf change 適用
      エラー継続 + nodeID 不明 learner の promote 抑止** (2026-07-06) —
      join レプリカの恒久乖離 → 強制破棄ストームの正帰還を解消（run 9 の対策）
- [x] **メンバー除去の out-of-band 通知 (COMMAND_REMOVE)** (2026-07-06) —
      除去済みメンバーの stale レプリカ残留（赤の主因）を解消（run 10 の対策）
- [x] **mergeBy の解放経路の Go 実装** (2026-07-10) — ReleaseMerge 提案 +
      mergeReleaseDuration ゲート（run 11 (A) の恒久停止を解消）
- [x] **leftover 循環待ちの解消 = activation の tail 切り詰め** (2026-07-10) —
      `activateHostingSector` の skip 1 を切り詰め activate に変更
      （run 13 の 14 分 yellow / run 11 の同型を解消）
- [x] **snapshot の本実装** (2026-07-12) — operator の store serialize +
      appliedIndex 更新 + トリガ有効化（run 6 / run 7 の残課題、ログ無限成長
      対策と表裏一体）。コード確認 2026-07-17: `consensus.go` の
      `maybeTriggerSnapshot` / `appliedIndex` 更新、`sector.go` の
      `ConsensusGetSnapshot` / `ConsensusApplySnapshot` として実装済み。
      設計・churn 検証の記録とパラメータ調整の残 TODO は
      [snapshot.md](snapshot.md)
- [ ] [**is_stable ゲートの緩和**](#todo-is-stable)（設計 + Go） —
      不安定時の修復凍結 = カバレッジ漸減の律速
- [ ] [**ManageMember のヒステリシス**](#todo-hysteresis)（Go） —
      run 4 改善候補 3
- [ ] [**mergeSector: ターゲット離脱時の abort**](#todo-merge-abort)（Go） —
      kvs.go の既存 TODO コメント
- [ ] [**強制破棄・タイムアウトしきい値の実測再調整**](#todo-thresholds) —
      2s / 15s / 30s / 45s は暫定値のまま
- [x] [**merge/overlap 抗争（生存 host の sector を leftover と誤認）**](#todo-merge-overlap)
      （設計 + Go + モデル） — `KvsSectorFalseLeftover.tla` で 8 欠陥を特定し
      修正 4 種を Go 実装（2026-07-19、回帰テスト 5 本）。run 17 で定量確認
      済み: 自壊 ping-pong 0（前 run 103 ペア）・CAS 重複は quorum 喪失系の
      1 組のみ・nohost 大幅改善。経緯の正典は [design.md](design.md)
      「churn 下の課題」
- [ ] [**leftover import 元 replica の遅れによる ack 済み末尾喪失**](#todo-stale-leftover-import)
      （Go） — run 17 の ep2/ep3（末尾 1〜11 rev）。force terminate 後の
      復旧品質の改善候補
- [ ] [**`Operator.SetRange` の全周セクター縮小の扱い（要検証）**](#todo-setrange)
      （Go, operator） — 2026-07-16 発見、2026-07-17 コード確認で未修正のまま
- [ ] [**Col.Stop() 後のセクター raft goroutine 残留**](#todo-col-stop)
      （Go, node/simulator） — 解析ノイズ
- [ ] [**同一 term 二重リーダー疑いの系譜特定**](#todo-dual-leader)（調査） —
      run 6 の未特定事項
- [ ] **アルゴリズム改善案 A / C / D / E / F / G と B の残り
      （CommitMerge の extendTailOnly）**（Go） — 「アルゴリズム改善案」の
      優先度サマリ参照。コード確認 2026-07-17: いずれも未実装のまま

#### Go 実装（node / seed レイヤー）

- [x] **リンク死活検知の短縮** (2026-07-06) — SessionTimeout 5min → 30s /
      KeepaliveInterval 1min → 10s（run 10 後の修正 1）
- [x] **network-zombie 対策: send 経路のロック保持解消 + watchdog 強化**
      (2026-07-06) — run 10 後の修正 2
- [x] **disconnect 経路の na.mtx 保持解消** (2026-07-10) — run 12 の zombie
      連鎖の根源。残候補だった connect() 内 newNodeLink は run 15 の gdump で
      無実を確認（重い ICE/mDNS 生成は非同期側）
- [x] **Transferer 自己デッドロックの修正** (2026-07-11) — subRoutine の
      mtx 保持中再送を「収集 → 解放後送信」に変更（run 16 の終端クラスタの
      根本原因）。回帰テスト `TestRetry_noRouteErrorDoesNotDeadlock`
- [x] **seed セッション喪失の「接続黒穴」: challenge 競合の解消** (2026-07-11) —
      run 14 の最上位残存要因（26 体）。run 15 (6.8h) で 0 件・
      never-active 2.7%→0.3% を確認。AssignNode リトライ等の残 TODO は
      [spec/seed/README.md](../seed/README.md) に移管
- [ ] [**Transferer: 送信 API の同一 goroutine 再入の構造的解消**](#todo-transferer)
      — 見送り中（「mtx 保持中に送信しない」規約 + 回帰テストで担保）

#### simulator

- [x] **loop-stuck 連鎖死（sync.Pool グローバルロック convoy、
      180 node/1 process 起因）** — run 16 でプロセス分割を実施し解消
      （colonio プロトコルのバグではない simulator アーティファクト）

### 詳細

未対応項目の背景・内容・検証項目。TODO-3 / TODO-4 は実装済みだが、
モデル検証（TODO-1）の前提資料として背景記録を残している。

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

**検証完了 (2026-07-25)**: `spec/kvs/KvsSectorFail.tla`（`KvsSectorRaft.tla` 派生、
~600 行）で上記 3 検証項目すべてに答えを得た。詳細は次節「TODO-1 の検証結果」。

<a id="todo-1-result"></a>
#### TODO-1 の検証結果（`KvsSectorFail.tla`、2026-07-25）

**モデル**: `stuck[h]`（h のグループが commit 不能、非決定的に発生・quorum を
精密にモデル化しない抽象）+ 回復アクション `TimeoutAbort(n)`（Go の
proposalWaitTimeout 相当、commit が進めない場合のみ発火）+ `LocalDestroy(h)`
（Go の checkQuorumLoss/TerminateLocally 相当、raft を経由せず破棄）。
`FixTombstone` 定数で「破棄後の再作成に旧実体宛ての遅延 commit が紛れ込まない」
（Go は sectorID 使い捨てで保証）の有無を切り替え可能にした。CommitMerge は
2026-07-19 の merge/overlap 修正後の Go 実装（victim 破棄確認 + 範囲再検証）に
追随させてある。

**Phase 方式**（各 phase は前 phase の結果を踏まえて設定を変える）:

| Phase | 設定 | 状態数 | 結果 |
|-------|------|--------|------|
| F1: 回復なし | `EnableLocalDestroy=FALSE` | 26,483 生成 / 5,260 distinct、depth 19 | **EventuallyAllActive 違反**（stuck した join が永久 inactive のまま）。TimeoutAbort だけでは不十分と確認 — kvs.go の「離脱を検知しても解消できない」NOTE の形式的対応物 |
| F2/F3 小規模 (N=3) | 回復 2 種 ON、`FixTombstone=FALSE`（permissive）、`MaxMisfire` 0→1 | misfire0: 80,913/14,706 distinct・misfire1: 556,426/85,469 distinct | 両方とも **safety + 全 liveness 成立** |
| N=4 全 property (churn1/stuck1/misfire1) | 全メンバー開始、permissive | 33,856,525 生成 / 4,254,181 distinct、depth 24、8分10秒 | **違反なし** |
| N=4 全 property (churn2/stuck2/misfire1) | 同上、churn/stuck を倍増 | 5,898,269,837 生成 / 540,596,955 distinct、depth 42、19時間42分 | **違反なし** |
| N=4 全 property (churn1/stuck1/misfire2) | churn/stuck を縮小し misfire だけ倍増 | 143,345,853 生成 / 16,612,165 distinct、depth 25、29分30秒 | **違反なし** |

**検証項目への回答**:

1. **回復ありで safety が保たれるか** → 保たれる。F2 以降のすべての phase で
   `TypeOK`/`ValidRange`/`ActiveFlagConsistent` 違反は 0 件。
2. **回復に WF を付けると EventuallyAllActive が復活するか** → 復活する。
   F1 の違反が F2 以降で解消したことで確認済み。
3. **`LocalDestroy` の誤発動（`MaxMisfire>=1`）を許すと safety が破れるか** →
   **破れない**。しかも `FixTombstone=FALSE`（破棄後の再作成に旧実体宛て
   commit が紛れ込むことを許す最も緩い設定）でも成立した。つまり Go の
   sectorID 使い捨て（tombstone）による ABA 対策は、この抽象レベルでは
   safety の必要条件ではなく、あくまで多重の安全網の一つという結論になった。

**未検証のまま残った領域（既知のツール限界）**: N=4 churn2/stuck2/misfire2
（churn・stuck・misfire を同時に最大化した設定）は depth 31・約19億 distinct
まで到達したところで、電源断からの `-recover` 中に TLC 自身の内部構造の
32-bit int オーバーフロー（`NegativeArraySizeException`、2^31 ≈ 21.5億）で
打ち切りとなり、完走できなかった。ヒープ増量（8g→48g）では解決しない、
TLC のこのバージョンのスケール限界。churn2/stuck2/misfire1（表内 3 行目）と
churn1/stuck1/misfire2（表内 5 行目）はそれぞれ独立に違反なしを確認して
いるため、misfire2 の効果自体および churn2/stuck2 の効果自体は個別に
検証済みだが、**両者を同時に最大化した組み合わせの網羅的検証はできて
いない**。回避策として、ring のノード ID に対する回転対称性（巡回群、
最大 N 分の1の状態数削減）を TLC の `SYMMETRY`（`Permutations(Nodes)` は
リングの向きを壊すため不健全 — 巡回群のみを手動列挙すれば健全）に
指定すれば再挑戦の余地はあるが、liveness 検証との健全な組み合わせを
別途検証する必要があり、未着手。エラーの詳細な時系列・再現手順・残置
チェックポイントの場所は [debug.md](debug.md) に記録。

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

<a id="todo-is-stable"></a>
#### is_stable ゲートの緩和（設計 + Go 実装）

- **背景**: `subRoutine` は is_stable（seed の reconcile と routing ビューの
  一致）でないと ManageMember にも operateSectors にも到達しないため、
  churn 中は修復を担うノードが何もできず、カバレッジ漸減の律速になっている。
  観測の系譜: 231 ノード解析（seed の失効タイマに律速される構造）→
  run 4 位相 1（churn 中の修復凍結）→ run 11 (B)（接続不良 node 1 つで隣接の
  activation が skip 2 で凍結、sectorActivate の失敗も観測不能）→
  run 14（黒穴 1 体 → 隣接 joiner が恒久 unstable → 領域全体のチェーン停止、
  という伝播経路の確認）。
- **対策候補**（run 11 (B) 参照）: (a) 不安定時も修復系操作
  （Extend / Terminate frontward）だけは許可する、(b) is_stable の要件から
  到達不能 node を除外する、(c) sectorActivate に失敗理由を返させて
  観測可能にする。周辺 node 側の防御は
  [spec/seed/README.md](../seed/README.md) の TODO とセットで判断する。

<a id="todo-hysteresis"></a>
#### ManageMember のヒステリシス（Go 実装）

routing ビューが N tick 連続で同一の場合のみメンバー変更を発行し、
view flap の追従で ConfChange を浪費しないようにする（run 4 改善候補 3）。
learner-first 導入後は quorum 毀損の主因ではなくなったが、メンバー
ConfChange の振動自体（run 3 クラス C で観測した追加 → 削除 → 追加の往復）は
残っている。

<a id="todo-merge-abort"></a>
#### mergeSector: ターゲット離脱時の abort（Go 実装）

`kvs.go` の `mergeSector` にある既存 TODO コメント。split には proposer 監視
（`proposedSplitRoutine`）があるが、merge には対象セクター離脱時の abort
処理がない。run 11 (A) の mergeBy 解放（merge する側の死亡）とは別の穴
（こちらは merge されるターゲット側の離脱）。timeout + abort（TODO-3）が
入っているため恒久ハングにはならず、優先度は低い。

<a id="todo-thresholds"></a>
#### 強制破棄・タイムアウトしきい値の実測再調整

proposeTimeout=2s / proposalWaitTimeout=15s / forceTerminateDuration=30s /
forcePendingDuration=45s / mergeReleaseDuration=30s は、`== retry proposals`
ログの実測に基づく再調整を想定した暫定値のまま。どこまで詰められるかは
誤発動時の安全性の境界（TODO-1 検証項目 3）に依存するため、TODO-1 が前提。

<a id="todo-merge-overlap"></a>
#### merge/overlap 抗争 — 生存 host の sector を leftover と誤認（設計 + Go + モデル）

routing 視界の不一致により、隣接 host が**生きている node** の active sector を
「host 死亡後の leftover」と誤認すると、merge → 重複 → terminate 自壊 →
再 activate のループに入り **ack 済み書き込みが破棄される**（2026-07-12
Stage B run で観測、run 2026-07-16 で機構を確定 — 経緯の正典は
[design.md](design.md) の「churn 下の課題」）。

**モデル検証 + Go 実装は完了 (2026-07-19)**: `KvsSectorFalseLeftover.tla` が
8 欠陥と修正パッケージ（詳細は「merge/overlap 抗争の解析とモデル検証」の節）
を確定し、モデルの Fix 定数と 1:1 対応で Go 実装済み:

1. **ConfirmVictim + 範囲再検証** (`kvs.go mergeSector`): victim へ
   `TerminateForMerge` を提案 → `WaitTerminated` で破棄を確認 →
   `hasActiveSectorHeadInRange` で拡張範囲を再検証してから CommitMerge。
   どちらかが失敗すれば abort（tail は切り詰め位置のままなので安全）
2. **世代付き scoped terminate** (`sector.go` + proto): `Terminate` proto に
   merge_handler / merge_generation を追加。prepare_merge の apply が
   `mergeGeneration` (複製状態、snapshot にも同梱) を bump し、merge 用
   terminate の apply は (mergeBy, 世代) 一致の CAS。不一致は no-op で
   pending も消化（DOA）
3. **被覆時 activation skip** (`kvs.go activateHostingSector`): 自位置を
   範囲に含む active セクターがある間は skip（ログ「skip 3」）
4. **Import の merge fence** (`sector.go processImportProposal`): mergeBy
   保持中の import は apply 側で決定的に no-op、proposer の `Import()` は
   `importFenceRejected` を観測してエラー復帰（merger は migrate 段で abort）

回帰テスト（いずれも修正前コードで失敗することを確認済み）:
`TestKVS_mergeSector_abortsWhenVictimUnterminated`（不死身 victim への
tail 拡張禁止 = 抗争ループの本体）/
`TestKVS_mergeSector_abortsOnStaleTailRange`（stale propTail）/
`TestKVS_activateHostingSector_skipsWhenCovered`（被覆盲点）/
`TestSector_terminateForMerge_staleTenureIsNoOp`（ABA/tenure）/
`TestSector_import_rejectedWhileMergeLockHeld`（Import fence）。

**run 17 (2026-07-19) で定量確認済み・完了**: 自壊 ping-pong 0（前 run
103 ペア）、CAS 重複は quorum 喪失リセットの revision 再利用 1 組のみ、
新ガード（WaitTerminated abort 15 回・範囲再検証 abort 18 回）は自己解消的に
機能し、nohost は median 5s / max 11s（run 15: median 285s）へ改善。
残った revision 後退 3 エピソードはすべて force terminate（quorum 喪失の
設計上の損失経路）系で、うち末尾数 rev の喪失 2 件は
[別 TODO](#todo-stale-leftover-import) に切り出した。詳細は
「run 17: merge/overlap 修正の定量確認」の節。

<a id="todo-stale-leftover-import"></a>
#### leftover import 元 replica の遅れによる ack 済み末尾喪失（Go 実装）

run 17 の ep2 (04:39:49) / ep3 (04:41:30) で観測。セクターが quorum 喪失で
force terminate された後、leftover import（tail 切り詰め activate に伴う
残存 replica からのデータ回収）の import 元 replica の applied 状態が
commit index より僅かに遅れていると、**ack 済みの末尾書き込み（実測 1〜11
revision）が失われて revision が後退**する。watcher には `watch back` として
観測される。

quorum を成す member が全滅した場合の喪失は Raft の耐久性モデル上不可避だが、
以下の best-effort 改善余地がある:

- import 元を選ぶ際に**最も applied の進んだ生存 replica** を選択する
  （現状の選択基準の確認から）
- 複数の生存 replica から record ごとに最大 revision を採る merge 型回収

頻度は低く（22 分 run で 2 件・数 rev）、全損（ep1 型）ではないため優先度は
中。force terminate 系の損失経路として
[しきい値再調整](#todo-thresholds) とも関連する。

<a id="todo-setrange"></a>
#### `Operator.SetRange` の全周セクター縮小の扱い（要検証、Go 実装）

`Operator.SetRange` は先頭分岐の `s.tail.IsBetween(&s.head, &tail)` が
head==tail（全周セクター）のとき常に真になるため、「全周 → 任意の tail」を
縮小でなく拡張として扱う。この経路では範囲外レコードの削除・lock index /
watch 購読の purge が走らない。単一 node の全周 hosting sector が split
（PreCommitSplit → SetRange）で縮小するケースが該当し、移譲済み範囲の stale
レコードが store/keys に残留 → 後の Extend で同範囲を再取得すると stale 値が
復活する可能性がある。Watch (Stage E) のテスト作成中に発見 (2026-07-16)、
実害は未確認。参照: [api.md](api.md)「Watch」、
`node/internal/kvs/sector/operator/operator.go` の `SetRange`。

<a id="todo-col-stop"></a>
#### Col.Stop() 後のセクター raft goroutine 残留（Go 実装 node/simulator）

停止済み node のセクター raft ループ goroutine が Col.Stop() 後も数十秒
生き残り、force terminate ログを出し続ける（run 11 その他 / run 15 続報で
確認）。実害はシミュレーションの解析ノイズだが、Stop で goroutine の終了を
確認する余地がある。コード確認 2026-07-17: `KVS.Start` の subRoutine ループは
ctx で止まるが、保持中の各 `Sector` を停止する明示的な shutdown 経路は
見当たらない。

<a id="todo-dual-leader"></a>
#### 同一 term 二重リーダー疑いの系譜特定（調査）

run 6 / run 7 で「リーダーの Next がログ範囲外に出た正確な系譜」
（同一 term での二重リーダー疑い = 空ログ再作成による votedFor 忘却の
残存経路の可能性）が未特定のまま。tombstone による raft メンバー ID の
使い捨て化（run 5 対策）後に該当経路が残っているかの確認を含め、
今後の run の観測対象。

<a id="todo-transferer"></a>
#### Transferer: 送信 API の同一 goroutine 再入の構造的解消（見送り中）

「送信 API が経路なしエラーを自ノードへ同期配送し、同一 goroutine で
Receive に再入する」構造自体は run 16 の修正後も残っており、送信 API の
新規呼び出し箇所では「mtx 保持中に送信しない」規約を守る必要がある。
Receive のローカル配送を goroutine に逃がす構造的解消は、順序保証への
影響評価が必要なため見送り（規約 + 回帰テスト
`TestRetry_noRouteErrorDoesNotDeadlock` で担保）。

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