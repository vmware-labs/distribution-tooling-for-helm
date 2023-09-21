package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
	t := suite.T()

	type author struct {
		Name  string
		Email string
	}
	type website struct {
		Url string
	}

	dest := sb.TempFile()
	require.NoError(tu.RenderScenario(scenarioDir, dest,
		map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL,
			"Authors": []author{{
				Name:  "VMware, Inc.",
				Email: "dt@vmware.com",
			}},
			"Websites": []website{{
				Url: "https://github.com/bitnami/charts/tree/main/bitnami/wordpress",
			}},
		},
	))
	chartDir := filepath.Join(dest, scenarioName)

	bundleData, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, ".imgpkg/bundle.yml.tmpl"),
		map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName},
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
		newBundleData, err := os.ReadFile(filepath.Join(chartDir, ".imgpkg/bundle.yml"))
		require.NoError(err)
		var newBundle map[string]interface{}
		require.NoError(yaml.Unmarshal(newBundleData, &newBundle))

		require.Equal(expectedBundle, newBundle)
	})

	t.Run("Generates Carvel images", func(t *testing.T) {
		newImagesData, err := os.ReadFile(filepath.Join(chartDir, ".imgpkg/images.yml"))
		require.NoError(err)
		var newImagesLock map[string]interface{}
		require.NoError(yaml.Unmarshal(newImagesData, &newImagesLock))

		require.Equal(expectedCarvelImagesLock, newImagesLock)
	})
}
