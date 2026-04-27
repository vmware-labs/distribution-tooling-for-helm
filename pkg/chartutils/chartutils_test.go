package chartutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"helm.sh/helm/v3/pkg/chart"
)

type ChartUtilsTestSuite struct {
	suite.Suite
	sb *tu.Sandbox
}

func (suite *ChartUtilsTestSuite) TearDownSuite() {
	_ = suite.sb.Cleanup()
}

func (suite *ChartUtilsTestSuite) SetupSuite() {
	suite.sb = tu.NewSandbox()
}

func TestChartUtilsTestSuite(t *testing.T) {
	suite.Run(t, new(ChartUtilsTestSuite))
}

func (suite *ChartUtilsTestSuite) TestAnnotateChart() {
	t := suite.T()
	require := suite.Require()

	sb := suite.sb
	serverURL := "localhost"
	scenarioName := "plain-chart"
	defaultAnnotationsKey := "images"
	// customAnnotationsKey := "artifacthub.io/images"
	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	type testImage struct {
		Name       string
		Registry   string
		Repository string
		Tag        string
		Digest     string
	}

	images := []testImage{
		{
			Name:       "bitnami-shell",
			Registry:   "docker.io",
			Repository: "bitnami/bitnami-shell",
			Tag:        "1.0.0",
		},
		{
			Name:       "wordpress",
			Registry:   "docker.io",
			Repository: "bitnami/wordpress",
			Tag:        "latest",
		},
	}
	t.Run("Annotates a chart", func(t *testing.T) {
		chartDir := sb.TempFile()
		annotationsKey := defaultAnnotationsKey
		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": serverURL, "ValuesImages": images},
		))

		expectedImages := make([]tu.AnnotationEntry, 0)
		for _, img := range images {
			url := fmt.Sprintf("%s/%s:%s", img.Registry, img.Repository, img.Tag)
			expectedImages = append(expectedImages, tu.AnnotationEntry{
				Name:  img.Name,
				Image: url,
			})
		}

		require.NoError(AnnotateChart(chartDir, WithAnnotationsKey(annotationsKey)))
		tu.AssertChartAnnotations(t, chartDir, annotationsKey, expectedImages)
	})
}

// TestValidateAndPreparePathLogic tests the path validation logic
func TestValidateAndPreparePathLogic(t *testing.T) {
	tempDir := t.TempDir()

	testCases := []struct {
		name        string
		headerName  string
		expectSkip  bool
		description string
	}{
		{
			name:        "Safe path",
			headerName:  "chart/Chart.yaml",
			expectSkip:  false,
			description: "Normal safe path should not be skipped",
		},
		{
			name:        "Directory traversal attempt",
			headerName:  "../../../etc/passwd",
			expectSkip:  true,
			description: "Directory traversal should be skipped",
		},
		{
			name:        "Relative path with dots",
			headerName:  "chart/../other/file.yaml",
			expectSkip:  true,
			description: "Relative path with .. should be skipped",
		},
		{
			name:        "Deep nested path",
			headerName:  "chart/templates/deployment.yaml",
			expectSkip:  false,
			description: "Deep nested safe path should not be skipped",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the validation logic from validateAndPreparePath
			// Check the original path first, then clean it
			shouldSkip := strings.Contains(tc.headerName, "..")

			if !shouldSkip {
				cleanPath := filepath.Clean(tc.headerName)
				target := filepath.Join(tempDir, cleanPath)

				// Additional safety check simulation
				if !strings.HasPrefix(target, tempDir+string(filepath.Separator)) && target != tempDir {
					shouldSkip = true
				}
			}

			if shouldSkip != tc.expectSkip {
				t.Errorf("Test %s: expected skip=%t, got skip=%t for path %s (cleanPath would be: %s)",
					tc.name, tc.expectSkip, shouldSkip, tc.headerName, filepath.Clean(tc.headerName))
			}
		})
	}
}

func TestResolveDependencyPath(t *testing.T) {
	// Create a temporary directory and render the compressed-deps-chart scenario
	tempDir := t.TempDir()
	serverURL := "localhost"

	err := tu.RenderScenario("../../testdata/scenarios/compressed-deps-chart", tempDir, map[string]interface{}{"ServerURL": serverURL})
	require.NoError(t, err)

	// Load the rendered chart - this gives us a real chart with .tgz dependencies
	testChart, err := LoadChart(tempDir)
	require.NoError(t, err)
	require.NotNil(t, testChart.chart.Dependencies(), "Chart should have dependencies")

	// Get the mariadb dependency for testing (it's one of the real dependencies)
	var mariadbDep *chart.Chart
	for _, dep := range testChart.chart.Dependencies() {
		if dep.Name() == "mariadb" {
			mariadbDep = dep
			break
		}
	}
	require.NotNil(t, mariadbDep, "Should find mariadb dependency")

	t.Run("Compressed dependency extraction", func(t *testing.T) {
		// The chart should have mariadb-12.2.8.tgz initially
		chartsDir := filepath.Join(tempDir, "charts")
		tgzFile := filepath.Join(chartsDir, "mariadb-12.2.8.tgz")

		// Verify .tgz exists before resolution
		assert.FileExists(t, tgzFile, "TGZ file should exist initially")

		// Test path resolution - should extract and return directory path
		resolvedPath, err := resolveDependencyPath(tempDir, mariadbDep)
		require.NoError(t, err)

		// Should return the extracted directory path
		expectedDir := filepath.Join(chartsDir, "mariadb")
		assert.Equal(t, expectedDir, resolvedPath)

		// Verify the directory was created and .tgz was removed
		assert.DirExists(t, expectedDir, "Directory should be created after extraction")
		assert.NoFileExists(t, tgzFile, "TGZ file should be removed after extraction")

		// Verify Chart.yaml exists in extracted directory
		chartYaml := filepath.Join(expectedDir, "Chart.yaml")
		assert.FileExists(t, chartYaml, "Chart.yaml should exist in extracted directory")
	})

	t.Run("Directory preference over compressed", func(t *testing.T) {
		// Create a fresh scenario for this test
		tempDir2 := t.TempDir()
		err := tu.RenderScenario("../../testdata/scenarios/compressed-deps-chart", tempDir2, map[string]interface{}{"ServerURL": serverURL})
		require.NoError(t, err)

		chartsDir := filepath.Join(tempDir2, "charts")

		// Create a directory-based dependency that matches a compressed one
		depDir := filepath.Join(chartsDir, "common")
		err = os.MkdirAll(depDir, 0755)
		require.NoError(t, err)

		chartYaml := filepath.Join(depDir, "Chart.yaml")
		err = os.WriteFile(chartYaml, []byte("name: common\nversion: 2.6.0"), 0644)
		require.NoError(t, err)

		// Get the common dependency
		var commonDep *chart.Chart
		testChart2, err := LoadChart(tempDir2)
		require.NoError(t, err)

		for _, dep := range testChart2.chart.Dependencies() {
			if dep.Name() == "common" {
				commonDep = dep
				break
			}
		}
		require.NotNil(t, commonDep, "Should find common dependency")

		// Test path resolution - should prefer the directory over .tgz
		resolvedPath, err := resolveDependencyPath(tempDir2, commonDep)
		require.NoError(t, err)
		assert.Equal(t, depDir, resolvedPath, "Should prefer directory over .tgz when both exist")

		// Verify the .tgz file still exists (wasn't processed)
		tgzFile := filepath.Join(chartsDir, "common-2.6.0.tgz")
		assert.FileExists(t, tgzFile, "TGZ file should still exist when directory is preferred")
	})

	t.Run("Fallback to directory path when neither exists", func(t *testing.T) {
		// Create a fake dependency that doesn't exist in the test data
		fakeDep := &chart.Chart{
			Metadata: &chart.Metadata{
				Name:    "nonexistent",
				Version: "1.0.0",
			},
		}

		// Test path resolution - should fallback to directory path
		resolvedPath, err := resolveDependencyPath(tempDir, fakeDep)
		require.NoError(t, err)

		expectedDir := filepath.Join(tempDir, "charts", "nonexistent")
		assert.Equal(t, expectedDir, resolvedPath)
	})
}

func TestGetChartRoot(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	t.Run("Regular directory", func(t *testing.T) {
		chartDir := filepath.Join(tempDir, "chart-dir")
		err := os.MkdirAll(chartDir, 0755)
		require.NoError(t, err)

		root, err := GetChartRoot(chartDir)
		require.NoError(t, err)
		absPath, _ := filepath.Abs(chartDir)
		assert.Equal(t, absPath, root)
	})

	t.Run("TGZ file", func(t *testing.T) {
		tgzFile := filepath.Join(tempDir, "chart.tgz")
		err := os.WriteFile(tgzFile, []byte("fake tgz content"), 0644)
		require.NoError(t, err)

		root, err := GetChartRoot(tgzFile)
		require.NoError(t, err)
		absPath, _ := filepath.Abs(tgzFile)
		assert.Equal(t, absPath, root, "Should return .tgz file path itself")
	})

	t.Run("Chart.yaml file", func(t *testing.T) {
		chartDir := filepath.Join(tempDir, "chart-yaml-dir")
		err := os.MkdirAll(chartDir, 0755)
		require.NoError(t, err)

		chartYaml := filepath.Join(chartDir, "Chart.yaml")
		err = os.WriteFile(chartYaml, []byte("name: test"), 0644)
		require.NoError(t, err)

		root, err := GetChartRoot(chartYaml)
		require.NoError(t, err)
		absPath, _ := filepath.Abs(chartDir)
		assert.Equal(t, absPath, root, "Should return directory containing Chart.yaml")
	})
}
