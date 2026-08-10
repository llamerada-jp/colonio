------------------------------- MODULE KvsSectorFailMC -------------------------------
(*
 * KvsSectorFail のモデルチェック用設定。
 *
 * Phase 方式 (結果は確定後にコメントで記録):
 *   F1: 故障再現 — EnableLocalDestroy=FALSE (TimeoutAbort のみ)。
 *       stuck グループが残ると EventuallyAllActive が壊れることを確認する
 *       (= TimeoutAbort だけでは不十分。kvs.go:615 の NOTE のシミュレーション
 *       観測「離脱を検知しても解消できない」の形式的対応物)。
 *   F2: 回復検証 — 両回復 ON, MaxMisfire=0。safety + liveness の成立を確認。
 *       FixTombstone=FALSE (permissive) から始め、破れたら TRUE で再検証。
 *   F3: 誤発動境界 — MaxMisfire>=1。safety の成立を確認 (検証項目 3)。
 *       FixTombstone の要否がここで確定する。
 *
 * パラメータの目安:
 *   N=3, {0,1}, churn 1, stuck 1: 数秒〜数十秒 (反復用)
 *   N=4, {0,1,2,3}, churn 2, stuck 2, misfire 2: 最終 safety 確認用
 *)

EXTENDS KvsSectorFail

MC_Nodes              == 0..3
MC_InitialMembers     == {0, 1, 2, 3}
MC_MaxChurn           == 1
MC_MaxStuck           == 1
MC_MaxMisfire         == 2
MC_EnableTimeoutAbort == TRUE
MC_EnableLocalDestroy == TRUE
MC_FixTombstone       == FALSE

================================================================================
