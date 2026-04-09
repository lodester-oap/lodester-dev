# govulncheck Report (Step 0.3 / M5 pre-release audit)

- **Date:** 2026-04-09
- **Toolchain:** Go 1.26.1 windows/amd64
- **Scanner:** govulncheck v1.1.4 (vuln.go.dev, DB updated 2026-04-08)
- **Command:** `govulncheck -show verbose ./...`

## TL;DR

**4 callable vulnerabilities** found, all in the Go standard library and **all
fixed in Go 1.26.2**. The fix is a single toolchain upgrade; no code changes
are required. A further 2 non-callable package vulnerabilities in
`jackc/pgx/v5` and 3 non-callable stdlib module-level vulnerabilities were
flagged but do not affect our call paths.

### Action items

- [ ] Upgrade local dev toolchain to **Go 1.26.2** (fixes all 4 callable
      stdlib vulns). Update `go.mod` `go 1.26.2` line accordingly.
- [ ] Monitor `jackc/pgx/v5` upstream for a patched release that addresses
      CVE-2026-33815 / CVE-2026-33816. Current status: "Fixed in: N/A". Our
      code does not currently exercise the affected symbols.
- [ ] Re-run `govulncheck ./...` after the upgrade and record a clean scan
      before cutting the M5 MVP release.

## Callable vulnerabilities (Symbol Results)

### #1 — GO-2026-4947 · crypto/x509 chain building DoS

- Found in `crypto/x509@go1.26.1`, fixed in `crypto/x509@go1.26.2`
- Trace: `cmd/lodester/main.go:63:31` → `http.Server.ListenAndServe` →
  `x509.Certificate.Verify`

### #2 — GO-2026-4946 · crypto/x509 inefficient policy validation

- Found in `crypto/x509@go1.26.1`, fixed in `crypto/x509@go1.26.2`
- Trace: same as #1

### #3 — GO-2026-4870 · crypto/tls TLS 1.3 KeyUpdate DoS

- Found in `crypto/tls@go1.26.1`, fixed in `crypto/tls@go1.26.2`
- Traces:
  - `cmd/lodester/main.go:63:31` → `tls.Conn.HandshakeContext`
  - `internal/handler/session.go:96:24` → `rand.Read` → `tls.Conn.Read`
  - `cmd/lodester/main.go:63:31` → `tls.Conn.Write`

### #4 — GO-2026-4866 · crypto/x509 name constraint auth bypass

- Found in `crypto/x509@go1.26.1`, fixed in `crypto/x509@go1.26.2`
- Trace: same as #1
- Severity: this is the most serious of the four — a malformed certificate
  could bypass `excludedSubtrees` name constraints. We do not currently
  configure custom name constraints, but upgrading closes the gap without
  any policy work on our side.

## Non-callable vulnerabilities (Package Results)

These are present in dependencies we import, but govulncheck's static
analysis did not find a reachable call path from our code.

### #1 — GO-2026-4772 · CVE-2026-33816 in `github.com/jackc/pgx/v5@v5.9.1`

- Fixed in: **N/A** (no patched release available yet as of 2026-04-09)
- Impact: not currently callable from our code

### #2 — GO-2026-4771 · CVE-2026-33815 in `github.com/jackc/pgx/v5@v5.9.1`

- Fixed in: **N/A**
- Impact: not currently callable from our code

## Non-callable vulnerabilities (Module Results)

Flagged because the affected stdlib subpackages exist in the toolchain, but
we do not call them.

- **GO-2026-4869** `archive/tar` unbounded allocation — fixed in `stdlib@go1.26.2`
- **GO-2026-4865** `html/template` XSS brace depth tracking — fixed in `stdlib@go1.26.2`
- **GO-2026-4864** `os.Root.Chmod` TOCTOU root escape (Linux only) — fixed in `stdlib@go1.26.2`

## Scope

Scanned modules:

- github.com/lodester-oap/lodester
- github.com/go-chi/chi/v5@v5.2.5
- github.com/jackc/pgpassfile@v1.0.0
- github.com/jackc/pgservicefile@v0.0.0-20240606120523-5a60cdf6a761
- github.com/jackc/pgx/v5@v5.9.1
- github.com/jackc/puddle/v2@v2.2.2
- golang.org/x/crypto@v0.49.0
- golang.org/x/sync@v0.20.0
- golang.org/x/sys@v0.42.0
- golang.org/x/text@v0.35.0

## Why this matters for M5

M5 is the MVP release. Shipping with 4 known callable stdlib vulnerabilities
— one of which is an auth-bypass in certificate validation — is not
acceptable for a security-focused zero-knowledge project. The upgrade to Go
1.26.2 is a small, well-scoped chore and should land before the release
branch is cut.
