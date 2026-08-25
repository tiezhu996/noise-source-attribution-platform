package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"industrial-noise-source-attribution/backend/internal/model"
)

// A panicking route must still produce a proper 500 JSON error response
// instead of leaving the client with an empty success.
func TestPanicRecoveryReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	engine := gin.New()
	engine.Use(Recovery(logger))
	engine.GET("/boom", func(c *gin.Context) {
		panic("kaboom")
	})
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500 (body %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "internal_error") {
		t.Fatalf("panic response missing error code: %s", w.Body.String())
	}
}

// The audit boundary must record the actual response status, not the default
// value before the handler runs.
func TestAuditLogsActualStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("actor", model.Actor{ID: 1, Username: "u", DisplayName: "U", Role: "admin"})
		c.Next()
	})
	engine.Use(AuditBoundary(logger))
	engine.POST("/write", func(c *gin.Context) {
		c.String(http.StatusBadRequest, "bad")
	})
	req := httptest.NewRequest(http.MethodPost, "/write", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("handler status = %d, want 400", w.Code)
	}
	if !strings.Contains(buf.String(), `status=400`) && !strings.Contains(buf.String(), "status 400") {
		t.Fatalf("audit log does not record actual status 400: %q", buf.String())
	}
}
