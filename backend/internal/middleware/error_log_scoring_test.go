package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"industrial-noise-source-attribution/backend/internal/util"
)

// A responded 5xx must be visible in the logs even when the route did not
// attach a gin error, otherwise server failures stay silent.
func TestErrorHandlerLogsRespondedFailures(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ErrorHandler(logger))
	engine.GET("/boom", func(c *gin.Context) {
		util.RespondError(c, util.NewError(http.StatusInternalServerError, "internal_error", "boom", nil))
	})
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(buf.String(), "server error") {
		t.Fatalf("server error not logged: %q", buf.String())
	}
}
