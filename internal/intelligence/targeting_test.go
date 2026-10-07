package intelligence

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func testProfile(class, os string, score int, runtimes []string) *DeviceProfile {
	return &DeviceProfile{
		DeviceID:     uuid.New(),
		DeviceClass:  class,
		OS:           os,
		Score:        score,
		Accelerators: AcceleratorProfile{SupportedRuntimes: runtimes},
	}
}

func TestTargetSelector_EmptyMatchesAll(t *testing.T) {
	ts := TargetSelector{}
	assert.True(t, ts.Matches(testProfile("android-flagship", "ANDROID", 80, nil), nil))
	assert.True(t, ts.Matches(testProfile("ios-pro", "IOS", 50, nil), nil))
}

func TestTargetSelector_DeviceClasses(t *testing.T) {
	ts := TargetSelector{DeviceClasses: []string{"android-flagship", "android-midrange"}}
	assert.True(t, ts.Matches(testProfile("android-flagship", "ANDROID", 80, nil), nil))
	assert.True(t, ts.Matches(testProfile("android-midrange", "ANDROID", 40, nil), nil))
	assert.False(t, ts.Matches(testProfile("android-lowend", "ANDROID", 15, nil), nil))
	assert.False(t, ts.Matches(testProfile("ios-pro", "IOS", 80, nil), nil))
}

func TestTargetSelector_Labels(t *testing.T) {
	ts := TargetSelector{Labels: map[string]string{"env": "prod", "region": "apac"}}
	assert.True(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), map[string]string{"env": "prod", "region": "apac", "extra": "ok"}))
	assert.False(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), map[string]string{"env": "prod"}))
	assert.False(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), nil))
}

func TestTargetSelector_MinScore(t *testing.T) {
	ts := TargetSelector{MinScore: 50}
	assert.True(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), nil))
	assert.True(t, ts.Matches(testProfile("any", "ANDROID", 80, nil), nil))
	assert.False(t, ts.Matches(testProfile("any", "ANDROID", 49, nil), nil))
}

func TestTargetSelector_OS(t *testing.T) {
	ts := TargetSelector{OS: []string{"ANDROID", "IOS"}}
	assert.True(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), nil))
	assert.True(t, ts.Matches(testProfile("any", "IOS", 50, nil), nil))
	assert.False(t, ts.Matches(testProfile("any", "LINUX", 50, nil), nil))
}

func TestTargetSelector_Runtimes(t *testing.T) {
	ts := TargetSelector{Runtimes: []string{"TFLITE"}}
	assert.True(t, ts.Matches(testProfile("any", "ANDROID", 50, []string{"TFLITE", "ONNX"}), nil))
	assert.False(t, ts.Matches(testProfile("any", "IOS", 50, []string{"COREML"}), nil))
}

func TestTargetSelector_CombinedCriteria(t *testing.T) {
	ts := TargetSelector{
		DeviceClasses: []string{"android-flagship"},
		MinScore:      50,
		OS:            []string{"ANDROID"},
		Runtimes:      []string{"TFLITE"},
	}

	// All match
	assert.True(t, ts.Matches(testProfile("android-flagship", "ANDROID", 80, []string{"TFLITE", "ONNX"}), nil))

	// Wrong class
	assert.False(t, ts.Matches(testProfile("android-midrange", "ANDROID", 80, []string{"TFLITE"}), nil))

	// Score too low
	assert.False(t, ts.Matches(testProfile("android-flagship", "ANDROID", 30, []string{"TFLITE"}), nil))

	// Wrong OS
	assert.False(t, ts.Matches(testProfile("android-flagship", "IOS", 80, []string{"TFLITE"}), nil))

	// Missing runtime
	assert.False(t, ts.Matches(testProfile("android-flagship", "ANDROID", 80, []string{"COREML"}), nil))
}

func TestTargetSelector_LabelsANDLogic(t *testing.T) {
	ts := TargetSelector{Labels: map[string]string{"a": "1", "b": "2"}}
	assert.True(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), map[string]string{"a": "1", "b": "2"}))
	assert.False(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), map[string]string{"a": "1", "b": "3"}))
	assert.False(t, ts.Matches(testProfile("any", "ANDROID", 50, nil), map[string]string{"a": "1"}))
}
