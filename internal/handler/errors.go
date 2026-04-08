// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"encoding/json"
	"net/http"
)

// Error codes. Do NOT reveal user existence in error responses.
const (
	CodeInvalidJSON        = "INVALID_JSON"
	CodeInvalidEmail       = "INVALID_EMAIL"
	CodeInvalidLoginHash   = "INVALID_LOGIN_HASH"
	CodeInvalidKDFParams   = "INVALID_KDF_PARAMS"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeAccountExists      = "ACCOUNT_EXISTS"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeMissingField       = "MISSING_FIELD"
)

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{
		Error: errorDetail{Code: code, Message: message},
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
