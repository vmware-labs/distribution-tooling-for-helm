package chartutils

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

func (suite *ChartUtilsTestSuite) TestPullImages() {
	require := suite.Require()
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

	serverURL := u.Host
	scenarioName := "complete-chart"
	chartName := "test"

	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	t.Run("Pulls images", func(_ *testing.T) {
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		imagesDir := filepath.Join(chartDir, "images")

		require.NoError(err)

		lock, err := imagelock.FromYAMLFile(filepath.Join(chartDir, "Images.lock"))
		require.NoError(err)
		require.NoError(PullImages(lock, imagesDir))

		require.DirExists(imagesDir)

		for _, imgData := range images {
			for _, digestData := range imgData.Digests {
				imgDir := filepath.Join(imagesDir, fmt.Sprintf("%s.layout", digestData.Digest.Encoded()))
				suite.Assert().DirExists(imgDir)
			}
		}
	})

	t.Run("Error when no images in Images.lock", func(_ *testing.T) {
		chartDir := sb.TempFile()

		images := []tu.ImageData{}
		scenarioName := "no-images-chart"
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		imagesDir := filepath.Join(chartDir, "images")

		require.NoError(err)

		lock, err := imagelock.FromYAMLFile(filepath.Join(chartDir, "Images.lock"))
		require.NoError(err)
		require.Error(PullImages(lock, imagesDir))

		require.DirExists(imagesDir)

		for _, imgData := range images {
			for _, digestData := range imgData.Digests {
				imgDir := filepath.Join(imagesDir, fmt.Sprintf("%s.layout", digestData.Digest.Encoded()))
				suite.Assert().DirExists(imgDir)
			}
		}
	})
}

func (suite *ChartUtilsTestSuite) TestPushImages() {

	t := suite.T()
	sb := suite.sb
	require := suite.Require()
	assert := suite.Assert()

	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()

	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	serverURL := u.Host

	t.Run("Pushing works", func(t *testing.T) {
		scenarioName := "complete-chart"
		chartName := "test"

		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		imageData := tu.ImageData{Name: "test", Image: "test:mytag"}
		architectures := []string{
			"linux/amd64",
			"linux/arm",
		}
		craneImgs, err := tu.CreateSampleImages(&imageData, architectures)

		if err != nil {
			t.Fatal(err)
		}

		require.Equal(len(architectures), len(imageData.Digests))

		images := []tu.ImageData{imageData}
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{
				"ServerURL": serverURL, "Images": images,
				"Name": chartName, "RepositoryURL": serverURL,
			},
		))

		imagesDir := filepath.Join(chartDir, "images")
		require.NoError(os.MkdirAll(imagesDir, 0755))
		for _, img := range craneImgs {
			d, err := img.Digest()
			if err != nil {
				t.Fatal(err)
			}
			imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.layout", d.Hex))
			if err := crane.SaveOCI(img, imgFile); err != nil {
				t.Fatal(err)
			}
		}

		t.Run("Push images", func(t *testing.T) {
			require.NoError(err)
			lock, err := imagelock.FromYAMLFile(filepath.Join(chartDir, "Images.lock"))
			require.NoError(err)
			require.NoError(PushImages(lock, imagesDir))

			// Verify the images were pushed
			for _, img := range images {
				src := fmt.Sprintf("%s/%s", u.Host, img.Image)
				remoteDigests, err := tu.ReadRemoteImageManifest(src)
				if err != nil {
					t.Fatal(err)
				}
				for _, dgstData := range img.Digests {
					assert.Equal(dgstData.Digest.Hex(), remoteDigests[dgstData.Arch].Digest.Hex())
				}
			}
		})
	})
}
