# OAP / Lodester / Postkey

> 全世界共通の住所表現プロトコルと、その日本発リファレンス実装

> ⚠️ **Claude Code 環境で実装フェーズに進む場合**: まず [HANDOFF.md](./HANDOFF.md) を読んでください。これは設計フェーズから実装フェーズへの引き継ぎ文書で、すべての必要なコンテキストが整理されています。

## 3階層の構造

| 階層 | 名前 | 性質 |
|---|---|---|
| **プロトコル** | OAP (Open Address Protocol) | 全世界共通の技術仕様、言語非依存 |
| **リファレンス実装** | Lodester | OSS ソフトウェア (AGPL v3) |
| **公式インスタンス** | Postkey (`postkey.jp`) | Taketo 運営、日本居住者向け |

ユーザーの住所コード例: `taketo.yamaguchi@postkey.jp`

## ビジョン

OAP プロトコルは、メール (SMTP) と同じ思想に基づいて設計されたオープンスタンダードです。プロトコルは全世界共通、インスタンスは地域・運営者ごとに分散します。Postkey は Taketo が運営する公式インスタンスで、日本居住者を対象とします。EU 居住者向けには将来別の運営者が独自インスタンスを立てられます。

## 戦略の構造(2段ロケット)

- **Phase 1(現在)**: 個人向け住所ウォレット。種まき期。認知拡大が第一目的
- **Phase 2(将来)**: 法人向け逆引き API(事前承認カプセル型、非リアルタイム用途に限定)
- Phase 1 は手段、Phase 2 が本来の目的

## ドキュメント

### 主要ドキュメント

- [DESIGN.md](./DESIGN.md) — すべての設計判断の総まとめ
- [STRATEGY.md](./STRATEGY.md) — 認知拡大戦略の骨格
- [DECISIONS.md](./DECISIONS.md) — 意思決定ログ(時系列)

### 調査記録

詳細は [research/README.md](./research/README.md) を参照してください。

- [research/01-libaddressinput.md](./research/01-libaddressinput.md) — Google libaddressinput 調査
- [research/02-zero-knowledge.md](./research/02-zero-knowledge.md) — ゼロ知識アーキテクチャ調査
- [research/03-appi.md](./research/03-appi.md) — APPI 2022 改正調査
- [research/04-gdpr.md](./research/04-gdpr.md) — GDPR 適用範囲調査
- ⏸ 05 (Signal Protocol / MLS) — Phase 2 設計時に着手予定
- ⏸ 06 (Web Push 通知) — Phase 2 設計時に着手予定
- [research/07-existing-wallets.md](./research/07-existing-wallets.md) — 既存住所ウォレット調査(優先度繰上で先行実施)

### レビュー記録

- [reviews/review-01.md](./reviews/review-01.md) — 第1回レビューパネル
- [reviews/review-02.md](./reviews/review-02.md) — 第2回レビューパネル
- [reviews/review-03.md](./reviews/review-03.md) — 第3回レビューパネル(分散アーキテクチャ含む)
- [reviews/review-04.md](./reviews/review-04.md) — 第4回レビューパネル(設計フェーズ総括)
- [reviews/review-05-business.md](./reviews/review-05-business.md) — 第5回レビュー(事業部長・社長レビュー、自己対峙型)

### プロトコル仕様

- [protocol/OAP-spec-outline.md](./protocol/OAP-spec-outline.md) — OAP プロトコル仕様書のアウトライン(凍結済み、本文は実装フェーズで執筆)

### 法的文書

詳細は [legal/README.md](./legal/README.md) を参照してください。**実際にローンチする前に弁護士レビュー必須**です。

- [legal/01-privacy-policy-outline.md](./legal/01-privacy-policy-outline.md) — プライバシーポリシー アウトライン
- [legal/02-terms-of-service-outline.md](./legal/02-terms-of-service-outline.md) — 利用規約 アウトライン
- [legal/03-security-measures-outline.md](./legal/03-security-measures-outline.md) — 安全管理措置 アウトライン
- [legal/04-warrant-response-policy-outline.md](./legal/04-warrant-response-policy-outline.md) — 令状対応方針 アウトライン

### インフラ構成

- [infra/architecture.md](./infra/architecture.md) — インフラ構成・CI/CD・デプロイパイプライン(Linode + Go + PostgreSQL)

### MVP 実装範囲

- [mvp/scope.md](./mvp/scope.md) — MVP 実装範囲、ロードマップ、チェックポイント方式

### 今後追加予定

- `protocol/OAP-spec.md` — プロトコル仕様書本文(実装フェーズ)

## 凍結ゲート進捗

- [x] データモデル
- [x] 主要技術選定
- [x] 法的要件 (APPI)
- [x] 差別化ポジション
- [x] GDPR 対応方針
- [x] プロジェクト命名 (OAP + Lodester + Postkey)
- [x] 認知拡大戦略の骨格
- [x] プロトコル仕様書のアウトライン
- [x] 規約アウトライン
- [x] インフラ構成の概要
- [x] MVP 実装範囲のラベリング

**11 項目すべて完了。設計フェーズ完了。**

次のステップ: 事業部長・社長レビュー(自己対峙型)→ 実装フェーズへ

## ライセンス

Lodester (リファレンス実装) は AGPL v3 で公開予定。OAP プロトコル仕様書は CC BY 4.0 で公開予定。
