// SPDX-License-Identifier: AGPL-3.0-or-later

package logging

import "log/slog"

// Redacted implements slog.LogValuer and fmt.Stringer.
// Use it wherever sensitive data (email, password, key material)
// might accidentally reach a log line.
//
// Example:
//
//	slog.Info("user created", "email", logging.Redacted{})
type Redacted struct{}

func (Redacted) LogValue() slog.Value {
	return slog.StringValue("REDACTED")
}

func (Redacted) String() string {
	return "REDACTED"
}
