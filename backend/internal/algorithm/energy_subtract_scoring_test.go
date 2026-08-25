package algorithm

import (
	"math"
	"sync"
	"testing"

	"industrial-noise-source-attribution/backend/internal/constants"
)

// Concurrent energy subtraction for the same frozen input must be race free
// and must never return the shared cache entry.
func TestEnergySubtractCacheConcurrentSafe(t *testing.T) {
	background := constantSpectrum(60)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			<-start
			for j := 0; j < 40; j++ {
				// A unique measured spectrum per goroutine keeps every call a
				// cache miss so concurrent writes to the shared map overlap.
				measured := constantSpectrum(70 + float64((offset+j)%8))
				corrected, unreliable, err := EnergySubtract(measured, background)
				if err != nil {
					t.Errorf("EnergySubtract error: %v", err)
					return
				}
				if len(corrected) != len(constants.OctaveBands) {
					t.Errorf("corrected band count = %d, want %d", len(corrected), len(constants.OctaveBands))
					return
				}
				if unreliable == nil {
					t.Errorf("unreliable must not be nil")
					return
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
}

// A caller mutating the returned spectrum or unreliable list must not poison
// the cached values seen by later calls with identical input.
func TestEnergySubtractCacheReturnsIsolatedValues(t *testing.T) {
	measured := constantSpectrum(70)
	background := constantSpectrum(60)

	first, unreliable, err := EnergySubtract(measured, background)
	if err != nil {
		t.Fatalf("EnergySubtract error: %v", err)
	}
	for band := range first {
		first[band] = -100
	}
	if len(unreliable) > 0 {
		unreliable[0] = "99999"
	}

	again, unreliableAgain, err := EnergySubtract(measured, background)
	if err != nil {
		t.Fatalf("EnergySubtract error: %v", err)
	}
	want := 10 * math.Log10(math.Pow(10, 7)-math.Pow(10, 6))
	if math.Abs(again["1000"]-want) > 0.001 {
		t.Fatalf("cached spectrum polluted: 1000 Hz = %.4f, want %.4f", again["1000"], want)
	}
	for _, band := range unreliableAgain {
		if band == "99999" {
			t.Fatalf("cached unreliable list polluted: %v", unreliableAgain)
		}
	}
}
