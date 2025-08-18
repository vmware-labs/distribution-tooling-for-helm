package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"gopkg.in/yaml.v3"
)

func (suite *CmdSuite) TestLockCommand() {
	require := suite.Require()

	s, err := tu.NewTestServer()
	require.NoError(err)

	defer s.Close()

	images, err := s.LoadImagesFromFile("../../testdata/images.json")
	require.NoError(err)

	sb := suite.sb
	serverURL := s.ServerURL
	scenarioName := "custom-chart"
	chartName := "test"

	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
	t := suite.T()

	t.Run("Generate lock file", func(t *testing.T) {
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName},
		)
		require.NoError(err)
		var expectedLock map[string]interface{}
		require.NoError(yaml.Unmarshal([]byte(data), &expectedLock))

		// Clear the timestamp
		expectedLock["metadata"] = nil

		// We need to provide the --insecure flag or our test server won't validate
		args := []string{"images", "lock", "--insecure", chartDir}
		res := dt(args...)
		res.AssertSuccess(t)

		newData, err := os.ReadFile(filepath.Join(chartDir, "Images.lock"))
		require.NoError(err)
		var newLock map[string]interface{}
		require.NoError(yaml.Unmarshal(newData, &newLock))
		// Clear the timestamp
		newLock["metadata"] = nil

		require.Equal(expectedLock, newLock)

	})
	t.Run("Errors", func(t *testing.T) {
		t.Run("Handles failure to write lock because of permissions", func(t *testing.T) {
			scenarioName := "plain-chart"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

			chartDir := sb.TempFile()

			require.NoError(tu.RenderScenario(scenarioDir, chartDir,
				map[string]interface{}{},
			))

			require.NoError(os.Chmod(chartDir, os.FileMode(0555)))
			defer os.Chmod(chartDir, os.FileMode(0755))

			args := []string{"images", "lock", "--insecure", chartDir}
			res := dt(args...)
			res.AssertErrorMatch(t, "Failed to generate lock: failed to write lock")
		})
		t.Run("Handles non-existent chart", func(t *testing.T) {
			args := []string{"images", "lock", sb.TempFile()}
			res := dt(args...)
			res.AssertErrorMatch(t, "failed to obtain Images.lock location: cannot access path")
		})
	})
}
