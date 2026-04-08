# Lodester 引き継ぎ文書(HANDOFF)

## このドキュメントについて

設計フェーズ(Claude.ai 上で実施)から実装フェーズ(Claude Code 環境)へ Lodester プロジェクトを引き継ぐためのブートストラップ文書です。

Claude Code を最初に立ち上げた時、このドキュメントを読ませることで、プロジェクトの全コンテキストを継承できます。

---

## プロジェクト概要(30 秒版)

**プロジェクト名**: Lodester(ロデスター)

**役割**: OAP(Open Address Protocol)のリファレンス実装

**目的**: 海外 EC で住所入力に苦労する経験から始まり、日本郵便のデジタルアドレスのグローバル版を作る。住所表記統一機関が動かない現実に対する、現実的・分散型の解決策。

**運営**: 個人(Taketo、本業傍ら)。AGPL v3、GitHub 公開、ゼロ知識暗号

**公式インスタンス**: Postkey(`postkey.jp`、Linode 東京リージョン)

**3 階層構造**:
- **OAP**: プロトコル名(全世界共通の住所表現プロトコル)
- **Lodester**: リファレンス実装名(Go 製、AGPL v3)
- **Postkey**: 公式インスタンス名(日本居住者向け)

---

## 設計フェーズの実績

- **凍結ゲート**: 11/11 完了
- **意思決定**: 41 件記録(`DECISIONS.md`)
- **技術レビュー**: 4 回(白石/Priya/Marcus/田村/黒木 の 5 人)
- **事業部長・社長レビュー**: 1 回(Taketo 自己対峙型)
- **設計書類**: 23 markdown ファイル、約 165 KB

---

## ファイルマップ

```
oap-lodester/
├── README.md                           # プロジェクト概要 + 凍結ゲート進捗
├── DESIGN.md                           # 全体設計判断
├── DECISIONS.md                        # 41 の ADR(Architecture Decision Records)
├── STRATEGY.md                         # 認知拡大戦略
├── HANDOFF.md                          # ★ このファイル(Claude Code への引き継ぎ)
├── protocol/
│   └── OAP-spec-outline.md            # OAP プロトコル仕様アウトライン(13 章)
├── legal/
│   ├── README.md
│   ├── 01-privacy-policy-outline.md
│   ├── 02-terms-of-service-outline.md
│   ├── 03-security-measures-outline.md
│   └── 04-warrant-response-policy-outline.md
├── infra/
│   └── architecture.md                 # インフラ・CI/CD・デプロイ(20 章)
├── mvp/
│   └── scope.md                        # MVP 実装範囲、ロードマップ、チェックポイント
├── research/
│   ├── README.md
│   ├── 01-libaddressinput.md
│   ├── 02-zero-knowledge.md
│   ├── 03-appi.md
│   ├── 04-gdpr.md
│   └── 07-existing-wallets.md
└── reviews/
    ├── review-01.md                    # 第 1 回技術レビュー
    ├── review-02.md                    # 第 2 回技術レビュー
    ├── review-03.md                    # 第 3 回技術レビュー(分散アーキテクチャ含む)
    ├── review-04.md                    # 第 4 回技術レビュー(設計フェーズ総括)
    └── review-05-business.md           # 第 5 回事業部長・社長レビュー
```

**Claude Code が読むべき優先順位**:

1. **最優先**: `HANDOFF.md`(このファイル)、`README.md`、`DESIGN.md`、`DECISIONS.md`
2. **次に**: `mvp/scope.md`、`infra/architecture.md`、`protocol/OAP-spec-outline.md`
3. **必要に応じて**: `legal/`、`research/`、`reviews/`

---

## 確定した技術スタック

### バックエンド

- **言語**: Go 1.23+
- **HTTP ルーティング**: `net/http` 標準 + `chi`
- **ORM/クエリ**: `sqlc`(型安全、生 SQL → Go コード生成)
- **マイグレーション**: `goose` または `golang-migrate`(実装時に決定)
- **設定管理**: 環境変数直読み(`os.Getenv`、`DATABASE_URL` など)
- **ログ**: `log/slog`(Go 1.21+ 標準)
- **暗号**:
  - 標準 `crypto/*`
  - `golang.org/x/crypto/argon2`(KDF)
  - `crypto/aes` + `crypto/cipher`(AES-GCM)
- **テスト**: `testing` 標準 + `testify`

### データベース

- **DB**: PostgreSQL 16+ のみ(SQLite は Phase 2 以降検討)
- **接続抽象化**: `DATABASE_URL` 環境変数
  - 公式 Postkey: Linode Managed Database
  - 自己ホスト: Docker Compose 同梱 PostgreSQL or 同居型

### フロントエンド

- **未確定**(MVP では基本 UI、実装時に SvelteKit / Svelte / Vanilla JS から選定)
- **i18n**: 日本語のみ(MVP)、英語は Phase 1b

### インフラ

- **クラウド**: Linode 東京リージョン
- **リバースプロキシ**: Caddy(自動 TLS)
- **CDN/WAF**: Cloudflare 無料プラン
- **メール**: Resend
- **監視**: BetterStack Uptime + Linode 標準
- **バックアップ**: Linode Object Storage(日次暗号化)

### CI/CD

- **プラットフォーム**: GitHub Actions
- **PR チェック**: gofmt / go vet / golangci-lint / go test -race / govulncheck / gosec
- **リリース**: クロスコンパイル(linux-amd64/arm64、darwin-arm64)+ SBOM(syft)+ 署名(cosign)
- **デプロイ**: GitHub Actions 手動トリガー(`workflow_dispatch`)
- **Reproducible Builds**: MVP から採用

---

## 暗号アーキテクチャ(ゼロ知識)

### 重要原則

1. **マスターパスワードは Lodester に送信しない**(Bitwarden 方式)
2. **サーバーは暗号文ブロブのみを保管**(復号鍵を持たない)
3. **クライアント側で暗号化・復号**

### 暗号スイート

| 用途 | アルゴリズム |
|---|---|
| KDF(マスターパスワード → 鍵) | Argon2id(RFC 9106 準拠) |
| 対称暗号(ボールト) | AES-GCM-256 |
| 鍵伸長 | HKDF-SHA256 |
| 非対称暗号(Phase 2) | RSA 3072 |
| 認証(ログイン) | Bitwarden 方式(KDF 派生ログインハッシュ) |

### 暗号文ヘッダ(クリプトアジリティ)

すべての暗号文には JSON ヘッダを付ける:

```json
{
  "schema_version": 1,
  "kdf": "argon2id",
  "kdf_params": {"memory": 65536, "iterations": 3, "parallelism": 4},
  "cipher": "aes-gcm-256",
  "wrapped_key_id": "...",
  "created_at": "2026-04-15T10:00:00Z"
}
```

これにより、将来アルゴリズムを変更しても旧データを復号できる。

### MFA

- **MUST**: TOTP(Phase 1b 第 1 波)
- **SHOULD**: WebAuthn(Phase 1b 第 2 波)
- **MUST**: バックアップコード(MFA と同時)

### Phase 1a(MVP)では MFA なし

MVP は MFA なしでローンチ。Phase 1b 第 1 波で TOTP + バックアップコードを追加する。これは段階的実装の方針。

---

## ⚠️ Claude Code への重要警告

### 暗号コードを生成する時

Lodester の核心はゼロ知識暗号です。AI 生成コードは慎重に扱う必要があります:

**❌ AI が誤りやすいパターン**:
- AES-GCM の **nonce を固定値** にする(致命的)
- Argon2id の **パラメータが弱すぎる**(memory < 32MB、iterations < 2 など)
- HKDF の **コンテキスト文字列を空にする**
- マスターパスワードを **サーバーに送信** してしまう
- **乱数生成に `math/rand` を使う**(必ず `crypto/rand` を使う)
- セッション ID を **JWT の payload に格納** してしまう(機密情報漏洩)
- ログに **マスターパスワードや鍵をデバッグ出力** してしまう

**✅ 推奨アプローチ**:
1. AI に暗号コードを生成させる時は、必ず `golang.org/x/crypto` の公式ドキュメントと照合
2. `gosec` を必ず実行し、すべての警告を潰す
3. 暗号関連のテストを充実させる(known-answer test、ランダム入力テスト)
4. PR レビューで「nonce/IV はランダムか」「鍵管理は正しいか」を必ずチェック
5. **疑問があれば実装より仕様確認を優先**

### ログとプライバシー

以下は **絶対にログに出さない**:

- リクエストボディ
- 暗号文ブロブ
- メールアドレス(ハッシュも含めて)
- Authorization ヘッダ
- セッショントークン
- マスターパスワード
- 任意の鍵素材

`log/slog` の構造化ログを使い、機密情報は `slog.String("password", "REDACTED")` のようにマスクする。

### ライセンス

- **コード**: AGPL v3
- **仕様書**: CC BY 4.0
- **コミット**: DCO サインオフ(`Signed-off-by`)を必須とする

---

## MVP スコープ(Phase 1a)

詳細は `mvp/scope.md` を参照。要約:

### MVP に含まれる機能

- アカウント作成(メール検証、KDF 鍵派生)
- ログイン(Bitwarden 方式、KDF 派生ログインハッシュ)
- 暗号化ボールト(AES-GCM-256、暗号文ヘッダ)
- Person/Address データモデル(libaddressinput 8 レター、多スクリプト名前)
- libaddressinput 統合(日本 + 主要 5 カ国)
- ボールト API(GET/PUT `/vaults/{user_id}`)
- インスタンス発見(`/.well-known/oap`、`/health`、`/status`)
- ランダム形式 GDA コード(例: `A7K2-9MQ3-XC4F@postkey.jp`、Crockford Base32 + Luhn mod 32)
- Web フロントエンド(基本 CRUD UI、日本語のみ)
- 共有機能(QR コード + 共有 URL)
- vCard エクスポート(vCard 4.0、`X-GDA-CODE` カスタムフィールド)
- 公式 Postkey 稼働(Linode 東京、Caddy、systemd、Reproducible Builds)
- Docker Compose セルフホストパック
- CI/CD パイプライン(GitHub Actions)
- SBOM 生成(syft)、バイナリ署名(cosign)
- 監視(BetterStack Uptime、Linode 標準)
- バックアップ(Linode Object Storage、日次暗号化)
- プライバシーポリシー、利用規約、安全管理措置(弁護士レビュー後公開)

### MVP に含まれない機能(Phase 1b へ)

- カスタムハンドル → Phase 1b 第 1 波
- TOTP MFA + バックアップコード → Phase 1b 第 1 波
- 英語 UI → Phase 1b 第 1 波
- WebAuthn / Passkey → Phase 1b 第 2 波

### データモデルは最初から完全版で定義(Marcus 推奨)

Person / Address / Vault / Handle / MFA / Capability などのテーブルは MVP から最初に定義。使わないフィールドは NULL 許可。後からマイグレーションを書く負荷を避ける。

### 目標期間

- **3〜5 ヶ月で MVP 公開**(AI 活用前提)
- 上限 6 ヶ月

### マイルストーン

| M | 目安時期 | 成果物 |
|---|---|---|
| M0 | 凍結直後 | Go 学習開始、GitHub Repo 整備、CI 雛形 |
| M1 | +1 ヶ月 | Go 基本習得、認証システム動作 |
| M2 | +2 ヶ月 | ボールト実装、完全な DB スキーマ、暗号化 |
| M3 | +3 ヶ月 | API 機能完成、共有機能、vCard |
| M4 | +4 ヶ月 | フロントエンド β 版(日本語)、統合テスト |
| M5 | +5 ヶ月 | セキュリティ監査、ドキュメント整備、ローンチ |

各 M でチェックポイント評価。進捗 70 % 未満なら対応を調整。

---

## 実装フェーズの初手

### M0(今日〜今週中)

1. **Go 環境セットアップ**
   - Go 1.23+ をインストール
   - VS Code / Cursor + Go 拡張
   - `go env` で環境確認
   - `golangci-lint` をインストール

2. **GitHub リポジトリ整備**
   - `lodester-oap` org を作成(または既存の `lodestar-oap` をリネーム)
   - `lodester` リポジトリを作成
   - 設計書(このリポジトリの内容)を別ブランチまたは別リポジトリに配置
   - LICENSE(AGPL v3)、README.md、CONTRIBUTING.md、CODE_OF_CONDUCT.md、SECURITY.md を準備
   - `.gitignore`(Go 用)
   - `.editorconfig`

3. **最小 Hello World**
   - `cmd/lodester/main.go`(エントリポイント)
   - `internal/server/server.go`(HTTP サーバー)
   - `GET /.well-known/oap/health` を実装(`{"status": "ok", "version": "0.0.1"}`)
   - `go run cmd/lodester/main.go` でローカル起動確認

4. **GitHub Actions の雛形**
   - `.github/workflows/ci.yml`(gofmt、vet、test、govulncheck)
   - `.github/workflows/release.yml`(タグ push でクロスコンパイル + GitHub Releases)
   - dependabot 設定(`.github/dependabot.yml`)

### M1(+1 ヶ月)の到達目標

- Go の基本(struct、interface、goroutine、channel、error handling)を習得
- 認証システムの最初の実装(ダミー実装で OK):
  - `POST /api/v1/accounts`(アカウント作成、メール送信は後回し)
  - `POST /api/v1/sessions`(ログイン、トークン発行)
  - `GET /api/v1/me`(認証チェック)
- PostgreSQL 接続の確立(`DATABASE_URL` 環境変数経由)
- 最初のマイグレーション(users テーブル、sessions テーブル)

### Taketo 側で並行して進めるべきこと

- [ ] `postkey.jp` ドメイン取得可能性の確認
- [ ] Postkey の正式商標調査(J-PlatPat、USPTO、EUIPO)
- [ ] GitHub organization `lodester-oap` の確保(`lodestar-oap` から移行)
- [ ] 勤務先の副業規程・成果物帰属の最終確認
- [ ] Linode アカウント開設準備(契約は M3 頃)
- [ ] 弁護士相談の予約(法的文書 4 本のレビュー、M4 頃までに)

---

## Claude Code への最初のプロンプト案

Claude Code を起動した時に、最初に貼り付けるプロンプトの例:

```
このプロジェクトは Lodester(OAP プロトコルのリファレンス実装)の実装フェーズです。
設計フェーズは Claude.ai で完了済みで、以下のドキュメントを参照できます:

- HANDOFF.md(この引き継ぎ文書)
- README.md(プロジェクト概要)
- DESIGN.md(全体設計判断)
- DECISIONS.md(41 の ADR)
- mvp/scope.md(MVP 実装範囲)
- infra/architecture.md(インフラ・CI/CD)
- protocol/OAP-spec-outline.md(プロトコル仕様アウトライン)

私(Taketo)は本業のシステムエンジニアで、Go は新規習得中です。
週 5〜10 時間のコミットで、3〜5 ヶ月で MVP 公開を目指します。

まずは M0(今週中の到達目標)を実行したいです:

1. Go プロジェクトの最小構造を作る
2. /.well-known/oap/health エンドポイントを実装する
3. GitHub Actions の CI 雛形を作る

HANDOFF.md と関連ドキュメントを読んだ上で、M0 から始めてください。

注意事項:
- 暗号コードは特に慎重に(HANDOFF.md の警告を参照)
- データモデルは最初から完全版で定義する(Marcus の推奨)
- 完璧を目指さず、完成を目指す(古谷社長の忠告)
- 段階的実装、Phase 1a → Phase 1b → Phase 2
```

---

## 設計の哲学(忘れないために)

### 4 つの根本原則

1. **ゼロ知識**: サーバーは暗号文しか見えない。Trust より Verify。
2. **メール型連合**: 単一運営者の継続性問題を構造で解消。
3. **段階的実装**: 小さく出して、育てる。完璧ではなく完成を目指す。
4. **個人運営でも持続可能**: 24/7 ベスト・エフォート、ノイズ最小化、ガードレール。

### 5 つの設計判断の核心

1. **Bitwarden 方式の認証**: マスターパスワードは送信しない、KDF 派生のログインハッシュのみ
2. **クリプトアジリティ**: 暗号文ヘッダで将来のアルゴリズム変更に備える
3. **DATABASE_URL 抽象化**: 公式マネージド DB と自己ホスト同梱を同じコードベースで対応
4. **OAP プロトコル標準化**: `/.well-known/oap/*` で外部監視も標準化
5. **Reproducible Builds**: 「信頼ではなく検証」の精神を実装に反映

### 段階的成熟度

| フェーズ | 目標 | 内容 |
|---|---|---|
| **Phase 1a (MVP)** | 3〜5 ヶ月 | コア機能 + 共有 + vCard |
| **Phase 1b 第 1 波** | MVP + 3 ヶ月 | カスタムハンドル、TOTP MFA、英語 UI |
| **Phase 1b 第 2 波** | MVP + 6 ヶ月 | WebAuthn、ステータスページ、追加言語 |
| **Phase 2** | ユーザー 1,000 人到達時 | B2B 逆引き API |
| **Phase 3** | 遠い将来 | 連合の本格化、SQLite、他言語実装 |

---

## 最後に

設計フェーズは終わりました。これからは、コードを書く時間です。

古谷社長の言葉をもう一度: **「完璧を目指すな。完成を目指せ。Lodester v0.1.0 が動いた瞬間が、君の本当のスタートだ。」**

黒木レビュアーの言葉も: **「Taketo が本当に評価されるべきは、Lodester のバイナリが実際に動いた瞬間からだ。」**

設計書はこれ以上必要ない。今日から、コードが本番です。

---

**設計フェーズ完了日**: 2026 年 4 月 8 日

**設計者**: Taketo

**設計支援**: Claude(Anthropic)

**次のフェーズ**: Claude Code 環境での実装フェーズ
