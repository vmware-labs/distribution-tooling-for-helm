package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

type rawImageEntry struct {
	Name  string
	Image string
}
type testImage struct {
	Name       string
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

func (img *testImage) URL() string {
	return fmt.Sprintf("%s/%s:%s", img.Registry, img.Repository, img.Tag)
}

func verifyChartAnnotations(suite *CmdSuite, chartDir string, annotationsKey string, expectedImages []testImage) *chart.Chart {
	rawExpectedImages := make([]rawImageEntry, 0)
	for _, img := range expectedImages {
		rawExpectedImages = append(rawExpectedImages, rawImageEntry{
			Name:  img.Name,
			Image: img.URL(),
		})
	}
	require := suite.Require()

	c, err := loader.Load(chartDir)
	require.NoError(err)

	gotImages := make([]rawImageEntry, 0)
	require.NoError(yaml.Unmarshal([]byte(c.Metadata.Annotations[annotationsKey]), &gotImages))
	suite.Assert().EqualValues(rawExpectedImages, gotImages)
	return c
}

func (suite *CmdSuite) TestAnnotateCommand() {
	sb := suite.sb
	require := suite.Require()
	serverURL := "localhost"
	scenarioName := "plain-chart"
	defaultAnnotationsKey := "images"
	customAnnotationsKey := "artifacthub.io/images"
	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	images := []testImage{
		{
			Name:       "bitnami-shell",
			Registry:   "docker.io",
			Repository: "bitnami/bitnami-shell",
			Tag:        "1.0.0",
		},
		{
			Name:       "wordpress",
			Registry:   "docker.io",
			Repository: "bitnami/wordpress",
			Tag:        "latest",
		},
	}
	for title, key := range map[string]string{
		"Successfully annotates a Helm chart":                 "",
		"Successfully annotates a Helm chart with custom key": customAnnotationsKey,
	} {
		suite.T().Run(title, func(t *testing.T) {
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{"ServerURL": serverURL, "ValuesImages": images},
			))
			chartDir := filepath.Join(dest, scenarioName)
			var args []string
			if key == "" || key == defaultAnnotationsKey {
				// enforce it if empty
				if key == "" {
					key = defaultAnnotationsKey
				}
				args = []string{"charts", "annotate", chartDir}
			} else {
				args = []string{"charts", "--annotations-key", key, "annotate", chartDir}
			}
			dt(args...).AssertSuccess(t)
			_ = verifyChartAnnotations(suite, chartDir, key, images)
		})
	}
	suite.T().Run("Corner cases", func(t *testing.T) {
		t.Run("Handle empty image list case", func(t *testing.T) {
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{"ServerURL": serverURL},
			))

			chartDir := filepath.Join(dest, scenarioName)
			dt("charts", "annotate", chartDir).AssertSuccess(t)

			c := verifyChartAnnotations(suite, chartDir, defaultAnnotationsKey, nil)
			suite.Assert().Equal(0, len(c.Metadata.Annotations))
		})
		t.Run("Handle errors annotating", func(t *testing.T) {
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{"ServerURL": serverURL, "ValuesImages": images},
			))
			chartDir := filepath.Join(dest, scenarioName)
			require.NoError(os.Chmod(chartDir, os.FileMode(0555)))
			// Make sure the sandbox can be cleaned
			defer os.Chmod(chartDir, os.FileMode(0755))

			dt("charts", "annotate", chartDir).AssertErrorMatch(t, regexp.MustCompile(`failed to annotate Helm chart.*failed to serialize.*`))
		})
	})
}
