package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	carvel "github.com/vmware-labs/distribution-tooling-for-helm/carvel"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"gopkg.in/yaml.v3"
)

func (suite *CmdSuite) TestCarvelizeCommand() {
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
	authors := []carvel.Author{{
		Name:  "VMware, Inc.",
		Email: "dt@vmware.com",
	}}
	websites := []carvel.Website{{
		URL: "https://github.com/bitnami/charts/tree/main/bitnami/wordpress",
	}}

	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
	t := suite.T()

	dest := sb.TempFile()
	require.NoError(tu.RenderScenario(scenarioDir, dest,
		map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL,
			"Authors": authors, "Websites": websites,
		},
	))
	chartDir := filepath.Join(dest, scenarioName)

	bundleData, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, ".imgpkg/bundle.yml.tmpl"),
		map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName,
			"Authors": authors, "Websites": websites,
		},
	)

	require.NoError(err)
	var expectedBundle map[string]interface{}
	require.NoError(yaml.Unmarshal([]byte(bundleData), &expectedBundle))

	carvelImagesData, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, ".imgpkg/images.yml.tmpl"),
		map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName},
	)
	require.NoError(err)
	var expectedCarvelImagesLock map[string]interface{}
	require.NoError(yaml.Unmarshal([]byte(carvelImagesData), &expectedCarvelImagesLock))

	// We need to provide the --insecure flag or our test server won't validate
	args := []string{"charts", "carvelize", "--insecure", chartDir}
	res := dt(args...)
	res.AssertSuccess(t)

	t.Run("Generates Carvel bundle", func(t *testing.T) {
		newBundleData, err := os.ReadFile(filepath.Join(chartDir, carvel.CarvelBundleFilePath))
		require.NoError(err)
		var newBundle map[string]interface{}
		require.NoError(yaml.Unmarshal(newBundleData, &newBundle))

		require.Equal(expectedBundle, newBundle)
	})

	t.Run("Generates Carvel images", func(t *testing.T) {
		newImagesData, err := os.ReadFile(filepath.Join(chartDir, carvel.CarvelImagesFilePath))
		require.NoError(err)
		var newImagesLock map[string]interface{}
		require.NoError(yaml.Unmarshal(newImagesData, &newImagesLock))

		require.Equal(expectedCarvelImagesLock, newImagesLock)
	})
}
