package repository

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"industrial-noise-source-attribution/backend/internal/config"
)

var auditListSeq int64

func openAuditScoringDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:audit-scoring-%d?mode=memory&cache=shared", atomic.AddInt64(&auditListSeq, 1)),
	}
	db, err := config.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	return db
}

// The audit log query must not prepend a phantom zero-valued row.
func TestAuditLogsNoPhantomEntry(t *testing.T) {
	db := openAuditScoringDB(t)
	repo := NewSupportRepository(db)
	logs, err := repo.ListAudits(context.Background(), AuditFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected seeded audit logs")
	}
	seen := map[uint]bool{}
	for _, log := range logs {
		if log.ID == 0 {
			t.Fatalf("phantom audit log with ID 0 returned by repository")
		}
		if seen[log.ID] {
			t.Fatalf("duplicate audit log ID %d returned by repository", log.ID)
		}
		seen[log.ID] = true
	}
}
