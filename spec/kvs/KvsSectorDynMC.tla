------------------------------- MODULE KvsSectorDynMC -------------------------------
(*
 * KvsSectorDyn のモデルチェック用設定モジュール
 *
 * 動的メンバーシップを扱うため状態空間が大きくなりやすい。
 * MaxChurn と InitialMembers の選び方で速度を調整する。
 *
 * 推奨パラメータ:
 *   N=3, MaxChurn=2: 数秒
 *   N=4, MaxChurn=2: 数十秒
 *   N=4, MaxChurn=3: 数分
 *)

EXTENDS KvsSectorDyn

\* ring 上のアドレス候補（N=3）
MC_Nodes == 0..2

\* 最初に居るメンバー（部分集合でも OK だが、空集合は不可）
MC_InitialMembers == {0, 1}

\* Join + Leave の総回数の上限
MC_MaxChurn == 2

================================================================================
