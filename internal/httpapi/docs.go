package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	contentdocs "github.com/limecloud/contentcloud/docs/content"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) docsCatalog(w http.ResponseWriter, r *http.Request) {
	setDocsResponseHeaders(w)
	catalog, err := contentdocs.LoadCatalog()
	s.dispatchResult(w, r, "docs.catalog", catalog, err)
}

func (s *Server) docsPage(w http.ResponseWriter, r *http.Request) {
	setDocsResponseHeaders(w)
	page, err := contentdocs.LoadPage(chi.URLParam(r, "*"))
	if errors.Is(err, contentdocs.ErrPageNotFound) {
		err = domain.NotFound("文档页面")
	}
	if err != nil {
		s.fail(w, r, "docs.page", err)
		return
	}
	if acceptsMarkdown(r) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page.Markdown))
		return
	}
	s.ok(w, r, "docs.page", page)
}

func setDocsResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Language", "zh-CN")
	w.Header().Set("Vary", "Accept")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
}

func acceptsMarkdown(r *http.Request) bool {
	for _, value := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil || !strings.EqualFold(mediaType, "text/markdown") {
			continue
		}
		quality := 1.0
		if rawQuality, ok := parameters["q"]; ok {
			quality, err = strconv.ParseFloat(rawQuality, 64)
			if err != nil {
				continue
			}
		}
		if quality > 0 {
			return true
		}
	}
	return false
}
