-------------------------------- MODULE KvsSectorDyn --------------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様（動的メンバーシップ版）
 *
 * 概要:
 *   KvsSector.tla をベースに、ノードの参加 (Join) と離脱 (Leave) を扱う。
 *   それに伴って必要となる Split / Extend / Merge / Terminate アクションを導入する。
 *
 *   Nodes 集合は ring 上の「あり得るアドレス」の全体。
 *   現在クラスタに居るノードは Members（state[n] # "absent" な n の集合）。
 *   Members は Join / Leave で時間的に変化する。
 *
 * 抽象化:
 *   - Raft 合意・ネットワーク遅延・パケット損失は省略（即時反映）
 *   - 状態空間爆発を避けるため、Join + Leave の総回数を MaxChurn で制限
 *   - 全 active メンバーが居なくなった場合は anyActive を FALSE に戻す
 *     （Go 実装の seed では一度 TRUE なら持続するが、モデルでは liveness 確保のため緩和）
 *
 * Go 実装との対応（追加分）:
 *   - Join(n)       : ノードがクラスタに参加（hosting.Manager が member に追加）
 *   - Leave(n)      : ノードがクラスタから離脱（hosting.Manager が member から削除）
 *   - Split(n)      : operateSectors の case "Split hosting sector ..."
 *   - Extend(n)     : operateSectors の case "Extend hosting sector ..."
 *   - Merge(n)      : operateSectors の case "Merge the hosting sector ..."
 *   - Terminate(n)  : operateSectors の case "Terminate hosting sector"
 *
 * 入門メモ（追加分）:
 *   - state[n] = "absent" は「ノード n は現在クラスタに居ない」
 *   - Members は state から導出される集合（変数ではない）
 *   - SUBSET S は S のべき集合
 *   - CHOOSE x \in S : P(x) は条件 P を満たす要素を1つ決定的に選ぶ
 *)

EXTENDS Integers, FiniteSets, Sequences

\* ──────────────────────────────────────────────
\* 定数
\* ──────────────────────────────────────────────

CONSTANTS
    Nodes,           \* ring 上のアドレス候補集合（例: 0..3）
    InitialMembers,  \* 最初に居るメンバー（⊆ Nodes、非空）
    MaxChurn         \* Join + Leave の総回数の上限（状態空間制御用）

N == Cardinality(Nodes)

\* ──────────────────────────────────────────────
\* ring 上の演算（KvsSector.tla と同じ）
\* ──────────────────────────────────────────────

NextNodeID(a) == (a + 1) % N
RingDist(a, b) == (b - a + N) % N

IsBetween(x, from, to) ==
    IF from = to
    THEN TRUE
    ELSE IF from < to
         THEN from <= x /\ x < to
         ELSE from <= x \/ x < to

\* ──────────────────────────────────────────────
\* 変数
\* ──────────────────────────────────────────────

VARIABLES
    state,      \* [Nodes -> {"absent", "inactive", "active"}]
    tail,       \* [Nodes -> Nodes \cup {-1}]   ( -1 は active でない印 )
    anyActive,  \* BOOLEAN: クラスタに 1 つ以上 active sector があるか
    churn       \* これまでに発生した Join + Leave の総回数

vars == <<state, tail, anyActive, churn>>

\* ──────────────────────────────────────────────
\* 派生集合
\* ──────────────────────────────────────────────

\* 現在クラスタに居るノード集合
Members == { n \in Nodes : state[n] # "absent" }

\* 現在 active なノード集合
Actives == { n \in Nodes : state[n] = "active" }

\* p から見て ring 上で最も近い「(Members \ {p}) の要素」
\* Members \ {p} が空のときは未定義なので、呼び出し側で非空を確認すること
NextMemberAfter(p) ==
    LET others == Members \ {p}
    IN CHOOSE x \in others :
         \A y \in others : RingDist(p, x) <= RingDist(p, y)

\* ──────────────────────────────────────────────
\* 型不変条件
\* ──────────────────────────────────────────────

TypeOK ==
    /\ state \in [Nodes -> {"absent", "inactive", "active"}]
    /\ tail \in [Nodes -> (Nodes \cup {-1})]
    /\ anyActive \in BOOLEAN
    /\ churn \in 0..MaxChurn

\* ──────────────────────────────────────────────
\* 初期状態
\* ──────────────────────────────────────────────

Init ==
    /\ state = [n \in Nodes |->
                  IF n \in InitialMembers THEN "inactive" ELSE "absent"]
    /\ tail = [n \in Nodes |-> -1]
    /\ anyActive = FALSE
    /\ churn = 0

\* ──────────────────────────────────────────────
\* アクション: Join(n)
\*   absent なノードがクラスタに参加して inactive になる
\* ──────────────────────────────────────────────
Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1

\* ──────────────────────────────────────────────
\* アクション: Leave(n)
\*   ノードがクラスタから離脱。自分の sector は即時消滅。
\*   最後の active メンバーが抜けた場合は anyActive を FALSE に戻す
\*   （これは Go 実装からの抽象化。詳細はファイル先頭コメント参照）。
\* ──────────────────────────────────────────────
Leave(n) ==
    /\ churn < MaxChurn
    /\ state[n] # "absent"
    /\ Cardinality(Members) > 1   \* 最後の 1 人は離脱させない（空クラスタ防止）
    /\ state' = [state EXCEPT ![n] = "absent"]
    /\ tail' = [tail EXCEPT ![n] = -1]
    /\ anyActive' = IF state[n] = "active" /\ Cardinality(Actives) = 1
                    THEN FALSE
                    ELSE anyActive
    /\ churn' = churn + 1

\* ──────────────────────────────────────────────
\* アクション: ActivateFirst(n)
\*   全 member が inactive のときに、誰か 1 人が最初に active になる
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ state' = [state EXCEPT ![n] = "active"]
    /\ tail' = [tail EXCEPT ![n] =
                  IF Cardinality(Members) = 1
                  THEN n                  \* 1 人だけ: 全周
                  ELSE NextMemberAfter(n)]
    /\ anyActive' = TRUE
    /\ UNCHANGED churn

\* ──────────────────────────────────────────────
\* アクション: ActivateFrontward(n)
\*   active な n が、tail 先（= ring-next member）の inactive ノードを起こす
\* ──────────────────────────────────────────────
ActivateFrontward(n) ==
    LET f == tail[n] IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \in Members
    /\ state[f] = "inactive"
    /\ f = NextMemberAfter(n)   \* tail が実際の ring-next と一致（frontwardNodeMatch）
    /\ state' = [state EXCEPT ![f] = "active"]
    /\ tail' = [tail EXCEPT ![f] =
                  IF Cardinality(Members) = 1
                  THEN f
                  ELSE NextMemberAfter(f)]
    /\ UNCHANGED <<anyActive, churn>>

\* ──────────────────────────────────────────────
\* アクション: Split(n)
\*   n の sector [n, tail[n]) の内側に新メンバー m （inactive）が出現した。
\*   n の tail を m に縮め、m を active にして m の tail を元の tail[n] にする。
\*   Go 実装の "Split hosting sector and make frontward next sector active" に対応。
\* ──────────────────────────────────────────────
Split(n) ==
    /\ state[n] = "active"
    /\ tail[n] \in Members         \* tail 先がメンバー（absent は Extend が担当）
                                   \* 全周 (tail[n] = n) もこの条件を満たす
    /\ \E m \in Members :
         /\ m # n
         /\ m # tail[n]            \* tail 先のノードそのもの（match 状態）は対象外
         /\ state[m] = "inactive"
         /\ IsBetween(m, n, tail[n])    \* 全周なら IsBetween は常に TRUE
         \* m は (n, tail[n]) の中で n に最も近い候補
         /\ \A m2 \in Members :
              ( m2 # n /\ m2 # tail[n] /\ state[m2] = "inactive"
                /\ IsBetween(m2, n, tail[n]) )
              => RingDist(n, m) <= RingDist(n, m2)
         /\ state' = [state EXCEPT ![m] = "active"]
         /\ tail' = [tail EXCEPT ![n] = m, ![m] = tail[n]]
    /\ UNCHANGED <<anyActive, churn>>

\* ──────────────────────────────────────────────
\* アクション: Extend(n)
\*   n の tail が absent ノードを指している（離脱）→ 次のメンバーまで伸ばす。
\*   Go 実装の "Extend hosting sector" に対応。
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n] IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \notin Members        \* tail 先のノードが居ない
    /\ tail' = [tail EXCEPT ![n] =
                  IF Cardinality(Members) = 1
                  THEN n                       \* 自分しか居ない: 全周
                  ELSE NextMemberAfter(f)]     \* 次に居るメンバーまで
    /\ UNCHANGED <<state, anyActive, churn>>

\* ──────────────────────────────────────────────
\* アクション: Merge(n)
\*   n の tail 先 f が active だが、間に他のメンバーが居なくなり
\*   f の sector を吸収できる状況。f を inactive に戻し、n の tail を
\*   f の元 tail にする。
\*   Go 実装の "Merge the hosting sector with frontward sector" に対応。
\*
\*   発火条件（このモデル）:
\*     - state[n] = active, state[f] = active, f = tail[n], f # n
\*     - n と f の間に他のメンバーは居ない（f は n の ring-next member）
\*     - ただし「単に隣接が active」だけでは発火させない:
\*       f の tail が n と一致 = 全周を 2 つで分けている状態のときは Merge しない
\*       （安定状態を壊さない）
\*     - f が absent になる訳ではないので、これは「sector としては 1 つに統合」
\*       するだけ。f は inactive に戻る。
\* ──────────────────────────────────────────────
Merge(n) ==
    LET f == tail[n] IN
    /\ state[n] = "active"
    /\ f # n
    /\ f \in Members
    /\ state[f] = "active"
    /\ f = NextMemberAfter(n)        \* n と f は隣接
    \* Merge は「過剰に分かれている」状況を解消するためなので、
    \* このモデル（即時反映）では Merge が必要となる定常的状況は発生しない。
    \* TLC で reachable にしないため、ガードに常偽の条件を入れて発火を抑止する。
    \* （将来、ネットワーク遅延などを入れて Merge 必要状況をモデル化する余地のため
    \*   アクション自体は残しておく）
    /\ FALSE
    /\ state' = [state EXCEPT ![f] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = tail[f], ![f] = -1]
    /\ UNCHANGED <<anyActive, churn>>

\* ──────────────────────────────────────────────
\* アクション: Terminate(n)
\*   active な n が自分の sector を放棄して inactive に戻る。
\*   Go 実装の "Terminate hosting sector" に対応。
\*
\*   発火条件（このモデル）:
\*     - 通常は不要（atomic で sector 状態を更新するので孤立 sector が生じない）。
\*     - 一応アクションとして定義しておくが、Merge 同様 ガードで抑止する。
\* ──────────────────────────────────────────────
Terminate(n) ==
    /\ state[n] = "active"
    /\ FALSE   \* 抑止（理由は上記コメント参照）
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = -1]
    /\ anyActive' = IF Cardinality(Actives) = 1 THEN FALSE ELSE anyActive
    /\ UNCHANGED churn

\* ──────────────────────────────────────────────
\* 遷移関係
\* ──────────────────────────────────────────────

Next ==
    \E n \in Nodes :
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
\*   修復系アクションには WF を付ける（永遠にサボらない）。
\*   Join / Leave は環境イベント扱いなので公平性を付けない。
\* ──────────────────────────────────────────────
Fairness ==
    \A n \in Nodes :
        /\ WF_vars(ActivateFirst(n))
        /\ WF_vars(ActivateFrontward(n))
        /\ WF_vars(Split(n))
        /\ WF_vars(Extend(n))

Spec == Init /\ [][Next]_vars /\ Fairness

\* ──────────────────────────────────────────────
\* 安全性 (Safety)
\* ──────────────────────────────────────────────

\* active な sector 同士の区間 [head, tail) は重ならない
NoOverlap ==
    \A a, b \in Nodes :
        ( a # b /\ state[a] = "active" /\ state[b] = "active" )
        => ~( IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]) )

\* active な sector の tail は妥当な位置にある
ValidRange ==
    \A n \in Nodes :
        state[n] = "active" =>
            /\ tail[n] \in Nodes
            /\ ( tail[n] = n \/ IsBetween(tail[n], NextNodeID(n), n) )

\* anyActive と Actives の整合: anyActive=TRUE なら必ず誰かが active
ActiveFlagConsistent ==
    anyActive = (Actives # {})

\* ──────────────────────────────────────────────
\* 活性 (Liveness)
\*   Join/Leave が止まった (churn = MaxChurn) 後、いつかは:
\*     - 全メンバーが active
\*     - 各 active sector の tail は自分（全周）か別の active の head を指す
\* ──────────────────────────────────────────────

\* churn が止まったあと、最終的に全メンバー active になる
EventuallyAllActive ==
    <>[]( \A n \in Members : state[n] = "active" )

\* churn が止まったあと、最終的に ring が全カバーされる
EventuallyFullCoverage ==
    <>[]( \A n \in Nodes :
            state[n] = "active" =>
                ( tail[n] = n \/ state[tail[n]] = "active" ) )

================================================================================
