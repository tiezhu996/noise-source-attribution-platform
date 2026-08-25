package constants

import "testing"

// Attribution runs must never be allowed to skip mandatory intermediate
// states or resurrect/rollback results.
func TestAttributionIllegalTransitionsRejected(t *testing.T) {
	illegal := [][2]AttributionState{
		{AttributionCalculating, AttributionReviewed},
		{AttributionFailed, AttributionCompleted},
		{AttributionVoided, AttributionCompleted},
		{AttributionConfirmed, AttributionCompleted},
	}
	for _, pair := range illegal {
		if CanTransitionAttribution(pair[0], pair[1]) {
			t.Fatalf("illegal attribution transition %s -> %s allowed", pair[0], pair[1])
		}
	}
}

// Measurements must flow captured -> validated -> normalized -> ready and
// must not be allowed to skip steps or re-open rejected records.
func TestMeasurementIllegalTransitionsRejected(t *testing.T) {
	illegal := [][2]MeasurementState{
		{MeasurementCaptured, MeasurementNormalized},
		{MeasurementValidated, MeasurementReady},
		{MeasurementNormalized, MeasurementSuperseded},
		{MeasurementRejected, MeasurementCaptured},
	}
	for _, pair := range illegal {
		if CanTransitionMeasurement(pair[0], pair[1]) {
			t.Fatalf("illegal measurement transition %s -> %s allowed", pair[0], pair[1])
		}
	}
}
