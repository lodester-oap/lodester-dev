// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html lodester-client.js share.html
var content embed.FS

// FS returns the embedded web assets as a filesystem.
func FS() fs.FS {
	return content
}
