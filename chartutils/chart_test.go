package chartutils

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *ChartUtilsTestSuite) TestLoadChart() {
	sb := suite.sb
	t := suite.T()
	type rawChartData struct {
		Dependencies []struct {
			Name       string
			Repository string
			Version    string
		}
		Annotations map[string]string
	}
	type imgAnnotation struct {
		Name  string
		Image string
	}
	readRawChart := func(f string) (*rawChartData, error) {
		fh, err := os.Open(f)
		if err != nil {
			return nil, fmt.Errorf("cannot open file %q: %v", f, err)
		}
		require.NoError(t, err)
		defer fh.Close()

		d := &rawChartData{}
		dec := yaml.NewDecoder(fh)
		if err := dec.Decode(d); err != nil {
			return nil, fmt.Errorf("cannot parse file %q: %v", f, err)
		}
		return d, nil
	}

	t.Run("Working Scenarios", func(t *testing.T) {
		dest := sb.TempFile()
		//serverURL :=	suite.testServer.ServerURL
		serverURL := "localhost"
		require.NoError(t, tu.RenderScenario("../testdata/scenarios/chart1", dest, map[string]interface{}{"ServerURL": serverURL}))
		chartDir := filepath.Join(dest, "chart1")
		t.Run("Fail Scenarios", func(t *testing.T) {
			t.Run("Fails to load non existing chart", func(t *testing.T) {
				_, err := LoadChart(sb.TempFile())
				require.ErrorContains(t, err, "no such file or directory")
			})

		})
		t.Run("Loads a chart from a directory", func(t *testing.T) {
			chart, err := LoadChart(chartDir)
			require.NoError(t, err)
			t.Run("RootDir", func(t *testing.T) {
				assert.Equal(t, chart.RootDir(), chartDir)
			})
			t.Run("AbsFilePath", func(t *testing.T) {
				for _, tail := range []string{"Chart.yaml", "ImagesLock.lock"} {
					assert.Equal(t, chart.AbsFilePath(tail), filepath.Join(chartDir, tail))

				}
			})
			t.Run("ValuesFile", func(t *testing.T) {
				f := chart.ValuesFile()
				require.NotNil(t, f)
				assert.Equal(t, f.Name, "values.yaml")
			})
			t.Run("Dependencies", func(t *testing.T) {
				dependencies := chart.Dependencies()
				d, err := readRawChart(filepath.Join(chartDir, "Chart.yaml"))
				require.NoError(t, err)
				assert.Equal(t, len(dependencies), len(d.Dependencies))
			OutLoop:
				for _, depData := range d.Dependencies {
					for _, dep := range dependencies {
						if dep.Name() == depData.Name {
							continue OutLoop
						}
					}
					assert.Fail(t, "cannot find dependant chart %q", depData.Name)
				}
			})

			t.Run("GetImageAnnotations", func(t *testing.T) {
				res, err := chart.GetAnnotatedImages()
				assert.NoError(t, err)

				d, err := readRawChart(filepath.Join(chartDir, "Chart.yaml"))

				require.NoError(t, err)
				annotationsData, ok := d.Annotations["images"]
				require.True(t, ok, "Cannot find images annotation")

				annBuff := bytes.NewBufferString(annotationsData)
				imgAnnotations := make([]imgAnnotation, 0)
				dec := yaml.NewDecoder(annBuff)
				require.NoError(t, dec.Decode(&imgAnnotations))

				require.Equal(t, len(imgAnnotations), len(res))

			OutLoop:
				for _, imgAnnotation := range imgAnnotations {
					for _, img := range res {
						if img.Name == imgAnnotation.Name && img.Image == imgAnnotation.Image {
							continue OutLoop
						}
					}
					assert.Fail(t, "Image %q was not found", imgAnnotation.Name)
				}

			})

		})
	})

}
