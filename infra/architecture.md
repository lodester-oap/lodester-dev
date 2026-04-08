# インフラ構成(Infrastructure Architecture)

**対象**: Postkey 公式インスタンス(`postkey.jp`)および Lodester 自己ホスト向けリファレンス構成

**ステータス**: 概要設計完了。実装時に具体的な値を確定する。

---

## 設計制約

| 制約 | 内容 |
|---|---|
| 運営者 | Taketo 1 人(本業傍ら) |
| 予算 | 月額 5,000〜10,000 円が現実的上限 |
| 地理 | データは日本国内必須(APPI 整合) |
| 既存資産 | Proxmox VE ホームラボ、Cloudflare 環境 |
| アーキテクチャ前提 | ゼロ知識(暗号文ブロブのみ保管) |
| 自己ホスト前提 | 他者がインスタンスを立てやすい構成 |

---

## 全体構成図

```
┌─────────────────────────────────────────────────────┐
│       ユーザー(ブラウザ / モバイルアプリ)             │
└──────────────────────┬──────────────────────────────┘
                       │ HTTPS
                       ▼
┌─────────────────────────────────────────────────────┐
│       Cloudflare(CDN + WAF + DDoS 防御)             │
│         無料プラン、SSL 終端、DNS                     │
└──────────────────────┬──────────────────────────────┘
                       │ HTTPS(Origin Certificate)
                       ▼
┌─────────────────────────────────────────────────────┐
│       Linode 東京リージョン(1 vCPU / 2 GB)          │
│  ┌────────────────┐                                 │
│  │   Caddy        │                                 │
│  │  (TLS / proxy) │                                 │
│  └───────┬────────┘                                 │
│          ▼                                          │
│  ┌────────────────┐                                 │
│  │ Lodester API   │  (Go binary, systemd 管理)      │
│  │ (localhost:8080)│                                 │
│  └───────┬────────┘                                 │
└──────────┼──────────────────────────────────────────┘
           │ PostgreSQL プロトコル
           ▼
┌─────────────────────────────────────────────────────┐
│       Linode Managed Database(PostgreSQL)          │
│         東京リージョン、自動バックアップ付き           │
└──────────────────────┬──────────────────────────────┘
                       │ 日次バックアップ
                       ▼
┌─────────────────────────────────────────────────────┐
│       Linode Object Storage(バックアップ用)          │
│            30 日保持、暗号化済みダンプ                │
└─────────────────────────────────────────────────────┘
```

---

## 1. クラウド事業者

**決定**: Linode 東京リージョン(Akamai 傘下)

### APPI 整合性の整理

- **データ所在**: 物理的に東京リージョン内
- **ゼロ知識アーキテクチャ**: Linode は暗号文しか見られない
- **越境移転該当性**: APPI Art. 28 の越境移転は「データの物理的所在」で判断されるため、日本リージョン限定であれば該当しない
- **委託先の整理**: Linode は「委託先」として扱い、APPI Art. 27 の監督義務を果たす
- **プライバシーポリシー記載**: 「Linode(米国法人、ただし東京リージョンのみ使用)」と明示

### 実施事項(ローンチ前)

- Linode の SOC 2 報告書等を入手・確認
- 委託契約書(DPA: Data Processing Agreement)を締結
- インシデント時の連絡経路を確認

### 代替候補(将来検討)

- 純国内: Sakura VPS、ConoHa VPS(APPI 整合性が最も明確)
- コスト最適: OCI Always Free(検証環境として)

---

## 2. バックエンド言語と技術スタック

**決定**: Go

### 選定理由

- 学習コスト低(本業傍らで習得可能)
- シングルバイナリで運用が究極にシンプル(Docker すら不要)
- 標準ライブラリだけで HTTP・暗号・JSON が揃う
- 他者実装の参照として読みやすい
- 暗号処理ライブラリが成熟(`golang.org/x/crypto`)

### 推奨スタック(実装時に確定)

| 層 | 選定 |
|---|---|
| HTTP ルーティング | `net/http` 標準 + `chi` |
| ORM / クエリ | `sqlc`(型安全、生 SQL → Go コード生成) |
| マイグレーション | `goose` or `golang-migrate` |
| 設定管理 | 環境変数直読み |
| ログ | `log/slog`(Go 1.21+ 標準) |
| 暗号 | `crypto/*` 標準 + `golang.org/x/crypto/argon2` |
| テスト | `testing` 標準 + `testify` |

---

## 3. サーバーサイジング

### MVP(0〜1,000 ユーザー)

| リソース | スペック | 月額 |
|---|---|---|
| vCPU | 1 | |
| メモリ | 2 GB | |
| ストレージ | 50 GB SSD | |
| Linode Nanode | | 約 1,800 円 |

Go は メモリ効率が良いため、1 vCPU/2GB で MVP は十分回る。

### 成長期(1,000〜10,000 ユーザー)

| リソース | スペック |
|---|---|
| vCPU | 2〜4 |
| メモリ | 4〜8 GB |
| 月額目安 | 約 3,500〜7,000 円 |

垂直スケールで対応可能。スケールアウトは Phase 2 以降検討。

---

## 4. データベース戦略

**決定**: PostgreSQL 16+ のみサポート

### 接続先の抽象化

`DATABASE_URL` 環境変数で接続先を切り替え、同一コードベースで公式インスタンスも自己ホストも対応する。

```bash
# 公式 Postkey(Linode Managed Database)
DATABASE_URL=postgresql://user:pass@lodester-db-xxx.linode.com:5432/postkey

# 自己ホスト(Docker Compose)
DATABASE_URL=postgresql://user:pass@postgres:5432/lodester

# 自己ホスト(同居型)
DATABASE_URL=postgresql://user:pass@localhost:5432/lodester
```

### SQLite サポートの扱い

Phase 2 以降の検討事項として保留。MVP は PostgreSQL 1 本に統一する。

### 公式 Postkey 用

- **Linode Managed Database(PostgreSQL)**
- 最小プラン: $15/月(約 2,250 円)
- 自動バックアップ付き
- 運用負荷軽減のメリット大

### 自己ホスト用

2 つのリファレンス構成を提供:

**方式 1: Docker Compose(推奨)**

```yaml
services:
  lodester:
    image: ghcr.io/lodester-oap/lodester:latest
    environment:
      DATABASE_URL: postgresql://lodester:${DB_PASSWORD}@db:5432/lodester
    ports:
      - "8080:8080"
    depends_on:
      - db
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: lodester
      POSTGRES_USER: lodester
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - lodester-data:/var/lib/postgresql/data
volumes:
  lodester-data:
```

**方式 2: systemd + 同居型**

1 台の VPS に PostgreSQL と Lodester バイナリを同居させる。セットアップスクリプト(`install.sh`)を提供する。

---

## 5. リバースプロキシ

**決定**: Caddy

### 選定理由

- 自動 TLS(Let's Encrypt)
- Go 製(技術スタック統一)
- 設定ファイル(`Caddyfile`)がシンプル
- HTTP/3 対応

### 代替候補

- Nginx(エコシステム広いが設定が冗長)
- Traefik(Kubernetes 向け、単一 VPS には過剰)

---

## 6. CDN / WAF

**決定**: Cloudflare 無料プラン

### 機能

- CDN(静的ファイル配信高速化)
- WAF(Web Application Firewall)
- DDoS 防御
- DNS 管理
- Origin Certificate(Cloudflare ⇔ Linode 間の認証)
- Rate Limiting(無料枠あり)

### アップグレード検討

Phase 1b 以降、必要に応じて Pro プラン(月 $25)を検討。

---

## 7. ネットワークとセキュリティ

### 構成

```
Internet
    │
    ▼
Cloudflare(WAF + DDoS + CDN + DNS)
    │ (Cloudflare Origin Certificate)
    ▼
Linode 東京(443 ポートのみ公開)
    │
    ▼
Caddy(TLS 終端、HTTP→HTTPS リダイレクト)
    │
    ▼
Lodester API(localhost:8080)
    │
    ▼
Linode Managed Database(内部ネットワーク)
```

### セキュリティ設定

| 項目 | 設定 |
|---|---|
| SSH | 鍵認証のみ、パスワード認証無効、root ログイン無効 |
| ファイアウォール | ufw / nftables、22(制限 IP)+ 443 のみ |
| fail2ban | SSH ブルートフォース対策 |
| 自動セキュリティ更新 | unattended-upgrades |
| HSTS | 有効(1 年、preload) |
| CSP | 厳格設定(eval なし、external script なし) |
| CORS | 最小限(同一オリジンのみ) |
| Rate Limit | Cloudflare WAF + アプリ層の二重 |

---

## 8. バックアップ戦略

### 3 段階のバックアップ

**日次フルダンプ**:
- 時間: 毎日 04:00 JST
- 対象: PostgreSQL 全データ + 暗号文ストレージ
- 形式: 暗号化 tar.gz(GPG 公開鍵で暗号化、復号鍵はオフライン保管)
- 転送先: Linode Object Storage(別リージョン推奨)
- 保持: 30 日

**週次基本バックアップ**:
- Linode Backup Service(自動)
- 保持: 4 週

**月次オフライン保管**:
- 毎月 1 日
- 暗号化済み ZIP
- 保管先: Taketo の個人ストレージ(物理オフライン)
- 保持: 12 ヶ月

### ディザスタリカバリ目標

- **RPO**(Recovery Point Objective): 24 時間以内
- **RTO**(Recovery Time Objective): 4 時間以内
- **検証**: 年 1 回、復旧訓練
- **手順書**: GitHub + オフライン保管(GitHub 障害時のため)

---

## 9. 監視とオブザーバビリティ

### 4 層構成

#### 層 1: 死活監視

**決定**: BetterStack Uptime(無料プラン)

- 10 monitors まで無料
- モダンな UX
- ステータスページ自動生成
- Email / ntfy / Slack 通知

**チェック対象**:
- `GET /.well-known/oap/health`(5 分間隔)
- `GET /.well-known/oap/status`(10 分間隔)
- HTTPS 証明書の有効期限(日次)
- ドメインの有効期限(日次)

#### 層 2: メトリクス

**MVP**: Linode 標準監視のみ(CPU / メモリ / ネット / ディスク)

**将来**: Prometheus + Grafana をホームラボ(Proxmox LXC)に立てる。Lodester コードには `/metrics` エンドポイントを準備しておく(`prometheus/client_golang`)。

#### 層 3: エラートラッキング

**保留事項**: 別途検討(Sentry / GlitchTip / 実装しない)

**仮決定**: MVP は実装しない。journald のログ監視で代替。決定は実装フェーズで。

#### 層 4: ログ

**決定**: journald + logrotate(サーバー内のみ、外部送信なし)

**ログに絶対書かないこと**(Go コードで徹底):
- リクエストボディ
- 暗号文ブロブ
- メールアドレスハッシュ
- Authorization ヘッダ
- セッショントークン

Go の `log/slog` で構造化ログ、機密情報は `REDACTED` で記録。

### OAP プロトコル標準監視エンドポイント

プロトコル仕様に以下を追加:

| エンドポイント | 必須度 | 認証 | 用途 |
|---|---|---|---|
| `GET /.well-known/oap/health` | MUST | 不要 | 生存確認 |
| `GET /.well-known/oap/status` | SHOULD | 不要 | 詳細ステータス |
| `GET /admin/metrics` | MAY | 管理者必須 | Prometheus 形式メトリクス |

**`/status` レスポンス例**:
```json
{
  "status": "ok",
  "version": "1.0.0",
  "protocol_version": "1.0",
  "uptime_seconds": 1234567,
  "features": ["mfa-totp", "webauthn"],
  "registration_open": true,
  "maintenance_mode": false
}
```

ユーザー数等の集計値は返さない(認証必須の `/admin/metrics` 経由)。

---

## 10. アラート戦略

### アラートレベル

| レベル | 基準 | 通知方法 |
|---|---|---|
| **P0 Critical** | サービス完全停止、データ損失 | Email + Push(BetterStack) |
| **P1 High** | 機能劣化、エラー急増、認証障害 | Email + Push |
| **P2 Medium** | リソース逼迫、異常検知 | Email のみ |
| **P3 Info** | デプロイ完了、定期レポート | Slack/Discord(任意) |

### 通知チャネル

**保留事項**: 別途検討(Email / ntfy.sh / Slack / Discord / SMS の組み合わせ)

### 個人運営の前提

- 24/7 対応は不可能
- 規約に "best effort" 明記
- 非常事態のみアラート、通常は翌朝対応
- ノイズ最小化

---

## 11. SLI / SLO

**MVP では設定しない**。Phase 2 以降で検討。

**将来の想定値**:
- 稼働率: 99.5%(月間 3.6 時間のダウン許容)
- レイテンシ: p95 < 500ms
- エラー率: < 0.1%

代わりに **透明性レポート** で実際の稼働率を事後的に公開する。

---

## 12. ドメインと DNS

- **ドメイン**: `postkey.jp`(取得可能性は要確認)
- **代替**: `postkey.jp` 不可なら `postkey.org`、`getpostkey.jp` 等
- **DNS プロバイダ**: Cloudflare DNS(無料、CDN と統合)
- **メール受信**: 同一ドメインで MX レコード設定(外部メールサービス経由)

---

## 13. メール(通知用)

**決定**: Resend

- 無料枠: 100 通/日
- 現代的な API、開発者体験良好
- Go SDK あり

### 送信する通知

- アカウント作成確認
- 重要な規約変更通知
- 不審なログイン警告
- P1 以上のアラート(Taketo 宛)

---

## 14. CI/CD パイプライン

**決定**: GitHub Actions

### PR 時のチェック

| チェック | ツール | 必須度 |
|---|---|---|
| フォーマット | `gofmt` / `goimports` | MUST |
| 静的解析 | `go vet` | MUST |
| リンター | `golangci-lint` | MUST |
| ユニットテスト | `go test ./...` | MUST |
| レース検知 | `go test -race ./...` | MUST |
| カバレッジ | `go test -cover` | SHOULD |
| 脆弱性スキャン | `govulncheck` | MUST |
| セキュリティ静的解析 | `gosec` | SHOULD |
| 依存関係更新 | `dependabot` | MUST |

### main push 時

- PR チェックすべて
- Docker イメージビルド & push(`ghcr.io/lodester-oap/lodester:main`)

### タグ push 時(リリース)

- セマンティックバージョニング(`v1.0.0`)
- クロスコンパイル:
  - `linux-amd64`
  - `linux-arm64`(Raspberry Pi 向け)
  - `darwin-arm64`(開発者向け)
- GitHub Releases にバイナリ添付
- Docker イメージにバージョンタグ
- **SBOM 生成**(`syft`)
- **バイナリ署名**(`cosign`)
- CHANGELOG 自動生成

### Reproducible Builds

**決定**: MVP から採用

Go のビルドオプション:

```bash
GOFLAGS="-trimpath -mod=readonly" \
CGO_ENABLED=0 \
go build -buildvcs=false -ldflags="-s -w -buildid=" -o lodester
```

**理由**:
- 「信頼ではなく検証」の精神と合致
- 誰でもソースから同じバイナリを再現可能
- Supply chain security の基盤
- Tailscale、NixOS 等の先行例あり

---

## 15. デプロイパイプライン

**決定**: 選択肢 B(GitHub Actions 手動トリガー)

### 動作

GitHub Actions の `workflow_dispatch` で UI から 1 クリックデプロイ:

1. 指定バージョンのバイナリを GitHub Releases からダウンロード
2. SSH で本番サーバーに接続
3. `/opt/lodester/versions/vX.Y.Z/` に配置
4. シンボリックリンク切り替え(`/opt/lodester/current` → 新バージョン)
5. メンテナンスモード有効化
6. `systemctl restart lodester`
7. ヘルスチェック確認
8. メンテナンスモード解除
9. 失敗時は自動ロールバック(前バージョンのシンボリックリンク復元)

### メリット

- 1 クリックでデプロイ、ログが GitHub に残る
- ロールバック手順が自動化
- 手順ミスが起きない
- デプロイ履歴が追える

### シークレット管理

| 場所 | 内容 |
|---|---|
| GitHub Secrets | SSH 秘密鍵、Linode API トークン、cosign 署名キー |
| 本番サーバー | `/etc/lodester/environment`(0600、root のみ) |
| systemd | `EnvironmentFile` で読み込み |

**禁止事項**:
- GitHub リポジトリにコミットしない(`.gitignore` で徹底)
- 環境変数をログに出さない
- クラッシュダンプに含めない

### ダウンタイム許容

MVP はゼロダウンタイムを目指さない:
- 深夜帯(04:00 JST)にデプロイ
- 30〜60 秒のダウンタイム許容
- 事前告知(ステータスページ)

Blue/Green デプロイは Phase 2 以降。

---

## 16. 環境管理

| 環境 | 場所 | MVP 段階 |
|---|---|---|
| **development** | 開発者ローカル(Docker Compose) | 必須 |
| **staging** | ホームラボ(Proxmox LXC) | **任意**(MVP では不要) |
| **production** | Linode 東京 | 必須 |

**MVP は staging なし**。ローカル Docker Compose で十分。Phase 1b 以降で必要性を見て Proxmox に立てる。

---

## 17. Proxmox ホームラボ活用

Taketo の既存ホームラボ(Proxmox VE)の活用案:

| 用途 | 実装 |
|---|---|
| 検証環境(staging 代替) | LXC コンテナで Lodester + PostgreSQL |
| 開発環境 | Docker Compose on Debian LXC |
| Prometheus + Grafana(将来) | 監視スタックのセルフホスト |
| Uptime Kuma(代替案) | BetterStack が使えない場合の外部監視 |
| CI ランナー(将来) | GitHub Actions self-hosted runner |

ホームラボは「本番 Linode が止まっても影響しない物理的分離」があるので、監視・検証環境として理想的。

---

## 18. 月額コスト

### MVP(年目 1)

| 項目 | 月額 |
|---|---|
| Linode Nanode(1vCPU/2GB) | 約 1,800 円 |
| Linode Managed DB(PostgreSQL 最小) | 約 2,250 円 |
| Linode Object Storage | 約 750 円 |
| Linode Backup Service | 約 300 円 |
| ドメイン `postkey.jp` | 約 350 円 |
| Cloudflare 無料プラン | 0 円 |
| Let's Encrypt | 0 円 |
| Resend(メール) | 0 円 |
| BetterStack Uptime | 0 円 |
| GitHub Actions | 0 円 |
| **月額合計** | **約 5,450 円** |
| **年間合計** | **約 65,400 円** |

### 成長期(1,000〜10,000 ユーザー)

| 項目 | 月額 |
|---|---|
| Linode(2vCPU/4GB) | 約 3,600 円 |
| Managed DB 拡張 | 約 4,500 円 |
| Object Storage 拡張 | 約 1,500 円 |
| Backup Service | 約 600 円 |
| Cloudflare Pro(必要に応じて) | 約 3,800 円 |
| メール拡張 | 約 1,000 円 |
| **月額合計** | **約 15,000 円** |

GitHub Sponsors や寄付で十分カバー可能なレベル。

---

## 19. 自己ホスト向けリファレンス構成

他者が Lodester を自己ホストする際の 3 つの標準構成:

### 最小構成(個人・家族向け)

| 要件 | スペック |
|---|---|
| サーバー | Raspberry Pi 4(4GB RAM)以上 |
| ストレージ | 16 GB 以上 |
| データベース | PostgreSQL(Docker Compose) |
| OS | Raspbian / Ubuntu Server |
| デプロイ | Docker Compose 1 ファイル |
| ドメイン | 自前 or Dynamic DNS |
| TLS | Caddy 自動取得 |
| バックアップ | USB ドライブへの定期コピー |

### 中規模構成(小規模組織向け)

| 要件 | スペック |
|---|---|
| サーバー | VPS(2vCPU / 4GB RAM) |
| データベース | PostgreSQL(同居 or 専用) |
| デプロイ | Docker Compose or systemd 同居型 |
| ドメイン | 独自ドメイン |
| バックアップ | 別 VPS or オブジェクトストレージ |

### Proxmox 上での構成(Taketo ホームラボ参考)

```
Proxmox VE
├── LXC: Lodester API container
├── LXC: PostgreSQL container
├── LXC: Caddy reverse proxy
└── ZFS: 暗号文ストレージ + ZFS スナップショット
```

Taketo の個人用検証環境として利用可能。

---

## 20. 保留事項(実装フェーズで決定)

以下は今回の設計で決めきれなかった項目です。MVP 実装範囲の決定には影響しないため、保留のまま凍結ゲートを通過します。

### 監視関連

- **エラートラッキング方式**: Sentry / GlitchTip / 実装しない
- **アラート通知チャネル**: Email / ntfy.sh / Slack / Discord / SMS の組み合わせ

### その他

- 具体的な Linode プラン(購入時に再評価)
- PostgreSQL のチューニング(接続プール、shared_buffers 等)
- CI/CD の詳細(`.github/workflows/*.yml` の具体内容)
- Infrastructure as Code(Terraform / Ansible の採否)
- 災害時の切り替え手順書
- SBOM フォーマット(SPDX or CycloneDX)
- cosign 署名鍵の管理方法(鍵のオフライン保管)

これらはいずれも実装フェーズで詰める。
