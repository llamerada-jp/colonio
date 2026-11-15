---------------------------- MODULE KvsSectorLeftoverMC ----------------------------
(*
 * KvsSectorLeftover のモデルチェック用設定。
 *
 * 検証フェーズ (定数を書き換えて実行する):
 *   Phase L1 (バグ再現): MC_ClipActivationTail = FALSE
 *     → EventuallyAllActive / EventuallyNoLeftover の違反を確認。
 *       想定系列: {0,1,2,3} 全員 active 化 → LeaveLeftover(2) → Leave(1) →
 *       Join(1)。leftover 2 が (1, NextInView(1)=3) 内にあるため
 *       CommitActivate(0) が skip し続け、1 が永遠に inactive。
 *       leftover 2 を merge できる active も存在しない (0 から見ると
 *       fs=2 は v=1 より外)。
 *   Phase L2 (修正確認): MC_ClipActivationTail = TRUE
 *     → 全 liveness 成立。0 が 1 を tail=2 (切り詰め) で activate し、
 *       1 が leftover 2 を merge で吸収してチェーンが伸びる。
 *   Phase L3 (誤検知 safety): KvsSectorLeftoverSafetyMC.tla を使用。
 *
 * パラメータ: バグ再現の最短系列は churn 3 (LeaveLeftover + Leave + Join)。
 *)

EXTENDS KvsSectorLeftover

MC_Nodes              == 0..3
MC_InitialMembers     == {0, 1, 2, 3}
MC_MaxChurn           == 3
MC_EnableRelease      == TRUE
MC_PermissiveRelease  == FALSE
MC_ClipActivationTail == TRUE

================================================================================
