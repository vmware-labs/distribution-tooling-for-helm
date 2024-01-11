package main

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"

	"helm.sh/helm/v3/pkg/chart/loader"
)

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

func (suite *CmdSuite) TestAnnotateCommand() {
	sb := suite.sb
	t := suite.T()
	require := suite.Require()
	assert := suite.Assert()

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
		t.Run(title, func(t *testing.T) {
			chartDir := sb.TempFile()

			require.NoError(tu.RenderScenario(scenarioDir, chartDir,
				map[string]interface{}{"ServerURL": serverURL, "ValuesImages": images},
			))
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

			expectedImages := make([]tu.AnnotationEntry, 0)
			for _, img := range images {
				expectedImages = append(expectedImages, tu.AnnotationEntry{
					Name:  img.Name,
					Image: img.URL(),
				})
			}
			tu.AssertChartAnnotations(t, chartDir, key, expectedImages)
		})
	}
	t.Run("Corner cases", func(t *testing.T) {
		t.Run("Handle empty image list case", func(t *testing.T) {
			chartDir := sb.TempFile()

			require.NoError(tu.RenderScenario(scenarioDir, chartDir,
				map[string]interface{}{"ServerURL": serverURL},
			))

			dt("charts", "annotate", chartDir).AssertSuccessMatch(t, regexp.MustCompile(`No container images found`))

			tu.AssertChartAnnotations(t, chartDir, defaultAnnotationsKey, make([]tu.AnnotationEntry, 0))

			c, err := loader.Load(chartDir)
			require.NoError(err)

			assert.Equal(0, len(c.Metadata.Annotations))
		})
		t.Run("Handle errors annotating", func(t *testing.T) {
			chartDir := sb.TempFile()

			require.NoError(tu.RenderScenario(scenarioDir, chartDir,
				map[string]interface{}{"ServerURL": serverURL, "ValuesImages": images},
			))
			require.NoError(os.Chmod(chartDir, os.FileMode(0555)))
			// Make sure the sandbox can be cleaned
			defer os.Chmod(chartDir, os.FileMode(0755))

			dt("charts", "annotate", chartDir).AssertErrorMatch(t, regexp.MustCompile(`failed to annotate Helm chart.*failed to serialize.*`))
		})
	})
}
