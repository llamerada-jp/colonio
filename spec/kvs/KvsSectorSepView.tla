-------------------------------- MODULE KvsSectorSepView --------------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   routing ビュー (rView) と sector-store ビュー (sActives) を分離した版
 *
 * 動機:
 *   KvsSectorDelay.tla では 2 つの情報源を rView 1 つに潰していたため
 *   Merge ケースが構造的に到達不能だった。本モデルでは Go 実装通り
 *   独立した 2 つのビューを持つ:
 *
 *     - rView[n]    : routing / gossip 由来の n のメンバービュー
 *                     -> NextInView(n) = frontwardNextNodeID
 *     - sActives[n] : Raft store 由来の「n が active と知っている head の集合」
 *                     -> FrontwardSector(n) = frontwardNextSector の head
 *
 *   2 つの情報源は独立に更新されるため、たとえば
 *   「f は store からまだ active に見えるが、routing 上は f より外側のノードが近い」
 *   といった状況が発生し、Go の Merge 経路が再現できる。
 *
 * Go 実装との対応:
 *   sActives[n]          : k.sectors のうち active な head の集合
 *   FrontwardSector(n)   : getFrontwardCondition が返す frontwardNextSector の head
 *   NextInView(n)        : frontwardNextNodeID (routing 由来)
 *   RefreshRView(n)      : routing/gossip による近傍更新
 *   LearnSActive(n)      : Raft membership 伝播 / sector 起動の通知
 *   ForgetSActive(n)     : sector 停止の通知、これが遅れると Merge/Terminate が発火
 *
 * Merge / Terminate の発火条件 (n が active のとき):
 *   fs := FrontwardSector(n) , v := NextInView(n) , t := tail[n]
 *   共通: fs # n, state[fs] = "active", v # n, v # fs
 *     - Merge     : IsBetween(fs, n, v) /\ ~IsBetween(fs, n, t)
 *                   (= fs は tail 以遠 かつ v より手前 → fs を吸収)
 *     - Terminate : IsBetween(fs, n, t)
 *                   (= fs が自分のセクター内側 → 両方放棄)
 *
 * 抽象化:
 *   - Raft 合意・パケット損失は省略 (即時反映)
 *   - rView, sActives は単純な集合差分 (メッセージキュー順序は省略)
 *   - 1 ノード = 1 セクター
 *   - Join + Leave の総回数を MaxChurn で制限
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
    state,      \* [Nodes -> {"absent","inactive","active"}]
    tail,       \* [Nodes -> Nodes \cup {-1}]
    rView,      \* [Nodes -> SUBSET Nodes]   routing ビュー
    sActives,   \* [Nodes -> SUBSET Nodes]   sector-store ビュー (active な head)
    anyActive,
    churn

vars == <<state, tail, rView, sActives, anyActive, churn>>

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

\* routing 由来 (= Go の frontwardNextNodeID)
NextInView(n) == NearestIn(n, rView[n])

\* sector-store 由来 (= Go の frontwardNextSector の head)
\* 自分を除いた sActives の中で最近接 head
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

\* ──────────────────────────────────────────────
\* 環境イベント
\* ──────────────────────────────────────────────

\* routing ビューの同期 (WF で公平性を確保)
RefreshRView(n) ==
    /\ n \in Members
    /\ rView[n] # Members
    /\ rView' = [rView EXCEPT ![n] = Members]
    /\ UNCHANGED <<state, tail, sActives, anyActive, churn>>

\* sector-store ビューに「新しく active になった head」を追加する。
\* これは Raft 通知 / sector 起動の伝播に相当する。 (WF)
LearnSActive(n) ==
    /\ n \in Members
    /\ \E h \in Actives :
         /\ h \notin sActives[n]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn>>

\* sector-store ビューから「active でなくなった head」を取り除く。
\* この遅延が Merge / Terminate の発火条件をつくる。 (WF)
ForgetSActive(n) ==
    /\ n \in Members
    /\ \E h \in sActives[n] :
         /\ state[h] # "active"
         /\ sActives' = [sActives EXCEPT ![n] = @ \ {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn>>

Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ rView' = [rView EXCEPT ![n] = Members \cup {n}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1

Leave(n) ==
    /\ churn < MaxChurn
    /\ state[n] # "absent"
    /\ Cardinality(Members) > 1
    /\ state' = [state EXCEPT ![n] = "absent"]
    /\ tail' = [tail EXCEPT ![n] = -1]
    /\ rView' = [rView EXCEPT ![n] = {}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ anyActive' = IF state[n] = "active" /\ Cardinality(Actives) = 1
                    THEN FALSE ELSE anyActive
    /\ churn' = churn + 1

\* ──────────────────────────────────────────────
\* セクター操作
\* ──────────────────────────────────────────────

\* 新しい sector を作るたびに「sector を作った当事者」だけ即座に sActives 更新。
\* 他ノードは RefreshSActives でラグして反映する。

ActivateFirst(n) ==
    LET t == NextInView(n) IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ \A other \in Actives : other # n => ~IsBetween(other, n, t)
    /\ state' = [state EXCEPT ![n] = "active"]
    /\ tail' = [tail EXCEPT ![n] = t]
    /\ sActives' = [sActives EXCEPT ![n] = @ \cup {n}]
    /\ anyActive' = TRUE
    /\ UNCHANGED <<rView, churn>>

ActivateFrontward(n) ==
    LET f == tail[n]
        ft == NextInView(tail[n]) IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \in Members
    /\ state[f] = "inactive"
    /\ NextInView(n) = f
    /\ \A other \in Actives : other # f => ~IsBetween(other, f, ft)
    /\ state' = [state EXCEPT ![f] = "active"]
    /\ tail' = [tail EXCEPT ![f] = ft]
    /\ sActives' = [sActives EXCEPT ![n] = @ \cup {f}, ![f] = @ \cup {f}]
    /\ UNCHANGED <<rView, anyActive, churn>>

\* Split: n の sector 内に、まだ active でないメンバー m がいる場合、
\*        m を active 化して sector を分割する。
\*        Go では m は k.sectors に inactive sector として現れるため
\*        routing rView ではなく Members 全体から探す (Raft 起動協調)。
Split(n) ==
    /\ state[n] = "active"
    /\ tail[n] \in Members
    /\ \E m \in Members :
         /\ m # n /\ m # tail[n]
         /\ state[m] = "inactive"
         /\ IsBetween(m, n, tail[n])
         /\ \A m2 \in Members :
              ( m2 # n /\ m2 # tail[n] /\ state[m2] = "inactive"
                /\ IsBetween(m2, n, tail[n]) )
              => RingDist(n, m) <= RingDist(n, m2)
         /\ state' = [state EXCEPT ![m] = "active"]
         /\ tail' = [tail EXCEPT ![n] = m, ![m] = tail[n]]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {m}, ![m] = @ \cup {m}]
    /\ UNCHANGED <<rView, anyActive, churn>>

Extend(n) ==
    LET f == tail[n]
        newTail == NearestIn(n, rView[n] \cup {n}) IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \notin rView[n]
    /\ \A other \in Actives : other # n => ~IsBetween(other, n, newTail)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Merge:
\*   sector-store ビューの前方 sector head fs が
\*   routing ビューの v より手前 (fs ∈ (n, v))
\*   かつ自分のセクター外 (~ fs ∈ (n, t))
\*   = tail 以遠の sector が、routing 上はもっと遠い node まで届いていない
\*   → fs を吸収して n の tail を tail[fs] まで伸ばす
\*
\* 結果として生まれる [n, tail[fs]) が、ほかの active sector と重ならないこと
\* (= 古い sector-store ビューによる誤吸収を防ぐガード) も要求する。
\* ──────────────────────────────────────────────
Merge(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ sActives[n] \ {n} # {}
    /\ fs # n
    /\ state[fs] = "active"
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, v)
    /\ ~IsBetween(fs, n, t)
    /\ \A other \in Actives :
         other # n /\ other # fs => ~IsBetween(other, n, tail[fs])
    /\ state' = [state EXCEPT ![fs] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = tail[fs], ![fs] = -1]
    /\ sActives' = [sActives EXCEPT ![n] = @ \ {fs}, ![fs] = {}]
    /\ UNCHANGED <<rView, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Terminate:
\*   sector-store ビューの fs が自分のセクター内 (IsBetween(fs, n, t))
\*   → 重なりが発生しているので両方の sector を放棄する
\* ──────────────────────────────────────────────
Terminate(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
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
        \/ ActivateFrontward(n)
        \/ Split(n)
        \/ Extend(n)
        \/ Merge(n)
        \/ Terminate(n)

\* ──────────────────────────────────────────────
\* 公平性
\* ──────────────────────────────────────────────
Fairness ==
    \A n \in Nodes :
        /\ WF_vars(RefreshRView(n))
        /\ WF_vars(LearnSActive(n))
        /\ WF_vars(ForgetSActive(n))
        /\ WF_vars(ActivateFirst(n))
        /\ WF_vars(ActivateFrontward(n))
        /\ WF_vars(Split(n))
        /\ WF_vars(Extend(n))
        /\ WF_vars(Merge(n))
        /\ WF_vars(Terminate(n))

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

================================================================================
