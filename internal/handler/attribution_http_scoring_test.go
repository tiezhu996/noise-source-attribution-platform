package handler_test

import (
	"bytes"
	"fmt"
	"sync/atomic"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"industrial-noise-source-attribution/backend/internal/config"
	"industrial-noise-source-attribution/backend/internal/handler"
	"industrial-noise-source-attribution/backend/internal/middleware"
	"industrial-noise-source-attribution/backend/internal/repository"
	"industrial-noise-source-attribution/backend/internal/router"
	"industrial-noise-source-attribution/backend/internal/service"
)

var scoringDBSeq int64

func newScoringEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite", DBDSN: fmt.Sprintf("file:scoring003-%d?mode=memory&cache=shared", atomic.AddInt64(&scoringDBSeq, 1)),
		JWTSecret: "scoring-only-change-me-secret", JWTExpiry: 8 * time.Hour,
		LoginLimitPerMinute: 10000, ImportLimitPerMinute: 10000, AttributionLimitPerMinute: 10000,
		ShutdownTimeout: 10 * time.Second,
	}
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	pointRepository := repository.NewMonitoringPointRepository(db)
	measurementRepository := repository.NewNoiseMeasurementRepository(db)
	sourceRepository := repository.NewSourceProfileRepository(db)
	runRepository := repository.NewAttributionRunRepository(db)
	supportRepository := repository.NewSupportRepository(db)

	pointHandler := handler.NewMonitoringPointHandler(service.NewMonitoringPointService(pointRepository))
	measurementHandler := handler.NewNoiseMeasurementHandler(service.NewNoiseMeasurementService(measurementRepository, pointRepository))
	sourceHandler := handler.NewSourceProfileHandler(service.NewSourceProfileService(sourceRepository))
	runHandler := handler.NewAttributionRunHandler(service.NewAttributionRunService(runRepository, measurementRepository, sourceRepository))
	supportHandler := handler.NewSupportHandler(service.NewAccessService(supportRepository, cfg.JWTSecret, cfg.JWTExpiry))

	authenticator := middleware.NewAuthenticator(cfg.JWTSecret)
	loginLimiter := middleware.NewRateLimiter(cfg.LoginLimitPerMinute)
	importLimiter := middleware.NewRateLimiter(cfg.ImportLimitPerMinute)
	attributionLimiter := middleware.NewRateLimiter(cfg.AttributionLimitPerMinute)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Recovery(logger))
	engine.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	v1 := engine.Group("/api/v1")
	api := v1.Group("")
	api.Use(authenticator.RequireAuth())
	router.RegisterSupportRoutes(v1, api, supportHandler, loginLimiter)
	router.RegisterMonitoringPointRoutes(api, pointHandler)
	router.RegisterNoiseMeasurementRoutes(api, measurementHandler, importLimiter)
	router.RegisterSourceProfileRoutes(api, sourceHandler)
	router.RegisterAttributionRunRoutes(api, runHandler, attributionLimiter)
	return engine
}

func loginToken(t *testing.T, engine *gin.Engine, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var payload struct {
		Code string `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("login decode: %v (status %d)", err, w.Code)
	}
	if payload.Data.Token == "" {
		t.Fatalf("login failed: status=%d body=%s", w.Code, w.Body.String())
	}
	return payload.Data.Token
}

func doJSON(t *testing.T, engine *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestAttributionRunMissingReturns404(t *testing.T) {
	engine := newScoringEngine(t)
	token := loginToken(t, engine, "admin", "admin123")
	w := doJSON(t, engine, http.MethodGet, "/api/v1/attribution-runs/999999", token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing run status = %d, want 404 (body %s)", w.Code, w.Body.String())
	}
}

func TestAttributionRunCreateSucceeds(t *testing.T) {
	engine := newScoringEngine(t)
	token := loginToken(t, engine, "admin", "admin123")
	w := doJSON(t, engine, http.MethodPost, "/api/v1/attribution-runs", token,
		map[string]any{"measurement_ids": []int{1, 2}, "source_profile_ids": []int{1, 2, 3}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create run status = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
}

func TestAttributionRunStaleVersionConflict409(t *testing.T) {
	engine := newScoringEngine(t)
	token := loginToken(t, engine, "admin", "admin123")
	created := doJSON(t, engine, http.MethodPost, "/api/v1/attribution-runs", token,
		map[string]any{"measurement_ids": []int{1, 2}, "source_profile_ids": []int{1, 2, 3}})
	if created.Code != http.StatusCreated {
		t.Fatalf("create run status = %d, want 201 (body %s)", created.Code, created.Body.String())
	}
	var payload struct {
		Data struct {
			ID      uint `json:"id"`
			Version uint `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode created run: %v", err)
	}
	w := doJSON(t, engine, http.MethodPost, "/api/v1/attribution-runs/"+itoa(payload.Data.ID)+"/review", token,
		map[string]any{"version": payload.Data.Version + 5, "note": "stale review"})
	if w.Code != http.StatusConflict {
		t.Fatalf("stale review status = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
}

func TestLoginInvalidCredentials401(t *testing.T) {
	engine := newScoringEngine(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong-password"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid login status = %d, want 401 (body %s)", w.Code, w.Body.String())
	}
}

func itoa(value uint) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[index:])
}
