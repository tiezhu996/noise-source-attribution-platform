package service

import (
	"math"
	"testing"

	"industrial-noise-source-attribution/backend/internal/model"
)

// The normalized bands surfaced by a measurement response must keep full
// precision, and processing one measurement must not poison the cached
// subtraction results reused by later consumers.
func TestMeasurementResponseKeepsNormalizedPrecision(t *testing.T) {
	svc := &NoiseMeasurementService{}
	measurement := model.NoiseMeasurement{
		ID:                 1,
		MonitoringPointID:  1,
		OctaveBandsJSON:    mustJSON(map[string]float64{"63": 70, "125": 70, "250": 70, "500": 70, "1000": 70, "2000": 70, "4000": 70, "8000": 70}),
		MeasurementState:   "normalized",
		MonitoringPoint:    model.MonitoringPoint{BackgroundProfileJSON: mustJSON(map[string]float64{"63": 60, "125": 60, "250": 60, "500": 60, "1000": 60, "2000": 60, "4000": 60, "8000": 60})},
	}
	response, err := svc.toResponse(measurement)
	if err != nil {
		t.Fatalf("toResponse error: %v", err)
	}
	want := 10 * math.Log10(math.Pow(10, 7)-math.Pow(10, 6))
	if math.Abs(response.NormalizedBands["1000"]-want) > 0.01 {
		t.Fatalf("normalized 1000 Hz = %.4f, want %.4f (precision lost)", response.NormalizedBands["1000"], want)
	}
	// The subtraction cache must not have been polluted by response rendering.
	if math.Abs(response.NormalizedBands["1000"]-want) > 0.001 {
		t.Fatalf("normalized 1000 Hz = %.4f, want %.4f (cache polluted)", response.NormalizedBands["1000"], want)
	}
}
