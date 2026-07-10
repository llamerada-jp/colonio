# seed の設計メモ

seed はノードのセッション管理 (AssignNode / Keepalive / lifespan) と、
WebRTC シグナリング (offer / answer / ICE) の中継 (SendSignal / PollSignal
ストリーム) を担う。**ノード間の新規リンク確立は必ず seed を経由する**ため、
seed セッションを失ったノードは新規リンクを一切張れなくなる。一方で確立済み
リンクは seed なしで生き続ける。この非対称が下記の問題の背景にある。

## 既知の問題: seed セッション喪失による「接続黒穴」ノード

シミュレーション run 14 (180 ノード・114 分、2026-07-10、詳細は
[spec/kvs/README.md](../kvs/README.md) の run 14 節) で特定。

### 現象 (発生機序)

1. keepalive は逆方向の long-poll 型: client の Keepalive RPC は seed 側で
   保留され、他ノードが `ReconcileNextNodes` で切断を報告すると seed が
   対象ノードに challenge を発行して応答させる。このとき seed は対象の
   lifespan を **ShortLifespan に短縮**し、client の次の Keepalive 到着
   (= 再購読) で NormalLifespan に復元する。
2. 旧実装では ShortLifespan = 10 秒に対し、client は keepalive 応答後に
   **無条件で 10 秒 sleep してから再購読**していたため、復元は構造的に
   約 10 秒 + RTT 後 = 猶予とほぼ同時になり、eviction tick (5 秒間隔) との
   際どい競合だった。負荷や GC で数百 ms 遅れると healthy なノードが
   evict される。
3. evict 後は Keepalive / PollSignal / SendSignal がすべて `CodeInternal`
   を返し続けるが、**client (SeedAccessor) は同じ失敗を 10 秒おきに再試行
   するだけで、AssignNode からやり直す回復経路がない**。セッションは
   ノードの寿命が尽きるまで回復しない。

### 意図外の挙動

- セッションを失ったノードは新規 WebRTC リンクを張れない「接続黒穴」に
  なる。確立済みリンクは生きているため routing 上は健在に見え、自己判定
  (`nextNodeMatched`) も stable のままになり得る。run 14 では 26 体発生し、
  ログには `failed to poll` / `failed to keepalive` (`internal: reqID:`)
  が 10 秒間隔で死ぬまで並ぶ (回復例ゼロ)。
- 波及: 黒穴の隣に join したノードは必須 1D リンクが張れず
  `is_stable=false` のまま → KVS の subRoutine 全体が skip → hosting
  sector を作れない → activation チェーンが領域ごと停止し、黒穴が寿命
  (最長 20 分) で死ぬまで領域全体が非 active に留まる。run 14 では
  120 秒以上 yellow の episode 169 件 (最長 1,005 秒)、一度も active に
  ならず死んだノード寿命 50 件の実質全数がこれで説明できた。

### 対応済み (2026-07-11)

challenge 競合の解消 (発生率対策) を両輪で実施:

- **ShortLifespan 10 秒 → 30 秒** (seed/seed.go のデフォルト値)。
  トレードオフは「本当に死んだノードの seed 上の残留が最長 10→30 秒」
  だが、リンク層の SessionTimeout も 30 秒であり整合的。
- **client の keepalive ループの sleep を「エラー時のみ」に変更**
  (node/internal/network/seed_accessor/seed_accessor.go)。Keepalive は
  long-poll なのでペーシングは seed 側が担っており、成功時に sleep する
  理由はない。challenge 応答後は即再購読になり、未購読窓は RTT のみ vs
  猶予 30 秒となって競合は実質消える。

### TODO

| 項目 | 内容 | 備考 |
|------|------|------|
| client の AssignNode リトライ | seed RPC が連続 N 回 (または T 秒) 失敗したら AssignNode からやり直す。nodeID が変わるため network 層の再起動 (= ノード再作成) が自然。**黒穴の恒久解はこれ**で、上記の対応は発生率を下げるだけ。セッション喪失の別トリガー (seed 再起動、challenge パケットロス等) には無力 | run 15 で黒穴シグネチャ (`failed to poll` の 10 秒間隔ストリーク) が残存したら着手 |
| 「already subscribed」の自己修復 | PollSignal / Keepalive は同一ノードの重複購読をエラーで拒否するが、古い stream の切断を server が検知できない場合、再購読が塞がれ続ける (keepalive 側の解放は normalLifespan/2 = 15 分のタイマーまで)。新しい購読要求が来たら古い channel を閉じて置き換えるべき | セッション再入 (AssignNode リトライ) を入れる場合は必須になる |
| 周辺ノード側の防御 | 特定 peer への接続試行が長時間失敗し続ける場合に routing の必須集合から外す / is_stable 判定の緩和 | 上記で黒穴自体が消えれば不要の可能性が高い。KVS 側の is_stable ゲート緩和 (spec/kvs 未完了表) と合わせて判断 |

### 検証方法

simulator の node.log で以下を確認する:

```sh
# 黒穴シグネチャ: 同一 slot (no=#N) の 10 秒間隔ストリークが残っていないか
grep "failed to poll" node.log | grep -o "no=#[0-9]*" | sort | uniq -c | sort -rn
```

ストリークがゼロなら challenge 競合が支配的トリガーだったと確定。残存する
場合は別トリガーがあるため AssignNode リトライ (TODO 1 行目) に進む。
