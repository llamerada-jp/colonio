--------------------------- MODULE KvsSectorLocalViewFail ---------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   ローカル視点オーバーラップガード (KvsSectorLocalView) + quorum 喪失
 *   backstop (KvsSectorFail の stuck/LocalDestroy) の統合（README TODO-2）
 *
 * 動機:
 *   KvsSectorLocalView.tla の Phase 1 (LocalViewGuards=TRUE) で
 *   EventuallyNoOverlap / EventuallyNoLeftover の違反を発見した:
 *   2つの leftover (host 死亡、レプリカ群は生存) の range が互いに重なると、
 *   隣接する active ノードはローカル視点でも「もう一方の leftover がいる」
 *   ことを検出して merge を安全側に拒否し、leftover 同士は自分では
 *   state="active" になれないため Terminate/Merge の発火主体になれない。
 *   これを最終的に救うはずの LeftoverQuorumLoss は
 *   「ring 全体のメンバーが1人以下」という極端に弱い条件でしか発火せず、
 *   間に合わない。
 *
 *   診断: これは KvsSectorLeftover.tla 系列の backstop が Go の実態
 *   (checkQuorumLoss = セクター単位の quorum 喪失検知、ring 全体の
 *   メンバー数に依存しない) より弱すぎることが本質。TODO-1 で検証済みの
 *   stuck/LocalDestroy (セクター単位で quorum 喪失を検知し強制破棄する)
 *   を組み合わせれば、孤立した leftover 同士の重なりも個別に解消できる
 *   はず、という仮説を検証する。
 *
 * KvsSectorLocalView.tla からの差分:
 *   - LeftoverQuorumLoss を削除し、stuck[h]/LocalDestroy(h) に一本化した
 *     (Go に quorum 喪失検知の経路が checkQuorumLoss ひとつしかないことに
 *     倣う。2つの弱い/強いバックストップを併存させない)。
 *   - stuck の対象は Sectored == {n : state[n] # "absent"} 全体
 *     (inactive/active/leftover を含む)。leftover も対象に含めることが
 *     本モデルの要点 — host 死亡後も残るレプリカ群自体が個別に
 *     quorum を失いうる。
 *   - LocalDestroy(h) は DestroyedState(h) (leftover→absent、
 *     inactive/active→inactive) に落とす。h が持つ/h に対する
 *     mergeLock も併せて解放する。
 *   - Commit 系アクション (CommitActivate/CommitSplit/CommitMerge/Extend)
 *     は対象の commit 能力 (~stuck[target]) を要求するようにした
 *     (TODO-1 の KvsSectorFail.tla と同じ方針)。
 *   - TerminateA/B は Fail 同様、fire-and-forget の片側 kill を許す
 *     (stuck でない側だけが commit される)。
 *   - TODO-1 の misfire/FixTombstone 次元は本モデルでは扱わない
 *     (TODO-1 で誤発動・tombstone なしでも安全なことは既に検証済み。
 *     本モデルは backstop を「正しく（誤発動なしで）動く」前提の
 *     構成要素として使い、TODO-2 の本題であるローカル視点ガードとの
 *     相互作用にだけ焦点を絞る)。
 *
 * 検証フェーズ:
 *   Phase 2 (backstop 強化確認): LocalViewGuards = TRUE, EnableLocalDestroy = TRUE
 *     → KvsSectorLocalView.tla Phase 1 の反例系列が本モデルでは
 *       解消される（EventuallyNoOverlap / EventuallyNoLeftover 含む
 *       全 liveness が成立する）ことを確認する。
 *   Phase 2' (対照): EnableLocalDestroy = FALSE
 *     → Phase 1 と同じ違反が再現することを確認する（backstop なしでは
 *       やはり救えないことの対照実験）。
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,
    InitialMembers,
    MaxChurn,
    EnableRelease,
    PermissiveRelease,
    ClipActivationTail,
    LocalViewGuards,
    MaxStuck,
    EnableTimeoutAbort,
    EnableLocalDestroy

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
    state,
    tail,
    rView,
    sActives,
    anyActive,
    churn,
    proposing,
    propTarget,
    propTail,
    mergeLock,
    stuck,       \* [Nodes -> BOOLEAN] 位置 n の sector (存在すれば) の commit 不能
    stuckCnt

vars == <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail,
          mergeLock, stuck, stuckCnt>>

Members == { n \in Nodes : state[n] \in {"inactive", "active"} }
Actives == { n \in Nodes : state[n] = "active" }
ActiveSectors == { n \in Nodes : state[n] \in {"active", "leftover"} }
\* Sectored: 何らかの sector 実体を持つ位置 (inactive/active/leftover)。
\* stuck/LocalDestroy の対象範囲はこれ全体（Members より広く leftover を含む）。
Sectored == { n \in Nodes : state[n] # "absent" }

EffectiveView(v) == IF LocalViewGuards THEN sActives[v] ELSE ActiveSectors

NearestIn(p, set) ==
    LET others == set \ {p}
    IN IF others = {} THEN p
       ELSE CHOOSE x \in others :
              \A y \in others : RingDist(p, x) <= RingDist(p, y)

NextInView(n) == NearestIn(n, rView[n])
FrontwardSector(n) == NearestIn(n, sActives[n] \ {n})

DestroyedState(h) == IF state[h] = "leftover" THEN "absent" ELSE "inactive"

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
    /\ proposing \in [Nodes -> {"none","activate","splitPre","splitCommit","merge"}]
    /\ propTarget \in [Nodes -> (Nodes \cup {-1})]
    /\ propTail \in [Nodes -> (Nodes \cup {-1})]
    /\ mergeLock \in [Nodes -> (Nodes \cup {-1})]
    /\ stuck \in [Nodes -> BOOLEAN]
    /\ stuckCnt \in 0..MaxStuck

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
    /\ stuckCnt = 0

\* ──────────────────────────────────────────────
\* 環境イベント
\* ──────────────────────────────────────────────
RefreshRView(n) ==
    /\ n \in Members
    /\ rView[n] # Members
    /\ rView' = [rView EXCEPT ![n] = Members]
    /\ UNCHANGED <<state, tail, sActives, anyActive, churn, proposing, propTarget, propTail,
                   mergeLock, stuck, stuckCnt>>

LearnSActive(n) ==
    /\ n \in Members
    /\ \E h \in ActiveSectors :
         /\ h \notin sActives[n]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget, propTail,
                   mergeLock, stuck, stuckCnt>>

ForgetSActive(n) ==
    /\ n \in Members
    /\ \E h \in sActives[n] :
         /\ h \notin ActiveSectors
         /\ sActives' = [sActives EXCEPT ![n] = @ \ {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget, propTail,
                   mergeLock, stuck, stuckCnt>>

Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ rView' = [rView EXCEPT ![n] = Members \cup {n}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1
    /\ mergeLock' = [x \in Nodes |-> IF mergeLock[x] = n THEN -1 ELSE mergeLock[x]]
    /\ UNCHANGED <<proposing, propTarget, propTail, stuck, stuckCnt>>

Leave(n) ==
    /\ churn < MaxChurn
    /\ state[n] \in {"inactive", "active"}
    /\ Cardinality(Members) > 1
    /\ proposing[n] \in {"none", "merge"}
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
    /\ UNCHANGED <<stuckCnt>>

LeaveLeftover(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "active"
    /\ Cardinality(Members \ {n}) >= 2
    /\ \E m \in Members \ {n} : state[m] = "active"
    /\ proposing[n] \in {"none", "merge"}
    /\ state' = [state EXCEPT ![n] = "leftover"]
    /\ rView' = [rView EXCEPT ![n] = {}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ churn' = churn + 1
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<tail, anyActive, mergeLock, stuck, stuckCnt>>

\* ──────────────────────────────────────────────
\* 故障の発生（環境イベント、公平性なし）
\*   対象は Sectored 全体 (inactive/active/leftover)。leftover も対象に
\*   含むのが KvsSectorLocalView.tla からの本質的な差分。
\* ──────────────────────────────────────────────
BecomeStuck(h) ==
    /\ stuckCnt < MaxStuck
    /\ h \in Sectored
    /\ ~stuck[h]
    /\ stuck' = [stuck EXCEPT ![h] = TRUE]
    /\ stuckCnt' = stuckCnt + 1
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget,
                   propTail, mergeLock>>

\* ──────────────────────────────────────────────
\* LocalDestroy（Go の checkQuorumLoss/TerminateLocally 相当）
\*   誤発動は本モデルでは扱わない (stuck[h] のときのみ発火。TODO-1 で
\*   誤発動の安全性は別途検証済み)。h が保持/h に保持されている
\*   mergeLock も併せて解放する。
\* ──────────────────────────────────────────────
LocalDestroy(h) ==
    /\ EnableLocalDestroy
    /\ h \in Sectored
    /\ stuck[h]
    /\ state' = [state EXCEPT ![h] = DestroyedState(h)]
    /\ tail' = [tail EXCEPT ![h] = -1]
    /\ sActives' = [sActives EXCEPT ![h] = {}]
    /\ proposing' = [proposing EXCEPT ![h] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![h] = -1]
    /\ propTail' = [propTail EXCEPT ![h] = -1]
    /\ mergeLock' = [x \in Nodes |-> IF x = h \/ mergeLock[x] = h THEN -1 ELSE mergeLock[x]]
    /\ stuck' = [stuck EXCEPT ![h] = FALSE]
    /\ anyActive' = (Actives \ {h} # {})
    /\ UNCHANGED <<rView, churn, stuckCnt>>

\* ──────────────────────────────────────────────
\* ActivateFirst（原子）
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    LET t == NextInView(n)
        blockers == { h \in EffectiveView(n) : h # n /\ IsBetween(h, n, t) } IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ blockers = {} \/ ClipActivationTail
    /\ LET t2 == IF blockers = {} THEN t
                 ELSE CHOOSE h \in blockers :
                        \A h2 \in blockers : RingDist(n, h) <= RingDist(n, h2) IN
       /\ state' = [state EXCEPT ![n] = "active"]
       /\ tail' = [tail EXCEPT ![n] = t2]
    /\ sActives' = [sActives EXCEPT ![n] = @ \cup {n}]
    /\ anyActive' = TRUE
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail, mergeLock, stuck, stuckCnt>>

\* ──────────────────────────────────────────────
\* ActivateFrontward: 2 ステップ
\*   CommitActivate は f 自身の commit 能力 (~stuck[f]) を要求する。
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
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, mergeLock, stuck, stuckCnt>>

CommitActivate(n) ==
    LET f == propTarget[n]
        ft == propTail[n]
        blockers == { h \in EffectiveView(f) : h # f /\ IsBetween(h, f, ft) } IN
    /\ proposing[n] = "activate"
    /\ f \in Members
    /\ ~stuck[f]
    /\ IF state[f] = "inactive" /\ (blockers = {} \/ ClipActivationTail)
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
    /\ UNCHANGED <<rView, churn, mergeLock, stuck, stuckCnt>>

\* ──────────────────────────────────────────────
\* AbortProposal（対象の離脱・状態変化によるキャンセル。KvsSectorLocalView と同じ）
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
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock, stuck, stuckCnt>>

\* ──────────────────────────────────────────────
\* TimeoutAbort（TODO-1 の KvsSectorFail.tla と同じ方針: commit が
\* stuck な対象に阻まれて進めない場合のみ発火）
\* ──────────────────────────────────────────────
CommitBlocked(n) ==
    LET f == propTarget[n] IN
    /\ f \in Members \cup ActiveSectors
    /\ CASE proposing[n] = "activate" -> stuck[f]
         [] proposing[n] = "splitPre" -> stuck[f]
         [] proposing[n] = "merge" ->
              \/ stuck[n]
              \/ (f \in ActiveSectors /\ stuck[f])
         [] OTHER -> FALSE

TimeoutAbort(n) ==
    /\ EnableTimeoutAbort
    /\ proposing[n] # "none"
    /\ propTarget[n] \in Nodes
    /\ CommitBlocked(n)
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, mergeLock, stuck, stuckCnt>>

ProposeSplit(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ mergeLock[n] = -1
    /\ ~stuck[n]
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
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock, stuck, stuckCnt>>

CommitSplit(n) ==
    LET m == propTarget[n]
        ft == propTail[n] IN
    /\ proposing[n] = "splitPre"
    /\ m \in Members
    /\ ~stuck[m]
    /\ IF state[m] = "inactive"
       THEN /\ state' = [state EXCEPT ![m] = "active"]
            /\ tail' = [tail EXCEPT ![m] = ft]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {m}, ![m] = @ \cup {m}]
            /\ anyActive' = TRUE
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn, mergeLock, stuck, stuckCnt>>

\* ──────────────────────────────────────────────
\* Extend（原子）: n 自身のローカル視点 + commit 能力を要求する
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n]
        newTail == NearestIn(n, rView[n] \cup {n}) IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ f # n
    /\ f \notin rView[n]
    /\ \A other \in EffectiveView(n) : other # n => ~IsBetween(other, n, newTail)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, proposing, propTarget, propTail,
                   mergeLock, stuck, stuckCnt>>

\* ──────────────────────────────────────────────
\* Merge: 2 ステップ。ProposeMerge の範囲再検証は n 自身のローカル視点。
\*   CommitMerge は n 自身の commit 能力 (~stuck[n]) と、fs がまだ
\*   ActiveSectors なら fs の commit 能力 (~stuck[fs]、victim 側の
\*   terminate が commit できることを要求する = TODO-1 の
\*   ConfirmVictim 相当) を要求する。
\* ──────────────────────────────────────────────
ProposeMerge(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ sActives[n] \ {n} # {}
    /\ fs # n
    /\ fs \in ActiveSectors
    /\ mergeLock[fs] \in {-1, n}
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, v)
    /\ ~IsBetween(fs, n, t)
    /\ \A other \in EffectiveView(n) :
         other # n /\ other # fs => ~IsBetween(other, n, tail[fs])
    /\ proposing' = [proposing EXCEPT ![n] = "merge"]
    /\ propTarget' = [propTarget EXCEPT ![n] = fs]
    /\ propTail' = [propTail EXCEPT ![n] = tail[fs]]
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = n]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, stuck, stuckCnt>>

CommitMerge(n) ==
    LET fs == propTarget[n]
        newTail == propTail[n] IN
    /\ proposing[n] = "merge"
    /\ ~stuck[n]
    /\ fs \in (Members \cup ActiveSectors)
    /\ IF fs \in ActiveSectors
       THEN /\ ~stuck[fs]
            /\ state' = [state EXCEPT ![fs] = DestroyedState(fs)]
            /\ tail' = [tail EXCEPT ![n] = newTail, ![fs] = -1]
            /\ sActives' = [sActives EXCEPT ![n] = @ \ {fs}, ![fs] = {}]
            /\ proposing' = [proposing EXCEPT ![n] = "none", ![fs] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![fs] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1, ![fs] = -1]
            /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
            /\ anyActive' = TRUE
       ELSE /\ tail' = [tail EXCEPT ![n] = newTail]
            /\ proposing' = [proposing EXCEPT ![n] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1]
            /\ UNCHANGED <<state, sActives, anyActive, mergeLock>>
    /\ UNCHANGED <<rView, churn, stuck, stuckCnt>>

\* ──────────────────────────────────────────────
\* Terminate（fire-and-forget: stuck でない側だけが commit される）
\* ──────────────────────────────────────────────
\* NOTE (2026-07-25): fs (被 kill 側) が dangling な pending proposal
\* (例: fs 自身が別の相手と merge 提案中) を持ったまま殺されると、後で
\* CommitMerge がその stale な proposing を使って発火し、実際には
\* active でないノードに対して anyActive'=TRUE を設定してしまう
\* (ActiveFlagConsistent 違反、churn=5 の探索で発見)。両側の proposing を
\* 必ずクリアする。KvsSectorLeftover.tla の TerminateA にも同じ潜在バグが
\* あり、churn=3 の探索では到達しなかっただけと判明した。
TerminateA(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ sActives[n] \ {n} # {}
    /\ fs # n
    /\ fs \in ActiveSectors
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, t)
    /\ (~stuck[n] \/ ~stuck[fs])
    /\ LET killed == { x \in {n, fs} : ~stuck[x] } IN
         /\ state' = [x \in Nodes |-> IF x \in killed THEN DestroyedState(x) ELSE state[x]]
         /\ tail' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE tail[x]]
         /\ sActives' = [x \in Nodes |-> IF x \in killed THEN {} ELSE sActives[x]]
         /\ proposing' = [x \in Nodes |-> IF x \in killed THEN "none" ELSE proposing[x]]
         /\ propTarget' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE propTarget[x]]
         /\ propTail' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE propTail[x]]
         /\ mergeLock' = [x \in Nodes |->
                            IF x \in killed \/ mergeLock[x] \in killed THEN -1 ELSE mergeLock[x]]
         /\ anyActive' = (Actives \ killed # {})
    /\ UNCHANGED <<rView, churn, stuck, stuckCnt>>

TerminateB(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ tail[n] \in Nodes
    /\ tail[n] \in Members
    /\ state[tail[n]] = "active"
    /\ \E other \in EffectiveView(n) :
         /\ other \in ActiveSectors
         /\ other # n
         /\ other # tail[n]
         /\ IsBetween(other, n, tail[n])
         /\ (~stuck[n] \/ ~stuck[other])
         /\ LET killed == { x \in {n, other} : ~stuck[x] } IN
              /\ state' = [x \in Nodes |-> IF x \in killed THEN DestroyedState(x) ELSE state[x]]
              /\ tail' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE tail[x]]
              /\ sActives' = [x \in Nodes |-> IF x \in killed THEN {} ELSE sActives[x]]
              /\ proposing' = [x \in Nodes |-> IF x \in killed THEN "none" ELSE proposing[x]]
              /\ propTarget' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE propTarget[x]]
              /\ propTail' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE propTail[x]]
              /\ mergeLock' = [x \in Nodes |->
                                 IF x \in killed \/ mergeLock[x] \in killed THEN -1 ELSE mergeLock[x]]
              /\ anyActive' = (Actives \ killed # {})
    /\ UNCHANGED <<rView, churn, stuck, stuckCnt>>

ReleaseMergeLock(fs) ==
    /\ EnableRelease
    /\ state[fs] # "absent"
    /\ mergeLock[fs] # -1
    /\ mergeLock[fs] \notin Members
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail,
                   stuck, stuckCnt>>

ReleaseMergeLockAny(fs) ==
    /\ PermissiveRelease
    /\ state[fs] # "absent"
    /\ mergeLock[fs] # -1
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail,
                   stuck, stuckCnt>>

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
        \/ LeaveLeftover(n)
        \/ BecomeStuck(n)
        \/ LocalDestroy(n)
        \/ ActivateFirst(n)
        \/ ProposeActivate(n)
        \/ CommitActivate(n)
        \/ ProposeSplit(n)
        \/ CommitSplit(n)
        \/ AbortProposal(n)
        \/ TimeoutAbort(n)
        \/ Extend(n)
        \/ ProposeMerge(n)
        \/ CommitMerge(n)
        \/ TerminateA(n)
        \/ TerminateB(n)
        \/ ReleaseMergeLock(n)
        \/ ReleaseMergeLockAny(n)

Fairness ==
    \A n \in Nodes :
        /\ WF_vars(RefreshRView(n))
        /\ WF_vars(LearnSActive(n))
        /\ WF_vars(ForgetSActive(n))
        /\ WF_vars(LocalDestroy(n))
        /\ WF_vars(ActivateFirst(n))
        /\ WF_vars(ProposeActivate(n))
        /\ WF_vars(CommitActivate(n))
        /\ WF_vars(ProposeSplit(n))
        /\ WF_vars(CommitSplit(n))
        /\ WF_vars(AbortProposal(n))
        /\ WF_vars(TimeoutAbort(n))
        /\ WF_vars(Extend(n))
        /\ WF_vars(ProposeMerge(n))
        /\ WF_vars(CommitMerge(n))
        /\ WF_vars(TerminateA(n))
        /\ WF_vars(TerminateB(n))
        /\ WF_vars(ReleaseMergeLock(n))

Spec == Init /\ [][Next]_vars /\ Fairness

\* ──────────────────────────────────────────────
\* 安全性
\* ──────────────────────────────────────────────
NoOverlap ==
    \A a, b \in Nodes :
        ( a # b /\ a \in ActiveSectors /\ b \in ActiveSectors )
        => ~( IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]) )

ValidRange ==
    \A n \in Nodes :
        n \in ActiveSectors =>
            /\ tail[n] \in Nodes
            /\ ( tail[n] = n \/ IsBetween(tail[n], NextNodeID(n), n) )

ActiveFlagConsistent == anyActive = (Actives # {})

LockOnExistingSector ==
    \A x \in Nodes : mergeLock[x] # -1 => state[x] # "absent"

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

================================================================================
