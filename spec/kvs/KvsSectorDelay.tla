-------------------------------- MODULE KvsSectorDelay --------------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   ネットワーク遅延 + Merge/Terminate を含む完全版
 *
 * 概要:
 *   KvsSectorDyn.tla をベースに「各ノードが持つメンバーシップのローカルビュー」
 *   rView[n] を導入する。rView は実際のメンバーから遅れることがあり、
 *   この遅延が原因で Merge/Terminate などの補正アクションが必要になる。
 *
 * Go 実装との対応:
 *   rView[n]            : n の k.handler.KvsGetStability() が返す近傍ノード集合
 *                          + n の k.sectors マップに見えているセクター集合
 *   NextInView(n)       : frontwardNextNodeID（routing の隣接ノード）
 *   tail[n]             : frontwardNextSector.GetHeadAddress()（active 側）
 *   RefreshView(n)      : routing/gossip でビューが更新されるイベント
 *
 * Merge / Terminate の発火条件（Go 実装より）:
 *   frontwardNodeMatch == false（= NextInView(n) # tail[n]）かつ
 *     - NextInView(n) が (n, tail[n]) の内側  → Terminate（両側）
 *     - NextInView(n) が (n, tail[n]) の外側  → Merge（吸収）
 *
 * 抽象化:
 *   - Raft 合意・パケット損失は省略（即時反映）
 *   - rView は単純な集合差分（順序付きメッセージキューは省略）
 *   - 1 ノード = 1 セクターに単純化
 *   - Join + Leave の総回数を MaxChurn で制限
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,           \* ring 上のアドレス候補
    InitialMembers,  \* 最初のメンバー（⊆ Nodes、非空）
    MaxChurn         \* Join+Leave の総回数上限

N == Cardinality(Nodes)

\* ring 演算
NextNodeID(a) == (a + 1) % N
RingDist(a, b) == (b - a + N) % N

\* x が半開区間 [from, to) に含まれるか（from = to は全周）
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
    rView,      \* [Nodes -> SUBSET Nodes]  n のローカルメンバービュー
    anyActive,  \* BOOLEAN
    churn       \* Join + Leave の総回数

vars == <<state, tail, rView, anyActive, churn>>

\* ──────────────────────────────────────────────
\* 派生集合
\* ──────────────────────────────────────────────

Members == { n \in Nodes : state[n] # "absent" }
Actives == { n \in Nodes : state[n] = "active" }

\* set 内で p から最近接の要素（ring 上の時計回り）。set \ {p} が空なら p。
NearestIn(p, set) ==
    LET others == set \ {p}
    IN IF others = {} THEN p
       ELSE CHOOSE x \in others :
              \A y \in others : RingDist(p, x) <= RingDist(p, y)

\* n の「ローカル視点での」ring-next member（= Go の frontwardNextNodeID）
NextInView(n) == NearestIn(n, rView[n])

\* 実際の ring-next member（参考用）
NextInReal(n) == NearestIn(n, Members)

\* ──────────────────────────────────────────────
\* 型不変条件
\* ──────────────────────────────────────────────
TypeOK ==
    /\ state \in [Nodes -> {"absent","inactive","active"}]
    /\ tail \in [Nodes -> (Nodes \cup {-1})]
    /\ rView \in [Nodes -> SUBSET Nodes]
    /\ anyActive \in BOOLEAN
    /\ churn \in 0..MaxChurn

\* ──────────────────────────────────────────────
\* 初期状態
\* ──────────────────────────────────────────────
Init ==
    /\ state = [n \in Nodes |-> IF n \in InitialMembers THEN "inactive" ELSE "absent"]
    /\ tail = [n \in Nodes |-> -1]
    /\ rView = [n \in Nodes |-> InitialMembers]
    /\ anyActive = FALSE
    /\ churn = 0

\* ──────────────────────────────────────────────
\* RefreshView(n)
\*   n のローカルビューを実メンバーシップと同期する。
\*   これは routing / gossip のメッセージ配送に相当する。
\*   liveness のため公平性 (WF) を要求する。
\* ──────────────────────────────────────────────
RefreshView(n) ==
    /\ n \in Members
    /\ rView[n] # Members
    /\ rView' = [rView EXCEPT ![n] = Members]
    /\ UNCHANGED <<state, tail, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Join: ノードが参加。自分のビューだけ最新化、他人のビューは古いまま。
\* ──────────────────────────────────────────────
Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ rView' = [rView EXCEPT ![n] = Members \cup {n}]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1

\* ──────────────────────────────────────────────
\* Leave: ノードが離脱。自分のビューはクリア。他人のビューは古いまま。
\* ──────────────────────────────────────────────
Leave(n) ==
    /\ churn < MaxChurn
    /\ state[n] # "absent"
    /\ Cardinality(Members) > 1
    /\ state' = [state EXCEPT ![n] = "absent"]
    /\ tail' = [tail EXCEPT ![n] = -1]
    /\ rView' = [rView EXCEPT ![n] = {}]
    /\ anyActive' = IF state[n] = "active" /\ Cardinality(Actives) = 1
                    THEN FALSE ELSE anyActive
    /\ churn' = churn + 1

\* ──────────────────────────────────────────────
\* ActivateFirst: 最初のアクティブ化
\*   tail = NextInView(n) が既存の active sector と重なる場合は発火しない
\*   （ビューが古い間は待機 → RefreshView 後に再評価）
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    LET t == NextInView(n) IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ \A other \in Actives :
         other # n => ~IsBetween(other, n, t)
    /\ state' = [state EXCEPT ![n] = "active"]
    /\ tail' = [tail EXCEPT ![n] = t]
    /\ anyActive' = TRUE
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* ActivateFrontward
\*   tail 先 f が inactive で、frontwardNodeMatch（= NextInView(n) = f）のとき起こす。
\*   f の新しい tail = NextInView(f) が既存 active sector と重なる場合は待機。
\* ──────────────────────────────────────────────
ActivateFrontward(n) ==
    LET f == tail[n]
        ft == NextInView(tail[n]) IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \in Members
    /\ state[f] = "inactive"
    /\ NextInView(n) = f                  \* frontwardNodeMatch
    /\ \A other \in Actives :
         other # f => ~IsBetween(other, f, ft)
    /\ state' = [state EXCEPT ![f] = "active"]
    /\ tail' = [tail EXCEPT ![f] = ft]
    /\ UNCHANGED <<rView, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Split: n の sector 内に「n のビューから見える」新規メンバーがいる場合
\*   Go の "Split hosting sector and make frontward next sector active" に対応
\* ──────────────────────────────────────────────
Split(n) ==
    /\ state[n] = "active"
    /\ tail[n] \in Members
    /\ \E m \in Members :
         /\ m # n /\ m # tail[n]
         /\ state[m] = "inactive"
         /\ m \in rView[n]                  \* n のビューに見えている
         /\ IsBetween(m, n, tail[n])
         /\ \A m2 \in Members :
              ( m2 # n /\ m2 # tail[n] /\ state[m2] = "inactive"
                /\ m2 \in rView[n]
                /\ IsBetween(m2, n, tail[n]) )
              => RingDist(n, m) <= RingDist(n, m2)
         /\ state' = [state EXCEPT ![m] = "active"]
         /\ tail' = [tail EXCEPT ![n] = m, ![m] = tail[n]]
    /\ UNCHANGED <<rView, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Extend: tail 先がローカルビュー上で見えなくなっている（離脱検知）
\*   → ビュー内の次のメンバーまで tail を伸ばす
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n] IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \notin rView[n]                  \* ビューから f が消えている
    /\ tail' = [tail EXCEPT ![n] = NearestIn(n, rView[n] \cup {n})]
    /\ UNCHANGED <<state, rView, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Merge: tail 先 f が active だが、ローカルビュー上の隣接ノードと f が一致しない
\*   かつ NextInView(n) が (n, f) の外側 → Go の Merge ケース
\*   f を inactive 化し、n の tail を f の元 tail まで伸ばす
\* ──────────────────────────────────────────────
Merge(n) ==
    LET f == tail[n]
        v == NextInView(n)
    IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \in Members
    /\ state[f] = "active"
    /\ v # f                              \* frontwardNodeMatch = false
    /\ v # n                              \* ビューが空でない
    /\ ~IsBetween(v, n, f)                \* v は (n, f) の外側 → Merge ケース
    /\ state' = [state EXCEPT ![f] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = tail[f], ![f] = -1]
    /\ UNCHANGED <<rView, anyActive, churn>>

\* ──────────────────────────────────────────────
\* Terminate: ローカルビュー上の隣接ノードが (n, tail) の内側にあるのに
\*   そこに想定外の active セクターがある → Go の Terminate-both ケース
\*   両方を inactive 化（n の sector を放棄、f の sector も放棄）
\*
\*   ※ Go では n の hostingSector も terminate するため、n も inactive に戻る。
\*     全 active が居なくなる場合は anyActive を FALSE に戻す。
\* ──────────────────────────────────────────────
Terminate(n) ==
    LET f == tail[n]
        v == NextInView(n)
    IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \in Members
    /\ state[f] = "active"
    /\ v # f                              \* frontwardNodeMatch = false
    /\ v # n
    /\ IsBetween(v, n, f)                 \* v は (n, f) の内側 → Terminate ケース
    /\ state' = [state EXCEPT ![n] = "inactive", ![f] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = -1, ![f] = -1]
    /\ anyActive' = (Cardinality(Actives) > 2)
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* 遷移関係
\* ──────────────────────────────────────────────
Next ==
    \E n \in Nodes :
        \/ RefreshView(n)
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
\*   RefreshView と修復系アクションには WF を付ける。
\*   Join / Leave は環境イベントなので公平性なし。
\* ──────────────────────────────────────────────
Fairness ==
    \A n \in Nodes :
        /\ WF_vars(RefreshView(n))
        /\ WF_vars(ActivateFirst(n))
        /\ WF_vars(ActivateFrontward(n))
        /\ WF_vars(Split(n))
        /\ WF_vars(Extend(n))
        /\ WF_vars(Merge(n))
        /\ WF_vars(Terminate(n))

Spec == Init /\ [][Next]_vars /\ Fairness

\* ──────────────────────────────────────────────
\* 安全性 (Safety)
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

ActiveFlagConsistent ==
    anyActive = (Actives # {})

\* ──────────────────────────────────────────────
\* 活性 (Liveness)
\*   churn が止まり、RefreshView が伝播した後、最終的に:
\*     - 全メンバーが active になる
\*     - ring がギャップなくカバーされる
\* ──────────────────────────────────────────────
EventuallyAllActive ==
    <>[]( \A n \in Members : state[n] = "active" )

EventuallyFullCoverage ==
    <>[]( \A n \in Nodes :
            state[n] = "active" =>
                ( tail[n] = n \/ state[tail[n]] = "active" ) )

================================================================================
