# OAP Protocol Specification — Outline (Frozen)

このドキュメントは OAP (Open Address Protocol) の仕様書を書くためのアウトライン(章立て)である。実際の仕様書本文は、実装フェーズ以降に各章を順次書き下ろす。本アウトライン自体は設計フェーズで凍結された。

## 仕様書のスコープと方針

| 項目 | 内容 |
|---|---|
| **目標仕様量** | 約 50 ページ(ActivityPub 250 ページのような大作にしない) |
| **書式** | RFC 風(IETF プロセスには乗せない、独自仕様として公開) |
| **規範語** | RFC 2119 / 8174 準拠(MUST / SHOULD / MAY) |
| **対象読者** | 他の OAP 実装を作る開発者、セキュリティ監査人、インスタンス運営者 |
| **書かないこと** | Lodester 固有の実装詳細、ユーザーガイド、法的情報 |

---

## 章立て(13 章 + 参考文献)

| # | 章 | 状態 | ページ数 | 主要内容 |
|---|---|---|---|---|
| 1 | Introduction | 🟢 | 3 | Abstract、動機、対象読者、規範語の定義 |
| 2 | Terminology | 🟢 | 2 | OAP / Instance / Handle / Vault / Capability 等の正式定義 |
| 3 | Architecture Overview | 🟢 | 4 | 3 階層モデル、メール型連合、ゼロ知識原則、図解 3 つ |
| 4 | Address Code Format | 🟢 | 4 | ABNF 構文、ハンドル規則、Luhn チェックサム |
| 5 | Instance Discovery | 🟢 | 3 | well-known URI 方式、TLS 要件 |
| 6 | Data Model | 🟢 | 6 | Person / Address スキーマ、libaddressinput、JSON 例 |
| 7 | Cryptographic Requirements | 🟢 | 6 | Argon2id / AES-GCM-256 / HKDF / 暗号文ヘッダ |
| 8 | Authentication and Authorization | 🟢 | 5 | Bitwarden 方式ログイン、TOTP MUST、WebAuthn SHOULD |
| 9 | API Surface | 🟢 | 8 | REST、URL バージョニング、エンドポイント一覧 |
| 10 | Federation Behavior | 🟢 | 4 | クロスインスタンス解決、信頼モデル(TLS のみ) |
| 11 | Privacy Properties | 🟢 | 4 | 脅威モデル、ゼロ知識保証 |
| 12 | Versioning and Evolution | 🟢 | 2 | セマンティックバージョニング、後方互換性 |
| 13 | IANA Considerations | 🟢 | 1 | 将来登録予定の placeholder |
| - | References | 🟢 | 1 | Normative / Informative |
| | **合計** | | **53** | |

🟢 = 大筋確定、本文を書くだけ

---

## 確定された主要設計判断

### Section 5: インスタンス発見方式

**決定**: well-known URI 方式

```
GET https://{instance}/.well-known/oap

→ {
  "version": "1.0",
  "endpoint": "https://api.{instance}/api/v1",
  "name": "...",
  "operator": "...",
  "tos": "..."
}
```

**根拠**:
- HTTPS だけで完結(DNS SRV レコード不要)
- ブラウザから直接読める
- WebFinger / Mastodon / OAuth Discovery と同じ思想
- JSON で拡張可能

### Section 4: インスタンス識別子

**決定**: 公開 DNS で解決可能なホスト名のみ(MUST)

`.onion` 等の代替ネットワークは Phase 3 以降の拡張仕様で扱う。

### Section 6: Data Model — Address スキーマ (DECISION-054)

**決定**: libaddressinput 8 レター + BCP 47 `script` タグで多スクリプト対応

Address 1 件は以下の JSON として表現される。この定義は Phase 1 MVP 用で、Section 6 本文で精密化される:

```json
{
  "script": "ja-Jpan",
  "country": "JP",
  "recipient": "山口 大翔",
  "organization": null,
  "address_lines": ["千代田区永田町 1-7-1"],
  "locality": "千代田区",
  "dependent_locality": null,
  "administrative_area": "東京都",
  "postal_code": "100-8914",
  "sorting_code": null,
  "phone": "+81-3-3581-5111",
  "notes": null
}
```

フィールドは libaddressinput の 8 レター規則に対応する:

| レター | JSON キー | 意味 | Optional |
|---|---|---|---|
| N | `recipient` | 受取人 | No |
| O | `organization` | 組織名 | Yes |
| A | `address_lines` | 番地・建物名(配列) | No |
| C | `locality` | 市区町村 | No |
| D | `dependent_locality` | 町名・大字 | Yes |
| S | `administrative_area` | 都道府県・州 | 国による |
| Z | `postal_code` | 郵便番号 | 国による |
| X | `sorting_code` | 仕分けコード(例: 仏 CEDEX) | Yes |

**多スクリプト対応** (Priya 要件): 1 件の Person は複数 Address を持てる。同じ住所の日本語版 (`ja-Jpan`) と翻字版 (`ja-Latn`) を並置することを想定。`script` フィールドは BCP 47 タグ、`country` は ISO 3166-1 alpha-2。

**バリデーション方針** (Phase 1 MVP):
- 重点実装対象: `JP`, `US`, `DE`, `FR`, `GB`, `AU`(郵便番号書式・必須フィールド)
- その他 234 カ国: フィールドは受け付けるが最小限のチェックのみ(未入力の拒否、長さ上限)
- 240 カ国完璧対応は M4 のスコープ外(DECISION-054 根拠)

**ゼロ知識との関係**: すべての Address JSON は Vault 暗号文内に収まり、サーバー DB には保存されない (DECISION-052)。

### Section 4: Address Code Format — GDA コード (DECISION-053)

**決定**: Crockford Base32 11 文字 + Luhn mod 32 チェックサム 1 文字 = 合計 12 文字。表示は `XXXX-XXXX-XXXX` の 4-4-4 形式。

- 符号化アルファベット: `0123456789ABCDEFGHJKMNPQRSTVWXYZ` (I/L/O/U なし)
- ランダム源: `crypto/rand` 55 bit
- 誤読耐性: Luhn mod 32 が単一文字誤入力を 100% 検出
- リファレンス実装: `internal/gda/generator.go` (Lodester)
- 詳細な ABNF 構文は Section 4 本文で定義

GDA コードは「公開識別子」として扱う。個人情報を含まないため Vault 外の `gda_codes` テーブルにインデックス可能。

### Section 8: 認証方式

**決定**: Bitwarden 方式(KDF 派生ログインハッシュ)

```
1. master_key = Argon2id(password, email, params)
2. login_hash = HKDF-SHA256(master_key, "oap-login-v1", 32 bytes)
3. クライアント → サーバー: (email_hash, login_hash) を HTTPS で送信
4. サーバー: Argon2id(login_hash, server_salt) を保存値と比較
```

**根拠**:
- 個人開発で実装可能なシンプルさ
- Bitwarden / Proton Pass 等で大規模実証済み
- 任意の言語・フレームワークで実装可能(プロトコル普及にとって重要)
- 真の PAKE と比較した実用上の安全性差はほぼゼロ
- マスターパスワードもマスターキーも MUST NOT 送信

### Section 8: MFA 方針

| MFA 方式 | 必須度 |
|---|---|
| TOTP (RFC 6238) | MUST(実装が提供) |
| WebAuthn / Passkey | SHOULD |
| バックアップコード(MFA 有効化時に発行) | MUST |
| SMS / Email OTP | MAY(非推奨) |
| ユーザーによる MFA 有効化 | MAY(強制 NOT) |

**バックアップコードの重要性**: GDA は「リカバリ機構なし」設計のため、MFA 有効化時のバックアップコード発行は MUST。最低 8 個、各 80 ビット以上のエントロピー。

### Section 9: API 形式

**決定**: REST 風 HTTP API、JSON 交換

**根拠**:
- 標準的、誰でも理解できる
- HTTP セマンティクスをそのまま使える
- curl / ブラウザ DevTools でデバッグ可能
- OpenAPI 3.1 で機械可読仕様化

### Section 9: API バージョニング

**決定**: URL パス方式(`/api/v1/...`)

**根拠**:
- 最もシンプルで可視性が高い
- Stripe / GitHub / Twitter / Slack が採用
- ログ・監視で一目瞭然

### Section 10: ブロックリスト機構

**決定**: MAY(任意機能)

**根拠**:
- Phase 1 では悪意あるインスタンスがほぼ存在しない
- インスタンス運営者の自由
- Phase 2 で乱用が見えたら SHOULD への昇格を検討

---

## API エンドポイント一覧(Phase 1 minimum)

Section 9 で詳細化される予定。現時点での想定リスト:

| メソッド | パス | 用途 |
|---|---|---|
| POST | `/api/v1/accounts` | アカウント作成 |
| POST | `/api/v1/sessions` | ログイン |
| DELETE | `/api/v1/sessions/{id}` | ログアウト |
| GET | `/api/v1/vaults/{user_id}` | 自分のボールト取得(暗号文) |
| PUT | `/api/v1/vaults/{user_id}` | ボールト更新(暗号文) |
| GET | `/api/v1/resolve/{handle}` | ハンドル → 公開メタデータ解決 |
| POST | `/api/v1/capabilities` | ケイパビリティ発行 |
| GET | `/api/v1/capabilities/{cap_id}` | ケイパビリティで暗号文取得 |
| GET | `/.well-known/oap` | インスタンス発見メタデータ |
| GET | `/.well-known/oap/health` | ヘルスチェック(MUST) |
| GET | `/.well-known/oap/status` | 詳細ステータス(SHOULD)|
| GET | `/admin/metrics` | Prometheus 形式メトリクス(MAY、管理者認証必須)|

---

## 残された詳細論点(本文執筆時に詰める)

これらは仕様書本文を書く段階で決める。アウトライン凍結時点では未決定だが、設計の根幹には影響しない。

1. **ハンドル最大長**: 64 文字想定、要再確認
2. **国際化ドメイン名(IDN)対応**: インスタンス側で許すか
3. **well-known JSON ドキュメントの正確なスキーマ**(Section 5)
4. **セッショントークン形式**: JWT vs opaque token(Section 8)
5. **レート制限の数値**(Section 9)
6. **クロスインスタンス信頼モデルの詳細**(Section 10)
7. **プロトコルバージョン交渉の詳細**(Section 12)

---

## 書く順序(本文執筆フェーズの推奨)

実際に仕様書本文を書く際の推奨順序:

1. **Section 2: Terminology** から開始(他の章で参照される用語を先に固める)
2. **Section 6: Data Model** と **Section 7: Cryptographic Requirements**(他の章の前提)
3. **Section 4: Address Code Format** と **Section 5: Instance Discovery**
4. **Section 8: Authentication** と **Section 9: API Surface**(相互依存が強い)
5. **Section 10: Federation** と **Section 11: Privacy**
6. **Section 3: Architecture Overview**(他の章の内容を踏まえて図解を整理)
7. **Section 12: Versioning** と **Section 13: IANA**
8. **Section 1: Introduction**(最後に書く、全体を踏まえた要約として)
9. **References**(完成度に応じて追加)

この順序で書くことで、相互参照の不整合が起きにくくなる。

---

## ライセンスと公開方針

仕様書(本文)は **Creative Commons BY 4.0** で公開予定。これは以下を意味する:

- 誰でも自由に複製・再配布できる
- 誰でも自由に翻訳・派生作品を作れる
- 商用利用可能
- 出典(OAP プロジェクト)の明示が必要

リファレンス実装(Lodester)は AGPL v3 で別途公開される。
