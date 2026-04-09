# M5 Step 1 引継書 — Capability URL 共有機能

- **対象範囲:** M5 Step 1 (Capability URL 共有) の実装完了
- **完了日:** 2026-04-09
- **実装者:** Taketo + Claude (opus-4-6)
- **レビュー対象:** 白石さやか(セキュリティ)・Marcus Holme(データ)・黒木健司(シニア)

## TL;DR

Bitwarden Send 方式の URL フラグメント共有を実装しました。サーバーは
暗号文しか見ません。復号鍵は URL の `#k=...` に入っており、ブラウザの
HTTP 仕様上サーバーには送信されません。

- DB マイグレーション: `migrations/00006_create_share_links.sql`
- ハンドラ: `internal/handler/share.go` (POST/GET/DELETE /api/v1/share)
- 受信者ビュー: `web/share.html` (依存ゼロの単独 HTML)
- テスト: `internal/handler/share_test.go` (18 テスト、全 PASS)
- ADR: DECISION-055 (フラグメント方式)、DECISION-056 (有効期限ポリシー)

全ハンドラテストカバレッジは 81.3% を維持。

## 変更ファイル一覧

| ファイル | 種別 | 概要 |
|---|---|---|
| `DECISIONS.md` | 追記 | DECISION-055 / DECISION-056 の追加 |
| `migrations/00006_create_share_links.sql` | 新規 | `share_links` テーブル作成 |
| `internal/store/queries/share_links.sql` | 新規 | sqlc クエリ 5 本 |
| `internal/store/share_links.sql.go` | 新規 (生成) | sqlc 生成コード |
| `internal/store/models.go` | 更新 (生成) | `ShareLink` 型追加 |
| `internal/store/querier.go` | 更新 (生成) | Querier インターフェース更新 |
| `internal/handler/share.go` | 新規 | ShareHandler 本体 |
| `internal/handler/share_test.go` | 新規 | 18 テスト |
| `internal/server/server.go` | 更新 | ルート登録 + `/share.html` 静的配信 |
| `web/share.html` | 新規 | 受信者向けフラグメント復号ビュー |
| `web/embed.go` | 更新 | `share.html` を `go:embed` に追加 |

## アーキテクチャの要点

### データフロー (送信側)

```
[送信者ブラウザ]
  └ AES-GCM 鍵を crypto.getRandomValues で生成 (セッション限りで保持)
  └ Vault 上の Person + Address を JSON 化
  └ [12 byte nonce][AES-GCM ct+tag] を組み立て
  └ POST /api/v1/share { ciphertext, expires_in_seconds }
  └ レスポンス { id } を受け取る
  └ https://<host>/share.html?id=<id>#k=<base64url(rawKey)> を組み立て
  └ QR コードとして表示 / コピーして相手に渡す
```

### データフロー (受信側)

```
[受信者ブラウザ]
  └ /share.html を開く
  └ 警告カードを表示し、ユーザーに「復号して表示する」ボタンを押させる
  └ fragment から #k= を取り出す (※ ここでサーバーにキーは届かない)
  └ GET /api/v1/share/{id} → { ciphertext, expires_at }
     レスポンスヘッダ: Cache-Control: no-store, Referrer-Policy: no-referrer
  └ ct を [nonce | aes-gcm ct+tag] に分割して crypto.subtle.decrypt
  └ 平文 JSON を DL (インメモリ)
```

### サーバー側が保持するもの

`share_links` テーブル:

| 列 | 用途 |
|---|---|
| `id` | URL 安全なランダム 22 文字 (base64url 16 byte) |
| `user_id` | CASCADE 削除で孤児化防止 |
| `ciphertext` | BYTEA (最大 64 KB) |
| `expires_at` | TIMESTAMPTZ |
| `created_at` | TIMESTAMPTZ DEFAULT NOW() |

サーバーは **復号鍵・平文・受信者の身元** を一切保持しません。

## セキュリティレビュー観点(白石さん向け)

以下はレビュー時に確認をお願いしたいポイントです。

### 1. 鍵はサーバーに届かないか

- `GET /api/v1/share/{id}` の `id` はクエリではなくパス要素です。
- 受信者ページ `web/share.html` は `fetch` で `/api/v1/share/<id>` を
  叩きますが、URL フラグメント (`#k=...`) はクライアント側に留まります
  (HTTP 仕様)。
- 念のため `referrerPolicy: "no-referrer"` と `credentials: "omit"` を
  設定しています。

### 2. ログで鍵/平文が漏れないか

`share.go:114 / share.go:192 / share.go:280` で `slog.Info` を
呼んでいますが、ログ出力は `user_id / share_id / expires_at` のみで、
ciphertext や鍵は含めていません (フィードバックメモリ
`feedback_crypto.md` の「no secrets in logs」に準拠)。

### 3. nonce 生成

- サーバー側は `crypto/rand` だけを使用 (`share.go:303`)
- ブラウザ側は `crypto.getRandomValues` を前提にする設計で、
  `share.html` 自体は nonce を生成しません (送信者は既存の
  `lodester-client.js` の `encryptVaultData` パターンを再利用予定)

### 4. エンタイトルメント (BOLA 防止)

- `DELETE /api/v1/share/{id}` は `GetShareLinkByID → uuidEqual` で
  所有者チェックし、失敗時は **404** を返します (403 だと存在有無が
  漏れるため)。`TestShareDelete_CrossUser` で回帰防止。
- `GET /api/v1/share/{id}` は公開エンドポイントですが、`id` が
  128 bit なのでブルートフォース列挙は非現実的。

### 5. 漏れうる経路 (未解決・既知リスク)

DECISION-055 に記載していますが、URL フラグメントには以下の漏洩経路が
あります。コードで防ぎようがないため、`share.html` の警告カードで
受信者に周知する方針です。

- ブラウザ履歴 / ブックマーク
- スクリーンショット
- 一部モバイル OS のクリップボード同期 / URL スキームハンドラ
- Chrome の URL 短縮表示(fragment も表示される)

**レビュアーへの相談事項**: この警告テキスト(`web/share.html` 内)
で情報開示として十分でしょうか? 追加したい注意事項はありますか?

## データレビュー観点(Marcus さん向け)

### 1. クリーンアップ戦略

DECISION-056 に従い、物理削除は有効期限の **30 日後**。これは
「期限切れ直後に何が起きたか」を診断するための猶予期間です。

MVP では `DeleteExpiredShareLinks(cutoff)` を手動 SQL で呼ぶ前提。
Phase 1b で cron + goroutine に昇格する計画です。

**相談事項**: 30 日という数字は妥当か? もっと短く (7 日) でも
運用上困らないかもしれません。

### 2. インデックス戦略

```sql
CREATE INDEX idx_share_links_user_id ON share_links(user_id);
CREATE INDEX idx_share_links_expires_at ON share_links(expires_at);
```

- `user_id`: `ListShareLinksByUserID` 用
- `expires_at`: `DeleteExpiredShareLinks` の時間レンジスキャン用

`id` は PK なので自動的にインデックスされます。

### 3. 最大サイズ

`maxShareCiphertextSize = 64 KB` (share.go:44)。これは汎用の
暗号化ストレージとして悪用されないためのガードで、実際の Person +
Address の典型サイズは 1〜2 KB 程度です。

## シニアレビュー観点(黒木さん向け)

### 1. これは本当に必要か

MVP の「ひとりで完結」思想と 1:1 共有機能は緊張関係があります。
ただし Taketo の現実ユースケース(家族へ住所を渡したい・通販の
宛先として使いたい)では、**共有のない連絡先アプリはアプリの体を
なさない**という判断で入れました。

代替案として検討し却下したもの:

| 代替案 | 却下理由 |
|---|---|
| サーバー側で鍵管理 | ゼロ知識設計が壊れる |
| Out-of-band で鍵を別送 | UX が悪すぎる (QR コード 2 枚) |
| メールで vCard 添付 | すでに vCard エクスポートで対応済 (M4) |

### 2. 削りどころ

Step 1 で入れたが MVP 後に削れそうなもの:

- `ListShareLinksByUserID` (監査 UI 専用): 現状 UI 未実装、curl 経由のみ
- `defaultShareTTL` が 7 日: より短く (24 時間) にしてもよいかも
- `maxShareTTL = 1 年`: 現実的にはもっと短くできる

Step 1 で入れ**なかった**もの:

- 一度きり読み取り (view-once): MVP スコープ外。Phase 1b で検討。
- パスワード保護: 同上。URL フラグメントの鍵とパスワードを重ねる
  二重暗号化案は DECISION-055 内で議論中。
- 自動 cron 削除: 手動 SQL + monitoring で MVP 凌ぎ

### 3. テスト戦略の穴

以下は**意図的にテストしていません**:

- DB エラーパス (500 系 6 分岐): `pgx.ErrNoRows` 以外のエラーは
  モック未整備。既存ハンドラ群と同様のスタンス。
- 実際の AES-GCM 復号の e2e: JS 側のテストハーネスを持たないため、
  手動ブラウザ検証で補います (下記「手動検証プラン」)。

## 手動検証プラン (マージ前)

次のセッションで実施する予定:

- [ ] 送信側 JS の実装 (`lodester-client.js` に `createShareLink` を追加)
- [ ] Chrome / Firefox / Safari で送受信ラウンドトリップ確認
- [ ] QR コード生成ライブラリの選定 (候補: `qrcode` — CDN with SRI)
- [ ] iOS Safari でフラグメントが保持されるか確認
- [ ] `expires_in_seconds=0` を不正値として扱っていないか改めて確認
     (現実装は 0 → デフォルト 7 日、負値 → 400)

## 付随して発生する次のタスク

- **M5 Step 1 後半**: 送信側 JS 実装 (別セッションで着手)
- **M5 Step 2**: QR コード表示 UI (share link の自然な延長)
- **M5 Step 3-6**: Postkey デプロイ、監視、リリース準備

## コミット

Step 1 一括のコミット (次のステップで作成予定)。以下を含みます:

```
feat(share): capability URL 共有機能 (M5 Step 1)

- DECISION-055 (フラグメント方式) / DECISION-056 (有効期限) の追加
- share_links テーブル + sqlc 生成
- POST/GET/DELETE /api/v1/share ハンドラ
- 受信者ビュー web/share.html (依存ゼロ)
- 18 テスト追加、handler カバレッジ 81.3%
```

## レビュアーへのお願い

優先確認したい観点:

1. **セキュリティ**: 鍵がサーバーに届かないこと、ログに漏れないこと、
   404 vs 403 の判断、警告テキストの十分性
2. **データ**: 削除ポリシー(30 日)、インデックス、サイズ上限
3. **シニア**: そもそもこのスコープで良いか、削れる機能はあるか

追加でコメント頂きたい観点があれば `reviews/review-06.md` に
書いてください (まだファイルは未作成、必要になったら作成します)。
