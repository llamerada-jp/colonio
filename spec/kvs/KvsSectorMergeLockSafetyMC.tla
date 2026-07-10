------------------------- MODULE KvsSectorMergeLockSafetyMC -------------------------
(*
 * KvsSectorMergeLock の Phase 3 (誤検知 safety) 用設定。
 *
 * ReleaseMergeLockAny (保持者の生死に関係なく lock を解放できる、公平性なし)
 * を有効にした状態空間で safety invariant のみを検証する。
 * 実装側の解放ゲート (死亡検知 + タイムアウト) がどれだけ誤発動しても
 * safety が保たれることの確認。公平性のない解放アクションを加えた状態空間では
 * liveness は主張できないため、KvsSectorMergeLockSafety.cfg は PROPERTY を持たない。
 *)

EXTENDS KvsSectorMergeLock

MC_Nodes             == 0..3
MC_InitialMembers    == {0, 1}
MC_MaxChurn          == 3
MC_EnableRelease     == TRUE
MC_PermissiveRelease == TRUE

================================================================================
