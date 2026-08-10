----------------------------- MODULE KvsSectorLocalView -----------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   オーバーラップ判定ガードのローカル視点化（README TODO-2 の再スコープ版）
 *
 * 動機:
 *   README TODO-2「stale active レプリカの掃除とガード緩和の検証」の原文は
 *   「ForgetSActive を raft ゲート付きにする」ことを検証項目としていたが、
 *   2026-07-25 の調査 (README TODO-1 の副産物) で以下が判明した:
 *     - TODO-2 が前提としていた「skip 1 の永久発動」は 2026-07-10 の
 *       tail 切り詰め修正 (KvsSectorLeftover.tla の ClipActivationTail) で
 *       既に解消されている。
 *     - TLA+ の sActives/ForgetSActive（gossip 的な「学習→忘却」）に対応する
 *       構造は Go には存在しない。Go の k.sectors は raft メンバーシップと
 *       いう構造化イベントでのみ増減し、中間的な「stale だが見えている」
 *       状態を持たない。TODO-2 原文の懸念はモデルの抽象と実装のズレが
 *       原因だった可能性が高い。
 *     - 一方で、KvsSectorLeftover.tla を含む全既存モデルのオーバーラップ
 *       ガード (CommitActivate の blockers、Extend、ProposeMerge、
 *       TerminateA/B の検出) は例外なく大域的な ActiveSectors を使っており、
 *       これは Go の実態（hasActiveSectorHeadInRange・activateHostingSector の
 *       skip 3・getFrontwardCondition が全て自ノードの k.sectors だけを見る、
 *       kvs.go:786-804, 1192-1202, 743-780）とは異なる理想化である。
 *       「ローカル観測に基づくガードの安全性」というモデル上の検証対象は
 *       これまで一度も設定されていなかった。
 *
 *   本モデルは TODO-2 を「ForgetSActive の raft ゲート化」ではなく、
 *   「オーバーラップ判定ガードを大域的 ActiveSectors からローカルな
 *   sActives[n]（Go の k.sectors 相当）に置き換えても NoOverlap 系の
 *   safety・liveness が保たれるか」として再スコープし検証する。
 *
 * KvsSectorLeftover.tla からの差分:
 *   - CONSTANT LocalViewGuards を追加。
 *     FALSE: 全ガードが大域的 ActiveSectors を使う（KvsSectorLeftover.tla と
 *       完全に同一の意味論。Phase 0 の健全性チェック用 — 配線ミスがないか、
 *       既に検証済みの結果を再現できるかを確認する）。
 *     TRUE: 各ガードは「判定を下すノード自身の」ローカルな sActives を使う
 *       （EffectiveView(n) 参照）。
 *   - EffectiveView(n) == IF LocalViewGuards THEN sActives[n] ELSE ActiveSectors
 *     を導入し、判定主体ノードの sActives を代入する形で各ガードを書き換えた:
 *       - ActivateFirst(n): EffectiveView(n)（n 自身の判定）
 *       - CommitActivate(n): EffectiveView(f)（Go では受信側 f が
 *         sectorActivate ハンドラ内で自分の k.sectors を見て判定する。
 *         kvs.go:911-944 の sectorActivate → activateHostingSector を参照。
 *         提案者 n の視点ではなく受信者 f の視点を使うのが本モデルの要点）
 *       - Extend(n) / ProposeMerge(n): EffectiveView(n)（n 自身の判定）
 *       - TerminateB(n): EffectiveView(n) から候補を選ぶが、選んだ other が
 *         今も本当に ActiveSectors に属することは別途明示的に確認する
 *         （ローカル視点が stale で「もう存在しない」ものを指していた場合に
 *         意味不明な遷移にならないための整合性条件であり、ガードを弱める
 *         ものではない。TerminateA の fs \in ActiveSectors チェックは
 *         KvsSectorLeftover.tla のまま = 「対象が本当に存在するか」という
 *         raft 適用時点の真実確認であり、検出トリガー自体ではないため変更
 *         不要）。
 *   - LocalViewGuards=FALSE のとき、本モジュールは KvsSectorLeftover.tla と
 *     意味論的に完全に等価になるよう配線している（Phase 0 で確認）。
 *
 * 検証フェーズ:
 *   Phase 0 (配線健全性): LocalViewGuards = FALSE
 *     → KvsSectorLeftover.tla Phase L2 相当の結果 (safety + 全 liveness 成立)
 *       を再現できることを確認する。
 *   Phase 1 (TODO-2 本題): LocalViewGuards = TRUE
 *     → ローカル視点の学習遅延 (LearnSActive の WF 待ち) が原因で、
 *       生きている active セクターと重なる activate / extend / merge を
 *       許してしまわないか (safety)。許してしまっても TerminateA/B が
 *       最終的に解消するか (EventuallyNoOverlap)。
 *
 * モデルの抽象度の限界（KvsSectorLeftover.tla を継承）:
 *   - leftover のレプリカ配置・quorum は表現しない。
 *   - CommitActivate のガード/切り詰めは commit 時に評価される。
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,
    InitialMembers,
    MaxChurn,
    EnableRelease,
    PermissiveRelease,
    ClipActivationTail,
    LocalViewGuards     \* TRUE: オーバーラップガードを判定主体のローカル sActives で評価する

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
    mergeLock

vars == <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

Members == { n \in Nodes : state[n] \in {"inactive", "active"} }
Actives == { n \in Nodes : state[n] = "active" }
ActiveSectors == { n \in Nodes : state[n] \in {"active", "leftover"} }

\* 判定主体ノード v の「オーバーラップ判定に使う視野」。
\* LocalViewGuards=FALSE では大域的な真実 (KvsSectorLeftover.tla と同一)、
\* TRUE では v 自身のローカル sActives (Go の k.sectors 相当)。
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

\* ──────────────────────────────────────────────
\* 環境イベント（KvsSectorLeftover.tla と同一）
\* ──────────────────────────────────────────────
RefreshRView(n) ==
    /\ n \in Members
    /\ rView[n] # Members
    /\ rView' = [rView EXCEPT ![n] = Members]
    /\ UNCHANGED <<state, tail, sActives, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

LearnSActive(n) ==
    /\ n \in Members
    /\ \E h \in ActiveSectors :
         /\ h \notin sActives[n]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

ForgetSActive(n) ==
    /\ n \in Members
    /\ \E h \in sActives[n] :
         /\ h \notin ActiveSectors
         /\ sActives' = [sActives EXCEPT ![n] = @ \ {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

Join(n) ==
    /\ churn < MaxChurn
    /\ state[n] = "absent"
    /\ state' = [state EXCEPT ![n] = "inactive"]
    /\ rView' = [rView EXCEPT ![n] = Members \cup {n}]
    /\ sActives' = [sActives EXCEPT ![n] = {}]
    /\ UNCHANGED <<tail, anyActive>>
    /\ churn' = churn + 1
    /\ mergeLock' = [x \in Nodes |-> IF mergeLock[x] = n THEN -1 ELSE mergeLock[x]]
    /\ UNCHANGED <<proposing, propTarget, propTail>>

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
    /\ UNCHANGED <<tail, anyActive, mergeLock>>

LeftoverQuorumLoss(h) ==
    /\ state[h] = "leftover"
    /\ Cardinality(Members) <= 1
    /\ state' = [state EXCEPT ![h] = "absent"]
    /\ tail' = [tail EXCEPT ![h] = -1]
    /\ mergeLock' = [mergeLock EXCEPT ![h] = -1]
    /\ UNCHANGED <<rView, sActives, anyActive, churn, proposing, propTarget, propTail>>

\* ──────────────────────────────────────────────
\* ActivateFirst（原子）: n 自身のローカル視点で blockers を判定する
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    LET t == NextInView(n)
        blockers == { h \in EffectiveView(n) : h # n /\ IsBetween(h, n, t) } IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ proposing[n] = "none"
    /\ blockers = {} \/ ClipActivationTail
    /\ LET t2 == IF blockers = {} THEN t
                 ELSE CHOOSE h \in blockers :
                        \A h2 \in blockers : RingDist(n, h) <= RingDist(n, h2) IN
       /\ state' = [state EXCEPT ![n] = "active"]
       /\ tail' = [tail EXCEPT ![n] = t2]
    /\ sActives' = [sActives EXCEPT ![n] = @ \cup {n}]
    /\ anyActive' = TRUE
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail, mergeLock>>

\* ──────────────────────────────────────────────
\* ActivateFrontward: 2 ステップ
\*   CommitActivate の blockers は「受信側 f」自身のローカル視点で判定する
\*   (kvs.go:911-944 sectorActivate → activateHostingSector の忠実な写像)。
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
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, mergeLock>>

CommitActivate(n) ==
    LET f == propTarget[n]
        ft == propTail[n]
        blockers == { h \in EffectiveView(f) : h # f /\ IsBetween(h, f, ft) } IN
    /\ proposing[n] = "activate"
    /\ f \in Members
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
    /\ UNCHANGED <<rView, churn, mergeLock>>

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
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock>>

ProposeSplit(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
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
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock>>

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
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn, mergeLock>>

\* ──────────────────────────────────────────────
\* Extend（原子）: n 自身のローカル視点で判定する
\*   (kvs.go hasActiveSectorHeadInRange の忠実な写像)
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n]
        newTail == NearestIn(n, rView[n] \cup {n}) IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ f # n
    /\ f \notin rView[n]
    /\ \A other \in EffectiveView(n) : other # n => ~IsBetween(other, n, newTail)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

\* ──────────────────────────────────────────────
\* Merge: 2 ステップ。ProposeMerge の範囲再検証は n 自身のローカル視点。
\* ──────────────────────────────────────────────
ProposeMerge(n) ==
    LET fs == FrontwardSector(n)
        v  == NextInView(n)
        t  == tail[n] IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
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
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn>>

CommitMerge(n) ==
    LET fs == propTarget[n]
        newTail == propTail[n] IN
    /\ proposing[n] = "merge"
    /\ fs \in (Members \cup ActiveSectors)
    /\ IF fs \in ActiveSectors
       THEN /\ state' = [state EXCEPT ![fs] = DestroyedState(fs)]
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
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* Terminate（原子: 修復アクション）
\*   fs / other の検出元はローカル視点だが、実在確認 (\in ActiveSectors) は
\*   常に大域的な真実で行う（raft 適用時点の冪等性チェックに相当し、
\*   ガードを弱めるものではない）。
\* ──────────────────────────────────────────────
\* NOTE (2026-07-25, KvsSectorLocalViewFail の churn=5 探索で発見):
\* fs が dangling な pending proposal (fs 自身が別の相手と何かを提案中) を
\* 持ったまま殺されると、そのまま放置すると後続の Commit 系アクションが
\* stale な proposing を使って発火し、実際には active でないノードに対して
\* 不整合な状態遷移を許してしまいうる。両側の proposing を必ずクリアする。
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
    /\ state' = [state EXCEPT ![n] = "inactive", ![fs] = DestroyedState(fs)]
    /\ tail' = [tail EXCEPT ![n] = -1, ![fs] = -1]
    /\ sActives' = [sActives EXCEPT ![n] = {}, ![fs] = {}]
    /\ proposing' = [proposing EXCEPT ![n] = "none", ![fs] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![fs] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1, ![fs] = -1]
    /\ mergeLock' = [mergeLock EXCEPT ![n] = -1, ![fs] = -1]
    /\ anyActive' = (Actives \ {n, fs} # {})
    /\ UNCHANGED <<rView, churn>>

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
         /\ state' = [state EXCEPT ![n] = "inactive", ![other] = DestroyedState(other)]
         /\ tail' = [tail EXCEPT ![n] = -1, ![other] = -1]
         /\ sActives' = [sActives EXCEPT ![n] = {}, ![other] = {}]
         /\ proposing' = [proposing EXCEPT ![n] = "none", ![other] = "none"]
         /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![other] = -1]
         /\ propTail' = [propTail EXCEPT ![n] = -1, ![other] = -1]
         /\ mergeLock' = [mergeLock EXCEPT ![n] = -1, ![other] = -1]
         /\ anyActive' = (Actives \ {n, other} # {})
    /\ UNCHANGED <<rView, churn>>

ReleaseMergeLock(fs) ==
    /\ EnableRelease
    /\ state[fs] # "absent"
    /\ mergeLock[fs] # -1
    /\ mergeLock[fs] \notin Members
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail>>

ReleaseMergeLockAny(fs) ==
    /\ PermissiveRelease
    /\ state[fs] # "absent"
    /\ mergeLock[fs] # -1
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail>>

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
        \/ LeftoverQuorumLoss(n)
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
        \/ ReleaseMergeLock(n)
        \/ ReleaseMergeLockAny(n)

Fairness ==
    \A n \in Nodes :
        /\ WF_vars(RefreshRView(n))
        /\ WF_vars(LearnSActive(n))
        /\ WF_vars(ForgetSActive(n))
        /\ WF_vars(LeftoverQuorumLoss(n))
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
