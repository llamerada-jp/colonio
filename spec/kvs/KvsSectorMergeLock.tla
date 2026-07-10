----------------------------- MODULE KvsSectorMergeLock -----------------------------
(*
 * KVS セクタープロトコルの TLA+ 仕様
 *   merge 排他ロック (Go 実装の Sector.mergeBy) とその解放 (ReleaseMerge) をモデル化
 *
 * 動機 (シミュレーション run 11, 2026-07-09):
 *   Go 実装の prepare_merge は対象セクターの複製状態 mergeBy で排他されるが、
 *   mergeBy を解放する経路が存在しない。preparer が merge 完了前に離脱すると
 *   後任の merge / 対象セクターの split (PreCommitSplit) が永久拒否され、
 *   activation チェーンが恒久停止する。
 *
 *   KvsSectorRaft.tla ではこのバグを表現できなかった:
 *   (1) mergeBy に相当する target 側ロックが存在しない (ProposeMerge は
 *       overlap ガードのみで排他)。
 *   (2) Leave(n) が proposing[n] = "none" を要求するため、
 *       「prepare を commit した後、merge 完了前に preparer が死ぬ」
 *       という系列そのものが到達不能 (TODO-1 の既知の制約)。
 *
 * KvsSectorRaft.tla からの差分:
 *   - mergeLock[fs] : fs のセクターに prepare_merge を commit したノード。
 *     Go の Sector.mergeBy に対応 (fs = セクターの head ノード)。
 *   - ProposeMerge(n) は mergeLock[fs] \in {-1, n} を要求し、lock を取得する。
 *     (Go: PrepareMerge は mergeBy が nil か自分のときだけ成功)
 *   - ProposeSplit(n) は mergeLock[n] = -1 を要求する。
 *     (Go: PreCommitSplit は mergeBy が set されていると拒否, sector.go)
 *   - セクター破棄 (CommitMerge 吸収 / TerminateA / TerminateB / Leave) は
 *     破棄されるセクターの lock を消す。(Go: セクター破棄で mergeBy も消え、
 *     再作成セクターは mergeBy=nil で始まる)
 *   - AbortProposal は lock を解放しない。(Go: abort 時に proposer 側の
 *     pending フラグは消えるが commit 済みの mergeBy はそのまま = バグの忠実な写像)
 *   - Leave(n) は proposing[n] = "merge" でも離脱可能 (バグの再現に必須)。
 *     n が「保持している」lock (mergeLock[x] = n) は残る = stale lock。
 *     activate / split の提案中の離脱は quorum 喪失の全面モデル化 (TODO-1) の
 *     スコープなので、ここでは緩和しない。
 *   - ReleaseMergeLock(fs) : 修正本体。lock 保持者が Members にいないとき、
 *     グループの生存メンバーが raft 経由で lock を解放する。
 *     (Go: ReleaseMerge 提案。apply は「mergeBy = 保持者のときだけ nil に戻す」
 *      の CAS で決定的)
 *   - ReleaseMergeLockAny(fs) : 誤検知 variant。保持者の生死に関係なく解放できる。
 *     公平性なし (発火してもしなくてもよい)。実装側の解放ゲート (死亡検知 +
 *     タイムアウト) がどれだけ誤発動しても safety が保たれることの検証用。
 *
 * 検証フェーズ (README「シミュレーション run 11」の項参照):
 *   Phase 1 (バグ再現): EnableRelease = FALSE
 *     → EventuallyAllActive が違反する counterexample を確認する。
 *       想定系列: p が fs に ProposeMerge (lock 取得) → p が Leave →
 *       joiner が (fs, tail[fs]) 内に Join → fs の ProposeSplit が
 *       stale lock で永久ブロック → joiner が active になれない。
 *   Phase 2 (修正確認): EnableRelease = TRUE, PermissiveRelease = FALSE
 *     → 全 liveness が成立する。
 *   Phase 3 (誤検知 safety): EnableRelease = TRUE, PermissiveRelease = TRUE
 *     → safety invariant のみ検証 (公平性のない解放は liveness を主張できない)。
 *
 * モデルの抽象度の限界 (Go 側の回帰テストで担保するもの):
 *   - 本モデルは sector = node で host とレプリカを区別しないため、
 *     run 11 で観測した「host 死亡後の leftover セクターへの merge が塞がる」
 *     形は表現できない (leftover が存在できない)。stale lock が split を塞ぐ
 *     形で同一バグを再現する。
 *   - セクターの世代 (sectorID) も区別しないため、破棄→再作成をまたぐ
 *     lock の帰属は Go 実装より粗い。
 *   - 「保持者が生存しているが二度と merge を再試行しない」stale lock は
 *     死亡検知では解放されない。実装はタイムアウトを併用して解放し、
 *     その安全性は Phase 3 (無条件解放でも safety 維持) が包含する。
 *)

EXTENDS Integers, FiniteSets

CONSTANTS
    Nodes,
    InitialMembers,
    MaxChurn,
    EnableRelease,     \* TRUE: ReleaseMergeLock (修正) を有効にする
    PermissiveRelease  \* TRUE: 保持者生存中でも解放できる (誤検知 safety 検証用)

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
    proposing,   \* [Nodes -> {"none","activate","splitPre","splitCommit","merge"}]
    propTarget,  \* [Nodes -> Nodes \cup {-1}]
    propTail,    \* [Nodes -> Nodes \cup {-1}]
    \* mergeLock[fs]: fs のセクターに prepare_merge を commit したノード (-1 = なし)。
    \* Go の Sector.mergeBy に対応。セクターの複製状態なので、変更は必ず
    \* raft commit (= 本モデルではアクション) を通る。
    mergeLock    \* [Nodes -> Nodes \cup {-1}]

vars == <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

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
    /\ \E h \in Actives :
         /\ h \notin sActives[n]
         /\ sActives' = [sActives EXCEPT ![n] = @ \cup {h}]
    /\ UNCHANGED <<state, tail, rView, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

ForgetSActive(n) ==
    /\ n \in Members
    /\ \E h \in sActives[n] :
         /\ state[h] # "active"
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
    /\ UNCHANGED <<proposing, propTarget, propTail, mergeLock>>

\* Leave: KvsSectorRaft からの変更点。
\*   proposing[n] = "merge" のままでも離脱できる (= prepare_merge を commit した
\*   後、merge 完了前に preparer が死ぬ = run 11 のバグの系列)。
\*   n 自身のセクターの lock (mergeLock[n]) はセクターと共に消えるが、
\*   n が他セクターに保持している lock (mergeLock[x] = n) は残る = stale lock。
Leave(n) ==
    /\ churn < MaxChurn
    /\ state[n] # "absent"
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

\* ──────────────────────────────────────────────
\* ActivateFirst（原子）
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
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail, mergeLock>>

\* ──────────────────────────────────────────────
\* ActivateFrontward: 2 ステップ（KvsSectorRaft と同じ）
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
        ft == propTail[n] IN
    /\ proposing[n] = "activate"
    /\ f \in Members
    /\ IF state[f] = "inactive"
       THEN /\ state' = [state EXCEPT ![f] = "active"]
            /\ tail' = [tail EXCEPT ![f] = ft]
            /\ sActives' = [sActives EXCEPT ![n] = @ \cup {f}, ![f] = @ \cup {f}]
            /\ anyActive' = TRUE
       ELSE UNCHANGED <<state, tail, sActives, anyActive>>
    /\ proposing' = [proposing EXCEPT ![n] = "none"]
    /\ propTarget' = [propTarget EXCEPT ![n] = -1]
    /\ propTail' = [propTail EXCEPT ![n] = -1]
    /\ UNCHANGED <<rView, churn, mergeLock>>

\* ──────────────────────────────────────────────
\* AbortProposal
\*   注意: lock は解放しない。Go 実装の abort (waitProposal timeout /
\*   状況変化によるリトライ断念) は commit 済みの mergeBy を消さない。
\*   保持者が生きていれば次の PrepareMerge は mergeLock \in {-1, n} で
\*   通るので、自力再試行は可能 (Go と同じ)。
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
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, mergeLock>>

\* ──────────────────────────────────────────────
\* Split: 2 ステップ
\*   KvsSectorRaft からの変更点: mergeLock[n] = -1 を要求。
\*   Go: PreCommitSplit は「cannot pre-commit split while merge is being
\*   prepared by X」で拒否する (sector.go)。stale lock はここを永久に塞ぐ。
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
    /\ UNCHANGED <<state, rView, sActives, anyActive, churn, proposing, propTarget, propTail, mergeLock>>

\* ──────────────────────────────────────────────
\* Merge: 2 ステップ
\*   KvsSectorRaft からの変更点: ProposeMerge が mergeLock[fs] \in {-1, n} を
\*   要求し、lock を取得する。ProposeMerge は Go の「PrepareMerge が commit
\*   された」時点に対応する (mergeBy が複製状態にセットされる)。
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
    /\ mergeLock[fs] \in {-1, n}
    /\ v # n
    /\ v # fs
    /\ IsBetween(fs, n, v)
    /\ ~IsBetween(fs, n, t)
    /\ \A other \in Actives :
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
    /\ fs \in Members
    \* fs がまだ active なら吸収。既に inactive なら tail 拡張のみ（冪等的）。
    \* 吸収 = fs のセクター破棄なので lock も消える (Go: セクター破棄で
    \* mergeBy も消え、再作成セクターは mergeBy=nil で始まる)。
    /\ IF state[fs] = "active"
       THEN /\ state' = [state EXCEPT ![fs] = "inactive"]
            /\ tail' = [tail EXCEPT ![n] = newTail, ![fs] = -1]
            /\ sActives' = [sActives EXCEPT ![n] = @ \ {fs}, ![fs] = {}]
            /\ proposing' = [proposing EXCEPT ![n] = "none", ![fs] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![fs] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1, ![fs] = -1]
            /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
            /\ anyActive' = IF Cardinality(Actives) <= 1 THEN FALSE ELSE anyActive
       ELSE /\ tail' = [tail EXCEPT ![n] = newTail]
            /\ proposing' = [proposing EXCEPT ![n] = "none"]
            /\ propTarget' = [propTarget EXCEPT ![n] = -1]
            /\ propTail' = [propTail EXCEPT ![n] = -1]
            /\ UNCHANGED <<state, sActives, anyActive, mergeLock>>
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* Terminate（原子: 修復アクション）
\*   破棄されるセクターの lock も消える。
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
    /\ state' = [state EXCEPT ![n] = "inactive", ![fs] = "inactive"]
    /\ tail' = [tail EXCEPT ![n] = -1, ![fs] = -1]
    /\ sActives' = [sActives EXCEPT ![n] = {}, ![fs] = {}]
    /\ mergeLock' = [mergeLock EXCEPT ![n] = -1, ![fs] = -1]
    /\ anyActive' = (Cardinality(Actives) > 2)
    /\ UNCHANGED <<rView, churn, proposing, propTarget, propTail>>

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
         /\ proposing' = [proposing EXCEPT ![n] = "none", ![other] = "none"]
         /\ propTarget' = [propTarget EXCEPT ![n] = -1, ![other] = -1]
         /\ propTail' = [propTail EXCEPT ![n] = -1, ![other] = -1]
         /\ mergeLock' = [mergeLock EXCEPT ![n] = -1, ![other] = -1]
    /\ anyActive' = (Cardinality(Actives) > 2)
    /\ UNCHANGED <<rView, churn>>

\* ──────────────────────────────────────────────
\* ReleaseMergeLock: 修正本体。
\*   fs のセクターの lock 保持者が離脱済みのとき、グループの生存メンバーが
\*   raft 経由で lock を解放する。
\*   Go: 「merge is prepared by X」で拒否された側が X の死亡 (routing /
\*   connected から一定時間消えている) を確認して ReleaseMerge{X} を提案。
\*   apply は「mergeBy = X のときだけ nil に戻す」CAS で決定的。
\*   本モデルではアクション = commit なので CAS は enabling に畳み込まれる。
\* ──────────────────────────────────────────────
ReleaseMergeLock(fs) ==
    /\ EnableRelease
    /\ state[fs] # "absent"
    /\ mergeLock[fs] # -1
    /\ mergeLock[fs] \notin Members
    /\ mergeLock' = [mergeLock EXCEPT ![fs] = -1]
    /\ UNCHANGED <<state, tail, rView, sActives, anyActive, churn, proposing, propTarget, propTail>>

\* 誤検知 variant: 保持者が生存していても解放できる。公平性なし。
\* 実装の解放ゲート (死亡検知 + タイムアウト) がどれだけ誤発動しても
\* safety が保たれることの検証用 (Phase 3)。
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

\* ──────────────────────────────────────────────
\* 公平性
\*   ReleaseMergeLock (死亡検知による解放) には WF を付ける = 保持者の離脱が
\*   確定していれば解放は最終的に行われる。
\*   ReleaseMergeLockAny (誤検知) には付けない = 発火してもしなくてもよい。
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
        /\ WF_vars(ReleaseMergeLock(n))

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

\* lock は存在するセクターにのみ付く (帳簿の整合性)
LockOnExistingSector ==
    \A x \in Nodes : mergeLock[x] # -1 => state[x] # "absent"

\* 到達可能性プローブ (真の不変条件ではない):
\* 「離脱済みノードが保持する stale lock」が到達可能なことを確認したいとき、
\* これを INVARIANT にすると違反 trace がバグ系列そのものになる。
\* EnableRelease = TRUE でも解放されるまでの間は一時的に破れる点に注意。
NoStaleLock ==
    \A x \in Nodes : mergeLock[x] = -1 \/ mergeLock[x] \in Members

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
