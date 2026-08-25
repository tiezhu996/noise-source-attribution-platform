package service

import (
	"context"
	"time"
	"errors"
	"net/http"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"industrial-noise-source-attribution/backend/internal/dto"
	"industrial-noise-source-attribution/backend/internal/model"
	"industrial-noise-source-attribution/backend/internal/repository"
	"industrial-noise-source-attribution/backend/internal/util"
)

// A source profile with an incomplete power spectrum must be rejected at
// creation time instead of being stored and failing attribution later.
func TestSourceProfileCreateRejectsInvalidPower(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:srcprof-scoring?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.SourceProfile{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewSourceProfileRepository(db)
	svc := NewSourceProfileService(repo)

	directivity := map[string]float64{"63": 0, "125": 0, "250": 0, "500": 0, "1000": 0, "2000": 0, "4000": 0, "8000": 0}
	req := dto.CreateSourceProfileRequest{
		SourceCode: "SRC-TEST-01", Name: "Test source", XM: 1, YM: 2, HeightM: 1,
		ReferenceDistanceM: 1, OctavePower: map[string]float64{"1000": 90}, Directivity: directivity,
		OperatingFactor: 0.8,
	}
	_, err = svc.Create(context.Background(), req, model.Actor{ID: 1, DisplayName: "Engineer", Role: "acoustic_engineer"})
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("incomplete power spectrum accepted: err=%v", err)
	}
}

// An active source profile must carry a complete power spectrum; activating a
// broken profile must be rejected instead of poisoning later attribution runs.
func TestSourceProfileActivateRejectsInvalidPower(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:srcprof-activate?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.SourceProfile{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewSourceProfileRepository(db)
	svc := NewSourceProfileService(repo)

	now := time.Now().UTC()
	directivity := map[string]float64{"63": 0, "125": 0, "250": 0, "500": 0, "1000": 0, "2000": 0, "4000": 0, "8000": 0}
	profile := model.SourceProfile{
		SourceCode: "SRC-TEST-02", Name: "Test source", XM: 1, YM: 2, HeightM: 1, ReferenceDistanceM: 1,
		OctavePowerJSON: mustJSON(map[string]float64{"1000": 90}), DirectivityJSON: mustJSON(directivity),
		OperatingFactor: 0.8, ProfileState: "draft", Version: 1, LockVersion: 1,
		CreatedBy: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	_, err = svc.Transition(context.Background(), profile.ID, dto.ProfileTransitionRequest{
		ToState:     "active",
		LockVersion: profile.LockVersion,
	}, model.Actor{ID: 1, DisplayName: "Engineer", Role: "acoustic_engineer"})
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("activating incomplete profile accepted: err=%v", err)
	}
}
