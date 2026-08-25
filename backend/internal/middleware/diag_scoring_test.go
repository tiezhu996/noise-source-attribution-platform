package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"industrial-noise-source-attribution/backend/internal/model"
)

func diagEngine(limiter *RateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(limiter.Middleware("scope"))
	engine.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return engine
}

// Every limiter instance must keep its own window.
func TestRateLimiterIsolatedCounters(t *testing.T) {
	login := NewRateLimiter(2)
	attribution := NewRateLimiter(2)
	loginEngine := diagEngine(login)
	attributionEngine := diagEngine(attribution)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		w := httptest.NewRecorder()
		loginEngine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("login request %d blocked: %d", i, w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	w := httptest.NewRecorder()
	attributionEngine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("attribution blocked by shared window: %d", w.Code)
	}
}

// Concurrent requests must not race on the window map (unique keys keep every
// call a write).
func TestRateLimiterConcurrentWindows(t *testing.T) {
	limiter := NewRateLimiter(100)
	engine := diagEngine(limiter)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			<-start
			for j := 0; j < 40; j++ {
				req := httptest.NewRequest(http.MethodGet, "/ping", nil)
				req.RemoteAddr = fmt.Sprintf("198.51.%d.%d:1", (offset+j)%200, j%250)
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
			}
		}(i)
	}
	close(start)
	wg.Wait()
}

// Concurrent permission denials must not race on the shared denial counter.
func TestRateLimiterConcurrentDenials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("actor", model.Actor{ID: 7, Username: "analyst", Role: "data_analyst"})
		c.Next()
	})
	engine.Use(RequirePermission(PermissionConfirm))
	engine.GET("/confirm", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 30; j++ {
				req := httptest.NewRequest(http.MethodGet, "/confirm", nil)
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
			}
		}()
	}
	close(start)
	wg.Wait()
}
