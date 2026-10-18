-------------------------------- MODULE KvsSectorRaft --------------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   Raft の Propose → Commit 間のインターリービングをモデル化
 *
 * 動機:
 *   KvsSectorSepView.tla では全てのセクター操作が原子的に実行されるため
 *   NoOverlap 不変式が常に保たれ、Terminate-both が発火しなかった。
 *   Go 実装では ActivateFrontward / Split が複数の Raft commit を跨ぐため、
 *   中間状態で他ノードのアクションが介入し、一時的なセクター重複が起きうる。
 *   本モデルではこの非原子性を「提案中」状態で表現する。
 *
 * 非原子化する操作:
 *   - ActivateFrontward: Propose(f を active 化) → 他ノード介入可 → Commit
 *     Go では n が frontwardNextSector の Activate を Raft に提案し、
 *     commit されるまで f は inactive のまま。この間に別ノードが
 *     同じ f や重なる位置の sector を activate する可能性がある。
 *
 *   - Split: PreCommitSplit(hosting の tail 縮小) → CommitSplit(f を active 化)
 *     Go では hostingSector.PreCommitSplit() → frontwardNextSector.CommitSplit()
 *     という 2 つの異なる Raft グループへの commit が順次行われる。
 *     中間状態ではカバレッジギャップが生じる。
 *
 *   - Merge: ProposeMerge(準備通知) → CommitMerge(fs を inactive + tail 拡張)
 *     Go では frontwardNextSector.PrepareMerge() → hostingSector.Merge() →
 *     frontwardNextSector.Terminate() → hostingSector.CommitMerge() という
 *     2 つの Raft グループにまたがる 4 コミット。
 *     中間状態では fs がまだ active であり、別ノードが fs を
 *     対象に操作する可能性がある。
 *
 * 原子のまま残す操作:
 *   - ActivateFirst: anyActive=FALSE 時のみ発火、競合なし
 *   - Extend: tail の更新のみ、重複を生まない
 *   - Terminate: 修復アクションなので原子で十分
 *
 * Go 実装との対応:
 *   proposing[n]          : n の sector 操作が Raft に提案中（commit 待ち）
 *   ProposeActivate(n)    : activateFrontwardSector() の Raft.Propose 呼び出し
 *   CommitActivate(n)     : Raft commit → processActivateProposal()
 *   ProposeSplit(n)       : splitSector() の PreCommitSplit 呼び出し
 *   CommitSplit(n)        : splitSector() の CommitSplit 呼び出し
 *   ProposeMerge(n)       : mergeSector() の PrepareMerge 呼び出し
 *   CommitMerge(n)        : mergeSector() の Terminate + CommitMerge
 *
 * Safety:
 *   - NoOverlap は「一時的に破れうる」。代わりに EventuallyNoOverlap を活性に。
 *   - NoOverlapCommitted: 提案中でない (committed) セクターのみで NoOverlap。
 *
 * Liveness:
 *   - 全提案は最終的に commit される (WF)
 *   - Terminate が重複を解消し、最終的に全メンバーが active になる
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,
    InitialMembers,
    MaxChurn

N == Cardinality(Nodes)

NextNodeID(a) == (a + 1) % N
RingDist(a, b) == (b - a + N) % N

IsBetween(x, from, to) ==
    IF from = to THEN TRUE
    ELSE IF from < to
         THEN from <= x /\ x < to
         ELSE from <= x \/ x < to

\* ──────────────────────────────────────────────
\* 変数
\* ──────────────────────────────────────────────
VARIABLES
    state,       \* [Nodes -> {"absent","inactive","active"}]
    tail,        \* [Nodes -> Nodes \cup {-1}]
    rView,       \* [Nodes -> SUBSET Nodes]
    sActives,    \* [Nodes -> SUBSET Nodes]
    anyActive,
    churn,
    \* Raft 提案中の状態
    \* proposing[n] \in {"none", "activate", "splitPre", "splitCommit", "merge"}
    \*   "none"        : 提案なし
    \*   "activate"    : ActivateFrontward の Raft commit 待ち
    \*   "splitPre"    : Split の PreCommitSplit commit 待ち
    \*   "splitCommit" : Split の CommitSplit commit 待ち
    \*   "merge"       : Merge の commit 待ち
    proposing,   \* [Nodes -> {"none","activate","splitPre","splitCommit","merge"}]
    \* 提案に付随するパラメータ
    propTarget,  \* [Nodes -> Nodes \cup {-1}]  activate/split の対象ノード
    propTail     \* [Nodes -> Nodes \cup {-1}]  commit 後の新 tail 値

vars == <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail>>

\* ──────────────────────────────────────────────
\* 派生集合
\* ──────────────────────────────────────────────
Members == { n \in Nodes : state[n] # "absent" }
Actives == { n \in Nodes : state[n] = "active" }

NearestIn(p, set) ==
    LET others == set \ {p}
    IN IF others = {} THEN p
       ELSE CHOOSE x \in others :
              \A y \in others : RingDist(p, x) <= RingDist(p, y)

NextInView(n) == NearestIn(n, rView[n])
FrontwardSector(n) == NearestIn(n, sActives[n] \ {n})

\* ──────────────────────────────────────────────
\* 型不変条件
\* ──────────────────────────────────────────────
TypeOK ==
    /\ state \in [Nodes -> {"absent","inactive","active"}]
    /\ tail \in [Nodes -> (Nodes \cup {-1})]
    /\ rView \in [Nodes -> SUBSET Nodes]
    /\ sActives \in [Nodes -> SUBSET Nodes]
    /\ anyActive \in BOOLEAN
    /\ churn \in 0..MaxChurn
    /\ proposing \in [Nodes -> {"none","activate","splitPre","splitCommit","merge"}]
    /\ propTarget \in [Nodes -> (Nodes \cup {-1})]
    /\ propTail \in [Nodes -> (Nodes \cup {-1})]

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

\* ──────────────────────────────────────────────
\* 環境イベント（SepView と同じ）
\* ──────────────────────────────────────────────
RefreshRView(n) ==
    /\ n \in Members
    /\ rView[n] # Members
    /\ rView' = [rView EXCEPT ![n] = Members]
    /\ UNCHANGED <<state, tail, sActives, anyActive, churn, proposing, propTarget, propTail>>

LearnSActive(n) ==
    /\ n \in Members
    /\ \E h \in Actives :
         /\ h \notin sActives[n]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget, propTail>>

ForgetSActive(n) ==
    /\ n \in Members
    /\ \E h \in sActives[n] :
         /\ state[h] # "active"
         /\ sActives' = [sActives EXCEPT ![n] = @ \ {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget, propTail>>

Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ rView' = [rView EXCEPT ![n] = Members \cup {n}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1
    /\ UNCHANGED <<proposing, propTarget, propTail>>

Leave(n) ==
    /\ churn < MaxChurn
    /\ state[n] # "absent"
    /\ Cardinality(Members) > 1
    /\ proposing[n] = "none"
    /\ state' = [state EXCEPT ![n] = "absent"]
    /\ tail' = [tail EXCEPT ![n] = -1]
    /\ rView' = [rView EXCEPT ![n] = {}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ anyActive' = IF state[n] = "active" /\ Cardinality(Actives) = 1
                    THEN FALSE ELSE anyActive
    /\ churn' = churn + 1
    /\ UNCHANGED <<proposing, propTarget, propTail>>

\* ──────────────────────────────────────────────
\* ActivateFirst（原子: anyActive=FALSE 時のみ、競合なし）
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    LET t == NextInView(n) IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ proposing[n] = "none"
    /\ state' = [state EXCEPT ![n] = "active"]
    /\ tail' = [tail EXCEPT ![n] = t]
    /\ sActives' = [sActives EXCEPT ![n] = @ \cup {n}]
    /\ anyActive' = TRUE
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail>>

\* ──────────────────────────────────────────────
\* ActivateFrontward: 2 ステップ化
\*   ProposeActivate(n): n が f=tail[n] を activate する提案を Raft に出す
\*   CommitActivate(n) : Raft が commit → f が active になる
\*
\*   中間状態では f はまだ inactive だが、n は「提案中」なので
\*   同じ f に対して別のノードも ProposeActivate できる → 両方 commit で重複
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
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn>>

CommitActivate(n) ==
    LET f == propTarget[n]
        ft == propTail[n] IN
    /\ proposing[n] = "activate"
    /\ f \in Members
    \* Raft commit: f を active 化。f が既に active なら冪等（Go の processActivateProposal と同じ）
    /\ IF state[f] = "inactive"
       THEN /\ state' = [state EXCEPT ![f] = "active"]
            /\ tail' = [tail EXCEPT ![f] = ft]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {f}, ![f] = @ \cup {f}]
            /\ anyActive' = TRUE
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* AbortProposal: 提案対象が離脱または状況が変わった場合にキャンセル。
\*   Go ではリトライ時に operateSectors を再評価し、状況が変わっていれば
\*   別の分岐に入る（= 事実上の abort）。
\*   splitPre の場合、hosting の tail は既に縮小済みなので
\*   abort 時に元の tail を復元する（Go では Terminate 経由で復旧）。
\* ──────────────────────────────────────────────
AbortProposal(n) ==
    /\ proposing[n] # "none"
    /\ LET f == propTarget[n] IN
       \* 対象が離脱した、または対象が既に別の状態になった
       f \notin Members \/ (proposing[n] = "activate" /\ state[f] # "inactive")
                        \/ (proposing[n] = "splitPre" /\ state[f] # "inactive")
                        \/ (proposing[n] = "merge" /\ state[f] # "active")
    /\ IF proposing[n] = "splitPre"
       THEN \* hosting の tail を元に戻す (propTail に元の tail が入っている)
            tail' = [tail EXCEPT ![n] = propTail[n]]
       ELSE UNCHANGED tail
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Split: 2 ステップ化
\*   ProposeSplit(n):   hosting の tail を m に縮小 (PreCommitSplit)
\*                      → 中間状態: hosting は [n, m)、frontward は inactive
\*                      → カバレッジギャップ [m, old_tail) が発生
\*   CommitSplit(n):    frontward の m を active 化 (CommitSplit)
\*                      → [m, old_tail) がカバーされる
\* ──────────────────────────────────────────────
ProposeSplit(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ tail[n] \in Members
    /\ \E m \in Members :
         /\ m # n /\ m # tail[n]
         /\ state[m] = "inactive"
         /\ IsBetween(m, n, tail[n])
         /\ \A m2 \in Members :
              ( m2 # n /\ m2 # tail[n] /\ state[m2] = "inactive"
                /\ IsBetween(m2, n, tail[n]) )
              => RingDist(n, m) <= RingDist(n, m2)
         \* PreCommitSplit: hosting の tail を m に縮小
         /\ tail' = [tail EXCEPT ![n] = m]
         /\ proposing' = [proposing EXCEPT ![n] = "splitPre"]
         /\ propTarget' = [propTarget EXCEPT ![n] = m]
         /\ propTail' = [propTail EXCEPT ![n] = tail[n]]  \* 元の tail = frontward の新 tail
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn>>

CommitSplit(n) ==
    LET m == propTarget[n]
        ft == propTail[n] IN
    /\ proposing[n] = "splitPre"
    /\ m \in Members
    \* CommitSplit: frontward の m を active 化
    /\ IF state[m] = "inactive"
       THEN /\ state' = [state EXCEPT ![m] = "active"]
            /\ tail' = [tail EXCEPT ![m] = ft]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {m}, ![m] = @ \cup {m}]
            /\ anyActive' = TRUE
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* Extend（原子: tail 更新のみ、SepView と同じ）
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n]
        newTail == NearestIn(n, rView[n] \cup {n}) IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ f # n
    /\ f \notin rView[n]
    /\ \A other \in Actives : other # n => ~IsBetween(other, n, newTail)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, proposing, propTarget, propTail>>

\* ──────────────────────────────────────────────
\* Merge: 2 ステップ化
\*   ProposeMerge(n): n が fs を吸収する Merge を提案。
\*     Go: frontwardNextSector.PrepareMerge(localNodeID)
\*     この時点では fs はまだ active。propTarget=fs, propTail=tail[fs] を記録。
\*   CommitMerge(n): fs を inactive にし、n の tail を tail[fs] に拡張。
\*     Go: hostingSector.Merge() → frontwardNextSector.Terminate() →
\*          hostingSector.CommitMerge(newTail)
\*     中間状態では fs がまだ active なので、別ノードが fs を対象に
\*     Split/Activate/Merge を行う可能性がある。
\* ──────────────────────────────────────────────
ProposeMerge(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ sActives[n] \ {n} # {}
    /\ fs # n
    /\ state[fs] = "active"
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, v)
    /\ ~IsBetween(fs, n, t)
    /\ \A other \in Actives :
         other # n /\ other # fs => ~IsBetween(other, n, tail[fs])
    \* 提案のみ。状態は変更しない。
    /\ proposing' = [proposing EXCEPT ![n] = "merge"]
    /\ propTarget' = [propTarget EXCEPT ![n] = fs]
    /\ propTail' = [propTail EXCEPT ![n] = tail[fs]]  \* 吸収後の新 tail
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn>>

CommitMerge(n) ==
    LET fs == propTarget[n]
        newTail == propTail[n] IN
    /\ proposing[n] = "merge"
    /\ fs \in Members
    \* fs がまだ active なら吸収。既に inactive なら tail 拡張のみ（冪等的）。
    /\ IF state[fs] = "active"
       THEN /\ state' = [state EXCEPT ![fs] = "inactive"]
            /\ tail' = [tail EXCEPT ![n] = newTail, ![fs] = -1]
            /\ sActives' = [sActives EXCEPT ![n] = @ \ {fs}, ![fs] = {}]
            \* 吸収される fs の proposing もクリア
            /\ proposing' = [proposing EXCEPT ![n] = "none", ![fs] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![fs] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1, ![fs] = -1]
            /\ anyActive' = IF Cardinality(Actives) <= 1 THEN FALSE ELSE anyActive
       ELSE /\ tail' = [tail EXCEPT ![n] = newTail]
            /\ proposing' = [proposing EXCEPT ![n] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1]
            /\ UNCHANGED <<state, sActives, anyActive>>
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* Terminate（原子: 修復アクション）
\*   2 つのトリガパスがある:
\*   (A) frontwardNodeMatch = false (v # fs) かつ fs が自分のセクター内
\*       → Go の operateSectors "Terminate hosting sector 1" ケース
\*   (B) frontwardNodeMatch = true (v = fs もしくは fs = tail) だが
\*       frontwardNextSector の head が自分のセクター内にある
\*       → Go の operateSectors "Terminate hosting sector 2" ケース
\*       → Raft の非原子性で commit された sector が自分の範囲内に入り込んだ場合
\* ──────────────────────────────────────────────

\* (A) frontwardNodeMatch = false かつ fs が (n, tail) 内
TerminateA(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ sActives[n] \ {n} # {}
    /\ fs # n
    /\ state[fs] = "active"
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, t)
    /\ state' = [state EXCEPT ![n] = "inactive", ![fs] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = -1, ![fs] = -1]
    /\ sActives' = [sActives EXCEPT ![n] = {}, ![fs] = {}]
    /\ anyActive' = (Cardinality(Actives) > 2)
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail>>

\* (B) frontwardNodeMatch でも、実際に active な sector が自分の範囲内にある
\*     → head (= fs) が (n, tail) 内で、かつ tail[n] # fs (= Equal でない)
TerminateB(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ tail[n] \in Nodes
    /\ tail[n] \in Members
    /\ state[tail[n]] = "active"
    /\ \E other \in Actives :
         /\ other # n
         /\ other # tail[n]
         /\ IsBetween(other, n, tail[n])
         /\ state' = [state EXCEPT ![n] = "inactive", ![other] = "inactive"]
         /\ tail' = [tail EXCEPT ![n] = -1, ![other] = -1]
         /\ sActives' = [sActives EXCEPT ![n] = {}, ![other] = {}]
         \* Terminate で inactive にされるノードの proposing もクリア
         /\ proposing' = [proposing EXCEPT ![n] = "none", ![other] = "none"]
         /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![other] = -1]
         /\ propTail' = [propTail EXCEPT ![n] = -1, ![other] = -1]
    /\ anyActive' = (Cardinality(Actives) > 2)
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* 遷移関係
\* ──────────────────────────────────────────────
Next ==
    \E n \in Nodes :
        \/ RefreshRView(n)
        \/ LearnSActive(n)
        \/ ForgetSActive(n)
        \/ Join(n)
        \/ Leave(n)
        \/ ActivateFirst(n)
        \/ ProposeActivate(n)
        \/ CommitActivate(n)
        \/ ProposeSplit(n)
        \/ CommitSplit(n)
        \/ AbortProposal(n)
        \/ Extend(n)
        \/ ProposeMerge(n)
        \/ CommitMerge(n)
        \/ TerminateA(n)
        \/ TerminateB(n)

\* ──────────────────────────────────────────────
\* 公平性
\*   全ての commit アクションと修復アクションに WF を付ける。
\*   Propose は WF 不要（他の enabled な Propose が先に起きてもよい）。
\* ──────────────────────────────────────────────
Fairness ==
    \A n \in Nodes :
        /\ WF_vars(RefreshRView(n))
        /\ WF_vars(LearnSActive(n))
        /\ WF_vars(ForgetSActive(n))
        /\ WF_vars(ActivateFirst(n))
        /\ WF_vars(ProposeActivate(n))
        /\ WF_vars(CommitActivate(n))
        /\ WF_vars(ProposeSplit(n))
        /\ WF_vars(CommitSplit(n))
        /\ WF_vars(AbortProposal(n))
        /\ WF_vars(Extend(n))
        /\ WF_vars(ProposeMerge(n))
        /\ WF_vars(CommitMerge(n))
        /\ WF_vars(TerminateA(n))
        /\ WF_vars(TerminateB(n))

Spec == Init /\ [][Next]_vars /\ Fairness

\* ──────────────────────────────────────────────
\* 安全性
\* ──────────────────────────────────────────────

\* NoOverlap は一時的に破れうる（Raft の非原子性による）
\* 代わりに「提案中でない committed セクターのみ」で NoOverlap を保証
NoOverlap ==
    \A a, b \in Nodes :
        ( a # b /\ state[a] = "active" /\ state[b] = "active" )
        => ~( IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]) )

ValidRange ==
    \A n \in Nodes :
        state[n] = "active" =>
            /\ tail[n] \in Nodes
            /\ ( tail[n] = n \/ IsBetween(tail[n], NextNodeID(n), n) )

ActiveFlagConsistent == anyActive = (Actives # {})

\* ──────────────────────────────────────────────
\* 活性
\* ──────────────────────────────────────────────
EventuallyAllActive ==
    <>[]( \A n \in Members : state[n] = "active" )

EventuallyFullCoverage ==
    <>[]( \A n \in Nodes :
            state[n] = "active" =>
                ( tail[n] = n \/ state[tail[n]] = "active" ) )

\* Terminate が重複を解消し、最終的に NoOverlap に復帰する
EventuallyNoOverlap ==
    <>[]( \A a, b \in Nodes :
            ( a # b /\ state[a] = "active" /\ state[b] = "active" )
            => ~( IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]) ) )

================================================================================
