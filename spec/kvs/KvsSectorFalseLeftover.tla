------------------------- MODULE KvsSectorFalseLeftover -------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   merge/overlap 抗争 (偽 leftover 誤認 + 不死身 active セクター) と
 *   ack 済み書き込みの修復起因喪失をモデル化
 *
 * 動機 (シミュレーション run 2026-07-16、design.md「churn 下の課題」):
 *   生存しているが routing 視界から消えた node h の active セクターを、
 *   backward n が leftover と誤認して merge する。h のグループが quorum を
 *   失っている (leaderless) と terminate が commit できず、セクターは
 *   「不死身の active セクター」として残る。Go の mergeSector は victim の
 *   terminate 完了を確認せずに CommitMerge で tail を拡張するため、
 *   不死身セクターとの重複が必ず生じ、次 tick の TerminateA が「commit
 *   できる側 = データ保持側」の hosting セクターを破棄する。backward の
 *   sectorActivate が再 activate して 4〜7 秒周期の無限ループになり、
 *   ack 済み書き込みが毎周破棄される (30 分 run で自壊 103 回 / 48 node、
 *   CAS 監査で同一 (key, base) の重複成功 126 組・revision リセット)。
 *
 * KvsSectorLeftover.tla からの差分:
 *   - merge を Go 忠実に非原子化:
 *       ProposeMerge      (lock 取得。stuck した victim には fast path のみ)
 *     → MergeMigrate      (レコード移送 + victim へ terminate 提案 =
 *                          fire-and-forget。Go: Merge() + Terminate())
 *     → CommitMergeExtend (自グループの tail 拡張。Go: CommitMerge + "done"。
 *                          FixConfirmVictim = FALSE なら victim の死を
 *                          確認しない = 現行 Go)
 *     victim の破棄は独立アクション ApplyVictimTerminate (WF) が担い、
 *     stuck した victim では永遠に発火しない。
 *   - stuck[h]: セクター h のグループが commit 不能 (quorum 喪失)。
 *     BecomeStuck で非決定的に発生 (予算 MaxStuck)。stuck 中は
 *     terminate apply / 新規 lock 取得 / 自グループ commit (Extend,
 *     Split, merge の migrate/extend, Write) が不能。回復は
 *     LocalDestroy (Go の checkQuorumLoss 強制破棄、WF) のみ。
 *   - DropFromRView: 生存 member が n の rView から消える (リンク喪失の
 *     写像、予算 MaxViewDrops)。RefreshRView (WF) が最終的に復元するため
 *     transient だが、divergence 窓内で誤認 merge が発火できる。
 *   - データ (追跡アーク 1 本): holder = 位置 Arc の最新 ack 済み
 *     書き込みを保持するセクター head。
 *       Write             : アークを cover する健全な live host が ack
 *       MergeMigrate      : victim → merger に移送
 *       CommitSplit       : 親 → 子に移送 (範囲内のみ)
 *       churn 系破棄       : LostChurn (design.md が許容するクラス)
 *       修復系破棄         : LostRepair (TerminateA/B の自壊・victim 内
 *                           未移送データの terminate apply)
 *     安全性 NoRepairLoss == holder # LostRepair が検証ターゲット。
 *   - FixConfirmVictim (修正候補): CommitMergeExtend が「victim が
 *     ActiveSectors から消えたこと」を要求する。victim が stuck なら
 *     AbortMergeStuck (Go の proposalWaitTimeout の写像、WF) で中断し、
 *     tail は切り詰め位置のまま = 重複が生じない。victim の掃除は
 *     LocalDestroy が担う。
 *
 * 検証フェーズ (定数を切り替えて実行):
 *   Phase FL1 (バグ再現): FixConfirmVictim = FixScopedTerminate = FALSE、
 *     safety のみ
 *     → NoRepairLoss の違反を確認する。最短反例 (深さ 10): チェーン活性化 →
 *       Write → DropFromRView(n, fs) → ProposeMerge → MergeMigrate →
 *       CommitMergeExtend (重複発生) → TerminateA(n) で holder = LostRepair。
 *       stuck は不要 = victim の terminate apply と TerminateA のレースだけで
 *       喪失する。stuck が加わると同じ系列が無限ループ化する (run 実測形)。
 *   Phase FL2 (修正確認): FixConfirmVictim = FixScopedTerminate = TRUE
 *     → NoRepairLoss を含む全 safety + 全 liveness が成立する。
 *     修正は 2 つで 1 組:
 *       修正 1 (ConfirmVictim) だけでは「merger 死亡 → ReleaseMerge で
 *       fence 解除 → 書き込み → 孤児 terminate の遅延 apply」で喪失する
 *       反例が残ることを TLC で確認済み (修正 2 の必要性の根拠)。
 *   Phase FL3 (誤検知 safety): + PermissiveRelease = TRUE, safety のみ
 *     → 無条件 lock 解放が併発しても safety 維持。
 *
 * モデルの抽象度の限界:
 *   - アークは 1 本のみ追跡 (リング対称性により一般性は保たれる)。
 *   - stuck は「グループ全体が commit 不能」の抽象で、レプリカ個体差や
 *     部分的な追随遅延は表現しない。
 *   - Go の migrate → terminate → commitMerge の 3 ステップを
 *     MergeMigrate / CommitMergeExtend の 2 ステップに約めている
 *     (victim 破棄との相対順序はどちらでも表現できるため)。
 *   - PrepareMerge の mergeBy fast path は「mergeLock[fs] = n なら stuck
 *     でも ProposeMerge が enabled」として表現する (commit 不要の再取得)。
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,
    InitialMembers,
    MaxChurn,
    MaxStuck,           \* BecomeStuck (グループ quorum 喪失) の発生予算
    MaxViewDrops,       \* DropFromRView (生存 member の視界喪失) の発生予算
    EnableRelease,      \* TRUE: ReleaseMergeLock (mergeBy 解放) を有効にする
    PermissiveRelease,  \* TRUE: 保持者生存中でも解放できる (誤検知 safety 検証用)
    ClipActivationTail, \* TRUE: skip 1 の代わりに tail を切り詰めて activate する
    FixConfirmVictim,   \* TRUE: victim の破棄を確認してから tail を拡張する (修正 1)
    FixScopedTerminate, \* TRUE: merge の victim terminate を lock の在任期間
                        \*       (tenure) に CAS スコープする (修正 2):
                        \*       apply 時に mergeBy が依頼者のままのときだけ
                        \*       破棄し、lock が解放・奪取済みなら no-op。
                        \*       さらに ReleaseMerge は同一保持者の pending
                        \*       terminate も無効化する — 保持者一致だけの
                        \*       CAS では「release → 再 prepare 後に旧
                        \*       terminate が遅延適用される」ABA で新 tenure の
                        \*       未 migrate データを破棄できてしまう (TLC で
                        \*       確認)。Go 実装は prepare 世代カウンタの一致
                        \*       検査に対応する
    FixNoActivateUnderCover,
                        \* TRUE: 自位置 f を範囲に含む active/leftover
                        \*       セクターが存在する間は activation を skip
                        \*       する (修正 3)。既存ガードは (f, ft) 内の
                        \*       head しか見ないため、背後からカバーする
                        \*       セクターと重複する activation を許し、
                        \*       merge ガードと Terminate の形に合致しない
                        \*       修復不能な恒久重複を作る (TLC で確認)
    FixNoAbsorbWhileLocked
                        \* TRUE: 自セクターの mergeBy が握られている間は
                        \*       吸収役にならない (修正 4)。Go の mergeFenced
                        \*       はクライアント書き込みしか塞がず merge の
                        \*       Import は素通しのため、被 merge 中のセクターが
                        \*       frontward を吸収すると、absorber の export に
                        \*       乗らなかったデータが scoped terminate で
                        \*       セクターごと破棄される (TLC で確認)

N == Cardinality(Nodes)

NextNodeID(a) == (a + 1) % N
RingDist(a, b) == (b - a + N) % N

IsBetween(x, from, to) ==
    IF from = to THEN TRUE
    ELSE IF from < to
         THEN from <= x /\ x < to
         ELSE from <= x \/ x < to

\* 追跡するアーク (リング位置)。対称性により 1 本で一般性を失わない。
Arc == 0

\* holder の特殊値
NoData     == -1  \* まだ書き込みがない
LostRepair == -2  \* 修復アクション (TerminateA/B 等) による喪失 = バグ
LostChurn  == -3  \* churn / quorum 喪失による喪失 = design.md が許容

\* termReqBy の特殊値: 重複修復 (TerminateA/B) による terminate は merge の
\* 文脈を持たない無条件破棄
Unscoped == -4

\* ──────────────────────────────────────────────
\* 変数
\* ──────────────────────────────────────────────
VARIABLES
    state,       \* [Nodes -> {"absent","inactive","active","leftover"}]
    tail,        \* [Nodes -> Nodes \cup {-1}]
    rView,       \* [Nodes -> SUBSET Nodes]
    sActives,    \* [Nodes -> SUBSET Nodes]
    anyActive,
    churn,
    proposing,   \* [Nodes -> {"none","activate","splitPre","splitCommit","merge","mergeCommit"}]
    propTarget,  \* [Nodes -> Nodes \cup {-1}]
    propTail,    \* [Nodes -> Nodes \cup {-1}]
    mergeLock,   \* [Nodes -> Nodes \cup {-1}]  Go の Sector.mergeBy
    stuck,       \* [Nodes -> BOOLEAN] セクター h のグループが commit 不能
    termReqBy,   \* [Nodes -> {-1} \cup Nodes \cup {Unscoped}]
                 \*   -1: terminate 提案なし / n: merger n の merge に伴う提案 /
                 \*   Unscoped: TerminateA/B (重複修復) による無条件提案
    stuckCount,  \* 0..MaxStuck
    viewDrops,   \* 0..MaxViewDrops
    holder       \* Nodes \cup {NoData, LostRepair, LostChurn}

vars == <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget,
          propTail, mergeLock, stuck, termReqBy, stuckCount, viewDrops, holder>>

\* ──────────────────────────────────────────────
\* 派生集合
\* ──────────────────────────────────────────────
Members == { n \in Nodes : state[n] \in {"inactive", "active"} }
Actives == { n \in Nodes : state[n] = "active" }
ActiveSectors == { n \in Nodes : state[n] \in {"active", "leftover"} }

NearestIn(p, set) ==
    LET others == set \ {p}
    IN IF others = {} THEN p
       ELSE CHOOSE x \in others :
              \A y \in others : RingDist(p, x) <= RingDist(p, y)

NextInView(n) == NearestIn(n, rView[n])
FrontwardSector(n) == NearestIn(n, sActives[n] \ {n})

DestroyedState(h) == IF state[h] = "leftover" THEN "absent" ELSE "inactive"

\* セクター h が追跡アークを cover しているか
Covers(h) == h \in ActiveSectors /\ tail[h] # -1 /\ IsBetween(Arc, h, tail[h])

\* f を背後から範囲に含む active/leftover セクターの集合 (修正 3 のガード)
CoveredBy(f) == { h \in ActiveSectors : h # f /\ IsBetween(f, h, tail[h]) }

\* セクター h の破棄に伴う holder の遷移
HolderChurnLoss(h)  == IF holder = h THEN LostChurn  ELSE holder
HolderRepairLoss(h) == IF holder = h THEN LostRepair ELSE holder

\* ──────────────────────────────────────────────
\* 型不変条件
\* ──────────────────────────────────────────────
TypeOK ==
    /\ state \in [Nodes -> {"absent","inactive","active","leftover"}]
    /\ tail \in [Nodes -> (Nodes \cup {-1})]
    /\ rView \in [Nodes -> SUBSET Nodes]
    /\ sActives \in [Nodes -> SUBSET Nodes]
    /\ anyActive \in BOOLEAN
    /\ churn \in 0..MaxChurn
    /\ proposing \in [Nodes -> {"none","activate","splitPre","splitCommit","merge","mergeCommit"}]
    /\ propTarget \in [Nodes -> (Nodes \cup {-1})]
    /\ propTail \in [Nodes -> (Nodes \cup {-1})]
    /\ mergeLock \in [Nodes -> (Nodes \cup {-1})]
    /\ stuck \in [Nodes -> BOOLEAN]
    /\ termReqBy \in [Nodes -> ({-1, Unscoped} \cup Nodes)]
    /\ stuckCount \in 0..MaxStuck
    /\ viewDrops \in 0..MaxViewDrops
    /\ holder \in (Nodes \cup {NoData, LostRepair, LostChurn})

\* ──────────────────────────────────────────────
\* 初期状態
\* ──────────────────────────────────────────────
Init ==
    /\ state = [n \in Nodes |-> IF n \in InitialMembers THEN "inactive" ELSE "absent"]
    /\ tail = [n \in Nodes |-> -1]
    /\ rView = [n \in Nodes |-> InitialMembers]
    /\ sActives = [n \in Nodes |-> {}]
    /\ anyActive = FALSE
    /\ churn = 0
    /\ proposing = [n \in Nodes |-> "none"]
    /\ propTarget = [n \in Nodes |-> -1]
    /\ propTail = [n \in Nodes |-> -1]
    /\ mergeLock = [n \in Nodes |-> -1]
    /\ stuck = [n \in Nodes |-> FALSE]
    /\ termReqBy = [n \in Nodes |-> -1]
    /\ stuckCount = 0
    /\ viewDrops = 0
    /\ holder = NoData

\* ──────────────────────────────────────────────
\* 環境イベント
\* ──────────────────────────────────────────────
RefreshRView(n) ==
    /\ n \in Members
    /\ rView[n] # Members
    /\ rView' = [rView EXCEPT ![n] = Members]
    /\ UNCHANGED <<state, tail, sActives, anyActive, churn, proposing, propTarget,
                   propTail, mergeLock, stuck, termReqBy, stuckCount, viewDrops, holder>>

\* 生存 member が n の視界から消える (リンク喪失・視界乖離の写像)。
\* RefreshRView (WF) が復元するため transient だが、乖離の窓内で
\* 誤認 merge / clip activation が発火できる。
DropFromRView(n) ==
    /\ viewDrops < MaxViewDrops
    /\ n \in Members
    /\ \E h \in (rView[n] \cap Members) :
         /\ h # n
         /\ rView' = [rView EXCEPT ![n] = @ \ {h}]
    /\ viewDrops' = viewDrops + 1
    /\ UNCHANGED <<state, tail, sActives, anyActive, churn, proposing, propTarget,
                   propTail, mergeLock, stuck, termReqBy, stuckCount, holder>>

LearnSActive(n) ==
    /\ n \in Members
    /\ \E h \in ActiveSectors :
         /\ h \notin sActives[n]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget,
                   propTail, mergeLock, stuck, termReqBy, stuckCount, viewDrops, holder>>

ForgetSActive(n) ==
    /\ n \in Members
    /\ \E h \in sActives[n] :
         /\ h \notin ActiveSectors
         /\ sActives' = [sActives EXCEPT ![n] = @ \ {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget,
                   propTail, mergeLock, stuck, termReqBy, stuckCount, viewDrops, holder>>

Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ rView' = [rView EXCEPT ![n] = Members \cup {n}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1
    \* 旧 incarnation の stale lock 解放 (KvsSectorLeftover と同じ理由)。
    \* 同一保持者の pending terminate も tenure ごと無効化する
    /\ mergeLock' = [x \in Nodes |-> IF mergeLock[x] = n THEN -1 ELSE mergeLock[x]]
    /\ termReqBy' = [x \in Nodes |-> IF termReqBy[x] = n THEN -1 ELSE termReqBy[x]]
    /\ UNCHANGED <<proposing, propTarget, propTail, stuck, stuckCount,
                   viewDrops, holder>>

\* Leave: node がセクターごと消える。active な holder のデータ喪失は
\* churn 起因 (design.md が許容するクラス)。
Leave(n) ==
    /\ churn < MaxChurn
    /\ state[n] \in {"inactive", "active"}
    /\ Cardinality(Members) > 1
    /\ proposing[n] \in {"none", "merge", "mergeCommit"}
    /\ state' = [state EXCEPT ![n] = "absent"]
    /\ tail' = [tail EXCEPT ![n] = -1]
    /\ rView' = [rView EXCEPT ![n] = {}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ anyActive' = IF state[n] = "active" /\ Cardinality(Actives) = 1
                    THEN FALSE ELSE anyActive
    /\ churn' = churn + 1
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ mergeLock' = [mergeLock EXCEPT ![n] = -1]
    /\ stuck' = [stuck EXCEPT ![n] = FALSE]
    /\ termReqBy' = [termReqBy EXCEPT ![n] = -1]
    /\ holder' = IF state[n] = "active" THEN HolderChurnLoss(n) ELSE holder
    /\ UNCHANGED <<stuckCount, viewDrops>>

\* LeaveLeftover: active ノードがセクターを残して離脱 (KvsSectorLeftover と
\* 同じ)。レプリカ群はデータもグループ状態 (stuck/termReq/mergeLock) も保持。
LeaveLeftover(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "active"
    /\ Cardinality(Members \ {n}) >= 2
    /\ \E m \in Members \ {n} : state[m] = "active"
    /\ proposing[n] \in {"none", "merge", "mergeCommit"}
    /\ state' = [state EXCEPT ![n] = "leftover"]
    /\ rView' = [rView EXCEPT ![n] = {}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ churn' = churn + 1
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<tail, anyActive, mergeLock, stuck, termReqBy, stuckCount,
                   viewDrops, holder>>

LeftoverQuorumLoss(h) ==
    /\ state[h] = "leftover"
    /\ Cardinality(Members) <= 1
    /\ state' = [state EXCEPT ![h] = "absent"]
    /\ tail' = [tail EXCEPT ![h] = -1]
    /\ mergeLock' = [mergeLock EXCEPT ![h] = -1]
    /\ stuck' = [stuck EXCEPT ![h] = FALSE]
    /\ termReqBy' = [termReqBy EXCEPT ![h] = -1]
    /\ holder' = HolderChurnLoss(h)
    /\ UNCHANGED <<rView, sActives, anyActive, churn, proposing, propTarget,
                   propTail, stuckCount, viewDrops>>

\* ──────────────────────────────────────────────
\* stuck: グループの quorum 喪失と回復
\* ──────────────────────────────────────────────
\* BecomeStuck: セクター h のグループが commit 不能になる (メンバーの死亡・
\* 到達不能で quorum を喪失。レプリカ自体は残る)。
BecomeStuck(h) ==
    /\ stuckCount < MaxStuck
    /\ h \in ActiveSectors
    /\ ~stuck[h]
    /\ stuck' = [stuck EXCEPT ![h] = TRUE]
    /\ stuckCount' = stuckCount + 1
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing,
                   propTarget, propTail, mergeLock, termReqBy, viewDrops, holder>>

\* LocalDestroy: Go の checkQuorumLoss (leaderless 30s) による強制破棄。
\* raft を経由しないため stuck でも発火できる唯一の破棄経路。
\* データ喪失は quorum 喪失起因 = churn クラス (許容)。
LocalDestroy(h) ==
    /\ stuck[h]
    /\ h \in ActiveSectors
    /\ state' = [state EXCEPT ![h] = DestroyedState(h)]
    /\ tail' = [tail EXCEPT ![h] = -1]
    /\ sActives' = [sActives EXCEPT ![h] = {}]
    /\ mergeLock' = [mergeLock EXCEPT ![h] = -1]
    /\ stuck' = [stuck EXCEPT ![h] = FALSE]
    /\ termReqBy' = [termReqBy EXCEPT ![h] = -1]
    /\ proposing' = [proposing EXCEPT ![h] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![h] = -1]
    /\ propTail' = [propTail EXCEPT ![h] = -1]
    /\ anyActive' = ((Actives \ {h}) # {})
    /\ holder' = HolderChurnLoss(h)
    /\ UNCHANGED <<rView, churn, stuckCount, viewDrops>>

\* ApplyVictimTerminate: 提案済み terminate の apply。健全なグループでは
\* 速やかに完了する (WF)。stuck なグループでは永遠に発火しない —
\* 「不死身の active セクター」。victim が最新データを未移送のまま保持して
\* いた場合 (merger 死亡後の孤児 terminate など) の喪失は修復起因 = バグクラス。
\* FixScopedTerminate (修正 2): merge 由来の提案 (termReqBy = merger) は
\* apply 時に mergeBy がその merger のままのときだけ破棄する CAS とし、
\* lock が解放・奪取済みなら no-op で消える (ReleaseMerge と同じ規約)。
\* 修正なし (現行 Go の裸の Terminate()) では文脈に関係なく必ず破棄する。
ApplyVictimTerminate(h) ==
    /\ termReqBy[h] # -1
    /\ ~stuck[h]
    /\ h \in ActiveSectors
    /\ IF \/ ~FixScopedTerminate
          \/ termReqBy[h] = Unscoped
          \/ mergeLock[h] = termReqBy[h]
       THEN \* 破棄が有効: sector を destroy する
            /\ state' = [state EXCEPT ![h] = DestroyedState(h)]
            /\ tail' = [tail EXCEPT ![h] = -1]
            /\ sActives' = [sActives EXCEPT ![h] = {}]
            /\ mergeLock' = [mergeLock EXCEPT ![h] = -1]
            /\ proposing' = [proposing EXCEPT ![h] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![h] = -1]
            /\ propTail' = [propTail EXCEPT ![h] = -1]
            /\ anyActive' = ((Actives \ {h}) # {})
            /\ holder' = HolderRepairLoss(h)
       ELSE \* 孤児 terminate: merge の文脈が消えているため no-op で消化
            UNCHANGED <<state, tail, sActives, mergeLock, proposing,
                        propTarget, propTail, anyActive, holder>>
    /\ termReqBy' = [termReqBy EXCEPT ![h] = -1]
    /\ UNCHANGED <<rView, churn, stuck, stuckCount, viewDrops>>

\* ──────────────────────────────────────────────
\* Write: 追跡アークへの ack 済み書き込み。アークを cover する健全な
\* live host だけが ack できる (stuck グループは commit 不能、leftover は
\* host 不在で受理できない)。mergeBy が握られている間は Go の merge write
\* fence (operator.mergeFenced、prepare_merge apply で設置・ReleaseMerge で
\* 解除) が書き込みを拒否する — これがないと「migrate 後・terminate apply
\* 前の書き込みが victim ごと破棄される」喪失窓が生じる (フェンスを外した
\* モデルで TLC が実際に反例を出すことを確認済み)。
\* ──────────────────────────────────────────────
Write ==
    /\ \E h \in Actives :
         /\ ~stuck[h]
         /\ mergeLock[h] = -1
         /\ Covers(h)
         /\ holder # h
         /\ holder' = h
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing,
                   propTarget, propTail, mergeLock, stuck, termReqBy, stuckCount,
                   viewDrops>>

\* ──────────────────────────────────────────────
\* ActivateFirst（原子）
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    LET t == NextInView(n)
        blockers == { h \in ActiveSectors : h # n /\ IsBetween(h, n, t) } IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ proposing[n] = "none"
    /\ blockers = {} \/ ClipActivationTail
    /\ ~FixNoActivateUnderCover \/ CoveredBy(n) = {}
    /\ LET t2 == IF blockers = {} THEN t
                 ELSE CHOOSE h \in blockers :
                        \A h2 \in blockers : RingDist(n, h) <= RingDist(n, h2) IN
       /\ state' = [state EXCEPT ![n] = "active"]
       /\ tail' = [tail EXCEPT ![n] = t2]
    /\ sActives' = [sActives EXCEPT ![n] = @ \cup {n}]
    /\ anyActive' = TRUE
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail, mergeLock,
                   stuck, termReqBy, stuckCount, viewDrops, holder>>

\* ──────────────────────────────────────────────
\* ActivateFrontward: 2 ステップ（KvsSectorLeftover と同じ。新規セクターの
\* グループは fresh なので stuck の考慮は不要）
\* ──────────────────────────────────────────────
ProposeActivate(n) ==
    LET f == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ f # n
    /\ f \in Members
    /\ state[f] = "inactive"
    /\ NextInView(n) = f
    /\ proposing' = [proposing EXCEPT ![n] = "activate"]
    /\ propTarget' = [propTarget EXCEPT ![n] = f]
    /\ propTail' = [propTail EXCEPT ![n] = NextInView(f)]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, mergeLock,
                   stuck, termReqBy, stuckCount, viewDrops, holder>>

CommitActivate(n) ==
    LET f == propTarget[n]
        ft == propTail[n]
        blockers == { h \in ActiveSectors : h # f /\ IsBetween(h, f, ft) } IN
    /\ proposing[n] = "activate"
    /\ f \in Members
    /\ IF /\ state[f] = "inactive"
          /\ blockers = {} \/ ClipActivationTail
          /\ ~FixNoActivateUnderCover \/ CoveredBy(f) = {}
       THEN LET ft2 == IF blockers = {} THEN ft
                       ELSE CHOOSE h \in blockers :
                              \A h2 \in blockers : RingDist(f, h) <= RingDist(f, h2) IN
            /\ state' = [state EXCEPT ![f] = "active"]
            /\ tail' = [tail EXCEPT ![f] = ft2]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {f}, ![f] = @ \cup {f}]
            /\ anyActive' = TRUE
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn, mergeLock, stuck, termReqBy, stuckCount,
                   viewDrops, holder>>

\* ──────────────────────────────────────────────
\* AbortProposal（merge は lock 取得後・migrate 前のみ対象。migrate 後
\* (mergeCommit) は CommitMergeExtend / AbortMergeStuck が解決する）
\* ──────────────────────────────────────────────
AbortProposal(n) ==
    /\ proposing[n] # "none"
    /\ LET f == propTarget[n] IN
       \/ (proposing[n] = "activate" /\ (f \notin Members \/ state[f] # "inactive"))
       \/ (proposing[n] = "splitPre" /\ (f \notin Members \/ state[f] # "inactive"))
       \/ (proposing[n] = "merge" /\ f \notin ActiveSectors)
    /\ IF proposing[n] = "splitPre"
       THEN tail' = [tail EXCEPT ![n] = propTail[n]]
       ELSE UNCHANGED tail
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock, stuck,
                   termReqBy, stuckCount, viewDrops, holder>>

\* ──────────────────────────────────────────────
\* Split: 2 ステップ（自グループ commit を要するため ~stuck[n]）
\* ──────────────────────────────────────────────
ProposeSplit(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ mergeLock[n] = -1
    /\ tail[n] \in Members
    /\ \E m \in Members :
         /\ m # n /\ m # tail[n]
         /\ state[m] = "inactive"
         /\ IsBetween(m, n, tail[n])
         /\ \A m2 \in Members :
              ( m2 # n /\ m2 # tail[n] /\ state[m2] = "inactive"
                /\ IsBetween(m2, n, tail[n]) )
              => RingDist(n, m) <= RingDist(n, m2)
         /\ tail' = [tail EXCEPT ![n] = m]
         /\ proposing' = [proposing EXCEPT ![n] = "splitPre"]
         /\ propTarget' = [propTarget EXCEPT ![n] = m]
         /\ propTail' = [propTail EXCEPT ![n] = tail[n]]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock, stuck,
                   termReqBy, stuckCount, viewDrops, holder>>

CommitSplit(n) ==
    LET m == propTarget[n]
        ft == propTail[n] IN
    /\ proposing[n] = "splitPre"
    /\ m \in Members
    /\ IF state[m] = "inactive"
       THEN /\ state' = [state EXCEPT ![m] = "active"]
            /\ tail' = [tail EXCEPT ![m] = ft]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {m}, ![m] = @ \cup {m}]
            /\ anyActive' = TRUE
            \* 移譲範囲 [m, ft) のレコードは子へ migrate される
            /\ holder' = IF holder = n /\ IsBetween(Arc, m, ft) THEN m ELSE holder
       ELSE UNCHANGED <<state, tail, sActives, anyActive, holder>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn, mergeLock, stuck, termReqBy, stuckCount, viewDrops>>

\* ──────────────────────────────────────────────
\* Extend（原子。自グループ commit を要するため ~stuck[n]）
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n]
        newTail == NearestIn(n, rView[n] \cup {n}) IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ f # n
    /\ f \notin rView[n]
    /\ \A other \in ActiveSectors : other # n => ~IsBetween(other, n, newTail)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, proposing,
                   propTarget, propTail, mergeLock, stuck, termReqBy, stuckCount,
                   viewDrops, holder>>

\* ──────────────────────────────────────────────
\* Merge: 3 ステップ (Go 忠実化)
\*   ProposeMerge      : mergeBy (lock) の取得。victim が stuck の場合、
\*                       新規取得はできないが、既に自分が保持していれば
\*                       raft を経由しない fast path で再突入できる
\*                       (Go: PrepareMerge の mergeBy == 自分 チェック)。
\*   MergeMigrate      : レコード移送 (自グループへの import commit) +
\*                       victim へ terminate 提案 (fire-and-forget)。
\*   CommitMergeExtend : 自グループの tail 拡張。FixConfirmVictim = FALSE
\*                       (現行 Go) では victim の生死を確認しない —
\*                       不死身の victim と必ず重複する。
\* ──────────────────────────────────────────────
ProposeMerge(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ ~FixNoAbsorbWhileLocked \/ mergeLock[n] = -1
    /\ sActives[n] \ {n} # {}
    /\ fs # n
    /\ fs \in ActiveSectors
    /\ IF stuck[fs] THEN mergeLock[fs] = n ELSE mergeLock[fs] \in {-1, n}
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, v)
    /\ ~IsBetween(fs, n, t)
    /\ \A other \in ActiveSectors :
         other # n /\ other # fs => ~IsBetween(other, n, tail[fs])
    /\ proposing' = [proposing EXCEPT ![n] = "merge"]
    /\ propTarget' = [propTarget EXCEPT ![n] = fs]
    /\ propTail' = [propTail EXCEPT ![n] = tail[fs]]
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = n]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, stuck,
                   termReqBy, stuckCount, viewDrops, holder>>

MergeMigrate(n) ==
    LET fs == propTarget[n] IN
    /\ proposing[n] = "merge"
    /\ ~stuck[n]
    /\ ~FixNoAbsorbWhileLocked \/ mergeLock[n] = -1
    /\ fs \in ActiveSectors
    /\ proposing' = [proposing EXCEPT ![n] = "mergeCommit"]
    \* terminate 予約は prepare で観測した世代を携行する (修正 2 の最終形)。
    \* migrate 前に lock が解放・奪取されていた場合、その terminate は
    \* 世代不一致で dead-on-arrival になる = ここでは予約自体を成立させない。
    \* 修正なしの場合は裸の Terminate() なので無条件に成立する。
    /\ termReqBy' = [termReqBy EXCEPT
                       ![fs] = IF ~FixScopedTerminate \/ mergeLock[fs] = n
                               THEN n ELSE @]
    /\ holder' = IF holder = fs THEN n ELSE holder
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, propTarget,
                   propTail, mergeLock, stuck, stuckCount, viewDrops>>

CommitMergeExtend(n) ==
    LET fs == propTarget[n]
        newTail == propTail[n]
        rangeClear == \A other \in ActiveSectors :
                        other # n => ~IsBetween(other, n, newTail) IN
    /\ proposing[n] = "mergeCommit"
    /\ ~stuck[n]
    \* 修正 (FixConfirmVictim): (a) victim の破棄が確認できるまで tail を
    \* 拡張しない。victim が健在のまま拡張すると重複が必ず生じ、次の
    \* TerminateA が自セクターを破棄する (現行 Go の挙動)。
    \* (b) propTail は prepare 時に捕獲した victim の tail であり、release
    \* 後に victim が split → 死亡すると stale になる。拡張前に Extend と
    \* 同じ範囲再検証を行い、生きたセクターを飲み込まない (TLC で確認)。
    /\ FixConfirmVictim => (fs \notin ActiveSectors /\ rangeClear)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock, stuck,
                   termReqBy, stuckCount, viewDrops, holder>>

\* AbortMergeTimeout: 修正モードで merge を完遂できない場合の中断
\* (Go: proposalWaitTimeout + CommitMerge 前の範囲再検証失敗の写像)。
\* 対象は (a) victim が stuck、(b) terminate 予約が世代不一致で
\* dead-on-arrival、(c) victim は消えたが拡張範囲に別の生きたセクターが
\* 入り込んだ (stale propTail) 場合。tail は切り詰め位置のままなので
\* 重複は生じない。掃除は LocalDestroy / 次の tenure の merge が担う。
AbortMergeTimeout(n) ==
    LET fs == propTarget[n]
        rangeClear == \A other \in ActiveSectors :
                        other # n => ~IsBetween(other, n, propTail[n]) IN
    /\ FixConfirmVictim
    /\ proposing[n] = "mergeCommit"
    /\ \/ fs \in ActiveSectors /\ (stuck[fs] \/ termReqBy[fs] # n)
       \/ fs \notin ActiveSectors /\ ~rangeClear
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, mergeLock,
                   stuck, termReqBy, stuckCount, viewDrops, holder>>

\* AbortMergeLocked: 修正 4 の中断経路。自分の merge 進行中に自セクターの
\* mergeBy を他ノードに握られた場合 (被 merge 側に回った場合) は吸収を
\* 中断する。Go では Import の apply 側ゲート (mergeBy 保持中は no-op) の
\* エラーで merger 側が abort する形に対応する。victim fs への lock は
\* 残るが、ReleaseMergeLock (競合タイマー) が解放する。
AbortMergeLocked(n) ==
    /\ FixNoAbsorbWhileLocked
    /\ proposing[n] \in {"merge", "mergeCommit"}
    /\ mergeLock[n] # -1
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, mergeLock,
                   stuck, termReqBy, stuckCount, viewDrops, holder>>

\* ──────────────────────────────────────────────
\* Terminate（Go 忠実化: 自セクターの破棄は自グループの commit で完了するが、
\* 相手セクターへの terminate は提案 (termReq) のみ = fire-and-forget。
\* 相手が stuck だと相手は死なず、自分 (データ保持側) だけが死ぬ）
\* ──────────────────────────────────────────────
TerminateA(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ sActives[n] \ {n} # {}
    /\ fs # n
    /\ fs \in ActiveSectors
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, t)
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = -1]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ mergeLock' = [mergeLock EXCEPT ![n] = -1]
    /\ stuck' = [stuck EXCEPT ![n] = FALSE]
    /\ termReqBy' = [termReqBy EXCEPT ![n] = -1, ![fs] = Unscoped]
    /\ anyActive' = ((Actives \ {n}) # {})
    /\ holder' = HolderRepairLoss(n)
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail, stuckCount,
                   viewDrops>>

TerminateB(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ tail[n] \in Nodes
    /\ tail[n] \in Members
    /\ state[tail[n]] = "active"
    /\ \E other \in ActiveSectors :
         /\ other # n
         /\ other # tail[n]
         /\ IsBetween(other, n, tail[n])
         /\ state' = [state EXCEPT ![n] = "inactive"]
         /\ tail' = [tail EXCEPT ![n] = -1]
         /\ sActives' = [sActives EXCEPT ![n] = {}]
         /\ proposing' = [proposing EXCEPT ![n] = "none"]
         /\ propTarget' = [propTarget EXCEPT ![n] = -1]
         /\ propTail' = [propTail EXCEPT ![n] = -1]
         /\ mergeLock' = [mergeLock EXCEPT ![n] = -1]
         /\ stuck' = [stuck EXCEPT ![n] = FALSE]
         /\ termReqBy' = [termReqBy EXCEPT ![n] = -1, ![other] = Unscoped]
         /\ anyActive' = ((Actives \ {n}) # {})
         /\ holder' = HolderRepairLoss(n)
    /\ UNCHANGED <<rView, churn, stuckCount, viewDrops>>

\* ──────────────────────────────────────────────
\* ReleaseMergeLock（解放も victim グループの commit を要するため ~stuck）
\* Go の実トリガーは「同一保持者の mergeBy に mergeReleaseDuration (30s)
\* 連続で拒否される」ことであり、保持者の生死は見ない。健全な merge は
\* 1 秒未満で完了するため、保持者が member として生存していても当該 merge を
\* 進めていなければ stale lock として解放してよい — その写像として
\* 「保持者不在 または 保持者がこの fs への merge を進行中でない」を
\* enabling にする (保持者の merge 進行中は健全な競合なので解放しない)。
\* ──────────────────────────────────────────────
ReleaseMergeLock(fs) ==
    LET holderN == mergeLock[fs] IN
    /\ EnableRelease
    /\ state[fs] # "absent"
    /\ ~stuck[fs]
    /\ holderN # -1
    /\ \/ holderN \notin Members
       \/ ~( proposing[holderN] \in {"merge", "mergeCommit"}
              /\ propTarget[holderN] = fs )
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
    \* 同一保持者の pending terminate も tenure ごと無効化する (下記 NOTE)
    /\ termReqBy' = [termReqBy EXCEPT ![fs] = IF @ = holderN THEN -1 ELSE @]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing,
                   propTarget, propTail, stuck, stuckCount, viewDrops, holder>>

ReleaseMergeLockAny(fs) ==
    /\ PermissiveRelease
    /\ state[fs] # "absent"
    /\ ~stuck[fs]
    /\ mergeLock[fs] # -1
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
    /\ termReqBy' = [termReqBy EXCEPT
                        ![fs] = IF @ = mergeLock[fs] THEN -1 ELSE @]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing,
                   propTarget, propTail, stuck, stuckCount, viewDrops, holder>>

\* ──────────────────────────────────────────────
\* 遷移関係
\* ──────────────────────────────────────────────
Next ==
    \/ Write
    \/ \E n \in Nodes :
        \/ RefreshRView(n)
        \/ DropFromRView(n)
        \/ LearnSActive(n)
        \/ ForgetSActive(n)
        \/ Join(n)
        \/ Leave(n)
        \/ LeaveLeftover(n)
        \/ LeftoverQuorumLoss(n)
        \/ BecomeStuck(n)
        \/ LocalDestroy(n)
        \/ ApplyVictimTerminate(n)
        \/ ActivateFirst(n)
        \/ ProposeActivate(n)
        \/ CommitActivate(n)
        \/ ProposeSplit(n)
        \/ CommitSplit(n)
        \/ AbortProposal(n)
        \/ Extend(n)
        \/ ProposeMerge(n)
        \/ MergeMigrate(n)
        \/ CommitMergeExtend(n)
        \/ AbortMergeTimeout(n)
        \/ AbortMergeLocked(n)
        \/ TerminateA(n)
        \/ TerminateB(n)
        \/ ReleaseMergeLock(n)
        \/ ReleaseMergeLockAny(n)

Fairness ==
    \A n \in Nodes :
        /\ WF_vars(RefreshRView(n))
        /\ WF_vars(LearnSActive(n))
        /\ WF_vars(ForgetSActive(n))
        /\ WF_vars(LeftoverQuorumLoss(n))
        /\ WF_vars(LocalDestroy(n))
        /\ WF_vars(ApplyVictimTerminate(n))
        /\ WF_vars(ActivateFirst(n))
        /\ WF_vars(ProposeActivate(n))
        /\ WF_vars(CommitActivate(n))
        /\ WF_vars(ProposeSplit(n))
        /\ WF_vars(CommitSplit(n))
        /\ WF_vars(AbortProposal(n))
        /\ WF_vars(Extend(n))
        /\ WF_vars(ProposeMerge(n))
        /\ WF_vars(MergeMigrate(n))
        /\ WF_vars(CommitMergeExtend(n))
        /\ WF_vars(AbortMergeTimeout(n))
        /\ WF_vars(AbortMergeLocked(n))
        /\ WF_vars(TerminateA(n))
        /\ WF_vars(TerminateB(n))
        /\ WF_vars(ReleaseMergeLock(n))

Spec == Init /\ [][Next]_vars /\ Fairness

\* ──────────────────────────────────────────────
\* 安全性
\* ──────────────────────────────────────────────
\* 検証ターゲット: 修復アクションが ack 済み書き込みを破棄しない。
\* churn 起因の喪失 (LostChurn) は design.md が許容するため区別する。
NoRepairLoss == holder # LostRepair

ValidRange ==
    \A n \in Nodes :
        n \in ActiveSectors =>
            /\ tail[n] \in Nodes
            /\ ( tail[n] = n \/ IsBetween(tail[n], NextNodeID(n), n) )

ActiveFlagConsistent == anyActive = (Actives # {})

LockOnExistingSector ==
    \A x \in Nodes : mergeLock[x] # -1 => state[x] # "absent"

\* 整合性 (モデル自己検査): データ保持者と stuck/termReqBy は実在する
\* セクターにのみ付く
HolderOnActiveSector == holder \in Nodes => holder \in ActiveSectors
FlagsOnActiveSector ==
    \A h \in Nodes : (stuck[h] \/ termReqBy[h] # -1) => h \in ActiveSectors

\* NoOverlap は merge の非原子化により一時的に破れる (それが本モデルの主題)。
\* INVARIANT ではなく EventuallyNoOverlap を PROPERTY で確認する。
NoOverlap ==
    \A a, b \in Nodes :
        ( a # b /\ a \in ActiveSectors /\ b \in ActiveSectors )
        => ~( IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]) )

\* ──────────────────────────────────────────────
\* 活性
\* ──────────────────────────────────────────────
EventuallyAllActive ==
    <>[]( \A n \in Members : state[n] = "active" )

EventuallyFullCoverage ==
    <>[]( \A n \in Nodes :
            state[n] = "active" =>
                ( tail[n] = n \/ tail[n] \in ActiveSectors ) )

EventuallyNoOverlap ==
    <>[]( \A a, b \in Nodes :
            ( a # b /\ a \in ActiveSectors /\ b \in ActiveSectors )
            => ~( IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]) ) )

EventuallyNoLeftover ==
    <>[]( \A h \in Nodes : state[h] # "leftover" )

\* stuck したグループは最終的に LocalDestroy で掃除される
EventuallyNoStuck ==
    <>[]( \A h \in Nodes : ~stuck[h] )

================================================================================
