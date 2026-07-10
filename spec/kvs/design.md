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

未決事項: 誤判定時の安全性のモデル検証（README の TODO-1 検証項目 3）、
死亡判定への routing 情報の組み合わせ、しきい値の実測に基づく調整。
詳細は [README.md の「今後の TODO」](README.md#今後の-todo)（TODO-1〜TODO-4）を参照。

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