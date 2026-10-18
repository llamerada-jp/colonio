------------------------------- MODULE KvsSectorRaftMC -------------------------------
(*
 * KvsSectorRaft のモデルチェック用設定。
 * proposing 状態が加わるため SepView 版よりさらに状態空間が大きい。
 * まず小さいパラメータで検証し、Terminate 発火を確認する。
 *
 * 推奨パラメータ:
 *   N=3, InitialMembers={0,1}, MaxChurn=1: 数秒〜数十秒
 *   N=4, InitialMembers={0,3}, MaxChurn=1: 数分（要確認）
 *)

EXTENDS KvsSectorRaft

MC_Nodes          == 0..2
MC_InitialMembers == {0, 1}
MC_MaxChurn       == 1

================================================================================
