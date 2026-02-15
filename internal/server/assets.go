package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var WebFS embed.FS

func AssetsHandler() http.Handler {
	sub, _ := fs.Sub(WebFS, "web")
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
