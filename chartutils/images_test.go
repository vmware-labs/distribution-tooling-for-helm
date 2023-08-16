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
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
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

	scenarioDir := fmt.Sprintf("../testdata/scenarios/%s", scenarioName)

	suite.T().Run("Pulls images", func(t *testing.T) {
		dest := sb.TempFile()
		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		chartDir := filepath.Join(dest, scenarioName)
		imagesDir := filepath.Join(chartDir, "images")

		require.NoError(err)

		lock, err := imagelock.FromYAMLFile(filepath.Join(chartDir, "Images.lock"))
		require.NoError(err)
		require.NoError(PullImages(lock, imagesDir))

		require.DirExists(imagesDir)

		for _, imgData := range images {
			for _, digestData := range imgData.Digests {
				imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.tar", digestData.Digest.Encoded()))
				suite.Assert().FileExists(imgFile)
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

		scenarioDir := fmt.Sprintf("../testdata/scenarios/%s", scenarioName)
		imageName := "test:mytag"

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
		dest := sb.TempFile()
		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{
				"ServerURL": serverURL, "Images": images,
				"Name": chartName, "RepositoryURL": serverURL,
			},
		))

		chartDir := filepath.Join(dest, scenarioName)
		imagesDir := filepath.Join(chartDir, "images")
		require.NoError(os.MkdirAll(imagesDir, 0755))
		for _, img := range craneImgs {
			d, err := img.Digest()
			if err != nil {
				t.Fatal(err)
			}
			imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.tar", d.Hex))
			if err := crane.Save(img, imageName, imgFile); err != nil {
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
