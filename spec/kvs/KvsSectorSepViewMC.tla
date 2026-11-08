------------------------------- MODULE KvsSectorSepViewMC -------------------------------
(*
 * KvsSectorSepView のモデルチェック用設定。
 * rView と sActives の 2 つの遅延状態が掛け算で増えるため、
 * Delay 版より状態空間が大きい。N と MaxChurn を控えめにすること。
 *
 * 推奨パラメータ:
 *   N=3, MaxChurn=1: 数秒
 *   N=3, MaxChurn=2: 数十秒
 *)

EXTENDS KvsSectorSepView

MC_Nodes          == 0..3
MC_InitialMembers == {0, 3}
MC_MaxChurn       == 1

================================================================================
