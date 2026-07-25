------------------------- MODULE KvsSectorLocalViewFailMC -------------------------
(*
 * KvsSectorLocalViewFail のモデルチェック用設定。
 *
 * 検証結果のまとめ (2026-07-25):
 *   Phase 2' (EnableLocalDestroy=FALSE): backstop 無しでは孤立 leftover が
 *     解消できない (EventuallyNoLeftover 違反)。churn=6 対照実験で、これは
 *     LocalView 固有ではなく KvsSectorLeftover 系列全体の既存の弱点
 *     (churn 予算が尽きると誰も救助できない) であると判明。
 *   Phase 2 (EnableLocalDestroy=TRUE, MaxStuck>=1): backstop を有効にしても
 *     `EventuallyNoLeftover` は依然として破れる。ただし原因はモデルの欠陥
 *     ではなく、`BecomeStuck` (quorum 喪失という事実そのもの) を意図的に
 *     non-fair にしているため — TLC は「その leftover には二度と quorum
 *     喪失が起きない」という反例を常に選べる。Go の checkQuorumLoss は
 *     無条件・周期実行だが (sector.go:210-238、WF(LocalDestroy) として
 *     正しくモデル化済み)、「本当に quorum を失うか」は外部要因次第で、
 *     Go 自身も leftover の無条件クリーンアップを保証していない。
 *     → EventuallyNoLeftover を正式な検証対象から外し、safety
 *       (TypeOK/ValidRange/ActiveFlagConsistent/LockOnExistingSector) を
 *       正式な検証対象とする (KvsSectorLocalViewFailSafety.cfg)。
 *       churn=3, MaxStuck=1 では safety 違反は一件も出ていない
 *       (EventuallyNoLeftover 以外の temporal property も違反なし)。
 *)

EXTENDS KvsSectorLocalViewFail

MC_Nodes              == 0..3
MC_InitialMembers     == {0, 1, 2, 3}
MC_MaxChurn           == 5
MC_EnableRelease      == TRUE
MC_PermissiveRelease  == FALSE
MC_ClipActivationTail == TRUE
MC_LocalViewGuards    == TRUE
MC_MaxStuck           == 2
MC_EnableTimeoutAbort == TRUE
MC_EnableLocalDestroy == TRUE

================================================================================
