package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/storage"
)

// serveMedia serves local-storage objects through HMAC-signed URLs so <img>
// tags work without an Authorization header.
func (s *Server) serveMedia(c *gin.Context) {
	local, ok := s.filestore.(*storage.LocalStore)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	key := strings.TrimPrefix(c.Param("key"), "/")
	expires, err := strconv.ParseInt(strings.TrimSpace(c.Query("e")), 10, 64)
	if err != nil {
		c.String(http.StatusForbidden, "媒体地址无效")
		return
	}
	if err := local.ValidateSignedURL(key, expires, c.Query("s")); err != nil {
		c.String(http.StatusForbidden, err.Error())
		return
	}
	file, info, err := local.Open(key)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	c.Header("Cache-Control", "private, max-age=3600")
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}
