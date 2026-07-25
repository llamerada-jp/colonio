--------------------------- MODULE KvsSectorLocalViewMC ---------------------------
(*
 * KvsSectorLocalView のモデルチェック用設定。
 *
 * 検証フェーズ (定数を書き換えて実行する):
 *   Phase 0 (配線健全性): MC_LocalViewGuards = FALSE
 *     → KvsSectorLeftoverMC.tla Phase L2 と完全に同一パラメータ・同一結果に
 *       なることを確認する (safety + 全 liveness 成立)。EffectiveView(v) が
 *       ActiveSectors に簡約されるため、KvsSectorLocalView.tla は
 *       KvsSectorLeftover.tla と意味論的に等価になるはず。
 *   Phase 1 (TODO-2 本題): MC_LocalViewGuards = TRUE
 *     → オーバーラップガードをローカル sActives（Go の k.sectors 相当）で
 *       判定しても safety (NoOverlap 系) が保たれるか、保たれない場合でも
 *       TerminateA/B が最終的に解消するか (EventuallyNoOverlap) を確認する。
 *
 * パラメータは KvsSectorLeftoverMC.tla の Phase L2 と同一 (churn 3 で
 * leftover シナリオの最短系列を再現できる)。
 *)

EXTENDS KvsSectorLocalView

MC_Nodes              == 0..3
MC_InitialMembers     == {0, 1, 2, 3}
MC_MaxChurn           == 6
MC_EnableRelease      == TRUE
MC_PermissiveRelease  == FALSE
MC_ClipActivationTail == TRUE
MC_LocalViewGuards    == FALSE

================================================================================
