package webassets

import "embed"

//go:embed index.html jquery-3.1.1.min.js
// FS contains static web assets.
var FS embed.FS
