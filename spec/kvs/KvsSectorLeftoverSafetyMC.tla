------------------------- MODULE KvsSectorLeftoverSafetyMC -------------------------
(*
 * KvsSectorLeftover の Phase L3 (誤検知 safety) 用設定。
 * tail 切り詰め activation (ClipActivationTail) と無条件 lock 解放
 * (PermissiveRelease、公平性なし) が併発しても safety が保たれることを
 * 検証する。公平性のない解放を加えた状態空間では liveness は主張できない
 * ため、KvsSectorLeftoverSafety.cfg は PROPERTY を持たない。
 *)

EXTENDS KvsSectorLeftover

MC_Nodes              == 0..3
MC_InitialMembers     == {0, 1, 2, 3}
MC_MaxChurn           == 3
MC_EnableRelease      == TRUE
MC_PermissiveRelease  == TRUE
MC_ClipActivationTail == TRUE

================================================================================
