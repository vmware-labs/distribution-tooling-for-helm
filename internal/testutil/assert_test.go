package testutil

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMeasure(t *testing.T) {
	delay := 500 * time.Millisecond
	t1 := time.Now()
	ellapsed := Measure(func() {
		time.Sleep(delay)
	})
	t2 := time.Now()
	assert.WithinDuration(t, t1.Add(ellapsed), t2, 10*time.Millisecond, "Measured time %v is out of the acceptable ranges", ellapsed)
}

func TestAssertFileExistsAndDoesnttExist(t *testing.T) {
	var sampleT *testing.T
	sb := NewSandbox()
	defer sb.Cleanup()

	sampleT = &testing.T{}
	AssertFileExists(sampleT, "foo")
	assert.True(t, sampleT.Failed())

	sampleT = &testing.T{}
	AssertFileDoesNotExist(sampleT, "foo")
	assert.False(t, sampleT.Failed())

	sampleT = &testing.T{}
	AssertFileExists(sampleT, sb.Root)
	assert.False(t, sampleT.Failed())

	sampleT = &testing.T{}
	AssertFileDoesNotExist(sampleT, sb.Root)
	assert.True(t, sampleT.Failed())
}

func TestAssertPanicsMatch(t *testing.T) {
	var sampleT *testing.T
	sampleT = &testing.T{}
	AssertPanicsMatch(sampleT, func() {
		// no panic
	}, regexp.MustCompile(".*"))
	assert.True(t, sampleT.Failed())

	sampleT = &testing.T{}
	AssertPanicsMatch(sampleT, func() {
		// wrong panic message
		panic("Wrong error")
	}, regexp.MustCompile("Unexpected error.*"))
	assert.True(t, sampleT.Failed())

	sampleT = &testing.T{}
	AssertPanicsMatch(sampleT, func() {
		// Matching error
		panic("Unexpected error in test")
	}, regexp.MustCompile("Unexpected error.*"))
	assert.False(t, sampleT.Failed())
}

func TestAssertErrorMatch(t *testing.T) {
	var sampleT *testing.T
	sampleT = &testing.T{}
	// no error
	AssertErrorMatch(sampleT, nil, regexp.MustCompile(".*"))
	assert.True(t, sampleT.Failed())

	sampleT = &testing.T{}
	// wrong error message
	AssertErrorMatch(sampleT,
		fmt.Errorf("Wrong error"),
		regexp.MustCompile("Unexpected error.*"))
	assert.True(t, sampleT.Failed())

	sampleT = &testing.T{}
	// Matching error
	AssertErrorMatch(sampleT,
		fmt.Errorf("Unexpected error in test"),
		regexp.MustCompile("Unexpected error.*"))
	assert.False(t, sampleT.Failed())
}
