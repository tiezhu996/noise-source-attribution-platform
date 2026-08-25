package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

var errWrapSeq int64

func errWrapEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:errwrap-scoring-%d?mode=memory&cache=shared", atomic.AddInt64(&errWrapSeq, 1)),
		JWTSecret: "errwrap-scoring-secret", JWTExpiry: 8 * time.Hour,
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

func errWrapToken(t *testing.T, engine *gin.Engine, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var payload struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &payload)
	return payload.Data.Token
}

func doReq(t *testing.T, engine *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
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

func TestCompareMissingRunKeeps404(t *testing.T) {
	engine := errWrapEngine(t)
	token := errWrapToken(t, engine, "admin", "admin123")
	w := doReq(t, engine, http.MethodGet, "/api/v1/attribution-runs/999999/compare/1", token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("compare missing run status = %d, want 404 (body %s)", w.Code, w.Body.String())
	}
}

func TestDeactivatePointStaleVersionKeeps409(t *testing.T) {
	engine := errWrapEngine(t)
	token := errWrapToken(t, engine, "engineer", "engineer123")
	w := doReq(t, engine, http.MethodPost, "/api/v1/monitoring-points/1/deactivate", token,
		map[string]any{"version": 999})
	if w.Code != http.StatusConflict {
		t.Fatalf("stale point deactivate status = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
}

func TestImportInvalidSpectrumKeeps422(t *testing.T) {
	engine := errWrapEngine(t)
	token := errWrapToken(t, engine, "engineer", "engineer123")
	body := map[string]any{
		"monitoring_point_id": 1, "measured_at": "2026-08-25T00:00:00Z", "duration_s": 900,
		"octave_bands": map[string]float64{"1000": 70}, "overall_dba": 70, "background_dba": 45,
	}
	w := doReq(t, engine, http.MethodPost, "/api/v1/noise-measurements", token, body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid spectrum import status = %d, want 422 (body %s)", w.Code, w.Body.String())
	}
}

func TestSourceTransitionIllegalKeeps409(t *testing.T) {
	engine := errWrapEngine(t)
	token := errWrapToken(t, engine, "engineer", "engineer123")
	// seeded source 1 is active; active -> draft is illegal
	body := map[string]any{"to_state": "draft", "lock_version": 1}
	w := doReq(t, engine, http.MethodPost, "/api/v1/source-profiles/1/transition", token, body)
	if w.Code != http.StatusConflict {
		t.Fatalf("illegal source transition status = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
}

func TestLoginWrongPasswordKeeps401(t *testing.T) {
	engine := errWrapEngine(t)
	w := doReq(t, engine, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": "admin", "password": "wrong-password"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password login status = %d, want 401 (body %s)", w.Code, w.Body.String())
	}
}
