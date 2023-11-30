package chartutils

import (
	"fmt"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"

	"github.com/stretchr/testify/suite"
)

type ChartUtilsTestSuite struct {
	suite.Suite
	sb *tu.Sandbox
}

func (suite *ChartUtilsTestSuite) TearDownSuite() {
	_ = suite.sb.Cleanup()
}

func (suite *ChartUtilsTestSuite) SetupSuite() {
	suite.sb = tu.NewSandbox()
}

func TestChartUtilsTestSuite(t *testing.T) {
	suite.Run(t, new(ChartUtilsTestSuite))
}

func (suite *ChartUtilsTestSuite) TestAnnotateChart() {
	t := suite.T()
	require := suite.Require()

	sb := suite.sb
	serverURL := "localhost"
	scenarioName := "plain-chart"
	defaultAnnotationsKey := "images"
	// customAnnotationsKey := "artifacthub.io/images"
	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	type testImage struct {
		Name       string
		Registry   string
		Repository string
		Tag        string
		Digest     string
	}

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
	t.Run("Annotates a chart", func(t *testing.T) {
		chartDir := sb.TempFile()
		annotationsKey := defaultAnnotationsKey
		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": serverURL, "ValuesImages": images},
		))

		expectedImages := make([]tu.AnnotationEntry, 0)
		for _, img := range images {
			url := fmt.Sprintf("%s/%s:%s", img.Registry, img.Repository, img.Tag)
			expectedImages = append(expectedImages, tu.AnnotationEntry{
				Name:  img.Name,
				Image: url,
			})
		}

		require.NoError(AnnotateChart(chartDir, WithAnnotationsKey(annotationsKey)))
		tu.AssertChartAnnotations(t, chartDir, annotationsKey, expectedImages)
	})
}
