package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

// content contains the complete offline web terminal, including xterm.js.
//
//go:embed public
var content embed.FS

// FileSystem returns the embedded web UI rooted at its public directory.
func FileSystem() http.FileSystem {
	root, err := fs.Sub(content, "public")
	if err != nil {
		panic("invalid embedded web UI: " + err.Error())
	}
	return http.FS(root)
}
