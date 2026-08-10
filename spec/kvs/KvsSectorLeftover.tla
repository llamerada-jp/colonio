----------------------------- MODULE KvsSectorLeftover -----------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   leftover セクター (host 死亡後もレプリカが生き残る active セクター) と
 *   activation の tail 切り詰め (ClipActivationTail) をモデル化
 *
 * 動機 (シミュレーション run 13, 2026-07-10):
 *   host が死んでも active セクターはレプリカ群 (quorum 健全) として残る。
 *   この leftover が inactive ノード f とその frontward の間にあると、
 *   f の activation は overlap ガード (Go の activateHostingSector "skip 1")
 *   で永久拒否される。一方 leftover を merge で掃除できるのは「leftover に
 *   隣接する active セクター」だけであり、その位置にいるのは f 自身。
 *   f は inactive なので merge を実行できない — 循環待ちで activation
 *   チェーンが恒久停止する (run 13: 1 セクターが 14 分 yellow、run 11 でも
 *   同型を観測)。design.md の分岐表には「hosting inactive + 範囲内に active
 *   leftover」の行が存在しない。
 *
 *   従来モデル (KvsSectorRaft / KvsSectorMergeLock) は sector = node の
 *   抽象化のため「node は死んだが sector は残る」leftover を表現できず、
 *   さらに Go の skip 1 ガード自体もモデル化されていなかった
 *   (モデルの CommitActivate は無条件適用で、重複は TerminateB が修復)。
 *
 * KvsSectorMergeLock.tla からの差分:
 *   - state[n] に "leftover" を追加: node は不在 (routing から消える) が、
 *     位置 n を head とする active セクターは残る。
 *     Members (routing に見えるノード) と ActiveSectors (セクター head) を
 *     分離する。leftover は rView から消え sActives に残る — Go の
 *     「sector store には見えるが routing にはいない」状態の写像。
 *   - LeaveLeftover(n): active ノードがセクターを残して離脱する。
 *     レプリカは他ノード上にあるという想定なので、他に active な member が
 *     残っている場合のみ発火できる (全滅はモデル外 = quorum 喪失で
 *     LocalDestroy される領域、TODO-1)。
 *   - leftover の自然消滅アクションは意図的に置かない: 実際には member の
 *     churn による quorum 喪失 → 強制破棄で消えるが、それに依存しない
 *     liveness (修復機構自身が leftover を吸収すること) を検証するため。
 *     merge (CommitMerge) と Terminate だけが leftover を破棄できる。
 *   - CommitActivate に Go の overlap ガード (skip 1) を忠実に追加:
 *     activation の対象 f と提案 tail ft の間に active セクター head が
 *     あると activation は no-op になる (commit は完了する = 受信側 skip)。
 *   - ClipActivationTail (修正本体): ガードで skip する代わりに、tail を
 *     範囲内最近傍の active head に切り詰めて activate する。
 *     [f, blocker) は重複を生まない。f が active になれば通常の merge
 *     (+ ReleaseMergeLock backstop) が leftover を吸収してチェーンが伸びる。
 *
 * 検証フェーズ (定数を切り替えて実行):
 *   Phase L1 (バグ再現): ClipActivationTail = FALSE
 *     → EventuallyAllActive / EventuallyNoLeftover の違反を確認する。
 *       想定系列: 全 member が active 化 → LeaveLeftover(2) → Leave(1) →
 *       Join(1) (leftover の backward に inactive member が入る) →
 *       0 の CommitActivate(1) が blockers={2} で永久 skip。
 *   Phase L2 (修正確認): ClipActivationTail = TRUE
 *     → 全 liveness (EventuallyNoLeftover 含む) が成立する。
 *   Phase L3 (誤検知 safety): + PermissiveRelease = TRUE, safety のみ
 *     → 切り詰め activation と無条件 lock 解放が併発しても safety 維持。
 *
 * モデルの抽象度の限界:
 *   - leftover のレプリカ配置・quorum は表現しない (存在するかしないかのみ)。
 *   - Go の「skip 2」(frontward node のセクターレプリカが手元に必要) は
 *     レプリカ概念がないため表現しない。
 *   - CommitActivate のガード/切り詰めは commit 時に評価される。Go では
 *     受信側が評価してから自グループに propose するため、評価と commit の
 *     間に世界が変わりうる — その差は既存の Propose/Commit レースと同型で、
 *     生じる重複は TerminateB が修復する (KvsSectorRaft で検証済みの構図)。
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,
    InitialMembers,
    MaxChurn,
    EnableRelease,      \* TRUE: ReleaseMergeLock (mergeBy 解放) を有効にする
    PermissiveRelease,  \* TRUE: 保持者生存中でも解放できる (誤検知 safety 検証用)
    ClipActivationTail  \* TRUE: skip 1 の代わりに tail を切り詰めて activate する (修正)

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
    \* state[n]:
    \*   "absent"   : node もセクターも存在しない
    \*   "inactive" : node は存在、hosting セクターは inactive
    \*   "active"   : node は存在、hosting セクターは active
    \*   "leftover" : node は不在 (routing から消える) だが、位置 n を head と
    \*                する active セクターがレプリカ群として残っている
    state,
    tail,        \* [Nodes -> Nodes \cup {-1}]  active/leftover セクターの tail
    rView,       \* [Nodes -> SUBSET Nodes]     routing 由来の近傍 (Members のみ)
    sActives,    \* [Nodes -> SUBSET Nodes]     sector-store 由来の active head 集合
    anyActive,
    churn,
    proposing,   \* [Nodes -> {"none","activate","splitPre","splitCommit","merge"}]
    propTarget,  \* [Nodes -> Nodes \cup {-1}]
    propTail,    \* [Nodes -> Nodes \cup {-1}]
    mergeLock    \* [Nodes -> Nodes \cup {-1}]  Go の Sector.mergeBy

vars == <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

\* ──────────────────────────────────────────────
\* 派生集合
\*   Members       : routing に見えるノード (leftover を含まない)
\*   ActiveSectors : active なセクターの head (leftover を含む)
\* ──────────────────────────────────────────────
Members == { n \in Nodes : state[n] \in {"inactive", "active"} }
\* Actives: active な member。seed の EntireState は生きているノードの
\* hosting セクター報告から作られるため、leftover を含まない。
Actives == { n \in Nodes : state[n] = "active" }
\* ActiveSectors: overlap 判定・merge 対象になる active セクターの head
\* (leftover を含む)。
ActiveSectors == { n \in Nodes : state[n] \in {"active", "leftover"} }

NearestIn(p, set) ==
    LET others == set \ {p}
    IN IF others = {} THEN p
       ELSE CHOOSE x \in others :
              \A y \in others : RingDist(p, x) <= RingDist(p, y)

NextInView(n) == NearestIn(n, rView[n])
FrontwardSector(n) == NearestIn(n, sActives[n] \ {n})

\* セクター破棄後の状態: leftover は node 不在なので absent へ、
\* member のセクターは inactive へ (再作成待ち)
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
\* 環境イベント
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
    \* 旧 incarnation が保持していた lock を解放する: 本モデルはノード ID を
    \* 再利用するため、rejoin した n が「lock 保持者として生存している」ように
    \* 見えて死亡検知の解放が永久に発火しない artifact が生じる。Go では
    \* 新 incarnation は別 ID であり、旧 ID の stale lock はタイムアウト
    \* (mergeReleaseDuration) で解放される — その解放を Join 時点に前倒しで表現。
    /\ mergeLock' = [x \in Nodes |-> IF mergeLock[x] = n THEN -1 ELSE mergeLock[x]]
    /\ UNCHANGED <<proposing, propTarget, propTail>>

\* Leave: node がセクターごと消える (レプリカ群も同時に死ぬ / 即座に quorum
\* 喪失で破棄されるケース)。KvsSectorMergeLock と同じく merge 提案中の離脱可。
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

\* LeaveLeftover: active ノードがセクターを残して離脱する (host 死亡、
\* レプリカ群は quorum 健全)。leftover は「host 以外にレプリカの quorum を
\* 保てるだけの member が残っている」状況の抽象化なので、他に member が
\* 2 つ以上 (うち 1 つは active) 残っている場合のみ発火できる。
\* member が 1 つしか残らない離脱は quorum 喪失であり、レプリカは
\* LocalDestroy (checkQuorumLoss) が掃除する — その経路は Leave (セクター
\* ごと消える) が表現する。
\* セクターの複製状態 (tail, mergeLock[n]) は保持される。
\* n が他セクターに保持している lock (mergeLock[x] = n) も残る (stale lock、
\* ReleaseMergeLock が解放する)。
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

\* LeftoverQuorumLoss: 残存 member が 1 つ以下になった leftover はレプリカの
\* quorum を維持できない (レプリカは member 上にしか存在しない)。
\* Go では checkQuorumLoss (leaderless 30s) が各レプリカをローカル破棄する。
\* 決定的に発火する実機構なので WF を付ける。member が 2 つ以上ある間は
\* 発火しない = 「quorum 健全な leftover は自然消滅に頼れない」というこの
\* モデルの主眼はそのまま。
LeftoverQuorumLoss(h) ==
    /\ state[h] = "leftover"
    /\ Cardinality(Members) <= 1
    /\ state' = [state EXCEPT ![h] = "absent"]
    /\ tail' = [tail EXCEPT ![h] = -1]
    /\ mergeLock' = [mergeLock EXCEPT ![h] = -1]
    /\ UNCHANGED <<rView, sActives, anyActive, churn, proposing, propTarget, propTail>>

\* ──────────────────────────────────────────────
\* ActivateFirst（原子）
\*   anyActive は leftover を含む ActiveSectors を追跡するため、leftover が
\*   残っている間は発火しない (seed は leftover のレプリカ報告を active と
\*   みなす、という保守的な仮定)。
\* ──────────────────────────────────────────────
ActivateFirst(n) ==
    LET t == NextInView(n)
        blockers == { h \in ActiveSectors : h # n /\ IsBetween(h, n, t) } IN
    /\ state[n] = "inactive"
    /\ anyActive = FALSE
    /\ \A m \in Members : state[m] = "inactive"
    /\ proposing[n] = "none"
    \* Go では ActivateFirst も activateHostingSector を通るため、leftover の
    \* overlap ガード (skip 1) と tail 切り詰めが同様に適用される。
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
\*   KvsSectorMergeLock からの変更点: CommitActivate に Go の overlap ガード
\*   (activateHostingSector の "skip 1") を追加した。
\*   - blockers # {} かつ ClipActivationTail = FALSE:
\*     activation は no-op (commit は完了する = 受信側 skip、Go と同じ)。
\*     backward は WF により ProposeActivate を再試行し続ける。
\*   - blockers # {} かつ ClipActivationTail = TRUE (修正):
\*     tail を f から最近傍の blocker head に切り詰めて activate する。
\*     [f, blocker) は blocker と重複しない。f が active になれば通常の
\*     merge が blocker (leftover) を吸収してチェーンが伸びる。
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
        blockers == { h \in ActiveSectors : h # f /\ IsBetween(h, f, ft) } IN
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

\* ──────────────────────────────────────────────
\* AbortProposal（KvsSectorMergeLock と同じ。lock は解放しない）
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
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock>>

\* ──────────────────────────────────────────────
\* Split: 2 ステップ（KvsSectorMergeLock と同じ: mergeLock ガードあり）
\* ──────────────────────────────────────────────
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
\* Extend（原子）
\*   leftover が (n, newTail) 内にあると発火しない (Go の
\*   hasActiveSectorHeadInRange と同じ)。leftover の掃除は merge が担う。
\* ──────────────────────────────────────────────
Extend(n) ==
    LET f == tail[n]
        newTail == NearestIn(n, rView[n] \cup {n}) IN
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ f # n
    /\ f \notin rView[n]
    /\ \A other \in ActiveSectors : other # n => ~IsBetween(other, n, newTail)
    /\ tail' = [tail EXCEPT ![n] = newTail]
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

\* ──────────────────────────────────────────────
\* Merge: 2 ステップ（KvsSectorMergeLock と同じ + fs が leftover でもよい）
\*   leftover の掃除はこの経路が主役: fs = FrontwardSector(n) は sActives
\*   由来なので leftover を含む。leftover は rView にいないため
\*   v # fs (frontwardNodeMatch = false) が自然に成立する。
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
    /\ \A other \in ActiveSectors :
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
    \* fs がまだ active なら吸収 (leftover なら absent へ)。
    \* 既に inactive なら tail 拡張のみ（冪等的）。
    /\ IF fs \in ActiveSectors
       THEN /\ state' = [state EXCEPT ![fs] = DestroyedState(fs)]
            /\ tail' = [tail EXCEPT ![n] = newTail, ![fs] = -1]
            /\ sActives' = [sActives EXCEPT ![n] = @ \ {fs}, ![fs] = {}]
            /\ proposing' = [proposing EXCEPT ![n] = "none", ![fs] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![fs] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1, ![fs] = -1]
            /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
            \* n (merge した側) は active のまま残る
            /\ anyActive' = TRUE
       ELSE /\ tail' = [tail EXCEPT ![n] = newTail]
            /\ proposing' = [proposing EXCEPT ![n] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1]
            /\ UNCHANGED <<state, sActives, anyActive, mergeLock>>
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* Terminate（原子: 修復アクション。leftover の破棄は absent へ）
\* ──────────────────────────────────────────────
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
    /\ mergeLock' = [mergeLock EXCEPT ![n] = -1, ![fs] = -1]
    /\ anyActive' = (Actives \ {n, fs} # {})
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail>>

TerminateB(n) ==
    /\ state[n] = "active"
    /\ proposing[n] = "none"
    /\ tail[n] \in Nodes
    /\ tail[n] \in Members
    /\ state[tail[n]] = "active"
    /\ \E other \in ActiveSectors :
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

\* ──────────────────────────────────────────────
\* ReleaseMergeLock（KvsSectorMergeLock と同じ。leftover のセクターの
\* stale lock も解放できる — leftover を merge で掃除するために必要）
\* ──────────────────────────────────────────────
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

\* 修復機構 (merge / terminate) が leftover を最終的に吸収する
EventuallyNoLeftover ==
    <>[]( \A h \in Nodes : state[h] # "leftover" )

================================================================================
