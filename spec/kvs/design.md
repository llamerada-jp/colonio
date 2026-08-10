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

- **ローカル強制破棄**: raft グループのリーダー不在が一定時間
  (forceTerminateDuration=30s) 続いた場合、各レプリカは raft を経由せず
  ローカルに sector を破棄する。破棄後は通常の create/append により
  新しい raft 構成が再作成される。誤判定（実際には生きているグループの破棄）は
  メンバー離脱と等価であり、生じうる sector 重複は既存の terminate/merge の
  修復経路で解消される。データは失われうるが、上記「その他の性質」で許容済み。
- **timeout / abort**: split/merge 実行中の import 等、raft 適用を待つ
  ブロッキング操作は proposalWaitTimeout=15s でエラー復帰し、呼び出し元が
  abort（frontward sector の terminate）する。terminate も commit できない
  場合は上記の強制破棄が後始末する。

未決事項: 誤判定時の安全性のモデル検証（README の TODO-1 検証項目 3）、
死亡判定への routing 情報の組み合わせ、しきい値の実測に基づく調整。
詳細は [README.md の「今後の TODO」](README.md#今後の-todo)（TODO-1〜TODO-4）を参照。

## 分岐

| hosting sector<br>- active<br>- inactive | frontward sector<br>- not exist<br>- active<br>- inactive | frontward node<br>- match<br>- not match (frontward sector head < frontward node addr) | frontward sector head  | note                                   | action                     |
| ---------------------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------- | ---------------------- | -------------------------------------- | -------------------------- |
| inactive                                 | (inactive)                                                | *                                                                                      | *                      | - entire: inactive<br>- member: stable | activate hosting sector    |
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