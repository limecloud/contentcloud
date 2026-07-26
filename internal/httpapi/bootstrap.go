package httpapi

import (
	_ "embed"
	"io"
	"net/http"
)

//go:embed bootstrap.md
var bootstrapDocument string

func (s *Server) bootstrap(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, bootstrapDocument)
}
