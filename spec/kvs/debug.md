# デバッグメモ: TODO-1 churn2/stuck2/misfire2 が TLC の内部エラーで完走しない

対象読者: この未解決ケースを引き継いで調査する人（自分含む）。
[README.md](README.md) の TODO-1「TODO-1 の検証結果」節に要約を書いた際、
詳細をここに切り出した。

## 背景

`spec/kvs/KvsSectorFail.tla`（quorum 喪失の故障モードモデル、README TODO-1）
の検証項目 3「`LocalDestroy` の誤発動（misfire）を許した場合に safety が
破れるか」を、churn・stuck・misfire を同時に最大化した設定
（`MC_MaxChurn=2, MC_MaxStuck=2, MC_MaxMisfire=2`）で確認しようとしたところ、
モデル自体の反例ではなく **TLC 実装側の内部エラー**で完走しなかった。

同じ misfire=2 は churn/stuck を 1 に落とせば 30 分弱で完走し違反なしを確認
できている（README 参照）。churn2/stuck2 は misfire=1 で完走し違反なしを
確認できている。**今回未完走なのは churn2×stuck2×misfire2 の同時最大化
だけ**であり、モデルや実装のバグの兆候ではなく、状態数が TLC のある内部
構造の限界（後述）を超えたために起きたと考えている。

## 関連ファイル

| ファイル | 役割 |
|---|---|
| `KvsSectorFail.tla` | モデル本体。書き換え不要（このケースの調査では） |
| `KvsSectorFailMC.tla` | MC 定数の実体。**ここのパラメータを書き換えて再現・調査する** |
| `KvsSectorFail.cfg` | TLC 設定。safety + 全 liveness（`EventuallyAllActive` 等）を含む |
| `KvsSectorFailSafety.cfg` | safety のみ（liveness プロパティなし）の設定。切り分けに使える |

### 失敗した設定（`KvsSectorFailMC.tla`、2026-07-25 時点で上書き済み）

現在の `KvsSectorFailMC.tla` は直近に成功した churn1/stuck1/misfire2 の設定
に書き換わっている。失敗ケースを再現するには以下に戻す:

```tla
MC_Nodes              == 0..3
MC_InitialMembers     == {0, 1, 2, 3}
MC_MaxChurn           == 2   \* ← 現在 1 になっている。2 に戻す
MC_MaxStuck           == 2   \* ← 現在 1 になっている。2 に戻す
MC_MaxMisfire         == 2
MC_EnableTimeoutAbort == TRUE
MC_EnableLocalDestroy == TRUE
MC_FixTombstone       == FALSE
```

## 実行コマンド

```bash
cd spec/kvs
# tla2tools.jar のパスは VSCode 拡張の自動更新でバージョンが変わる。
# 事前に確認すること: ls ~/.vscode-server/extensions/ | grep tla
JAR=~/.vscode-server/extensions/tlaplus.vscode-ide-<version>/tools/tla2tools.jar

# 新規実行（-recover なし）
java -XX:+UseParallelGC -Xmx8g -cp "$JAR" tlc2.TLC \
  -workers auto -checkpoint 15 -config KvsSectorFail.cfg KvsSectorFailMC.tla \
  > /path/to/log 2>&1 &
disown
```

**注意**: 長時間バックグラウンド実行するプロセスを `| head -N` のような
パイプに繋ぐと、`head` が N 行読んで pipe を閉じた瞬間に上流の java へ
SIGPIPE が飛んで巻き添え終了する。ログは必ずファイルへ直接リダイレクトし、
`tail -f` 等で別途覗くこと（一度この事故で正常稼働中のプロセスを誤って
落とした）。

## 何が起きたか（時系列）

1. **フレッシュ実行**: 上記コマンドで新規実行。約4日かけて
   **depth 31・約210億状態生成・約19.03億 distinct・キュー6696万** まで
   到達（2026-07-25 10:29:48 時点、この時点のチェックポイントは正常完了）。
   この間、TLC 自体のエラーは一度も出ていない。
2. **電源断**: 2026-07-25 11:54 にホスト OS が再起動（本セッション中に複数回
   発生した電源断の一つ）。java プロセスは強制終了。
3. **`-recover` 試行 1**（`-Xmx8g`、直前まで使っていたヒープ量）:

   ```
   Starting recovery from checkpoint states/26-07-22-19-30-01.996/
   AAAAAA
   Error: Java ran out of memory.  Running Java with a larger memory allocation
   pool (heap) may fix this.  But it won't help if some state has an enormous
   number of successor states, or if TLC must compute the value of a huge set.
   ```

   ヒープ不足と判断し、システム空きメモリ（62GB 中 59GB 空き）を確認して
   ヒープを増やす方針にした。

4. **`-recover` 試行 2**（`-Xmx48g`）: 34 分 19 秒実行して以下で終了。

   ```
   Starting recovery from checkpoint states/26-07-22-19-30-01.996/
   AAAAAA
   Error: TLC threw an unexpected exception.
   This was probably caused by an error in the spec or model.
   See the User Output or TLC Console for clues to what happened.
   The exception was a java.lang.NegativeArraySizeException
   : -1577058301
   Finished in 34min 19s at (2026-07-25 12:56:34)
   ```

   ヒープを 6 倍にしても解決しなかった。`NegativeArraySizeException` は
   典型的には「配列サイズを 32-bit int で計算していて、意図した正の値が
   `Integer.MAX_VALUE`（2^31-1 ≈ 21.5億）を超えてラップアラウンドし負値に
   なった」ときに出る。到達していた distinct 状態数（約19.03億）や
   チェックポイントサイズ（512GB、後述）の規模から見て、TLC 内部の
   何らかのカウンタ/バッファサイズがこの規模でオーバーフローしたと推測
   しているが、**スタックトレースは出力されなかった**ため未確定（下記
   「次にやること」参照）。

5. churn1/stuck1/misfire2（規模を落とした設定）に切り替えて再実行し、
   29 分 30 秒で正常完走（違反なし）。以降の作業はこちらの結果を採用し、
   churn2/stuck2/misfire2 は「未検証のまま」として記録した。

## 重要な未確認事項

**この `NegativeArraySizeException` は `-recover` パス固有の問題であり、
最初からやり直した場合に同じ規模でも再現するかは未確認**。フレッシュ実行
（手順1）は約4日間・21億状態生成・depth 31 まで一度も内部エラーを出さずに
動いていた。エラーが出たのはいずれも `-recover` でチェックポイントから
状態空間を再構築する初期フェーズ（ログの `AAAAAA` の直後）だけ。つまり:

- **通常実行の途中でこの規模に達しても落ちない可能性がある**（ただし
  確認するには数日かかる上、次に電源断が起きたら同じ壁に当たる）
- **`-recover` のチェックポイント読み込みロジックに、通常実行では通らない
  別のコードパス／別のサイズ計算がある可能性が高い**（DiskFPSet や
  DiskStateQueue を読み戻して再構築する処理）

## 残置しているチェックポイント

`spec/kvs/states/26-07-22-19-30-01.996/`（512GB、8578 ファイル、
2026-07-25 12:39 時点の最終更新）をまだ削除していない。`-recover` は
2回とも失敗しているため**このチェックポイントからの再開は現状使えないと
見なしてよい**が、再現実験や TLC 側への bug report のために残してある。
不要と判断したら削除して構わない（`spec/**/states/` は `.gitignore` 対象
で追跡外）。ディスクは 1TB 中 414GB 空き（2026-07-25 時点）。

## 次にやること（候補、優先順位なし）

- **切り分け: liveness を外して safety のみで再現するか確認**。
  `KvsSectorFailSafety.cfg`（temporal property なし）で同じ
  churn2/stuck2/misfire2 をフレッシュ実行し、同程度の規模まで到達しても
  同じ例外が出ないなら、原因は liveness 用ディスクグラフ
  （`tlc2.tool.liveness.DiskGraph` 系、以前 misfire1 run の電源断で
  `EOFException` を起こしたのと同じサブシステム）に絞り込める。
- **TLC のバージョンを変える**。現在使っているのは
  `TLC2 Version 2026.07.18.145032 (rev: 30cc360)`
  （`~/.vscode-server/extensions/tlaplus.vscode-ide-2026.7.211258/tools/tla2tools.jar`、
  拡張の自動更新でパス・バージョンは変わりうる）。上流の tla2tools
  リポジトリで `NegativeArraySizeException` や大規模 `-recover` 関連の
  既知 issue / 修正がないか確認する価値がある。
- **`-recover` を使わない運用に倒す**。電源断対策として checkpoint
  interval を詰める（`-checkpoint 15` は既に設定済み）よりも、そもそも
  `-recover` が信頼できない規模になったら「最初からやり直す」方を前提にする。
- **対称性（symmetry）による状態数削減**。Node のリング構造は巡回群
  （回転）に関しては対称（`RingDist`/`IsBetween` が mod N の差分にしか
  依存しないため）。ただし TLC の `Permutations(Nodes)`
  （全置換）はリングの向きを壊すため**不健全**——巡回群だけを手動で
  列挙した集合を `SYMMETRY` に渡す必要がある。効果は最大 N 分の1
  （N=4 なら最大4倍）で、19.03億 distinct を割れば 2^31 の壁の下に
  収まる可能性はあるが、liveness 検証との健全な組み合わせを別途
  検証する必要があり未着手（詳細は README「TODO-1 の検証結果」末尾）。
- **`-fpbits` / `-fpmem` 等のチューニング**。DiskFPSet 側のパラメータを
  変えることで recovery 時のメモリ計算経路が変わり、症状が変化するか
  試す価値はある（未検証）。

## 環境情報（参考）

- ホスト: WSL2 (Linux 5.15.167.4-microsoft-standard-WSL2-custom+)、24 コア、
  62GB RAM（クラッシュ時点で 59GB 空き）
- ディスク: `/` 1007GB（クラッシュ時点で ~400GB 台の空き、状況により変動）
- 電源断が検証中に複数回発生する不安定な環境だった。今回のケース以前にも
  churn2/stuck2/misfire1 run が一度電源断で中断し、`-recover` で
  チェックポイント本体（fingerprint set）は正常に復元できたが liveness
  ディスクグラフだけが torn write で壊れ `EOFException` になった実績が
  ある（そのときは該当チェックポイントを破棄してフレッシュ再実行し
  完走した）。今回の `NegativeArraySizeException` はそれとは異なる規模
  （512GB チェックポイント、約19億 distinct）でのみ発生している。
