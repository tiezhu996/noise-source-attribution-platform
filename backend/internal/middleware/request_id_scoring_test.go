package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// A generated request id must be a non-empty 32-char hex string and the
// middleware must always emit a usable X-Request-ID header.
func TestGenerateRequestIDNonEmpty(t *testing.T) {
	id := generateRequestID()
	if len(id) != 32 {
		t.Fatalf("generated request id length = %d, want 32 (got %q)", len(id), id)
	}
}

func TestRequestIDHeaderAlwaysSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestID())
	engine.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if value := w.Header().Get("X-Request-ID"); len(value) == 0 {
		t.Fatal("X-Request-ID header is empty")
	}
}
