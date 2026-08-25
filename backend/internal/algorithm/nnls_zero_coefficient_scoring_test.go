package algorithm

import (
	"math"
	"testing"

	"industrial-noise-source-attribution/backend/internal/constants"
)

// A candidate source that contributes nothing (coefficient clamped to zero)
// must still produce a complete, non-panicking contribution result.
func TestFitAttributionZeroCoefficientSourceNoPanic(t *testing.T) {
	background := constantSpectrum(35)
	points := []PointInput{
		{ID: 1, PointCode: "P1", XM: 20, YM: 0, HeightM: 1.5, Background: background},
		{ID: 2, PointCode: "P2", XM: 0, YM: 30, HeightM: 1.5, Background: background},
	}
	sources := []SourceInput{
		{ID: 1, SourceCode: "S1", Name: "Source 1", XM: 0, YM: 0, HeightM: 1, ReferenceDistanceM: 1, Power: varyingSpectrum(94, .8), Directivity: constantSpectrum(0), OperatingFactor: .9, Version: 1},
		{ID: 2, SourceCode: "S2", Name: "Source 2", XM: 35, YM: 18, HeightM: 4, ReferenceDistanceM: 1, Power: varyingSpectrum(88, -.45), Directivity: constantSpectrum(-1), OperatingFactor: .7, Version: 2},
		{ID: 3, SourceCode: "S3", Name: "Distant source", XM: 800, YM: 950, HeightM: 2, ReferenceDistanceM: 1, Power: varyingSpectrum(66, -.3), Directivity: constantSpectrum(0), OperatingFactor: .5, Version: 1},
	}
	wantCoefficients := []float64{1.7, .45, 0}
	measurements := make([]MeasurementInput, 0, len(points))
	for index, point := range points {
		bands := make(Spectrum, len(constants.OctaveBands))
		for _, band := range constants.OctaveBands {
			energy := DBToRelativeEnergy(background[BandKey(band)])
			for sourceIndex, source := range sources {
				propagated, err := PropagatedDB(source, point, band)
				if err != nil {
					t.Fatalf("PropagatedDB error: %v", err)
				}
				energy += wantCoefficients[sourceIndex] * DBToRelativeEnergy(propagated)
			}
			bands[BandKey(band)] = RelativeEnergyToDB(energy)
		}
		measurements = append(measurements, MeasurementInput{ID: uint(index + 1), Checksum: "fixture", Point: point, Bands: bands})
	}

	result, err := FitAttribution(measurements, sources)
	if err != nil {
		t.Fatalf("FitAttribution error: %v", err)
	}
	coefficients := map[uint]float64{}
	for _, contribution := range result.Contributions {
		coefficients[contribution.SourceProfileID] = contribution.Coefficient
	}
	if math.Abs(coefficients[3]) > 1e-6 {
		t.Fatalf("distant source coefficient = %v, want 0", coefficients[3])
	}
	for _, contribution := range result.Contributions {
		if len(contribution.Bands) != len(constants.OctaveBands) {
			t.Fatalf("source %s produced %d bands, want %d", contribution.SourceCode, len(contribution.Bands), len(constants.OctaveBands))
		}
		if math.IsNaN(contribution.ContributionPct) || contribution.ContributionPct < 0 {
			t.Fatalf("source %s invalid contribution pct: %v", contribution.SourceCode, contribution.ContributionPct)
		}
	}
}
