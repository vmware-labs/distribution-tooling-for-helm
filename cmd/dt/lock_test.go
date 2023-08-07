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
	s, err := tu.NewTestServer()
	suite.Require().NoError(err)

	defer s.Close()

	images, err := s.LoadImagesFromFile("../../testdata/images.json")
	suite.Require().NoError(err)

	sb := suite.sb
	require := suite.Require()
	serverURL := s.ServerURL
	scenarioName := "custom-chart"
	chartName := "test"
	// defaultAnnotationsKey := "images"
	// customAnnotationsKey := "artifacthub.io/images"
	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	suite.T().Run("Generate lock file", func(t *testing.T) {
		dest := sb.TempFile()
		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		chartDir := filepath.Join(dest, scenarioName)

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName},
		)
		suite.Require().NoError(err)
		var expectedLock map[string]interface{}
		suite.Require().NoError(yaml.Unmarshal([]byte(data), &expectedLock))

		// Clear the timestamp
		expectedLock["metadata"] = nil

		// We need to provide the --insecure flag or our test server won't validate
		args := []string{"images", "lock", "--insecure", chartDir}
		res := dt(args...)
		res.AssertSuccess(t)
		fmt.Println(res.stdout)

		newData, err := os.ReadFile(filepath.Join(chartDir, "Images.lock"))
		suite.Require().NoError(err)
		var newLock map[string]interface{}
		suite.Require().NoError(yaml.Unmarshal(newData, &newLock))
		// Clear the timestamp
		newLock["metadata"] = nil

		suite.Assert().Equal(expectedLock, newLock)

	})
}
