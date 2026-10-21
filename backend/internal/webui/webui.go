package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var content embed.FS

// Handler serves the Vite production build from /admin, including SPA fallback.
type Handler struct {
	files      fs.FS
	index      []byte
	fileServer http.Handler
}

func Available() bool {
	_, err := fs.Stat(content, "dist/index.html")
	return err == nil
}

func New() (*Handler, bool) {
	sub, err := fs.Sub(content, "dist")
	if err != nil {
		return nil, false
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, false
	}
	return &Handler{
		files:      sub,
		index:      index,
		fileServer: http.FileServer(http.FS(sub)),
	}, true
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/admin")
	if raw == "" || raw == "/" {
		h.serveIndex(w)
		return
	}
	clean := path.Clean("/" + raw)
	if clean == "/" || clean == "/index.html" {
		h.serveIndex(w)
		return
	}
	if strings.HasPrefix(clean, "/assets/") {
		h.serveFile(w, r, clean)
		return
	}
	if h.exists(clean) {
		h.serveFile(w, r, clean)
		return
	}
	h.serveIndex(w)
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, clean string) {
	if strings.HasPrefix(clean, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	clone := r.Clone(r.Context())
	clone.URL.Path = clean
	h.fileServer.ServeHTTP(w, clone)
}

func (h *Handler) exists(clean string) bool {
	name := strings.TrimPrefix(clean, "/")
	file, err := h.files.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	stat, err := file.Stat()
	return err == nil && !stat.IsDir()
}

func (h *Handler) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.index)
}
