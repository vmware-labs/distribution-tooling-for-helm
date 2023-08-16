package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

func (suite *CmdSuite) TestVerifyCommand() {
	t := suite.T()
	sb := suite.sb
	require := suite.Require()

	s, err := tu.NewTestServer()
	suite.Require().NoError(err)

	defer s.Close()

	serverURL := s.ServerURL

	renderLockedChart := func(destDir string, chartName string, scenarioName string, serverURL string, images []*tu.ImageData) string {
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		require.NoError(tu.RenderScenario(scenarioDir, destDir,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		chartDir := filepath.Join(destDir, scenarioName)

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName},
		)
		require.NoError(err)
		require.NoError(os.WriteFile(filepath.Join(chartDir, "Images.lock"), []byte(data), 0644))
		return chartDir
	}

	t.Run("Handle errors", func(t *testing.T) {
		t.Run("Non-existent Helm chart", func(t *testing.T) {
			dt("images", "verify", sb.TempFile()).AssertErrorMatch(t, "Helm chart.*does not exist")
		})
		t.Run("Missing Images.lock", func(t *testing.T) {
			chartName := "test"
			scenarioName := "plain-chart"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			chartDir := filepath.Join(dest, scenarioName)
			dt("images", "verify", chartDir).AssertErrorMatch(t, "failed to open Images.lock file")
		})
		t.Run("Handle malformed Images.lock", func(t *testing.T) {
			chartName := "test"
			scenarioName := "plain-chart"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			chartDir := filepath.Join(dest, scenarioName)
			require.NoError(os.WriteFile(filepath.Join(chartDir, imagelock.DefaultImagesLockFileName), []byte("malformed lock"), 0644))
			dt("images", "verify", chartDir).AssertErrorMatch(t, "failed to load Images.lock")
		})
		t.Run("Handle verify error", func(t *testing.T) {
			images, err := s.LoadImagesFromFile("../../testdata/images.json")
			require.NoError(err)
			scenarioName := "custom-chart"
			chartName := "test"

			chartDir := renderLockedChart(sb.TempFile(), chartName, scenarioName, serverURL, images)
			// Modify images and override lock file
			newDigest := digest.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
			oldDigest := images[0].Digests[0].Digest
			images[0].Digests[0].Digest = newDigest
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

			data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
				map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName},
			)
			require.NoError(err)
			require.NoError(os.WriteFile(filepath.Join(chartDir, "Images.lock"), []byte(data), 0644))
			dt("images", "verify", "--insecure", chartDir).AssertErrorMatch(t, fmt.Sprintf(`.*Images.lock does not validate:
.*Helm chart "test": image ".*%s": digests do not match:\s*.*- %s\s*\s*\+ %s.*`, images[0].Image, newDigest, oldDigest))
		})
	})
	t.Run("Verify Helm chart", func(t *testing.T) {
		images, err := s.LoadImagesFromFile("../../testdata/images.json")
		require.NoError(err)

		scenarioName := "custom-chart"
		chartName := "test"
		originChart := renderLockedChart(sb.TempFile(), chartName, scenarioName, serverURL, images)

		dt("images", "verify", "--insecure", originChart).AssertSuccessMatch(t, "")
	})

}
