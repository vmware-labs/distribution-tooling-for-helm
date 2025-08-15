package relocator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"gopkg.in/yaml.v3"

	"helm.sh/helm/v3/pkg/chart/loader"
)

func TestRelocateChartDir(t *testing.T) {
	scenarioName := "chart1"
	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
	valuesFiles := []string{"values.yaml", "values.prod.yaml"}

	chartDir := sb.TempFile()
	serverURL := "localhost"

	require.NoError(t, tu.RenderScenario(scenarioDir, chartDir, map[string]interface{}{"ServerURL": serverURL}))

	newServerURL := "test.example.com"
	repositoryPrefix := "airgap"
	fullNewURL := fmt.Sprintf("%s/%s", newServerURL, repositoryPrefix)

	err := RelocateChartDir(chartDir, fullNewURL, WithValuesFiles(valuesFiles...))
	require.NoError(t, err)

	t.Run("Values Relocated", func(t *testing.T) {
		for _, valuesFile := range valuesFiles {
			t.Logf("checking %s file", valuesFile)
			data, tErr := os.ReadFile(filepath.Join(chartDir, valuesFile))
			require.NoError(t, tErr)
			relocatedValues, tErr := tu.NormalizeYAML(string(data))
			require.NoError(t, tErr)

			expectedData, tErr := tu.RenderTemplateFile(filepath.Join(scenarioDir, fmt.Sprintf("%s.tmpl", valuesFile)), map[string]string{"ServerURL": newServerURL, "RepositoryPrefix": repositoryPrefix})
			require.NoError(t, tErr)

			expectedValues, tErr := tu.NormalizeYAML(expectedData)
			require.NoError(t, tErr)
			assert.Equal(t, expectedValues, relocatedValues)
		}
	})
	t.Run("Annotations Relocated", func(t *testing.T) {
		c, tErr := loader.Load(chartDir)
		require.NoError(t, tErr)

		relocatedAnnotations, tErr := tu.NormalizeYAML(c.Metadata.Annotations["images"])
		require.NoError(t, tErr)

		require.NotEqual(t, relocatedAnnotations, "")

		expectedData, tErr := tu.RenderTemplateFile(filepath.Join(scenarioDir, "images.partial.tmpl"), map[string]string{"ServerURL": fullNewURL})
		require.NoError(t, tErr)

		expectedAnnotations, tErr := tu.NormalizeYAML(expectedData)
		require.NoError(t, tErr)
		assert.Equal(t, expectedAnnotations, relocatedAnnotations)
	})
	t.Run("ImageLock Relocated", func(t *testing.T) {
		data, tErr := os.ReadFile(filepath.Join(chartDir, "Images.lock"))
		assert.NoError(t, tErr)
		var lockData map[string]interface{}

		require.NoError(t, yaml.Unmarshal(data, &lockData))

		imagesElemData, tErr := yaml.Marshal(lockData["images"])
		require.NoError(t, tErr)

		relocatedImagesData, tErr := tu.NormalizeYAML(string(imagesElemData))
		require.NoError(t, tErr)

		expectedData, tErr := tu.RenderTemplateFile(filepath.Join(scenarioDir, "lock_images.partial.tmpl"), map[string]string{"ServerURL": fullNewURL})
		require.NoError(t, tErr)
		expectedData, tErr = tu.NormalizeYAML(expectedData)
		require.NoError(t, tErr)

		assert.Equal(t, expectedData, relocatedImagesData)

	})

	// create a new chart dir to reset for the SkipImageRelocation tests
	chartDir = sb.TempFile()
	require.NoError(t, tu.RenderScenario(scenarioDir, chartDir, map[string]interface{}{"ServerURL": serverURL}))
	err = RelocateChartDir(chartDir, "", WithValuesFiles(valuesFiles...), WithSkipImageRelocation(true))
	require.NoError(t, err)

	t.Run("Values Relocated SkipImageRelocation", func(t *testing.T) {
		for _, valuesFile := range valuesFiles {
			t.Logf("checking %s file", valuesFile)
			data, tErr := os.ReadFile(filepath.Join(chartDir, valuesFile))
			require.NoError(t, tErr)
			relocatedValues, tErr := tu.NormalizeYAML(string(data))
			require.NoError(t, tErr)

			expectedData, tErr := tu.RenderTemplateFile(filepath.Join(scenarioDir, fmt.Sprintf("%s.tmpl", valuesFile)), map[string]string{"ServerURL": serverURL})
			require.NoError(t, tErr)

			expectedValues, tErr := tu.NormalizeYAML(expectedData)
			require.NoError(t, tErr)
			assert.Equal(t, expectedValues, relocatedValues)
		}
	})

	t.Run("Annotations Relocated ", func(t *testing.T) {
		c, tErr := loader.Load(chartDir)
		require.NoError(t, tErr)

		relocatedAnnotations, tErr := tu.NormalizeYAML(c.Metadata.Annotations["images"])
		require.NoError(t, tErr)

		require.NotEqual(t, relocatedAnnotations, "")

		expectedData, tErr := tu.RenderTemplateFile(filepath.Join(scenarioDir, "images.partial.tmpl"), map[string]string{"ServerURL": serverURL})
		require.NoError(t, tErr)

		expectedAnnotations, tErr := tu.NormalizeYAML(expectedData)
		require.NoError(t, tErr)
		assert.Equal(t, expectedAnnotations, relocatedAnnotations)
	})

	t.Run("ImageLock Relocated SkipImageRelocation", func(t *testing.T) {
		data, tErr := os.ReadFile(filepath.Join(chartDir, "Images.lock"))
		assert.NoError(t, tErr)
		var lockData map[string]interface{}

		require.NoError(t, yaml.Unmarshal(data, &lockData))

		imagesElemData, tErr := yaml.Marshal(lockData["images"])
		require.NoError(t, tErr)

		relocatedImagesData, tErr := tu.NormalizeYAML(string(imagesElemData))
		require.NoError(t, tErr)

		expectedData, tErr := tu.RenderTemplateFile(filepath.Join(scenarioDir, "lock_images.partial.tmpl"), map[string]string{"ServerURL": serverURL})
		require.NoError(t, tErr)
		expectedData, tErr = tu.NormalizeYAML(expectedData)
		require.NoError(t, tErr)

		assert.Equal(t, expectedData, relocatedImagesData)

	})
}
