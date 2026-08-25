package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"industrial-noise-source-attribution/backend/internal/config"
	"industrial-noise-source-attribution/backend/internal/constants"
	"industrial-noise-source-attribution/backend/internal/dto"
	"industrial-noise-source-attribution/backend/internal/model"
	"industrial-noise-source-attribution/backend/internal/repository"
)

var deferScoringSeq int64

func openDeferDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:defer-scoring-%d?mode=memory&cache=shared", atomic.AddInt64(&deferScoringSeq, 1)),
	}
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	return db
}

func validSpectrum() map[string]float64 {
	return map[string]float64{"63": 50, "125": 48, "250": 45, "500": 42, "1000": 40, "2000": 38, "4000": 36, "8000": 34}
}

func TestMonitoringPointCreatePersists(t *testing.T) {
	db := openDeferDB(t)
	svc := NewMonitoringPointService(repository.NewMonitoringPointRepository(db))
	req := dto.CreateMonitoringPointRequest{
		PointCode: "MP-TEST-01", Name: "Test point", XM: 1, YM: 2, HeightM: 1.5, AreaType: "boundary",
		BackgroundProfile: validSpectrum(), OwnerTeam: "Occupational Hygiene",
	}
	created, err := svc.Create(context.Background(), req, model.Actor{ID: 1, DisplayName: "Engineer", Role: "acoustic_engineer"})
	if err != nil {
		t.Fatalf("create monitoring point failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created point has no ID")
	}
	if _, err := svc.Get(context.Background(), created.ID); err != nil {
		t.Fatalf("created point not persisted: %v", err)
	}
}

func TestMonitoringPointDeactivatePersists(t *testing.T) {
	db := openDeferDB(t)
	svc := NewMonitoringPointService(repository.NewMonitoringPointRepository(db))
	createReq := dto.CreateMonitoringPointRequest{
		PointCode: "MP-TEST-03", Name: "Test point", XM: 1, YM: 2, HeightM: 1.5, AreaType: "boundary",
		BackgroundProfile: validSpectrum(), OwnerTeam: "Occupational Hygiene",
	}
	created, err := svc.Create(context.Background(), createReq, model.Actor{ID: 1, DisplayName: "Engineer", Role: "acoustic_engineer"})
	if err != nil {
		t.Fatalf("create monitoring point failed: %v", err)
	}
	deactivated, err := svc.Deactivate(context.Background(), created.ID, created.Version, model.Actor{ID: 1, DisplayName: "Engineer", Role: "acoustic_engineer"})
	if err != nil {
		t.Fatalf("deactivate monitoring point failed: %v", err)
	}
	if deactivated.PointState != string(constants.PointInactive) {
		t.Fatalf("state = %q, want inactive", deactivated.PointState)
	}
	got, err := svc.Get(context.Background(), created.ID)
	if err != nil || got.PointState != string(constants.PointInactive) {
		t.Fatalf("deactivated point not persisted: state=%q err=%v", got.PointState, err)
	}
}

func TestAttributionRunCreatePersists(t *testing.T) {
	db := openDeferDB(t)
	svc := NewAttributionRunService(
		repository.NewAttributionRunRepository(db),
		repository.NewNoiseMeasurementRepository(db),
		repository.NewSourceProfileRepository(db),
	)
	created, _, err := svc.Create(context.Background(), dto.CreateAttributionRunRequest{
		MeasurementIDs:   []uint{1, 2},
		SourceProfileIDs: []uint{1, 2, 3},
	}, model.Actor{ID: 1, DisplayName: "Admin", Role: "admin"}, "defer-scoring-key")
	if err != nil {
		t.Fatalf("create attribution run failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created run has no ID")
	}
	if _, err := svc.Get(context.Background(), created.ID); err != nil {
		t.Fatalf("created run not persisted: %v", err)
	}
}

func TestAttributionRunReviewPersists(t *testing.T) {
	db := openDeferDB(t)
	now := time.Now().UTC()
	run := model.AttributionRun{
		RunCode: "AR-TEST-REVIEW", MeasurementIDsJSON: "[1]", SourceProfileIDsJSON: "[1]",
		AlgorithmVersion: constants.AlgorithmVersion, InputHash: "review-hash", InputSnapshotJSON: "{}",
		NormalizedBandsJSON: "[]", ContributionsJSON: "[]", EvidenceJSON: "{}",
		AttributionState: string(constants.AttributionCompleted), Explanation: "fixture",
		StartedAt: now, FinishedAt: &now, CreatedBy: 1, Version: 3, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("seed run: %v", err)
	}
	svc := NewAttributionRunService(
		repository.NewAttributionRunRepository(db),
		repository.NewNoiseMeasurementRepository(db),
		repository.NewSourceProfileRepository(db),
	)
	reviewed, err := svc.Review(context.Background(), run.ID, dto.ReviewAttributionRequest{Note: "looks good", Version: run.Version},
		model.Actor{ID: 2, DisplayName: "Reviewer", Role: "reviewer"})
	if err != nil {
		t.Fatalf("review attribution run failed: %v", err)
	}
	if reviewed.AttributionState != string(constants.AttributionReviewed) {
		t.Fatalf("state = %q, want reviewed", reviewed.AttributionState)
	}
	got, err := svc.Get(context.Background(), run.ID)
	if err != nil || got.AttributionState != string(constants.AttributionReviewed) {
		t.Fatalf("reviewed run not persisted: state=%q err=%v", got.AttributionState, err)
	}
}
