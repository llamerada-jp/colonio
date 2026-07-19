# sector activation の設計

## モデル

### 基本構造

- 処理対象の自 node を node[i], frontward 方向の次の node を node[i+1] とする, backward 方向の次の node を node[i-1] とする
- node の持つアドレスを node[i].addr とする
- 各 node は各々が管理の責任を持つ hosting sector を持ち、それぞれ sector[i] とする
- sector[i] は管理するアドレスを持っており、それを [sector[i].head, sector[i].tail) と表す
- node のアドレスと sector のアドレスは同じ系であり、円環(最大アドレスの次は 0 になる)になっているとする

### raft の構成

- node[i] の hosting sector の raft メンバーは、routing で得た近傍 node 群に応じて可変とする
  (実装では nextNodeIDs を基に create/append/remove で調整される)
- node[i] は sector[i] の hosting node であり、管理上の責任を持つ
- raft は独立したアルゴリズムで動いているため、node[i] が Leader でない場合もある
- sector は raft のステートマシンによって状態管理とレプリケーションが行われる

### sector activation の処理

- active な sector は R/W を受け付ける。inactive の sector は R/W どちらも受け付けない
- sector は raft が構成され、かつ前後の sector と重複していない場合に active になる
- 全 sector は最初 inactive から始まり、seed を利用して最初に active になる node を決める
- active な sector を持つ node は frontward 方向の node と協力して、frontward 方向の sector を安全に active にする
- クラスタを構成数全 sector は eventually に active になる
- node 任意のタイミングで offline になったり、新しくクラスタに参加する。参加する node のアドレスはそのタイミングで unique で任意の値をとる

### その他の性質

- sector のアドレスは基本的に被らないが、何らかの理由で被る可能性もある。被った場合のデータは信用できないため、被った sector は両方 terminate してデータを破棄する
- sector を構成する node が同時に過半数 offline になる可能性もあるため、データが失われる可能性はある

### quorum 喪失時の脱出経路（2026-07-04 設計判断・実装済み）

シミュレーション（100 ノード・ランダム停止）で、**quorum を失った raft グループは
terminate を含む一切の提案を commit できず、sector が誰にも破棄できない状態に陥る**
ことを確認した。この状態は activation チェーンを恒久停止させるため、raft を
経由しない以下の脱出経路を導入した。

- **ローカル強制破棄**: 「commit できない raft グループ」を各レプリカが
  ローカル判定し、raft を経由せず sector を破棄する。判定は 2 系統:
  (1) リーダー不在が一定時間 (forceTerminateDuration=30s) 続く。
  leader-without-quorum（リーダーは生きているが過半数が死んでいる状態。
  シミュレーションでリーダー不在検知をすり抜けることを確認）を検出するため
  raft の CheckQuorum を有効化し、quorum を失ったリーダーを降格させる。
  (2) 提案が pending のまま commit が一定時間 (forcePendingDuration=45s)
  進まない。破棄後は通常の create/append により
  新しい raft 構成が再作成される。誤判定（実際には生きているグループの破棄）は
  メンバー離脱と等価であり、生じうる sector 重複は既存の terminate/merge の
  修復経路で解消される。データは失われうるが、上記「その他の性質」で許容済み。
- **timeout / abort**: split/merge 実行中の import 等、raft 適用を待つ
  ブロッキング操作は proposalWaitTimeout=15s でエラー復帰し、呼び出し元が
  abort（frontward sector の terminate）する。terminate も commit できない
  場合は上記の強制破棄が後始末する。
- **メンバー除去の out-of-band 通知（2026-07-06 追加）**: remove された
  メンバーは自分の除去 commit をグループから学習できない（除去適用後
  リーダーは送信を止める = raft の既知の性質）ため、stale レプリカが
  上記の強制破棄で刈られるまで 30〜60 秒残留する。これがシミュレーション
  （README run 10）で残存した赤描画（レプリカ間 tail 不一致）の主因だった。
  対策として、除去 conf change の適用時に host が removed node へ
  `SectorManageMember COMMAND_REMOVE` を直接送り、受信側は tombstone +
  ローカル破棄（`Sector.TerminateLocally`、強制破棄と同一経路）で即時解消
  する。通知は best-effort の one-shot で、喪失時は強制破棄が backstop の
  まま残る。**モデル上は新アクションではなく、LocalDestroy の発火条件が
  早まっただけ**（グループが除去を commit 済みなので構成上メンバー離脱
  そのもの。TODO-1 でモデル化する場合は単一の LocalDestroy アクションの
  発火条件違いとして扱う。README TODO-1 の追記参照）。
  なお hosting sector 宛の REMOVE は拒否する（host は自グループから除去
  されない仕様のため、受理すると無認証パケット 1 つで active sector を
  破棄できてしまう）。

また、**raft メンバー ID（sectorNo）は使い捨て**とする。ローカル破棄や remove
適用で消えたレプリカを同じ {sectorID, sectorNo} で再作成すると、グループが記憶する
その ID の複製進捗・投票と矛盾し raft が破綻する（実例: 空ログ再作成による
etcd raft 内部 panic。README「run 5」参照）。再作成は必ず新しい sectorNo で行い、
旧キーは tombstone で再作成を拒否する。

また、raft 適用（apply）ハンドラは**冪等かつ必ず完了する**ことを規約とする。
apply が失敗して提案の完了フラグを立てられないと、「commit は成功するが状態が
進まない」再提案ループになり、健全なグループが quorum 喪失と同一の症状を示す
（実例 1: inactive セクターへの terminate が store 未割り当てを理由に失敗し続けた。
README「シミュレーション再実行での発見」参照）。エラーによる失敗だけでなく、
**状態ゲートによる黙殺も同じ違反**である（実例 2: `ConsensusApplyProposal` の
activation ゲート `tail == nil` が commit 済みの Import / CommitSplit を
inactive セクターで捨てており、import 先が定義上 inactive である split は
構造的に一度も成功できなかった。2026-07-06 修正、README run 8 参照）。
モデルの Commit 系アクション（CommitSplit 等）は「commit されたら状態が
遷移する」を暗黙の前提としており、この規約はその前提を実装側で保証するもの。

**モデル検証完了 (2026-07-25)**: `spec/kvs/KvsSectorFail.tla` で quorum 喪失
（stuck）+ TimeoutAbort + LocalDestroy をモデル化し、誤発動（misfire）を
許した場合でも safety（NoOverlapCommitted/ActiveFlagConsistent/ValidRange）+
全 liveness（EventuallyAllActive 等）が成立することを確認した（N=3/N=4、
複数規模で検証、最大 5.9 億状態 19 時間 42 分完走）。**FixTombstone=FALSE
（sectorID 使い捨てなしの最も緩い設定）でも成立**しており、tombstone は
safety の必要条件ではなく多重防御の一つという結論。churn・stuck・misfire を
同時に最大化した設定は TLC のスケール限界（32-bit int オーバーフロー、
約 19 億 distinct で発生）で未完走のまま残る。詳細は README「TODO-1 の
検証結果」を参照。

未決事項: 死亡判定への routing 情報の組み合わせ、しきい値の実測に基づく調整。
詳細は [README.md の「今後の TODO」](README.md#今後の-todo)（TODO-2〜TODO-4）を参照。

### churn 下のメンバーシップ管理の課題（2026-07-04 run4 で確認・未解決）

恒久停止バグの解消後、律速要因は churn 下のメンバーシップ管理に移った
（詳細は README「シミュレーション run 4」参照）。

- ~~未同期 voter による quorum 毀損~~ → **learner-first メンバーシップで対策済み
  (2026-07-04)**: 追加メンバーは learner として参加し、リーダーがログ追随
  （Match ≥ Commit）を確認してから voter へ昇格する。未同期/死亡ノードの
  append は quorum に影響しない。昇格前の learner はメンバー状態機械上
  「追加完了」にならず、routing から消えれば通常経路で除去される。
  初期メンバー（グループ bootstrap）は従来どおり voter（スコープ外）。
- ~~join レプリカの履歴 replay 失敗による恒久乖離~~ → **bootstrap conf change
  への nodeID context 付与ほかで対策済み (2026-07-06)**: ログ未圧縮のため
  join メンバーは bootstrap の conf change entry を必ず replay するが、
  context が無いと離脱済み初期メンバーを解決できず、apply バッチ中断で
  全履歴を失い恒久乖離していた（乖離 voter が leaderless ストームの起点に
  なる正帰還。README run 9 参照）。これらは raft メンバーシップと
  メッセージ経路の実装詳細であり、**モデルの抽象度（セクター状態のみ、
  レプリカ・メンバー表なし）より下の層**にあたる。
- ~~除去されたメンバーの stale レプリカ残留~~ → **out-of-band 除去通知で
  対策済み (2026-07-06)**: 上記「quorum 喪失時の脱出経路」の項を参照。
- **is_stable ゲートによる修復凍結**: `subRoutine` は is_stable でないと
  ManageMember / operateSectors に到達しないため、churn 中は穴の修復
  （Extend / terminate frontward）も止まる。修復系操作の許可条件の再検討が必要。
  **run 11 (2026-07-09) で新しい現れ方を確認**: 接続不良 node が required 1d に
  1 つあるだけで隣接 node が is_stable=false のまま hosting sector を作れず
  （実例: 5 分間一度も安定せず）、backward node の activateHostingSector が
  「frontward node の sector レプリカが手元にある」ことを要求するため
  「skip 2」で永久リトライ → その先の activation チェーン全体が凍結。
  sectorActivate は skip しても成功応答を返すため送信側は検知できない。
- ~~prepare_merge の解放条件が未定義~~ → **ReleaseMerge で対策済み
  (2026-07-10、モデル検証 → Go 実装)**: `mergeBy` は最初の prepare_merge の
  commit でセットされたきりクリア経路がなく、preparer が merge 完了前に
  死ぬと後任の merge / 対象セクターの split が永久拒否され activation
  チェーンが恒久停止していた（run 11 で観測。`KvsSectorMergeLock.tla`
  Phase 1 で liveness 違反として再現）。対策: 同一保持者の mergeBy に
  30 秒（mergeReleaseDuration）連続で拒否されたレプリカが raft 経由で
  `ReleaseMerge` を提案し、apply は「mergeBy = 保持者のときだけ nil に戻す」
  CAS で決定的に解放する。解放ゲートの誤発動（保持者が実は生存）は
  ロックなしのインターリービングに戻るだけで、safety はモデルの Phase 3
  （無条件解放）で検証済み、生じうる重複は既存 Terminate 系が修復する。
  詳細は README「mergeBy 解放のモデル検証と実装」参照。
- ~~自己メンバーシップ~~（撤回）: run4 で疑ったが、force terminate ログの
  読み違いと判明。routing は自ノードを近傍リストから構造的に除外しており、
  「nextNodeIDs に自分が現れない」は不変条件（README run4 の訂正参照）。
  検出強化として `ManageMember` にも initHostSector と同じ panic ガードを
  置く余地はある。
- **生存 host の sector を leftover と誤認する merge/overlap 抗争
  （2026-07-12 Stage B run で観測・未解決）**: routing 視界の不一致
  （リンク喪失・視界更新の遅延）により、隣接 host が**生きている node** の
  active sector を「host 死亡後の leftover」と誤認すると、次のループに入る:
  tail 切り詰め activate → merge で吸収 → 生存側が自 sector を再 activate →
  重複検知（Terminate hosting sector 1 / TerminateB 系）で**両方 terminate・
  データ破棄** → backward node の sectorActivate で再 activate → 再 merge…。
  実測では単一 range で 46 秒間に Terminate 27 回。吸収後の range への
  **ack 済み書き込みが overlap 解消の破棄で失われる**ため、データプレーンの
  CAS 監査（api.md Stage B の `@@ kvs cas ok` 短間隔重複、同一 (key, base) で
  3 回成功）として可視化された（発生率 ~0.04% of CAS）。データロス自体は
  「その他の性質」で許容済みのクラスだが、抗争中は range の可用性も劣化する。
  注意: tail 切り詰め activate（`KvsSectorLeftover.tla`）は「host 死亡後の
  leftover」を前提に検証しており、**生存 host の active sector を対象にした
  場合のインターリービングはモデル未検証**。対策候補: merge 実行前の
  leftover host 生存確認（直接リンク試行 or seed 照会）、生存 host 側を
  優先する overlap 解消規則。上記 is_stable / 死活検知の課題と同じ
  「routing 視界と sector 状態の不一致」ファミリー。

  **追加解析 (2026-07-17、run 2026-07-16T23:25〜55Z・30 分)**:
  ループ 1 周をログで完全に対応付け、機構を確定した。

  - **定量**: CAS 成功 23,237 件中、同一 (key, base) の重複成功 126 組
    （全て別 client 発）。「merge done → 2 秒以内に TerminateA
    (`Terminate hosting sector 1`)」の自壊ペアは **103 回 / merge 総数 505 /
    関与 48 node** — 全 merge の約 2 割が直後に自セクターを破棄している。
    長間隔（数分〜25 分）の重複は range の **revision リセット**を示す
    （例: kvs-load-6 は 23:48:52 に rev 1042 で CAS 成功 → 抗争後の
    23:49:45 に base=1 の CAS が 2 client で成功 = 履歴全損）。
  - **代表インシデント** (backward 3b9365a5 vs 生存 host 3bfbed25、
    23:49:14〜55、7 周):
    1. 3bfbed25 は生存（load/lock 統計を出力し続ける）だが is_stable に
       なれず routing から脱落（3b9365a5 の frontwardNextNodeID は
       1 つ先の 3e36e065）。その hosting sector (019f6d50-…0b66/8) の
       グループが leaderless 化（StateCandidate, term 空回り）し、
       terminate が commit 不能な「**不死身の active sector**」S として
       残存（host 生存版の stale active レプリカ = TODO-2 の変種）。
    2. 3b9365a5 が S.head を blocker に clip activate（[自, S.head)）。
    3. operateSectors: frontward=S・!frontwardNodeMatch・head >= tail →
       **merge**。`PrepareMerge` は初回 commit の mergeBy が残るため
       2 周目以降 **raft を経由せず即成功**（sector.go の
       mergeBy == 自分 fast path）。migrate 後、frontward の
       `Terminate()` は fire-and-forget（完了未確認・2 回目以降 no-op）、
       `CommitMerge` は自グループの tail を S.tail まで拡張して「done」。
    4. S は死んでいないため次 tick で S.head が [自, tail) 内 →
       **TerminateA が健全な自セクターごと両殺し**（吸収済みレコード +
       窓中の ack 済み書き込みを破棄）。S 側の terminate は commit
       できず生き残る。
    5. backward の sectorActivate が再 activate → 2 へ。周期 4〜7 秒。
       S の force terminate（leaderless 30s）でループ終了。
  - **コード上の急所 3 点**: (a) `kvs.go mergeSector` が frontward の
    terminate 完了を確認せず CommitMerge で tail を拡張する（Extend 1/2 に
    ある `hasActiveSectorHeadInRange` 相当のガードもこの経路にはない）、
    (b) `sector.go PrepareMerge` の mergeBy fast path がループを毎周
    無償で成功させる、(c) TerminateA が「commit できる側 = データ保持側」を
    殺し「commit できない側」が生き残る非対称。
  - **変種の整理**: 従来記載の「生存 host が再 activate して重複」変種
    (2026-07-12) に加え、「victim の terminate が commit 不能で重複が
    残る」変種を確定。overlap 修復は「相手グループが commit できる」ことを
    暗黙の前提にしており、モデル化は両変種をカバーする必要がある。

  **モデル検証完了 (2026-07-17〜19)**: `spec/kvs/KvsSectorFalseLeftover.tla`
  で偽 leftover 誤認・不死身セクター (stuck)・データ喪失 (追跡アーク +
  NoRepairLoss 不変式) をモデル化し、上記に加えて計 8 欠陥を特定
  （孤児 terminate、同一保持者 ABA、release 後 migrate の無世代 terminate、
  activation の背後カバー盲点、被 merge 中の吸収、stale propTail など）。
  修正パッケージ「CommitMerge 前の victim 破棄確認 + 範囲再検証 /
  prepare 世代付き scoped terminate / 被覆時 activation skip /
  mergeBy 保持中の Import 拒否」で NoRepairLoss + 全 liveness の成立を
  TLC で確認した (safety: churn 2 で 75 億状態、liveness: churn 1、
  誤検知 release 併発 safety も成立)。**Go 実装も完了 (2026-07-19)**:
  mergeSector の victim 破棄確認 (`TerminateForMerge`/`WaitTerminated`) +
  範囲再検証、prepare 世代付き scoped terminate (proto 変更)、被覆時
  activation skip、Import の merge fence。回帰テスト 5 本 (修正前コードで
  失敗確認済み)。**run 17 (2026-07-19、22 分) で定量確認済み・解消と判定**:
  自壊 ping-pong 0 (前 run 103 ペア)、CAS 重複は quorum 喪失リセットの
  revision 再利用 1 組のみ (前 run 126 組)、新ガードは WaitTerminated abort
  15 回・範囲再検証 abort 18 回で自己解消的に機能し、nohost は median 5s /
  max 11s へ改善。残った revision 後退はすべて force terminate (quorum 喪失、
  「その他の性質」で許容済みのクラス) 系で、うち leftover import 元 replica
  の遅れによる末尾数 rev 喪失は README の TODO に切り出した — 詳細は
  README「merge/overlap 抗争の解析とモデル検証」「run 17」と TODO の
  同名項目を参照。

## 分岐

| hosting sector<br>- active<br>- inactive | frontward sector<br>- not exist<br>- active<br>- inactive | frontward node<br>- match<br>- not match (frontward sector head < frontward node addr) | frontward sector head  | note                                   | action                     |
| ---------------------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------- | ---------------------- | -------------------------------------- | -------------------------- |
| inactive                                 | (inactive)                                                | *                                                                                      | *                      | - entire: inactive<br>- member: stable | activate hosting sector    |
| inactive                                 | active (leftover 等) が (自 addr, frontward node addr) 内に存在 | *                                                                              | *                      | 2026-07-10 追加                        | activate hosting sector<br>(tail を範囲内最近傍の active head に切り詰める) |
| active                                   | not exist                                                 | *                                                                                      | *                      |                                        | skip                       |
| active                                   | inactive                                                  | not match                                                                              | *                      |                                        | terminate frontward sector |
| active                                   | inactive                                                  | match                                                                                  | < hosting sector tail  |                                        | split                      |
| active                                   | inactive                                                  | match                                                                                  | = hosting sector tail  |                                        | activate frontward sector  |
| active                                   | inactive                                                  | match                                                                                  | > hosting sector tail  |                                        | expand                     |
| active                                   | active                                                    | not match                                                                              | < hosting sector tail  |                                        | release & recreate         |
| active                                   | active                                                    | not match                                                                              | >= hosting sector tail |                                        | merge                      |
| active                                   | active                                                    | match                                                                                  | < hosting sector tail  |                                        | release & recreate         |
| active                                   | active                                                    | match                                                                                  | = hosting sector tail  |                                        | skip                       |
| active                                   | active                                                    | match                                                                                  | > hosting sector tail  |                                        | expand                     |

note: address は円環になっているため、実装時は between に適宜読み替える

## 各処理

### activate hosting sector

- node[i] は seed に他に active な node がいないことを確認する
- seed は lease に似た仕組みで、active な node を選ぶ。選ばれた node は active な sector を持つことができる
- node[i] は sector[i] を [node[i].addr, node[i+1].addr) の範囲で active にする
- node[i] は seed に対して active になったことを通知する
- **tail の切り詰め (2026-07-10 追加)**: [node[i].addr, node[i+1].addr) 内に
  active な sector head (host 死亡後の leftover 等) が存在する場合、skip せず
  tail をその範囲内最近傍の active head に切り詰めて activate する。
  切り詰めた範囲は重複を生まない。active になった sector が通常の merge で
  leftover を吸収し、tail が伸びる。skip すると「activate は leftover が
  邪魔で不可、leftover の掃除 (merge) は active でないと不可」の循環待ちで
  activation チェーンが恒久停止する (run 11/13 で観測、
  `KvsSectorLeftover.tla` で検証。activate frontward sector 側も同じ)。
  **注意**: この設計とモデル検証は「host 死亡後の leftover」を前提とする。
  routing 視界の不一致で**生存 host の active sector** を leftover と誤認した
  場合は merge/overlap 抗争ループに入り、ack 済み書き込みが破棄される
  (2026-07-12 観測 → `KvsSectorFalseLeftover.tla` で 8 欠陥を特定し
  修正 4 種を 2026-07-19 実装、run 17 で解消を確認。「churn 下の
  メンバーシップ管理の課題」の同名項目を参照)。

### activate frontward sector

- node[i] は node[i+1] に対して SectorActivate RPC を送る
- node[i+1] は sector[i+1] を [node[i+1].addr, node[i+2].addr) の範囲で active にする
- node[i+1] は seed に対して active になったことを通知する

### terminate frontward sector, release & recreate

- node[i] は sector[i+1] に terminate を通知する
- node[i+1] は sector[i+1] が terminate になったことを検知し、各種リソースを解放する
- node[i+1] の sector は一度削除される
- その後、hosting 管理の member 設定メッセージ (create/append) により
  新しい raft 構成が再作成され、inactive な状態になる
  (terminate 直後に同期的に recreate されるとは限らない)

### expand

- node[i] は sector[i] を [node[i].addr, node[i+2].addr) の範囲で active にしなおす

### merge

- node[i] は sector[i+1] に prepare_merge を通知する
  prepare_merge は sector に対して同時に1つの node しか通知できない。すでに他の node から prepare_merge を受けている場合は失敗する
- prepare_merge の解放条件: 同じ保持者による prepare_merge に一定時間
  （mergeReleaseDuration = 30s）連続で拒否された sector のレプリカは、
  release_merge を raft に提案してよい。release_merge の適用は
  「保持者が一致するときだけ解放する」CAS とする（preparer が merge 完了前に
  死ぬと解放経路がなく恒久停止するため。2026-07-10 追加、
  `KvsSectorMergeLock.tla` で検証済み）
- node[i] は sector[i+1] のデータを export して sector[i] に import する
- node[i] は sector[i+1] に terminate を通知する
- node[i] は sector[i] に commit_merge を通知し、sector[i] を [node[i].addr, sector[i+1].tail) の範囲で active にしなおす

### split

- node[i] は node[i+1] に対して SectorPrepareSplit RPC を送る
  node[i+1] は 同時に1つの SectorPrepareSplit を受けることができる。すでに他の node から SectorPrepareSplit を受けている場合は失敗する
- node[i+1] は node[i] を監視し始め、sector[i] が active になる前に node[i] が消失したら sector[i+1] を terminate する
- node[i] は sector[i] へのアクセスのうち、[node[i+1], sector[i].tail) のアクセスを止める
- node[i] は sector[i] の持つデータのうち [node[i+1], sector[i].tail) のデータを export して sector[i+1] に import する
- node[i] は sector[i] に pre_commit_split を通知し、sector[i] を [node[i].addr, node[i+1].addr) の範囲で active にしなおす
- node[i] は sector[i+1] に commit_split を通知し、sector[i+1] を [node[i+1].addr, sector[i].tail(pre_commit_split 前の値)) の範囲で active にする