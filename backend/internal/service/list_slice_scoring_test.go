package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"industrial-noise-source-attribution/backend/internal/config"
	"industrial-noise-source-attribution/backend/internal/repository"
)

var listScoringSeq int64

func openListScoringDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:list-scoring-%d?mode=memory&cache=shared", atomic.AddInt64(&listScoringSeq, 1)),
	}
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	return db
}

// The monitoring point list must return exactly the stored points with no
// zero-valued phantom entries.
func TestMonitoringPointListNoDupes(t *testing.T) {
	db := openListScoringDB(t)
	svc := NewMonitoringPointService(repository.NewMonitoringPointRepository(db))
	points, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list points: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("list returned %d points, want 3", len(points))
	}
	for _, point := range points {
		if point.ID == 0 {
			t.Fatalf("phantom monitoring point with ID 0 in list")
		}
	}
}

// The audit list must contain only real audit logs, never zero-valued phantoms.
func TestAuditListNoDupes(t *testing.T) {
	db := openListScoringDB(t)
	svc := NewAccessService(repository.NewSupportRepository(db), "scoring-secret-secret-secret", 8*time.Hour)
	logs, err := svc.ListAudits(context.Background(), repository.AuditFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected seeded audit logs")
	}
	for _, log := range logs {
		if log.ID == 0 {
			t.Fatalf("phantom audit log with ID 0 in list")
		}
	}
}
