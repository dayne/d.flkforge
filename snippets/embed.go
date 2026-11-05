// Package snippets is flkforge's built-in catalog, embedded in the binary.
// The layout is NAMESPACE/NAME/snippet.toml (SPEC 4).
package snippets

import (
	"embed"
	"io/fs"
)

//go:embed all:core
var files embed.FS

// FS is the built-in catalog's root: core/NAME/...
var FS fs.FS = files
