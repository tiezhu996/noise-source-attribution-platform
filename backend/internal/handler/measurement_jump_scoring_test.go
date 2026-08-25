package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"fmt"
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

var jumpSeq int64

func jumpEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:jump-scoring-%d?mode=memory&cache=shared", atomic.AddInt64(&jumpSeq, 1)),
		JWTSecret: "jump-scoring-secret", JWTExpiry: 8 * time.Hour,
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

func jumpToken(t *testing.T, engine *gin.Engine) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "engineer", "password": "engineer123"})
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
	if payload.Data.Token == "" {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	return payload.Data.Token
}

// A measurement still in captured state must not be able to jump straight to
// ready, skipping validated and normalized.
func TestMeasurementCannotJumpToReady(t *testing.T) {
	engine := jumpEngine(t)
	token := jumpToken(t, engine)

	bands := map[string]float64{"63": 65, "125": 68, "250": 70, "500": 67, "1000": 63, "2000": 59, "4000": 55, "8000": 50}
	createBody, _ := json.Marshal(map[string]any{
		"monitoring_point_id": 1, "measured_at": "2026-08-25T00:00:00Z", "duration_s": 900,
		"octave_bands": bands, "overall_dba": 70, "background_dba": 45,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/noise-measurements", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var created struct {
		Data struct {
			ID      uint   `json:"id"`
			Version uint   `json:"version"`
			State   string `json:"measurement_state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.Data.ID == 0 {
		t.Fatalf("create measurement failed: %d %s", w.Code, w.Body.String())
	}
	if created.Data.State != "captured" {
		t.Fatalf("state = %q, want captured", created.Data.State)
	}
	transBody, _ := json.Marshal(map[string]any{"to_state": "normalized", "version": created.Data.Version})
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/noise-measurements/%d/transition", created.Data.ID), bytes.NewReader(transBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("captured -> normalized transition status = %d, want 409 (body %s)", w2.Code, w2.Body.String())
	}
}
