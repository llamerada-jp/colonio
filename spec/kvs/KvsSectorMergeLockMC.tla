---------------------------- MODULE KvsSectorMergeLockMC ----------------------------
(*
 * KvsSectorMergeLock のモデルチェック用設定。
 *
 * 検証フェーズ (定数を書き換えて 3 回実行する):
 *   Phase 1 (バグ再現):   MC_EnableRelease = FALSE, MC_PermissiveRelease = FALSE
 *     → EventuallyAllActive が違反することを確認 (counterexample がバグ系列)。
 *   Phase 2 (修正確認):   MC_EnableRelease = TRUE,  MC_PermissiveRelease = FALSE
 *     → 全 liveness が成立することを確認。
 *   Phase 3 (誤検知 safety): MC_EnableRelease = TRUE, MC_PermissiveRelease = TRUE
 *     → cfg の PROPERTY (liveness) をコメントアウトし INVARIANT のみで実行
 *       (公平性のない解放を加えた状態空間では liveness は主張できない)。
 *
 * パラメータ:
 *   バグ再現の最短系列は churn 3 (Join + Leave + Join) と N=4 を要する:
 *     0 ActivateFirst → 0 が 1 を activate (tail[1]=0) → Join(2) が (1,0) 内
 *     → 1 が split して 2 が active (tail[2]=0) → 1 が 2 に ProposeMerge
 *     (rView[1] が stale で 2 を含まないため発火可能, mergeLock[2]=1)
 *     → Leave(1) → Join(3) が (2,0) 内 → 2 の ProposeSplit が stale lock で
 *     永久ブロック → 3 が active になれない。
 *   N=4, InitialMembers={0,1}, MaxChurn=3。
 *)

EXTENDS KvsSectorMergeLock

MC_Nodes             == 0..3
MC_InitialMembers    == {0, 1}
MC_MaxChurn          == 3
MC_EnableRelease     == TRUE
MC_PermissiveRelease == FALSE

================================================================================
