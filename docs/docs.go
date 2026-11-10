// Package docs embeds gosd's top-level Markdown documentation so `gosd docs`
// can show it offline. It exists only because go:embed cannot reach parent
// directories, so the embed must live beside the files; it is not part of
// gosd's public API.
package docs

import "embed"

// FS holds the docs/*.md files, not docs/design or docs/development.
//
//go:embed *.md
var FS embed.FS
