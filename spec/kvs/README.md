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
  非隣接ノード間の重複は解消されない。要追加対応。

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

| 案 | 効果 | 実装コスト | 推奨度 |
|---|------|----------|--------|
| A: proposing をセクター内蔵 | バグ予防 (大) | 中 | ★★★ |
| B: 操作の冪等化 | バグ予防 (中) | 小 | ★★★ |
| C: anyActive のローカル化 | バグ予防 (中) | 小 | ★★★ |
| D: TerminateA 再検討 | 保守性 | 小 (調査のみ) | ★★ |
| E: Split のアトミック化 | 構造改善 | 大 | ★ (将来) |
| F: Merge 2 コミット化 | レース面縮小 | 中 | ★★ |
| G: TerminateB 即時化 | 修復遅延短縮 | 小 | ★★ |

## 今後の TODO

- [x] **routing ビューと sector-store を分離** → `KvsSectorSepView.tla` で実装済
- [x] **Raft 合意の非原子性をモデル化** → `KvsSectorRaft.tla` で実装済
      - ActivateFrontward / Split を Propose → Commit の 2 ステップに分割
      - TerminateB が発火することを確認 (N=3: 8回, N=4: 14回)
- [x] **複数の Join/Leave 同時発生のレース検証** → N=3 MaxChurn=4 まで検証済
      - MaxChurn=2 で Merge 吸収側の proposing 未クリア / CommitActivate の anyActive 未更新を発見・修正
- [x] **Merge も非原子化** → `KvsSectorRaft.tla` で ProposeMerge/CommitMerge に分割
      - MaxChurn=3 で TerminateB→CommitMerge 間のインターリーブバグ 2 件を発見・修正
      - TerminateA が MaxChurn=3 で初めて発火 (20回)

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