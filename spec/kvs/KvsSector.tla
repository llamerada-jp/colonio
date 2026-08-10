-------------------------------- MODULE KvsSector --------------------------------
(*
 * KVS セクター活性化プロトコルの TLA+ 仕様
 *
 * 概要:
 *   ノードが ring 上に並んでおり、各ノードは自分の hosting sector を持つ。
 *   最初は全て inactive。最初の1ノードだけが自発的に activate でき（seed 経由で判定）、
 *   以降は active なノードが frontward 隣接の inactive ノードを順次 activate していく。
 *   各 active sector は [head, tail) の半開区間を持ち、これらが重ならず、
 *   かつ最終的に ring 全体をカバーすることを検証する。
 *
 * 抽象化:
 *   - Raft の合意プロセスは省略（即座に反映されると仮定）
 *   - ネットワーク遅延・パケット損失は省略
 *   - sector の split/merge は省略（まず activate チェーンの正しさに集中）
 *   - sector メンバーシップ管理（hosting.Manager）は省略
 *
 * ring のアドレス空間:
 *   NodeAddr = 0..(N-1) の整数で表現。ring 上の順序は時計回りに 0 → 1 → ... → N-1 → 0。
 *   ノードのアドレスは固定で、ソート済みとする。
 *
 * TLA+ 入門メモ:
 *   - \in は「〜に属する」（集合のメンバーシップ）
 *   - \/ は OR、 /\ は AND
 *   - ' （プライム）は「次の状態での値」
 *   - UNCHANGED x は「x は次の状態で変わらない」
 *   - [x \in S |-> v] は「S の各要素 x に対して値 v を持つ関数」
 *   - EXCEPT は関数の一部だけ変える構文
 *   - <<a, b>> はタプル（順序付きペア）
 *   - CHOOSE は条件を満たす値を1つ選ぶ（決定的）
 *   - \A は「全ての」（全称量化）、\E は「ある〜が存在する」（存在量化）
 *   - SUBSET S は S のべき集合（S の全部分集合の集合）
 *   - Nat は自然数の集合
 *)

EXTENDS Integers, FiniteSets, Sequences

\* ──────────────────────────────────────────────
\* 定数: モデルチェック時に具体的な値を与える
\* ──────────────────────────────────────────────

\* Nodes: ring 上のノード集合（例: {0, 1, 2}）
\* 各ノードのアドレスはそのまま整数値として使う
CONSTANT Nodes

\* N: ノード数。Nodes = 0..(N-1) を想定
N == Cardinality(Nodes)

\* ──────────────────────────────────────────────
\* ring 上のアドレス演算ヘルパー
\* ──────────────────────────────────────────────

\* ring 上で a の次のノード（時計回り）
\* 例: N=3 のとき NextNode(2) = 0
NextNode(a) == (a + 1) % N

\* ring 上で a から b への時計回り距離
\* 例: N=4 のとき RingDist(3, 1) = 2
RingDist(a, b) == (b - a + N) % N

\* ring 上で x が半開区間 [from, to) に含まれるか
\* ring 構造なので wrap-around を考慮する
\* Go 実装の IsBetween(back, front) に対応
\* 注意: from = to のケースは「全周」を意味する（1ノードのみの場合）
IsBetween(x, from, to) ==
    IF from = to
    THEN TRUE  \* 全周カバー: 全ての x が含まれる
    ELSE IF from < to
         THEN from <= x /\ x < to         \* 通常ケース: [from, to)
         ELSE from <= x \/ x < to          \* wrap-around ケース

\* ──────────────────────────────────────────────
\* 変数
\* ──────────────────────────────────────────────

VARIABLES
    \* 各ノードの状態: "inactive" または "active"
    state,

    \* 各ノードの sector の tail アドレス（active のときのみ意味を持つ）
    \* head は常に自分自身のアドレスなので変数にしない
    \* inactive のノードは tail = -1 とする（無効値）
    tail,

    \* seed が管理する「クラスタ全体に active なノードが存在するか」フラグ
    \* Go 実装の entireState に対応
    \* TRUE = 誰か1人以上が active（EntireStateActive）
    \* FALSE = 誰も active でない（EntireStateInactive）
    anyActive

\* 全変数をまとめたタプル（UNCHANGED で使う）
vars == <<state, tail, anyActive>>

\* ──────────────────────────────────────────────
\* 型不変条件: 各変数の型を定義
\* TLC がモデルチェック中に型エラーを検出するのに使う
\* ──────────────────────────────────────────────
TypeOK ==
    /\ state \in [Nodes -> {"inactive", "active"}]
    /\ tail \in [Nodes -> -1..N-1]
    /\ anyActive \in BOOLEAN

\* ──────────────────────────────────────────────
\* 初期状態
\* ──────────────────────────────────────────────
Init ==
    /\ state = [n \in Nodes |-> "inactive"]
    /\ tail = [n \in Nodes |-> -1]
    /\ anyActive = FALSE

\* ──────────────────────────────────────────────
\* アクション: 最初のノードの自発的活性化
\* ──────────────────────────────────────────────

(*
 * ActivateFirst(n):
 *   seed に問い合わせて entireState が inactive なら、自分が最初に activate する。
 *   Go 実装の activateHostingSector(checkEntireState=true) に対応。
 *
 *   1ノードの場合: tail = head（= 自分）で全周カバー
 *   複数ノードの場合: tail = NextNode(n) で隣接ノードまでカバー
 *
 *   seed の SetKvsFirstActiveCandidate 相当のレース制御:
 *   anyActive が FALSE のときだけ許可（TLC の非決定性で1ノードが選ばれる）
 *)
ActivateFirst(n) ==
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    \* seed から EntireStateInactive が返ってくる条件
    /\ \A m \in Nodes : state[m] = "inactive"
    /\ state' = [state EXCEPT ![n] = "active"]
    /\ tail' = [tail EXCEPT ![n] = IF N = 1
                                    THEN n              \* 1ノード: 全周
                                    ELSE NextNode(n)]   \* 複数ノード: 隣接まで
    /\ anyActive' = TRUE

\* ──────────────────────────────────────────────
\* アクション: frontward ノードの活性化
\* ──────────────────────────────────────────────

(*
 * ActivateFrontward(n):
 *   active なノード n が、自分の tail（= frontward 隣接ノード f）を activate する。
 *   Go 実装の operateSectors → activateFrontwardSector に対応。
 *
 *   前提条件:
 *   - n は active
 *   - f = tail[n] は inactive
 *   - f は n の ring 上の frontward next（tail と head が一致 = case 0 に対応）
 *
 *   結果:
 *   - f が active になり、f の tail は f の frontward 隣接ノード（1ホップ先）。
 *   - それ以上の調整（隣接ノードも既に active 等）は次ステップの Extend で行う。
 *)
ActivateFrontward(n) ==
    LET f == tail[n]  \* frontward ノード
    IN
    /\ state[n] = "active"
    /\ f # n                     \* 全周カバー中でない（自分自身でない）
    /\ state[f] = "inactive"     \* frontward が未活性
    \* f を activate する
    /\ state' = [state EXCEPT ![f] = "active"]
    /\ tail' = [tail EXCEPT ![f] = IF N = 2
                                    THEN n               \* 2ノード: 互いに全周を分割
                                    ELSE NextNode(f)]    \* 3ノード以上: 次のノードまで
    /\ anyActive' = TRUE  \* 既に TRUE のはずだが冪等

\* ──────────────────────────────────────────────
\* アクション: Extend について
\* ──────────────────────────────────────────────

(*
 * Extend は本仕様では扱わない。
 *
 * 理由: 本仕様はノード離脱・split/merge を抽象化しており、
 *   ActivateFirst → ActivateFrontward のチェーンだけで
 *   各ノードが [n, NextNode(n)) の sector を持つ状態に到達する。
 *   この状況では Extend が必要となるケース（離脱ノードを飛ばす／
 *   隣接 active sector を吸収する）が論理的に発生しない。
 *
 * 将来ノード離脱や split/merge を取り込む場合は、相手 sector の
 * deactivate（Merge）と組み合わせて Extend を再導入する必要がある。
 *)

\* ──────────────────────────────────────────────
\* 全体の遷移関係
\* ──────────────────────────────────────────────

(*
 * Next: システムの1ステップ。
 * いずれかのノードが、いずれかのアクションを非決定的に実行する。
 * TLC はこの全ての組み合わせを網羅的に探索する。
 *)
Next ==
    \E n \in Nodes :
        \/ ActivateFirst(n)
        \/ ActivateFrontward(n)

\* ──────────────────────────────────────────────
\* 公平性: Liveness 検証に必要
\* ──────────────────────────────────────────────

(*
 * Fairness（公平性）:
 *   「各アクションが永遠にブロックされない」ことを保証する仮定。
 *   WF（弱い公平性）= アクションが永遠に有効なら、いつか実行される。
 *
 *   これがないと TLC は「何も起きない」実行を許容してしまい、
 *   liveness 性質（eventually 〜）が自明に偽になる。
 *)
Fairness ==
    \A n \in Nodes :
        /\ WF_vars(ActivateFirst(n))
        /\ WF_vars(ActivateFrontward(n))

\* 時相論理仕様（Spec）: 初期状態 + 遷移 + 公平性
Spec == Init /\ [][Next]_vars /\ Fairness

\* ──────────────────────────────────────────────
\* 安全性（Safety）の不変条件
\* ──────────────────────────────────────────────

(*
 * NoOverlap:
 *   全ての active なセクター対について、区間 [head, tail) が重ならない。
 *
 *   2つの半開区間 [h1, t1) と [h2, t2) が重なるとは:
 *   - h2 が [h1, t1) に含まれる、または h1 が [h2, t2) に含まれる
 *
 *   ただし同一ノードは比較しない。
 *)
NoOverlap ==
    \A a, b \in Nodes :
        (a # b /\ state[a] = "active" /\ state[b] = "active")
        =>
        ~(IsBetween(b, a, tail[a]) \/ IsBetween(a, b, tail[b]))

(*
 * NoGap:
 *   全ての active なセクターについて、その tail が指す先が:
 *   - 自分自身（1ノードで全周）、または
 *   - 別の active なノード（ギャップなく繋がっている）、または
 *   - inactive なノード（まだ拡張中）
 *
 *   最終状態では全ての tail の先が active ノードの head に一致する。
 *   これは Liveness の AllActive 到達後に成立する。
 *
 *   途中状態での不変条件: active ノードの区間は常に自分の head から始まり、
 *   tail は ring 上で head より先にある（または等しい＝全周）。
 *)
ValidRange ==
    \A n \in Nodes :
        state[n] = "active" =>
            /\ tail[n] \in Nodes
            \* tail[n] = n（全周）か、(n, n] の範囲内（= NextNode(n) から時計回りに n まで）
            /\ (tail[n] = n \/ IsBetween(tail[n], NextNode(n), n))

\* ──────────────────────────────────────────────
\* 活性（Liveness）の時相性質
\* ──────────────────────────────────────────────

(*
 * AllActive:
 *   「最終的に全てのノードが active になる」
 *   ◇（いつか） \A n \in Nodes : state[n] = "active"
 *)
AllActive ==
    <>(\A n \in Nodes : state[n] = "active")

(*
 * FullCoverage:
 *   「最終的に ring 全体がギャップなくカバーされる」
 *   全ての active ノードの tail が、別の active ノードの head（= そのノード自身）を指す。
 *)
FullCoverage ==
    <>(\A n \in Nodes :
        state[n] = "active" /\
        (tail[n] = n \/ state[tail[n]] = "active"))

\* ──────────────────────────────────────────────
\* 検証対象のまとめ
\* ──────────────────────────────────────────────
\* TLC で検証するもの:
\*   INVARIANT TypeOK       ... 型の整合性
\*   INVARIANT NoOverlap    ... active 区間の非重複
\*   INVARIANT ValidRange   ... 区間の妥当性
\*   PROPERTY  AllActive    ... 全ノード活性化（liveness）
\*   PROPERTY  FullCoverage ... 全域カバー（liveness）

================================================================================
