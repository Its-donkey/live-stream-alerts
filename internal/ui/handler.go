package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var content embed.FS

// Handler serves the embedded UI assets.
func Handler() http.Handler {
	sub, err := fs.Sub(content, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	fsys := http.FS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)
		if p == "/" || p == "." {
			p = "/index.html"
		}
		p = strings.TrimPrefix(p, "/")
		file, err := fsys.Open(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
}
