package testutil

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	"helm.sh/helm/v3/pkg/chart/loader"
)

// Measure executes fn and returns the time taken for it to finish
func Measure(fn func()) time.Duration {
	t1 := time.Now()
	fn()
	t2 := time.Now()
	return t2.Sub(t1)
}

func functionAborted(fn func()) (bool, string) {
	aborted := false
	msg := ""
	func() {
		defer func() {
			if r := recover(); r != nil {
				aborted = true
				switch v := r.(type) {
				case string:
					msg = v
				case fmt.Stringer:
					msg = v.String()
				default:
					msg = fmt.Sprintf("%v", v)
				}
			}
		}()
		fn()
	}()
	return aborted, msg
}

// AssertFileExists failts the test t if path does not exists
func AssertFileExists(t *testing.T, path string, msgAndArgs ...interface{}) bool {
	fullPath, _ := filepath.Abs(path)
	if fileExists(fullPath) {
		return true
	}
	assert.Fail(t, fmt.Sprintf("File '%s' should exist", path), msgAndArgs...)
	return false
}

// AssertFileDoesNotExist failts the test t if path exists
func AssertFileDoesNotExist(t *testing.T, path string, msgAndArgs ...interface{}) bool {
	fullPath, _ := filepath.Abs(path)
	if !fileExists(fullPath) {
		return true
	}
	assert.Fail(t, fmt.Sprintf("File '%s' should not exist", path), msgAndArgs...)
	return false
}

// AssertPanicsMatch fails the test t if fn does not panic or if the panic
// message does not match the provided regexp re
func AssertPanicsMatch(t *testing.T, fn func(), re *regexp.Regexp, msgAndArgs ...interface{}) bool {
	if assert.Panics(t, fn, msgAndArgs...) {
		_, msg := functionAborted(fn)
		return assert.Regexp(t, re, msg, msgAndArgs...)
	}
	return false
}

// AssertErrorMatch fails the test t if err is nil or if its message
// does not match the provided regexp re
func AssertErrorMatch(t *testing.T, err error, re *regexp.Regexp, msgAndArgs ...interface{}) bool {
	if assert.Error(t, err, msgAndArgs...) {
		return assert.Regexp(t, re, err.Error(), msgAndArgs...)
	}
	return false
}

// AnnotationEntry defines an annotation entry
type AnnotationEntry struct {
	Name  string
	Image string
}

// AssertChartAnnotations checks if the specified chart contains the provided annotations
func AssertChartAnnotations(t *testing.T, chartDir string, annotationsKey string, expectedImages []AnnotationEntry, msgAndArgs ...interface{}) bool {
	c, err := loader.Load(chartDir)
	if err != nil {
		assert.Fail(t, fmt.Sprintf("Failed to load chart %q: %v", chartDir, err))
		return false
	}

	gotImages := make([]AnnotationEntry, 0)
	if err := yaml.Unmarshal([]byte(c.Metadata.Annotations[annotationsKey]), &gotImages); err != nil {
		assert.Fail(t, fmt.Sprintf("Failed to unmarshal chart annotations: %v", err))
		return false
	}
	return assert.EqualValues(t, expectedImages, gotImages, msgAndArgs...)
}
