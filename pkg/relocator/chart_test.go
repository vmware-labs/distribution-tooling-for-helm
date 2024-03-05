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
			data, err := os.ReadFile(filepath.Join(chartDir, valuesFile))
			require.NoError(t, err)
			relocatedValues, err := tu.NormalizeYAML(string(data))
			require.NoError(t, err)

			expectedData, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, fmt.Sprintf("%s.tmpl", valuesFile)), map[string]string{"ServerURL": newServerURL, "RepositoryPrefix": repositoryPrefix})
			require.NoError(t, err)

			expectedValues, err := tu.NormalizeYAML(expectedData)
			require.NoError(t, err)
			assert.Equal(t, expectedValues, relocatedValues)
		}
	})
	t.Run("Annotations Relocated", func(t *testing.T) {
		c, err := loader.Load(chartDir)
		require.NoError(t, err)

		relocatedAnnotations, err := tu.NormalizeYAML(c.Metadata.Annotations["images"])
		require.NoError(t, err)

		require.NotEqual(t, relocatedAnnotations, "")

		expectedData, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "images.partial.tmpl"), map[string]string{"ServerURL": fullNewURL})
		require.NoError(t, err)

		expectedAnnotations, err := tu.NormalizeYAML(expectedData)
		require.NoError(t, err)
		assert.Equal(t, expectedAnnotations, relocatedAnnotations)
	})
	t.Run("ImageLock Relocated", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(chartDir, "Images.lock"))
		assert.NoError(t, err)
		var lockData map[string]interface{}

		require.NoError(t, yaml.Unmarshal(data, &lockData))

		imagesElemData, err := yaml.Marshal(lockData["images"])
		require.NoError(t, err)

		relocatedImagesData, err := tu.NormalizeYAML(string(imagesElemData))
		require.NoError(t, err)

		expectedData, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "lock_images.partial.tmpl"), map[string]string{"ServerURL": fullNewURL})
		require.NoError(t, err)
		expectedData, err = tu.NormalizeYAML(expectedData)
		require.NoError(t, err)

		assert.Equal(t, expectedData, relocatedImagesData)

	})
}
