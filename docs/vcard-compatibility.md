# vCard Compatibility Report

Lodester が出力する vCard 4.0 の、主要アドレス帳実装での互換性レポート。
Priya Raman(i18n レビュアー)と黒木(シニアエンジニアレビュアー)の M4 レビューで「実機検証必須」と指摘された内容の記録。

## 結論(TL;DR)

Lodester は **vCard 3.0(RFC 2426)を出力する**。 Apple Contacts は
vCard 4.0 を完全にはサポートしておらず、BOM 付き 4.0 ではパースに失敗する
(ファイルプレビューのみ表示され、連絡先として取り込めない)。3.0 なら
BOM なしで UTF-8 を正しく解釈する。

- **BOM は出さない**(Apple パーサーを壊す)
- **ALTID は使わない**(vCard 3.0 非対応、かつ iOS は 4.0 でも無視する)
- **複数スクリプトの N / ADR 変種は最初(プライマリ)のみ出力**
- **単一 ADR に `LANGUAGE=` パラメータも付けない**(iOS が field label として
  表示してしまうため)
- **FN は Latin 変種があればそれを優先**(ビジネスカード用途での互換性)

## 背景

vCard 4.0(RFC 6350 §3.1)は UTF-8 を唯一の文字エンコーディングとして定めている。
BOM は必須でも禁止でもない。多くの実装は UTF-8 を前提にパーサが動作するため BOM は不要だが、
Apple Contacts は歴史的経緯から BOM なしの vCard をレガシーエンコーディング(macOS: Mac Roman、
iOS: 同等)として読み込むことが知られている。

## 実施日

- **2026-04-09**: 初回検証(Taketo、iPhone)
- **2026-04-09**: 2 回目検証(BOM 追加で回帰 → vCard 3.0 にダウングレード)

## 検証環境

| 項目 | 値 |
|---|---|
| Lodester バージョン | v0.0.1(M4 完了時点) |
| iOS バージョン | iOS 26(仮) |
| 受信経路 | Windows → iCloud 経由で iPhone に転送 → Contacts アプリでインポート |

## 初回検証結果(BOM 適用前)

### 症状

| フィールド | 表示内容 | 評価 |
|---|---|---|
| FN(ラテン) | `Taketo YANAI` | OK |
| TEL | `080 2895 6190` | OK |
| ADR 数字部 | `744-0073` / `JP` | OK(ASCII のみ) |
| ADR 日本語部 | `±±Âè£Áúå ‰∏ăÊùæÂ∏Ç` 等 | **文字化け** |
| ADR のラベル | `LANGUAGE` | **パラメータ名が露出** |
| N(日本語プライマリ) | 表示されず、FN のラテン名のみ | 仕様どおり |
| X-GDA-CODE | 表示されず | iOS は未知の X- 拡張を非表示 |

### 文字化けの詳細

`山` (U+5C71) → UTF-8 で `e5 b1 b1` → Mac Roman として解釈すると:
- `e5` → `Â`
- `b1` → `±`
- `b1` → `±`
- 結果: `Â±±`

スクリーンショット上の `Â±±Âè£Áúå ‰∏ăÊùæÂ∏Ç` は `山口県 下松市` を
UTF-8 → Mac Roman 誤変換で得られる文字列と完全に一致する。
**ファイル自体は正しい UTF-8**(`xxd` でバイトレベル確認済み)。

### 原因

1. **BOM なし** → Apple Contacts が UTF-8 を認識せず Mac Roman にフォールバック
2. **`ADR;LANGUAGE=ja-Jpan:`** → iOS がパラメータ `LANGUAGE` をフィールドラベルとして表示

## 2 回目検証(BOM 適用後、失敗)

### 試した変更

- `Export()` 先頭で U+FEFF を書き出す
- `ADR;LANGUAGE=ja-Jpan:...` を bare `ADR:...` に変更

### 結果

- iPhone で `.vcf` を開くと **ファイルプレビュー画面** が表示され、連絡先の
  インポート UI に遷移しない(`電子名刺 249 バイト` とだけ表示)
- 文字化けの検証以前に、そもそもパースに失敗している
- スクリーンショット: `docs/screenshots/ios_bom_fail.png`(参考)

### 原因

Apple Contacts のパーサーは UTF-8 BOM を前提としておらず、`BEGIN:VCARD` が
先頭バイトから始まらないと vCard ファイルとして識別しないと推定される。

## 3 回目の対策(採用)— vCard 3.0 へのダウングレード

### 決定事項

| 項目 | M4 時点(4.0) | 修正後(3.0) |
|---|---|---|
| VERSION | `4.0` | `3.0` |
| BOM | なし | なし |
| 複数 N 変種 | `ALTID=2;LANGUAGE=` で並列 | プライマリのみ、2 番目以降は破棄 |
| 複数 ADR 変種 | `ALTID=2;LANGUAGE=` で並列 | プライマリのみ、2 番目以降は破棄 |
| 単一 ADR の LANGUAGE | 付ける | 付けない |
| FN の選び方 | Latin を優先 | Latin を優先(変更なし) |

### 実装

[internal/vcard/exporter.go](../internal/vcard/exporter.go) の `Export()`:

```go
writeLine(&b, "BEGIN:VCARD")
writeLine(&b, "VERSION:3.0")
writeLine(&b, "FN:"+escape(formatFN(card)))
if len(card.Names) > 0 {
    writeLine(&b, "N:"+joinN(card.Names[0]))
}
// ...
if len(card.Addresses) > 0 {
    writeLine(&b, "ADR:"+joinADR(card.Addresses[0]))
}
```

### 根拠

- Apple Contacts は vCard 3.0 を一等市民として扱う。UTF-8 も BOM なしで
  正しく認識する(Google Contacts の vCard 3.0 エクスポートが iPhone で
  問題なく動作することで傍証される)
- vCard 3.0 は ALTID を持たないが、iOS は vCard 4.0 でも ALTID 変種を
  表示しないため、機能的な損失は iOS ユーザーにとってゼロ
- Priya(M4 レビュー)が「ALTID が主要実装で動かない場合は実装改修が必要」
  と事前に警告しており、その通りに対応した形

### 既知のトレードオフ

- **複数スクリプトの名前 / 住所を同時に保持できない**。たとえば「柳井 建人 / YANAI Taketo」
  の両方を vCard に埋め込めない。ユーザーがどちらの表記を優先するかは Lodester 側の
  入力順で決まる(最初に入れた方がプライマリ)。
- Phase 1b で次の改善を検討:
  - `X-PHONETIC-FIRST-NAME` / `X-PHONETIC-LAST-NAME` による Latin 変種の保持
  - ユーザーごとの「優先スクリプト」設定(エクスポート時にどちらをプライマリにするか)
  - サーバー側で Accept-Language を見て自動切り替え

## 再検証(3 回目、vCard 3.0 ダウングレード後、合格)

### iOS Contacts(2026-04-09、Taketo 実機)

- [x] 日本語 ADR が化けずに表示される(`山口県 下松市 美里町１丁目１番１４号`)
- [x] 住所ブロックのラベルが `住所`(`LANGUAGE` の露出なし)
- [x] ファイルをタップすると連絡先詳細画面が開く(ファイルプレビューではない)
- [x] TEL は `voice` ラベル付きで `080 2895 6190`
- [x] POSTAL(`744-0073`)、COUNTRY(`JP`)正しい
- [x] 表示名は **N フィールドの日本語変種**(`柳井建人`)。iOS は日本語ロケールでは
  FN(Latin 優先)より N を優先する模様
- [ ] X-GDA-CODE は非表示(iOS は未知の X- 拡張をフィルタ)。エクスポート逆検証で
  保持されるかは M5 で確認予定

### Google Contacts(Web / Android)

- [ ] 日本語 ADR 表示
- [ ] ALTID の 2 番目の名前変種の扱い
- [ ] X-GDA-CODE 保持

### macOS Contacts(Apple)

- [ ] iOS と同じ挙動か確認

### Outlook(Microsoft 365)

- [ ] 日本語 ADR 表示
- [ ] ALTID の扱い

## 既知の制限

- **iOS は ALTID による多スクリプト名前変種を非表示**: FN の値のみを表示する。
  これは RFC 違反ではなく「iOS の実装選択」。Lodester 側で対応する場合は、
  ユーザー設定で FN をどのスクリプトに固定するかを選ばせる UI を Phase 1b で検討。
- **iOS は X-GDA-CODE を非表示**: vCard からのエクスポート時に保持されるかは未検証。
  Lodester → iOS → 別デバイス → Lodester の往復で GDA コードが復元できれば合格。
  M5 の実機検証で確認する。

## 参考資料

- RFC 6350 (vCard 4.0) §3.1 Charset, §6.2.2 N, §6.3.1 ADR, §6.7.1 ALTID
- Apple Contacts vCard parsing(公式ドキュメントなし、経験則のみ)
- Google Contacts vCard import/export 仕様
