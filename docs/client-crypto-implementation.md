# Client-Side Cryptography Implementation

This document describes the cryptographic implementation in `web/lodester-client.js`, as mandated by the M4 review (Kuroki's requirement).

## Overview

Lodester follows a zero-knowledge architecture: the master password **never** leaves the browser. All sensitive data is encrypted client-side before being sent to the server.

The client uses three cryptographic primitives:

1. **Argon2id** — Key Derivation Function (KDF) for password → key material
2. **HKDF-SHA256** — Key derivation to separate login_hash from encryption key
3. **AES-GCM-256** — Symmetric encryption for vault data

## Implementation Details

### 1. Argon2id — via `argon2-browser` (WASM)

**Library**: [argon2-browser](https://github.com/antelle/argon2-browser) v1.18.0
**Distribution**: CDN (jsdelivr) at load time
**Backend**: WebAssembly (`argon2-bundled.min.js`)

**Parameters** (must match DECISION-045):

| Parameter | Value | Rationale |
|---|---|---|
| Memory | 65536 KB (64 MB) | OWASP 2024 recommendation |
| Iterations | 3 | OWASP 2024 recommendation |
| Parallelism | 4 | OWASP 2024 recommendation |
| Hash length | 32 bytes | For AES-256 key material |
| Salt | normalized email (UTF-8) | Per DECISION-046 |

**Important notes**:
- Argon2id is **NOT** available in Web Crypto API. Browser support for Argon2id
  requires a WASM library.
- The `argon2-browser` library is a WebAssembly port of the reference
  Argon2 C implementation, maintained by @antelle.
- This is **NOT** PBKDF2 — it is genuine Argon2id with the parameters above.

**Known limitations (M4 tech debt)**:
- **CDN dependency**: The current implementation loads argon2-browser from
  jsdelivr at runtime. This is a supply-chain risk.
- **SRI hash**: Added as of 2026-04-09 (M4 Step 0.2). The script tag includes
  `integrity="sha384-XOR3aNvHciLPIf6r+2glkrmbBbLmIJ1EChMXjw8eBKBf8gE0rDq1TyUNuRdorOqi"`
  which prevents a compromised CDN from serving malicious code. If the hash
  does not match, the browser refuses to execute the script.
- **Mitigation plan**: Phase 1b will bundle argon2-browser locally (embedded
  via `go:embed`) to eliminate the CDN dependency entirely.

### 2. HKDF-SHA256 — via Web Crypto API

**API**: `crypto.subtle.deriveKey` with `{ name: "HKDF", hash: "SHA-256" }`

**Purpose**: Derive two separate keys from the single Argon2id output,
ensuring the login_hash sent to the server is unrelated to the encryption key.

**Derivation**:

```
masterKey = Argon2id(password, salt=normalized_email)

loginHash = masterKey (used directly, hex-encoded)
encKey    = HKDF-SHA256(masterKey, salt=normalized_email, info="lodester-encryption")
```

**Note on info parameter**: The `info` string `"lodester-encryption"` is a
domain separator per RFC 5869. This ensures the encryption key cannot be
confused with any future derived key (e.g., a signing key).

**Security note**: Currently `loginHash = masterKey` directly (no HKDF for
the login hash). This is acceptable because:
- The server applies a second Argon2id layer before storing (`HashLoginHash`)
- Login hash and encryption key are distinct due to the HKDF info parameter

### 3. AES-GCM-256 — via Web Crypto API

**API**: `crypto.subtle.encrypt` / `crypto.subtle.decrypt`
**Mode**: GCM (authenticated encryption)
**Key length**: 256 bits

**Nonce handling** (CRITICAL):
- **Fresh nonce every encryption** via `crypto.getRandomValues(new Uint8Array(12))`
- Nonce length: 12 bytes (96 bits), the standard for AES-GCM
- **Never reused** — this is the most important security property.
  Nonce reuse with GCM is catastrophic (key recovery possible).
- Nonce is stored in the ciphertext header, base64url-encoded

**Wire format** (matches `internal/crypto/vault_header.go`):

```
[4-byte header_len (big-endian)] [header JSON UTF-8] [AES-GCM ciphertext]
```

Header JSON:

```json
{
  "v": 1,
  "alg": "aes-gcm-256",
  "kdf": "argon2id",
  "kdf_params": {"memory":65536,"iterations":3,"parallelism":4},
  "nonce": "<12 bytes, base64url>",
  "ct_len": <ciphertext length in bytes>
}
```

## Supply Chain Considerations

### Current state (M3)
- `argon2-browser@1.18.0` loaded from `cdn.jsdelivr.net` at runtime
- No Subresource Integrity (SRI) hash

### Planned improvements
| Phase | Action |
|---|---|
| M4 (immediate) | Add SRI hash to CDN script tag |
| Phase 1b | Bundle `argon2-browser` locally via `go:embed` |
| Phase 1b | Add CSP (Content-Security-Policy) header to restrict script sources |

## Verification Checklist

When reviewing or auditing this code, verify:

- [ ] `crypto.getRandomValues()` is used for nonces (not `Math.random()`)
- [ ] Nonce is freshly generated on every encryption operation
- [ ] KDF parameters match DECISION-045
- [ ] Argon2id is genuinely Argon2id (not PBKDF2 or other substitute)
- [ ] Master password is never sent to the server (check all `apiPost` / `apiPut` call sites)
- [ ] Master password is never logged (`console.log` inspection)
- [ ] HKDF info parameter is non-empty (per RFC 5869 recommendation)
- [ ] Keys are `extractable: false` in Web Crypto API (check `deriveKey` calls)

## References

- [RFC 9106: Argon2 Memory-Hard Function](https://datatracker.ietf.org/doc/rfc9106/)
- [RFC 5869: HKDF](https://datatracker.ietf.org/doc/html/rfc5869)
- [RFC 5116: AEAD interfaces (incl. AES-GCM)](https://datatracker.ietf.org/doc/html/rfc5116)
- [NIST SP 800-38D: AES-GCM](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf)
- [argon2-browser GitHub](https://github.com/antelle/argon2-browser)
- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
