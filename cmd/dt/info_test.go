package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

func (suite *CmdSuite) TestInfoCommand() {
	t := suite.T()
	require := suite.Require()
	assert := suite.Assert()

	sb := suite.sb

	t.Run("Get Wrap Info", func(t *testing.T) {
		imageName := "test"
		imageTag := "mytag"

		serverURL := "localhost"
		scenarioName := "complete-chart"
		chartName := "test"
		version := "1.0.0"
		appVersion := "2.3.4"
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		wrapDir := sb.TempFile()
		chartDir := sb.TempFile()

		images, err := writeSampleImages(imageName, imageTag, filepath.Join(wrapDir, "images"))
		require.NoError(err)
		err = utils.CopyDir(filepath.Join(wrapDir, "images"), chartDir)
		require.NoError(err)

		for _, chartPath := range []string{filepath.Join(wrapDir, "chart"), chartDir} {
			require.NoError(tu.RenderScenario(scenarioDir, chartPath,
				map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version, "AppVersion": appVersion, "RepositoryURL": serverURL},
			))
		}
		tarFile := sb.TempFile()
		if err := utils.Tar(wrapDir, tarFile, utils.TarConfig{
			Prefix: chartName,
		}); err != nil {
			require.NoError(err)
		}
		for _, inputChart := range []string{tarFile, chartDir} {
			t.Run("Short info", func(t *testing.T) {
				var archList []string
				for _, digest := range images[0].Digests {
					archList = append(archList, digest.Arch)
				}

				res := dt("info", inputChart)
				res.AssertSuccess(t)
				imageURL := fmt.Sprintf("%s/%s:%s", serverURL, imageName, imageTag)

				imageEntryRe := fmt.Sprintf(`%s\s+\(%s\)`, imageURL, strings.Join(archList, ", "))
				assert.Regexp(fmt.Sprintf(`(?s).*Wrap Information.*Chart:.*%s\s*.*Version:.*%s.*%s\s*.*Metadata.*Images.*%s`, chartName, version, appVersion, imageEntryRe), res.stdout)
			})
			t.Run("Detailed info", func(t *testing.T) {
				res := dt("info", "--detailed", inputChart)
				res.AssertSuccess(t)
				imageURL := fmt.Sprintf("%s/%s:%s", serverURL, imageName, imageTag)

				imgDetailedInfo := fmt.Sprintf(`%s/%s.*Image:\s+%s.*Digests.*`, chartName, imageName, imageURL)
				for _, digest := range images[0].Digests {
					imgDetailedInfo += fmt.Sprintf(`.*- Arch:\s+%s.*Digest:\s+%s.*`, digest.Arch, digest.Digest)
				}
				assert.Regexp(fmt.Sprintf(`(?s).*Wrap Information.*Chart:.*%s\s*.*Version:.*%s.*Metadata.*Images.*%s`, chartName, version, imgDetailedInfo), res.stdout)
			})
			t.Run("YAML format", func(t *testing.T) {
				res := dt("info", "--yaml", inputChart)
				res.AssertSuccess(t)
				data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
					map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version, "AppVersion": appVersion},
				)
				require.NoError(err)

				lockFileData, err := tu.NormalizeYAML(data)
				require.NoError(err)
				yamlInfoData, err := tu.NormalizeYAML(res.stdout)
				require.NoError(err)

				assert.Equal(lockFileData, yamlInfoData)
			})

		}
	})
	t.Run("Errors", func(t *testing.T) {
		serverURL := "localhost"
		scenarioName := "plain-chart"
		chartName := "test"
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": serverURL},
		))

		tarFile := sb.TempFile()
		if err := utils.Tar(chartDir, tarFile, utils.TarConfig{
			Prefix: chartName,
		}); err != nil {
			require.NoError(err)
		}
		for _, inputChart := range []string{tarFile, chartDir} {
			t.Run("Fails when missing Images.lock", func(t *testing.T) {
				dt("info", inputChart).AssertErrorMatch(t, "failed to load Images.lock")
			})
		}
		t.Run("Handles non-existent wraps", func(t *testing.T) {
			dt("info", sb.TempFile()).AssertErrorMatch(t, `wrap file.* does not exist`)
		})
	})
}
