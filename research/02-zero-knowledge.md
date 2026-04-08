# 調査 2: ゼロ知識アーキテクチャ

## 目的

パスワードマネージャ業界で確立されたゼロ知識アーキテクチャパターンを調査し、GDA のデータ保護設計に適用可能な方式を選定する。

## 調査対象

- Bitwarden
- 1Password
- Proton Pass

## 主な発見

### Bitwarden 流(マスターパスワードのみ)

- **KDF**: PBKDF2 SHA-256 → Argon2id に切替可能(600,000 iter 相当)
- **Master Key**: マスターパスワードからソルト付き KDF で生成、256 bit
- **Stretched Master Key**: HKDF で 512 bit に伸長
- **Symmetric Key**: CSPRNG で生成、Stretched Master Key で暗号化されて保存
- **クラウド DB**: Microsoft Azure、TDE 暗号化、ユーザーデータは二重暗号化
- **特徴**:
  - シンプル、依存パラメータが少ない
  - 復旧不能(マスターパスワード忘失 = データ喪失)
  - 自己ホスト可(Vaultwarden 含む)

### 1Password 流(マスターパスワード + Secret Key)

- **Two-Secret Architecture**:
  - マスターパスワード(ユーザーが覚える)
  - Secret Key(高エントロピー、デバイス内のみ、Emergency Kit に印刷)
  - 両方ないと復号不可
- **メリット**: サーバー漏えい+パスワード推測でも復号できない
- **デメリット**: Secret Key 紛失リスク、複雑さ
- **認証**: SRP(Secure Remote Password)でパスワードを送信せずに証明

### Proton Pass 流(PGP ベース)

- **方式**: Mailbox Password が秘密鍵を暗号化、データは PGP 暗号化
- **メリット**: 既存の OpenPGP 標準に基づく
- **デメリット**: PGP 特有の複雑さ、鍵管理の煩雑さ

## 比較表

| 項目 | Bitwarden | 1Password | Proton Pass |
|---|---|---|---|
| KDF | PBKDF2/Argon2id | PBKDF2 | bcrypt 系 |
| 対称暗号 | AES-CBC-256 | AES-GCM-256 | AES-256 |
| 復旧 | なし | Emergency Kit | なし |
| OSS | 完全 | プロプライエタリ | クライアント OSS |
| 自己ホスト | 公式可 | 不可 | 不可 |
| 監査 | Cure53 | あり | Cure53 |

## GDA への適用判断

### 採用: Bitwarden 流(シンプル)

**理由**:

1. **シンプルさ**: 個人開発で実装可能な範囲
2. **AGPL との親和性**: Bitwarden 自体が GPL 系で公開されている
3. **自己ホスト前提と合致**: メール型連合モデルとの整合性
4. **住所データの特性**: パスワードと違い**記憶から再入力可能**なので、復旧機構なしのリスクが許容できる

### 採用しないもの

- **Secret Key**: 1Password 風の二要素方式は不採用(複雑すぎる)
- **PGP**: Proton 風の PGP 基盤は不採用(クライアント実装が複雑)

### 具体的アルゴリズム選定

| 用途 | 採用 |
|---|---|
| KDF | Argon2id(600,000 iter) |
| 対称暗号 | AES-GCM-256 |
| 鍵伸長 | HKDF-SHA256 |
| 非対称(Phase 2) | RSA 3072 bit |

### 復旧機構

**なし**。ユーザーがマスターパスワードを忘れたらデータは喪失する。

**根拠**:
- 住所は記憶から再入力可能
- 復旧機構を持つと「ゼロ知識」が崩れる(運営者が何らかの形でデータにアクセス可能になる)
- Bitwarden の哲学を継承

### 暗号文ヘッダ(クリプトアジリティ)

すべての暗号文ブロブに以下のヘッダを必須:

```json
{
  "schema_version": "1.0",
  "kdf": "argon2id",
  "kdf_params": { "iter": 600000, "salt": "..." },
  "cipher": "aes-gcm-256",
  "wrapped_key_id": "...",
  "created_at": "2026-..."
}
```

将来のアルゴリズム変更(ポスト量子等)にスムーズに対応可能。

## 関連決定

- DECISION-006: Bitwarden 流ゼロ知識を採用
- DECISION-007: アルゴリズム選定
- DECISION-008: Secret Key 不採用
- DECISION-009: 復旧機構なし
- DECISION-010: 暗号文ヘッダにメタデータ必須
