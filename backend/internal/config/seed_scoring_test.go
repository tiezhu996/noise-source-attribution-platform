package config

import (
	"fmt"
	"sync/atomic"
	"testing"

	"industrial-noise-source-attribution/backend/internal/model"
)

var seedSeq int64

// A fresh deployment must seed all five demo accounts so login works out of
// the box; a swallowed seed failure must never leave the users table empty.
func TestSeedUsersAvailable(t *testing.T) {
	cfg := Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:seed-scoring-%d?mode=memory&cache=shared", atomic.AddInt64(&seedSeq, 1)),
	}
	db, err := OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count == 0 {
		t.Fatal("seed produced no users; demo accounts cannot log in")
	}
	if count != 5 {
		t.Fatalf("seeded user count = %d, want 5", count)
	}
}

// A fresh deployment must also seed the monitoring points and source profiles;
// a swallowed seed failure must not leave those tables empty either.
func TestSeedPointsAvailable(t *testing.T) {
	cfg := Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:seed-points-%d?mode=memory&cache=shared", atomic.AddInt64(&seedSeq, 1)),
	}
	db, err := OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	var count int64
	if err := db.Model(&model.MonitoringPoint{}).Count(&count).Error; err != nil {
		t.Fatalf("count points: %v", err)
	}
	if count != 3 {
		t.Fatalf("seeded point count = %d, want 3", count)
	}
}

func TestSeedProfilesAvailable(t *testing.T) {
	cfg := Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:seed-profiles-%d?mode=memory&cache=shared", atomic.AddInt64(&seedSeq, 1)),
	}
	db, err := OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	var count int64
	if err := db.Model(&model.SourceProfile{}).Count(&count).Error; err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if count != 3 {
		t.Fatalf("seeded profile count = %d, want 3", count)
	}
}
