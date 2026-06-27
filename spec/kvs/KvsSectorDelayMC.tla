------------------------------- MODULE KvsSectorDelayMC -------------------------------
(*
 * KvsSectorDelay のモデルチェック用設定。
 * rView の状態空間が大きいため、N と MaxChurn を控えめにする。
 *
 * 推奨パラメータ:
 *   N=3, MaxChurn=1: 数秒
 *   N=3, MaxChurn=2: 数十秒〜
 *)

EXTENDS KvsSectorDelay

MC_Nodes          == 0..2
MC_InitialMembers == {0, 1}
MC_MaxChurn       == 1

================================================================================
