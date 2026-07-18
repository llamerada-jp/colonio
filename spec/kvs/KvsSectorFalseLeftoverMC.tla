----------------------- MODULE KvsSectorFalseLeftoverMC -----------------------
(*
 * KvsSectorFalseLeftover のモデルチェック用設定。
 *
 * 検証フェーズ (定数を書き換えて実行する):
 *   Phase FL1 (バグ再現): MC_FixConfirmVictim = MC_FixScopedTerminate = FALSE、
 *     KvsSectorFalseLeftoverSafety.cfg (safety のみ) で実行
 *     → NoRepairLoss の違反を確認 (深さ 10、1 秒)。系列: チェーン活性化 →
 *       Write → DropFromRView(n, fs) → ProposeMerge(n) → MergeMigrate →
 *       CommitMergeExtend (victim との重複発生) → TerminateA(n) が
 *       holder = n の hosting セクターを破棄 → holder = LostRepair。
 *   Phase FL2 (修正確認): MC_FixConfirmVictim = MC_FixScopedTerminate = TRUE
 *     (デフォルト)、KvsSectorFalseLeftover.cfg (safety + liveness) で実行
 *     → NoRepairLoss を含む全 safety + 全 liveness が成立。
 *     片方だけ TRUE にすると残る反例 (修正の必要性の根拠) も確認できる:
 *     ConfirmVictim のみ → merger 死亡後の孤児 terminate で喪失。
 *   Phase FL3 (誤検知 safety): MC_PermissiveRelease = TRUE、
 *     KvsSectorFalseLeftoverSafety.cfg で実行
 *     → 無条件 lock 解放が併発しても safety 維持。
 *
 * パラメータ: バグ再現の最短系列は churn 0 + stuck 0 + drop 1 で足りる
 * (視界乖離だけで terminate apply と TerminateA のレースが起きる)。
 * safety は MaxChurn = 2、liveness は状態空間の都合で MaxChurn = 1 で検証
 * した (デフォルトは liveness 実行向けの 1)。
 *
 * 検証結果 (2026-07-19、N=4 InitialMembers={0,1,2,3} stuck 1 drop 1、
 * 24 workers):
 *   FL1: 修正なしで NoRepairLoss 違反 (深さ 10、1 秒)。
 *   FL2 safety (churn 2): 修正 4 種で違反なし
 *     (75 億状態生成 / 6.98 億 distinct / 深さ 48、2h59m)。
 *   FL2 liveness (churn 1): 全 safety + 全 liveness 成立
 *     (4.35 億状態生成 / 4,769 万 distinct / 深さ 39、2h04m)。
 *   FL3 permissive safety (churn 2): 違反なし
 *     (117 億状態生成 / 10.4 億 distinct / 深さ 45、4h36m)。
 *     注意: この規模では TLC の fingerprint 衝突推定が高く (actual 1.0)、
 *     統計上の取りこぼしの可能性がある。churn 2 の非 permissive safety は
 *     衝突推定 0.033 で別途完走している。
 *   反例の系譜 (修正を 1 つずつ外すと再現する欠陥):
 *     修正 1 なし → terminate 未確認の tail 拡張 → TerminateA 自壊
 *     修正 1 のみ → merger 死亡後の孤児 terminate / 同一保持者 ABA /
 *                  release 後 migrate の無世代 terminate / stale propTail
 *     修正 3 なし → 背後カバー activation の修復不能重複 (liveness)
 *     修正 4 なし → 被 merge 中の吸収でデータが export に乗らず消滅
 *)

EXTENDS KvsSectorFalseLeftover

MC_Nodes              == 0..3
MC_InitialMembers     == {0, 1, 2, 3}
MC_MaxChurn           == 1
MC_MaxStuck           == 1
MC_MaxViewDrops       == 1
MC_EnableRelease      == TRUE
MC_PermissiveRelease  == FALSE
MC_ClipActivationTail == TRUE
MC_FixConfirmVictim   == TRUE
MC_FixScopedTerminate == TRUE
MC_FixNoActivateUnderCover == TRUE
MC_FixNoAbsorbWhileLocked  == TRUE

================================================================================
