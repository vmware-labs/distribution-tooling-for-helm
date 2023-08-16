package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"gopkg.in/yaml.v3"
)

func readYamlFile(f string) (map[string]interface{}, error) {
	var data map[string]interface{}
	fh, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	dec := yaml.NewDecoder(fh)
	err = dec.Decode(&data)
	return data, err
}

func (suite *CmdSuite) TestRelocateCommand() {
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

	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	renderLockedChart := func(destDir string, scenarioName string, serverURL string) string {
		require.NoError(tu.RenderScenario(scenarioDir, destDir,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		chartDir := filepath.Join(destDir, scenarioName)

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName},
		)
		suite.Require().NoError(err)
		suite.Require().NoError(os.WriteFile(filepath.Join(chartDir, "Images.lock"), []byte(data), 0644))
		return chartDir
	}
	suite.T().Run("Relocate Helm chart", func(t *testing.T) {
		relocateURL := "custom.repo.example.com"
		originChart := renderLockedChart(sb.TempFile(), scenarioName, serverURL)
		expectedRelocatedDir := renderLockedChart(sb.TempFile(), scenarioName, relocateURL)
		cmd := dt("charts", "relocate", originChart, relocateURL)
		cmd.AssertSuccess(suite.T())

		for _, tail := range []string{"Chart.yaml", "Images.lock"} {
			got, err := readYamlFile(filepath.Join(originChart, tail))
			suite.Require().NoError(err)
			expected, err := readYamlFile(filepath.Join(expectedRelocatedDir, tail))
			suite.Require().NoError(err)
			suite.Assert().Equal(expected, got)
		}
	})
}
