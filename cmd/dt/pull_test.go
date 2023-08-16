package main

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
)

func (suite *CmdSuite) TestPullCommand() {
	t := suite.T()
	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}

	imageName := "test:mytag"

	images, err := tu.AddSampleImagesToRegistry(imageName, u.Host)
	if err != nil {
		t.Fatal(err)
	}

	sb := suite.sb
	require := suite.Require()
	serverURL := u.Host
	scenarioName := "complete-chart"
	chartName := "test"

	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	createSampleChart := func(dest string) string {
		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		return filepath.Join(dest, scenarioName)
	}
	verifyChartDir := func(chartDir string) {
		imagesDir := filepath.Join(chartDir, "images")
		suite.Require().DirExists(imagesDir)
		for _, imgData := range images {
			for _, digestData := range imgData.Digests {
				imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.tar", digestData.Digest.Encoded()))
				suite.Assert().FileExists(imgFile)
			}
		}
	}
	t.Run("Pulls images", func(t *testing.T) {
		chartDir := createSampleChart(sb.TempFile())
		dt("images", "pull", chartDir).AssertSuccessMatch(t, "")
		verifyChartDir(chartDir)
	})
	t.Run("Pulls images and compress into filename", func(t *testing.T) {
		chartDir := createSampleChart(sb.TempFile())
		outputFile := fmt.Sprintf("%s.tar.gz", sb.TempFile())
		dt("images", "pull", "--output-file", outputFile, chartDir).AssertSuccess(t)

		tmpDir, err := sb.Mkdir(sb.TempFile(), 0755)
		require.NoError(err)

		require.NoError(utils.Untar(outputFile, tmpDir, utils.TarConfig{StripComponents: 1}))

		verifyChartDir(tmpDir)
	})

	t.Run("Errors", func(t *testing.T) {
		t.Run("Fails when Images.lock is not found", func(t *testing.T) {
			chartDir := createSampleChart(sb.TempFile())
			require.NoError(os.RemoveAll(filepath.Join(chartDir, "Images.lock")))

			dt("images", "pull", chartDir).AssertErrorMatch(t, `(?s).*failed to read Images\.lock file.*`)
		})
	})
}
