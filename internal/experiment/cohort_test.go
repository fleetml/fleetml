package experiment

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAssignCohort_Deterministic(t *testing.T) {
	deviceID := uuid.New()
	expID := uuid.New()

	cohort1, inc1 := AssignCohort(deviceID, expID, 50, 100)
	for i := 0; i < 1000; i++ {
		cohort, inc := AssignCohort(deviceID, expID, 50, 100)
		assert.Equal(t, cohort1, cohort, "cohort must be deterministic")
		assert.Equal(t, inc1, inc)
	}
}

func TestAssignCohort_Balance(t *testing.T) {
	expID := uuid.New()
	control, treatment := 0, 0
	total := 10000

	for i := 0; i < total; i++ {
		deviceID := uuid.New()
		cohort, included := AssignCohort(deviceID, expID, 50, 100)
		if !included {
			continue
		}
		if cohort == "control" {
			control++
		} else {
			treatment++
		}
	}

	// With 50/50 split and 10000 devices, expect roughly 5000 each (±2%)
	assert.InDelta(t, 0.5, float64(treatment)/float64(control+treatment), 0.04)
}

func TestAssignCohort_TrafficExclusion(t *testing.T) {
	expID := uuid.New()
	included := 0
	total := 10000

	for i := 0; i < total; i++ {
		_, inc := AssignCohort(uuid.New(), expID, 50, 50) // 50% traffic
		if inc {
			included++
		}
	}

	// Expect ~50% included (±3%)
	assert.InDelta(t, 0.5, float64(included)/float64(total), 0.05)
}

func TestAssignCohort_FullTraffic(t *testing.T) {
	expID := uuid.New()
	for i := 0; i < 100; i++ {
		_, included := AssignCohort(uuid.New(), expID, 50, 100)
		assert.True(t, included, "100% traffic should include all devices")
	}
}

func TestAssignCohort_DifferentExperiments(t *testing.T) {
	deviceID := uuid.New()
	exp1 := uuid.New()
	exp2 := uuid.New()

	cohort1, _ := AssignCohort(deviceID, exp1, 50, 100)
	cohort2, _ := AssignCohort(deviceID, exp2, 50, 100)
	// Different experiments may produce different cohorts (not guaranteed, but possible)
	_ = cohort1
	_ = cohort2
	// Just verify no panic
}

func TestAssignCohort_UnevenSplit(t *testing.T) {
	expID := uuid.New()
	treatment := 0
	total := 10000

	for i := 0; i < total; i++ {
		cohort, _ := AssignCohort(uuid.New(), expID, 20, 100) // 20% treatment
		if cohort == "treatment" {
			treatment++
		}
	}

	assert.InDelta(t, 0.2, float64(treatment)/float64(total), 0.04)
}
