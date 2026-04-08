# API Error Codes

All API errors return JSON with the following structure:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable description"
  }
}
```

## Error Code Reference

| Code | HTTP Status | Description |
|---|---|---|
| `INVALID_JSON` | 400 | Request body is not valid JSON |
| `MISSING_FIELD` | 400 | Required field is missing |
| `INVALID_EMAIL` | 400 | Email format is invalid |
| `INVALID_LOGIN_HASH` | 400 | login_hash format is invalid (must be 32 bytes hex-encoded) |
| `INVALID_KDF_PARAMS` | 400 | KDF parameters do not meet minimum requirements |
| `INVALID_CREDENTIALS` | 401 | Authentication failed (intentionally does not distinguish between "user not found" and "wrong password") |
| `ACCOUNT_EXISTS` | 409 | An account with this email already exists |
| `INTERNAL_ERROR` | 500 | Server-side error |

## Security Notes

- `INVALID_CREDENTIALS` is always returned for any authentication failure, regardless of whether the user exists. This prevents user enumeration attacks.
- Response timing is kept constant using dummy Argon2id computation when a user is not found.
