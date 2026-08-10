-------------------------------- MODULE KvsSectorFail --------------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様 — quorum 喪失の故障モードと回復 (TODO-1)
 *   KvsSectorRaft.tla 派生。「提案は必ず commit される」前提を外す。
 *
 * 動機:
 *   全既存モデルは Commit 系アクションに WF を付け「commit は必ず来る」を
 *   前提としていた。シミュレーション (2026-07-04, クラス A) で、グループ
 *   メンバーの死亡により commit が永久に来ず、activation チェーンが恒久停止
 *   する故障モードが主因と確定した。TODO-3 (timeout+abort) / TODO-4
 *   (checkQuorumLoss = raft を経由しないローカル強制破棄) は本検証を待たずに
 *   実装済みであり、本モデルはその設計判断の検証負債を払う。
 *   merge/overlap 系の stuck + LocalDestroy は KvsSectorFalseLeftover.tla で
 *   検証済み。本モデルの主対象は activate/split 提案の stuck と TimeoutAbort。
 *
 * 追加する故障:
 *   - stuck[h]: h を head とするセクターの raft グループが quorum を失い
 *     commit が永久に来ない状態 (非決定的に発生、MaxStuck 回まで)。
 *     quorum やレプリカは精密にモデル化しない。「commit が来ないことがある」
 *     の抽象で十分 (TODO-1 の指示どおり)。
 *     stuck[h] が TRUE の間、そのグループへの commit を要するアクション
 *     (CommitActivate/CommitSplit/CommitMerge/Extend/Terminate の該当側) は
 *     発火しない。
 *
 * 回復アクション (Go 実装との対応):
 *   - TimeoutAbort(n): proposalWaitTimeout (15s) で提案を諦める。
 *     Go 忠実性の要点: splitSector の abort は frontwardNextSector.Terminate()
 *     のみで hosting の tail は復元しない (縮小されたまま)。生じた
 *     カバレッジギャップは後続の activate (ProposeActivate/CommitActivate)
 *     が修復する。ベースの AbortProposal (propose 時縮小の巻き戻し) は
 *     「migrate 段で失敗し PreCommitSplit まで到達しなかった」ケースの写像
 *     としてそのまま残す。
 *     発火条件は「commit が進めない場合のみ」に絞る (健全なら commit は
 *     timeout (15s) より速い、という抽象。merge の範囲再検証失敗による
 *     abort も Go では即時だが同アクションに含める)。
 *   - LocalDestroy(h): sector.checkQuorumLoss / TerminateLocally。
 *     raft を経由せず h のセクターを破棄し inactive 相当へ戻す。
 *     stuck[h] を解消し、incarnation (Go の sectorID 使い捨て) を進める。
 *     正発動 = stuck[h] のとき。誤発動 = 健全なグループへの発動
 *     (MaxMisfire 回まで)。COMMAND_REMOVE 経由の破棄も発火条件違いの
 *     同一抽象アクション (README TODO-1 追記 2026-07-06)。
 *
 * FixTombstone (検証項目 3 の答えを機械化する定数):
 *   Go では破棄→再作成で sectorID が変わるため、旧グループ宛の遅延 commit が
 *   新実体に適用されることはない (tombstone)。FixTombstone=TRUE はこれを
 *   inc/propInc (incarnation 対応) でモデル化する。FALSE では遅延 commit が
 *   再作成後の実体にも適用されうる permissive な世界になる。
 *   FALSE で safety が破れるなら「誤発動の安全性は tombstone が前提」が結論。
 *
 * ベースからの意味変更 (Go 現行実装への追随):
 *   - CommitMerge: 2026-07-19 の merge/overlap 修正を反映し、
 *     (a) victim (fs) が active なら fs グループの terminate commit を要する
 *     (TerminateForMerge/WaitTerminated の写像 = ~stuck[fs])、
 *     (b) commit 時に拡張範囲の再検証 (hasActiveSectorHeadInRange) を行う。
 *     どちらも成立しない間は CommitMerge は発火せず TimeoutAbort が回収する。
 *   - TerminateA/TerminateB: Go の Terminate() は各グループへの独立な
 *     fire-and-forget 提案のため、stuck でない側だけが commit される
 *     片側 terminate を許す (両側 stuck なら不発)。
 *
 * 意図的な過大近似 (ベース踏襲):
 *   - Leave→Join の再参加に tombstone は課さない (ベースが許容し検証済みの
 *     挙動。実装は sectorID で排除しており、モデルはより広い挙動で safety を
 *     示す)。inc の適用は LocalDestroy 経路のみ。
 *   - sActives の掃除 (ForgetSActive) は WF 付きの「タダの」ローカル更新の
 *     まま (TODO-2 の検証対象。本モデルのスコープ外)。
 *
 * 検証項目 (README TODO-1):
 *   1. 回復アクションを入れても safety (ValidRange, ActiveFlagConsistent) が
 *      保たれること。
 *   2. 回復アクションに WF を付与すると EventuallyAllActive が復活すること。
 *      (stuck 追加 + 回復なしでは液性が壊れることを先に確認する)
 *   3. LocalDestroy の誤発動を許した場合に safety が破れるか。
 *      破れるなら発動条件に何が必要かを特定する (→ FixTombstone)。
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,
    InitialMembers,
    MaxChurn,
    MaxStuck,           \* BecomeStuck の発生回数上限
    MaxMisfire,         \* LocalDestroy 誤発動 (健全グループへの発動) の上限
    EnableTimeoutAbort, \* BOOLEAN: 回復 1 = 提案の timeout abort
    EnableLocalDestroy, \* BOOLEAN: 回復 2 = quorum 喪失グループのローカル破棄
    FixTombstone        \* BOOLEAN: 破棄後の遅延 commit を新実体に適用しない

N == Cardinality(Nodes)

MaxInc == MaxStuck + MaxMisfire  \* LocalDestroy 総回数の上限 = incarnation の上限

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
    proposing,   \* [Nodes -> {"none","activate","splitPre","merge"}]
    propTarget,  \* [Nodes -> Nodes \cup {-1}]
    propTail,    \* [Nodes -> Nodes \cup {-1}]
    \* ── 故障モード ──
    stuck,       \* [Nodes -> BOOLEAN] h のグループが commit 不能
    stuckCnt,    \* 0..MaxStuck
    misfireCnt,  \* 0..MaxMisfire
    inc,         \* [Nodes -> 0..MaxInc] LocalDestroy ごとに +1 (Go: sectorID)
    propInc      \* [Nodes -> 0..MaxInc \cup {-1}] 提案時に捕獲した対象の inc

vars == <<state, tail, rView, sActives, anyActive, churn,
          proposing, propTarget, propTail,
          stuck, stuckCnt, misfireCnt, inc, propInc>>

failVars == <<stuck, stuckCnt, misfireCnt, inc>>

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

\* 対象 f の実体が提案時と同一か (FixTombstone=FALSE なら常に同一とみなす)
IncOK(n, f) == ~FixTombstone \/ inc[f] = propInc[n]

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
    /\ proposing \in [Nodes -> {"none","activate","splitPre","merge"}]
    /\ propTarget \in [Nodes -> (Nodes \cup {-1})]
    /\ propTail \in [Nodes -> (Nodes \cup {-1})]
    /\ stuck \in [Nodes -> BOOLEAN]
    /\ stuckCnt \in 0..MaxStuck
    /\ misfireCnt \in 0..MaxMisfire
    /\ inc \in [Nodes -> 0..MaxInc]
    /\ propInc \in [Nodes -> (0..MaxInc \cup {-1})]

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
    /\ stuck = [n \in Nodes |-> FALSE]
    /\ stuckCnt = 0
    /\ misfireCnt = 0
    /\ inc = [n \in Nodes |-> 0]
    /\ propInc = [n \in Nodes |-> -1]

\* ──────────────────────────────────────────────
\* 環境イベント（ベースと同じ + stuck の扱い）
\* ──────────────────────────────────────────────
RefreshRView(n) ==
    /\ n \in Members
    /\ rView[n] # Members
    /\ rView' = [rView EXCEPT ![n] = Members]
    /\ UNCHANGED <<state, tail, sActives, anyActive, churn,
                   proposing, propTarget, propTail, failVars, propInc>>

LearnSActive(n) ==
    /\ n \in Members
    /\ \E h \in Actives :
         /\ h \notin sActives[n]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn,
                   proposing, propTarget, propTail, failVars, propInc>>

ForgetSActive(n) ==
    /\ n \in Members
    /\ \E h \in sActives[n] :
         /\ state[h] # "active"
         /\ sActives' = [sActives EXCEPT ![n] = @ \ {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn,
                   proposing, propTarget, propTail, failVars, propInc>>

Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ rView' = [rView EXCEPT ![n] = Members \cup {n}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1
    /\ UNCHANGED <<proposing, propTarget, propTail, failVars, propInc>>

\* Leave: host 消滅でグループも消えるため stuck はクリアする
\* (再 Join は新実体。ベース同様 tombstone は課さない過大近似)
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
    /\ stuck' = [stuck EXCEPT ![n] = FALSE]
    /\ UNCHANGED <<proposing, propTarget, propTail, stuckCnt, misfireCnt, inc, propInc>>

\* ──────────────────────────────────────────────
\* 故障の発生
\*   グループの quorum 喪失。active でも未活性 (作成済み) でも起きる。
\*   環境イベントなので公平性は付けない。
\* ──────────────────────────────────────────────
BecomeStuck(h) ==
    /\ stuckCnt < MaxStuck
    /\ h \in Members
    /\ ~stuck[h]
    /\ stuck' = [stuck EXCEPT ![h] = TRUE]
    /\ stuckCnt' = stuckCnt + 1
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn,
                   proposing, propTarget, propTail, misfireCnt, inc, propInc>>

\* ──────────────────────────────────────────────
\* ActivateFirst（原子: 自グループへの commit → ~stuck[n]）
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    LET t == NextInView(n) IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ state' = [state EXCEPT ![n] = "active"]
    /\ tail' = [tail EXCEPT ![n] = t]
    /\ sActives' = [sActives EXCEPT ![n] = @ \cup {n}]
    /\ anyActive' = TRUE
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail, failVars, propInc>>

\* ──────────────────────────────────────────────
\* ActivateFrontward
\*   Propose は RPC 発射のみで commit を要さない (Go: activateFrontwardSector は
\*   fire-and-forget)。Commit は f のグループへの commit → ~stuck[f]。
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
    /\ propInc' = [propInc EXCEPT ![n] = inc[f]]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, failVars>>

CommitActivate(n) ==
    LET f == propTarget[n]
        ft == propTail[n] IN
    /\ proposing[n] = "activate"
    /\ f \in Members
    /\ ~stuck[f]
    /\ IncOK(n, f)
    /\ IF state[f] = "inactive"
       THEN /\ state' = [state EXCEPT ![f] = "active"]
            /\ tail' = [tail EXCEPT ![f] = ft]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {f}, ![f] = @ \cup {f}]
            /\ anyActive' = TRUE
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ propInc' = [propInc EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn, failVars>>

\* ──────────────────────────────────────────────
\* AbortProposal（ベース踏襲: 対象の離脱・状態変化によるキャンセル）
\*   splitPre の tail 復元は「migrate 段で失敗し PreCommitSplit 未到達」の写像。
\* ──────────────────────────────────────────────
AbortProposal(n) ==
    /\ proposing[n] # "none"
    /\ LET f == propTarget[n] IN
       f \notin Members \/ (proposing[n] = "activate" /\ state[f] # "inactive")
                        \/ (proposing[n] = "splitPre" /\ state[f] # "inactive")
                        \/ (proposing[n] = "merge" /\ state[f] # "active")
    /\ IF proposing[n] = "splitPre"
       THEN tail' = [tail EXCEPT ![n] = propTail[n]]
       ELSE UNCHANGED tail
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ propInc' = [propInc EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, failVars>>

\* ──────────────────────────────────────────────
\* TimeoutAbort（TODO-3: proposalWaitTimeout。commit が進めない場合のみ発火）
\*   Go 忠実性: splitPre でも tail は復元しない (abort は frontward の
\*   Terminate のみ)。縮小されたままの範囲は後続の activate が修復する。
\*   merge の範囲再検証失敗 (Go では即時 abort) も本アクションに含める。
\* ──────────────────────────────────────────────
CommitBlocked(n) ==
    LET f == propTarget[n] IN
    /\ f \in Members   \* 離脱はベースの AbortProposal が担当
    /\ CASE proposing[n] = "activate" ->
              stuck[f] \/ ~IncOK(n, f)
         [] proposing[n] = "splitPre" ->
              stuck[f] \/ ~IncOK(n, f)
         [] proposing[n] = "merge" ->
              \/ stuck[n]
              \/ ~IncOK(n, f)
              \/ (state[f] = "active" /\ stuck[f])
              \/ \E other \in Actives :
                   other # n /\ other # f /\ IsBetween(other, n, propTail[n])
         [] OTHER -> FALSE

TimeoutAbort(n) ==
    /\ EnableTimeoutAbort
    /\ proposing[n] # "none"
    /\ CommitBlocked(n)
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ propInc' = [propInc EXCEPT ![n] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, failVars>>

\* ──────────────────────────────────────────────
\* LocalDestroy（TODO-4: checkQuorumLoss / TerminateLocally）
\*   raft を経由せず h のセクターを破棄。stuck を解消し incarnation を進める。
\*   h 自身の提案中操作は後続ステップの失敗として消える (クリア)。
\*   他ノードの sActives に残る stale エントリは ForgetSActive (WF) が掃除する。
\*   誤発動: stuck していない (commit 可能な) グループへの発動。
\*   MaxMisfire 回まで許し、safety が保たれるかを検証する (検証項目 3)。
\* ──────────────────────────────────────────────
LocalDestroy(h) ==
    /\ EnableLocalDestroy
    /\ h \in Members
    /\ IF stuck[h]
       THEN misfireCnt' = misfireCnt
       ELSE /\ misfireCnt < MaxMisfire
            /\ misfireCnt' = misfireCnt + 1
    /\ state' = [state EXCEPT ![h] = "inactive"]
    /\ tail' = [tail EXCEPT ![h] = -1]
    /\ sActives' = [sActives EXCEPT ![h] = @ \ {h}]
    /\ anyActive' = (Actives \ {h} # {})
    /\ stuck' = [stuck EXCEPT ![h] = FALSE]
    /\ inc' = [inc EXCEPT ![h] = @ + 1]
    /\ proposing' = [proposing EXCEPT ![h] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![h] = -1]
    /\ propTail' = [propTail EXCEPT ![h] = -1]
    /\ propInc' = [propInc EXCEPT ![h] = -1]
    /\ UNCHANGED <<rView, churn, stuckCnt>>

\* ──────────────────────────────────────────────
\* Split
\*   ProposeSplit は accept + migrate (f グループへの Import commit) +
\*   PreCommitSplit (自グループへの commit) をまとめた写像
\*   → ~stuck[n] /\ ~stuck[m] を要する。
\*   CommitSplit は m のグループへの commit → ~stuck[m]。
\* ──────────────────────────────────────────────
ProposeSplit(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ tail[n] \in Members
    /\ \E m \in Members :
         /\ m # n /\ m # tail[n]
         /\ state[m] = "inactive"
         /\ ~stuck[m]
         /\ IsBetween(m, n, tail[n])
         /\ \A m2 \in Members :
              ( m2 # n /\ m2 # tail[n] /\ state[m2] = "inactive"
                /\ IsBetween(m2, n, tail[n]) )
              => RingDist(n, m) <= RingDist(n, m2)
         /\ tail' = [tail EXCEPT ![n] = m]
         /\ proposing' = [proposing EXCEPT ![n] = "splitPre"]
         /\ propTarget' = [propTarget EXCEPT ![n] = m]
         /\ propTail' = [propTail EXCEPT ![n] = tail[n]]
         /\ propInc' = [propInc EXCEPT ![n] = inc[m]]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, failVars>>

CommitSplit(n) ==
    LET m == propTarget[n]
        ft == propTail[n] IN
    /\ proposing[n] = "splitPre"
    /\ m \in Members
    /\ ~stuck[m]
    /\ IncOK(n, m)
    /\ IF state[m] = "inactive"
       THEN /\ state' = [state EXCEPT ![m] = "active"]
            /\ tail' = [tail EXCEPT ![m] = ft]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {m}, ![m] = @ \cup {m}]
            /\ anyActive' = TRUE
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ propInc' = [propInc EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn, failVars>>

\* ──────────────────────────────────────────────
\* Extend（自グループへの commit → ~stuck[n]）
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n]
        newTail == NearestIn(n, rView[n] \cup {n}) IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ ~stuck[n]
    /\ f # n
    /\ f \notin rView[n]
    /\ \A other \in Actives : other # n => ~IsBetween(other, n, newTail)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn,
                   proposing, propTarget, propTail, failVars, propInc>>

\* ──────────────────────────────────────────────
\* Merge
\*   ProposeMerge: PrepareMerge は fs のグループへの commit → ~stuck[fs]。
\*   CommitMerge: 2026-07-19 の merge/overlap 修正後の Go 実装に追随:
\*     - fs が active なら TerminateForMerge の commit (→ ~stuck[fs]) と
\*       WaitTerminated による破棄確認を経てから拡張
\*     - 拡張前に範囲再検証 (hasActiveSectorHeadInRange 相当)
\*     - CommitMerge 自体は自グループへの commit → ~stuck[n]
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
    /\ ~stuck[fs]
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, v)
    /\ ~IsBetween(fs, n, t)
    /\ \A other \in Actives :
         other # n /\ other # fs => ~IsBetween(other, n, tail[fs])
    /\ proposing' = [proposing EXCEPT ![n] = "merge"]
    /\ propTarget' = [propTarget EXCEPT ![n] = fs]
    /\ propTail' = [propTail EXCEPT ![n] = tail[fs]]
    /\ propInc' = [propInc EXCEPT ![n] = inc[fs]]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, failVars>>

CommitMerge(n) ==
    LET fs == propTarget[n]
        newTail == propTail[n] IN
    /\ proposing[n] = "merge"
    /\ fs \in Members
    /\ ~stuck[n]
    /\ IncOK(n, fs)
    \* 範囲再検証: 拡張範囲に他の active head がいれば commit しない (abort へ)
    /\ \A other \in Actives :
         other # n /\ other # fs => ~IsBetween(other, n, newTail)
    /\ IF state[fs] = "active"
       THEN \* victim の terminate commit + 破棄確認を経て拡張
            /\ ~stuck[fs]
            /\ state' = [state EXCEPT ![fs] = "inactive"]
            /\ tail' = [tail EXCEPT ![n] = newTail, ![fs] = -1]
            /\ sActives' = [sActives EXCEPT ![n] = @ \ {fs}, ![fs] = {}]
            /\ proposing' = [proposing EXCEPT ![n] = "none", ![fs] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![fs] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1, ![fs] = -1]
            /\ propInc' = [propInc EXCEPT ![n] = -1, ![fs] = -1]
            /\ anyActive' = IF Cardinality(Actives) <= 1 THEN FALSE ELSE anyActive
       ELSE \* fs は既に破棄済み (他者の terminate 等)。範囲再検証つきで拡張のみ
            /\ tail' = [tail EXCEPT ![n] = newTail]
            /\ proposing' = [proposing EXCEPT ![n] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1]
            /\ propInc' = [propInc EXCEPT ![n] = -1]
            /\ UNCHANGED <<state, sActives, anyActive>>
    /\ UNCHANGED <<rView, churn, failVars>>

\* ──────────────────────────────────────────────
\* Terminate
\*   Go の Terminate() は各グループへの独立な fire-and-forget 提案。
\*   stuck でない側だけが commit される片側 terminate を許す。
\*   両側 stuck なら不発 (LocalDestroy が回収する)。
\* ──────────────────────────────────────────────
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
    /\ (~stuck[n] \/ ~stuck[fs])
    /\ LET killed == { x \in {n, fs} : ~stuck[x] } IN
         /\ state' = [x \in Nodes |-> IF x \in killed THEN "inactive" ELSE state[x]]
         /\ tail' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE tail[x]]
         /\ sActives' = [x \in Nodes |-> IF x \in killed THEN {} ELSE sActives[x]]
         /\ anyActive' = (Actives \ killed # {})
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail, failVars, propInc>>

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
         /\ (~stuck[n] \/ ~stuck[other])
         /\ LET killed == { x \in {n, other} : ~stuck[x] } IN
              /\ state' = [x \in Nodes |-> IF x \in killed THEN "inactive" ELSE state[x]]
              /\ tail' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE tail[x]]
              /\ sActives' = [x \in Nodes |-> IF x \in killed THEN {} ELSE sActives[x]]
              \* terminate されるノードの proposing もクリア (ベース踏襲)
              /\ proposing' = [x \in Nodes |-> IF x \in killed THEN "none" ELSE proposing[x]]
              /\ propTarget' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE propTarget[x]]
              /\ propTail' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE propTail[x]]
              /\ propInc' = [x \in Nodes |-> IF x \in killed THEN -1 ELSE propInc[x]]
              /\ anyActive' = (Actives \ killed # {})
    /\ UNCHANGED <<rView, churn, failVars>>

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
        \/ BecomeStuck(n)
        \/ ActivateFirst(n)
        \/ ProposeActivate(n)
        \/ CommitActivate(n)
        \/ ProposeSplit(n)
        \/ CommitSplit(n)
        \/ AbortProposal(n)
        \/ TimeoutAbort(n)
        \/ LocalDestroy(n)
        \/ Extend(n)
        \/ ProposeMerge(n)
        \/ CommitMerge(n)
        \/ TerminateA(n)
        \/ TerminateB(n)

\* ──────────────────────────────────────────────
\* 公平性
\*   ベースの Commit/修復系 WF に加え、回復アクション (TimeoutAbort /
\*   LocalDestroy) に WF を付ける。BecomeStuck は環境イベントなので付けない。
\*   誤発動 (stuck していないグループへの LocalDestroy) は公平性の対象に
\*   しない (SF/WF なしの \E 分岐として探索される)。
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
        /\ WF_vars(TimeoutAbort(n))
        /\ WF_vars(LocalDestroy(n) /\ stuck[n])   \* 正発動のみ公平
        /\ WF_vars(Extend(n))
        /\ WF_vars(ProposeMerge(n))
        /\ WF_vars(CommitMerge(n))
        /\ WF_vars(TerminateA(n))
        /\ WF_vars(TerminateB(n))

Spec == Init /\ [][Next]_vars /\ Fairness

\* ──────────────────────────────────────────────
\* 安全性
\* ──────────────────────────────────────────────
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

EventuallyNoOverlap ==
    <>[]( \A a, b \in Nodes :
            ( a # b /\ state[a] = "active" /\ state[b] = "active" )
            => ~( IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]) ) )

================================================================================
